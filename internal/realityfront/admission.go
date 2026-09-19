package realityfront

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"io"
	"net"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

const (
	RecordVersionV1 uint16 = 1

	admissionMagic      = "WBAD"
	admissionRequestLen = 14
	admissionReplyLen   = 25
	maxAdmissionUserLen = 255
	maxAdmissionPassLen = 1024
	tunnelIDLen          = 16

	admissionOK          byte = 0
	admissionAuthFail    byte = 1
	admissionVersionFail byte = 2
	admissionParamFail   byte = 3
)

var (
	ErrAdmissionAuth    = errors.New("realityfront: admission authentication failed")
	ErrAdmissionVersion = errors.New("realityfront: unsupported record version")
	ErrAdmissionParams  = errors.New("realityfront: invalid admission parameters")
)

type AdmissionRequest struct {
	RecordVersion uint16
	TunnelID      []byte
	ClientLimit   uint16
	Username      string
	Password      string
}

type AdmissionResult struct {
	RecordVersion    uint16
	IncarnationNonce [16]byte
	TunnelID         []byte
	ClientLimit      uint16
	ServerLimit      uint16
	Keys             tlsrecord.KeyPair
}

func (r AdmissionResult) exporterParams() ExporterParams {
	return ExporterParams{
		Version:          r.RecordVersion,
		IncarnationNonce: r.IncarnationNonce,
		TunnelID:         append([]byte(nil), r.TunnelID...),
		ClientLimit:      r.ClientLimit,
		ServerLimit:      r.ServerLimit,
	}
}

type ClientAdmissionConfig struct {
	TLS         ClientConfig
	Username    string
	Password    string
	TunnelID    []byte
	ClientLimit uint16
}

type ServerAdmissionConfig struct {
	TLS              ServerConfig
	ExpectedUsername string
	ExpectedPassword string
	ServerLimit      uint16
	Random           io.Reader
}

type ClientAdmissionSession struct {
	TLS        *ClientSession
	Negotiated AdmissionResult
}

type ServerAdmissionSession struct {
	TLS          *ServerSession
	Negotiated   AdmissionResult
	Boundary     uint32
	EarlyRecords []faketcp.TransitionPacket
}

// EstablishClient performs real TLS first, sends one protected admission
// request, fully reads and validates the protected response, and only then
// derives the record keys from the original uTLS ConnectionState.
func EstablishClient(ctx context.Context, conn net.Conn, cfg ClientAdmissionConfig) (*ClientAdmissionSession, error) {
	req := AdmissionRequest{
		RecordVersion: RecordVersionV1,
		TunnelID:      append([]byte(nil), cfg.TunnelID...),
		ClientLimit:   cfg.ClientLimit,
		Username:      cfg.Username,
		Password:      cfg.Password,
	}
	wire, err := marshalAdmissionRequest(req)
	if err != nil {
		return nil, err
	}

	uconn, err := handshakeClientConn(ctx, conn, cfg.TLS)
	if err != nil {
		return nil, err
	}
	if err := writeFull(uconn, wire); err != nil {
		return nil, err
	}
	result, err := readAdmissionReply(uconn, req)
	if err != nil {
		return nil, err
	}

	state := uconn.ConnectionState()
	keys, err := deriveRecordKeys(&state, result.exporterParams())
	if err != nil {
		return nil, err
	}
	result.Keys = keys
	return &ClientAdmissionSession{
		TLS:        &ClientSession{Conn: uconn, Keys: keys},
		Negotiated: result,
	}, nil
}

// EstablishServer classifies and takes over the same FakeTCP bootstrap
// association, authenticates one protected request, derives exporter keys,
// installs the receive transition boundary BEFORE the final TLS response, then
// writes the response. BootstrapStream.Write waits for the response ACK. Detach
// happens after that ACK; any first new-mode record that raced the response is
// transferred in EarlyRecords instead of being fed back into TLS.
func EstablishServer(ctx context.Context, assoc *faketcp.ServerAssociation, cfg ServerAdmissionConfig) (*ServerAdmissionSession, error) {
	if assoc == nil {
		return nil, ErrAdmissionParams
	}
	if cfg.ExpectedUsername == "" || cfg.ExpectedPassword == "" || !validRecordLimit(cfg.ServerLimit) {
		return nil, ErrAdmissionParams
	}
	conn := assoc.BootstrapConn()
	hello, err := ReadHello(conn, cfg.TLS.ServerName, cfg.TLS.RouteKey, cfg.TLS.Timeout)
	if err != nil {
		return nil, err
	}
	if !hello.Recognized {
		return nil, ErrMarker
	}
	tlsConn, err := handshakeServerRecognizedConn(ctx, conn, hello, cfg.TLS)
	if err != nil {
		return nil, err
	}

	req, err := readAdmissionRequest(tlsConn)
	if err != nil {
		_ = writeAdmissionFailure(tlsConn, admissionStatus(err))
		return nil, err
	}
	if !credentialsMatch(req.Username, req.Password, cfg.ExpectedUsername, cfg.ExpectedPassword) {
		_ = writeAdmissionFailure(tlsConn, admissionAuthFail)
		return nil, ErrAdmissionAuth
	}

	source := cfg.Random
	if source == nil {
		source = rand.Reader
	}
	var nonce [16]byte
	if _, err := io.ReadFull(source, nonce[:]); err != nil {
		return nil, err
	}
	result := AdmissionResult{
		RecordVersion:    req.RecordVersion,
		IncarnationNonce: nonce,
		TunnelID:         append([]byte(nil), req.TunnelID...),
		ClientLimit:      req.ClientLimit,
		ServerLimit:      cfg.ServerLimit,
	}
	state := tlsConn.ConnectionState()
	keys, err := deriveRecordKeys(&state, result.exporterParams())
	if err != nil {
		return nil, err
	}
	result.Keys = keys

	boundary, err := assoc.PrepareTransition(int(cfg.ServerLimit))
	if err != nil {
		return nil, err
	}
	reply, err := marshalAdmissionReply(result)
	if err != nil {
		assoc.AbortTransition()
		return nil, err
	}
	if err := writeFull(tlsConn, reply); err != nil {
		assoc.AbortTransition()
		return nil, err
	}
	early, err := assoc.DetachTransition()
	if err != nil {
		assoc.AbortTransition()
		return nil, err
	}
	return &ServerAdmissionSession{
		TLS:          &ServerSession{Conn: tlsConn, Hello: hello, Keys: keys},
		Negotiated:   result,
		Boundary:     boundary,
		EarlyRecords: early,
	}, nil
}

func marshalAdmissionRequest(req AdmissionRequest) ([]byte, error) {
	if req.RecordVersion != RecordVersionV1 {
		return nil, ErrAdmissionVersion
	}
	if !validRecordLimit(req.ClientLimit) || len(req.TunnelID) != tunnelIDLen ||
		len(req.Username) == 0 || len(req.Username) > maxAdmissionUserLen ||
		len(req.Password) == 0 || len(req.Password) > maxAdmissionPassLen {
		return nil, ErrAdmissionParams
	}
	out := make([]byte, admissionRequestLen+len(req.TunnelID)+len(req.Username)+len(req.Password))
	copy(out[:4], admissionMagic)
	binary.BigEndian.PutUint16(out[4:6], req.RecordVersion)
	binary.BigEndian.PutUint16(out[6:8], req.ClientLimit)
	binary.BigEndian.PutUint16(out[8:10], uint16(len(req.TunnelID)))
	binary.BigEndian.PutUint16(out[10:12], uint16(len(req.Username)))
	binary.BigEndian.PutUint16(out[12:14], uint16(len(req.Password)))
	off := admissionRequestLen
	copy(out[off:], req.TunnelID)
	off += len(req.TunnelID)
	copy(out[off:], req.Username)
	off += len(req.Username)
	copy(out[off:], req.Password)
	return out, nil
}

func readAdmissionRequest(r io.Reader) (AdmissionRequest, error) {
	var out AdmissionRequest
	var hdr [admissionRequestLen]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return out, err
	}
	if string(hdr[:4]) != admissionMagic {
		return out, ErrAdmissionParams
	}
	out.RecordVersion = binary.BigEndian.Uint16(hdr[4:6])
	if out.RecordVersion != RecordVersionV1 {
		return out, ErrAdmissionVersion
	}
	out.ClientLimit = binary.BigEndian.Uint16(hdr[6:8])
	tunnelLen := int(binary.BigEndian.Uint16(hdr[8:10]))
	userLen := int(binary.BigEndian.Uint16(hdr[10:12]))
	passLen := int(binary.BigEndian.Uint16(hdr[12:14]))
	if !validRecordLimit(out.ClientLimit) || tunnelLen != tunnelIDLen ||
		userLen <= 0 || userLen > maxAdmissionUserLen ||
		passLen <= 0 || passLen > maxAdmissionPassLen {
		return out, ErrAdmissionParams
	}
	body := make([]byte, tunnelLen+userLen+passLen)
	if _, err := io.ReadFull(r, body); err != nil {
		return out, err
	}
	off := 0
	out.TunnelID = append([]byte(nil), body[off:off+tunnelLen]...)
	off += tunnelLen
	out.Username = string(body[off : off+userLen])
	off += userLen
	out.Password = string(body[off : off+passLen])
	return out, nil
}

func marshalAdmissionReply(result AdmissionResult) ([]byte, error) {
	if result.RecordVersion != RecordVersionV1 || !validRecordLimit(result.ClientLimit) ||
		!validRecordLimit(result.ServerLimit) || len(result.TunnelID) != tunnelIDLen {
		return nil, ErrAdmissionParams
	}
	out := make([]byte, admissionReplyLen+len(result.TunnelID))
	out[0] = admissionOK
	binary.BigEndian.PutUint16(out[1:3], result.RecordVersion)
	copy(out[3:19], result.IncarnationNonce[:])
	binary.BigEndian.PutUint16(out[19:21], result.ClientLimit)
	binary.BigEndian.PutUint16(out[21:23], result.ServerLimit)
	binary.BigEndian.PutUint16(out[23:25], uint16(len(result.TunnelID)))
	copy(out[25:], result.TunnelID)
	return out, nil
}

func readAdmissionReply(r io.Reader, req AdmissionRequest) (AdmissionResult, error) {
	var out AdmissionResult
	var status [1]byte
	if _, err := io.ReadFull(r, status[:]); err != nil {
		return out, err
	}
	switch status[0] {
	case admissionOK:
	case admissionAuthFail:
		return out, ErrAdmissionAuth
	case admissionVersionFail:
		return out, ErrAdmissionVersion
	default:
		return out, ErrAdmissionParams
	}

	var rest [admissionReplyLen - 1]byte
	if _, err := io.ReadFull(r, rest[:]); err != nil {
		return out, err
	}
	out.RecordVersion = binary.BigEndian.Uint16(rest[0:2])
	copy(out.IncarnationNonce[:], rest[2:18])
	out.ClientLimit = binary.BigEndian.Uint16(rest[18:20])
	out.ServerLimit = binary.BigEndian.Uint16(rest[20:22])
	tunnelLen := int(binary.BigEndian.Uint16(rest[22:24]))
	if out.RecordVersion != RecordVersionV1 || out.RecordVersion != req.RecordVersion ||
		out.ClientLimit != req.ClientLimit || !validRecordLimit(out.ServerLimit) ||
		tunnelLen != tunnelIDLen {
		return AdmissionResult{}, ErrAdmissionParams
	}
	tunnel := make([]byte, tunnelLen)
	if _, err := io.ReadFull(r, tunnel); err != nil {
		return AdmissionResult{}, err
	}
	if !bytes.Equal(tunnel, req.TunnelID) {
		return AdmissionResult{}, ErrAdmissionParams
	}
	out.TunnelID = tunnel
	return out, nil
}

func writeAdmissionFailure(w io.Writer, status byte) error {
	if status == admissionOK {
		status = admissionParamFail
	}
	return writeFull(w, []byte{status})
}

func admissionStatus(err error) byte {
	switch {
	case errors.Is(err, ErrAdmissionVersion):
		return admissionVersionFail
	case errors.Is(err, ErrAdmissionAuth):
		return admissionAuthFail
	default:
		return admissionParamFail
	}
}

func credentialsMatch(gotUser, gotPass, wantUser, wantPass string) bool {
	if wantUser == "" || wantPass == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(gotUser), []byte(wantUser)) == 1 &&
		subtle.ConstantTimeCompare([]byte(gotPass), []byte(wantPass)) == 1
}

func validRecordLimit(limit uint16) bool {
	return int(limit) >= tlsrecord.FixedWireOverhead && int(limit) <= tlsrecord.MaxWireLen
}

func writeFull(w io.Writer, p []byte) error {
	for len(p) != 0 {
		n, err := w.Write(p)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrUnexpectedEOF
		}
		p = p[n:]
	}
	return nil
}
