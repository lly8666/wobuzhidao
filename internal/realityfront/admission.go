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
	admissionRequestLen = 15
	admissionReplyLen   = 26
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
	LaneID        uint8
	TunnelID      []byte
	ClientLimit   uint16
	Username      string
	Password      string
}

type AdmissionResult struct {
	RecordVersion    uint16
	LaneID           uint8
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
	// LaneID is the protected Logical Tunnel lane identity. Zero keeps legacy
	// single-lane callers source-compatible and is normalized to lane 1.
	LaneID      uint8
}

type AdmissionRequestValidator func(AdmissionRequest) error

type ServerAdmissionConfig struct {
	TLS              ServerConfig
	ExpectedUsername string
	ExpectedPassword string
	ServerLimit      uint16
	Random           io.Reader
	// ValidateRequest runs after TLS protection + credential verification but
	// before the success reply is emitted. It lets the runtime fail closed when
	// a TunnelID/LaneID cannot be bound to current lifecycle state.
	ValidateRequest  AdmissionRequestValidator
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
	laneID := cfg.LaneID
	if laneID == 0 {
		laneID = 1
	}
	req := AdmissionRequest{
		RecordVersion: RecordVersionV1,
		LaneID:        laneID,
		TunnelID:      append([]byte(nil), cfg.TunnelID...),
		ClientLimit:   cfg.ClientLimit,
		Username:      cfg.Username,
		Password:      cfg.Password,
	}
	wire, err := marshalAdmissionRequest(req)
	if err != nil {
		return nil, err
	}

	guard, err := beginCandidateDeadline(ctx, conn, cfg.TLS.Timeout)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		guard.Finish(success)
		if !success {
			_ = conn.Close()
		}
	}()

	tlsCfg := cfg.TLS
	tlsCfg.Timeout = guard.Remaining()
	uconn, err := handshakeClientConn(ctx, conn, tlsCfg)
	if err != nil {
		return nil, candidateError(ctx, err)
	}
	if err := guard.Rearm(); err != nil {
		return nil, err
	}
	if err := writeFull(uconn, wire); err != nil {
		return nil, candidateError(ctx, err)
	}
	result, err := readAdmissionReply(uconn, req)
	if err != nil {
		return nil, candidateError(ctx, err)
	}

	state := uconn.ConnectionState()
	keys, err := deriveRecordKeys(&state, result.exporterParams())
	if err != nil {
		return nil, err
	}
	result.Keys = keys
	success = true
	// Successful admission transfers the underlying association to the
	// TLS-like data plane. Do not return the uTLS writer: Close, KeyUpdate-like
	// operations, or any future post-handshake write must not be able to append
	// TLS records after the sequence-space ownership boundary.
	return &ClientAdmissionSession{
		TLS:        &ClientSession{Keys: keys},
		Negotiated: result,
	}, nil
}

// EstablishServer is the recognized-only compatibility entry point. It reads
// the ClientHello once and rejects unrecognized sessions instead of dialing a
// fallback target. HandleServerAssociation owns the branch when fallback is
// configured.
func EstablishServer(ctx context.Context, assoc *faketcp.ServerAssociation, cfg ServerAdmissionConfig) (*ServerAdmissionSession, error) {
	if assoc == nil {
		return nil, ErrAdmissionParams
	}
	conn := assoc.BootstrapConn()
	guard, err := beginCandidateDeadline(ctx, conn, cfg.TLS.Timeout)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		guard.Finish(success)
		if !success {
			assoc.Close()
		}
	}()

	hello, err := ReadHello(conn, cfg.TLS.ServerName, cfg.TLS.RouteKey, guard.Remaining())
	if err != nil {
		return nil, candidateError(ctx, err)
	}
	if err := guard.Rearm(); err != nil {
		return nil, err
	}
	if !hello.Recognized {
		return nil, ErrMarker
	}
	session, err := establishServerRecognized(ctx, assoc, hello, cfg, guard)
	if err != nil {
		return nil, err
	}
	success = true
	return session, nil
}

// establishServerRecognized takes ownership after one ClientHello has already
// been classified. It must never read or classify another ClientHello.
func establishServerRecognized(ctx context.Context, assoc *faketcp.ServerAssociation, hello Hello, cfg ServerAdmissionConfig, guard *candidateDeadline) (*ServerAdmissionSession, error) {
	if assoc == nil || !hello.Recognized {
		return nil, ErrAdmissionParams
	}
	if cfg.ExpectedUsername == "" || cfg.ExpectedPassword == "" || !validRecordLimit(cfg.ServerLimit) {
		return nil, ErrAdmissionParams
	}
	conn := assoc.BootstrapConn()
	tlsCfg := cfg.TLS
	if guard != nil {
		tlsCfg.Timeout = guard.Remaining()
	}
	tlsConn, err := handshakeServerRecognizedConn(ctx, conn, hello, tlsCfg)
	if err != nil {
		return nil, candidateError(ctx, err)
	}
	if guard != nil {
		if err := guard.Rearm(); err != nil {
			return nil, err
		}
	}

	req, err := readAdmissionRequest(tlsConn)
	if err != nil {
		_ = writeAdmissionFailure(tlsConn, admissionStatus(err))
		return nil, candidateError(ctx, err)
	}
	if !credentialsMatch(req.Username, req.Password, cfg.ExpectedUsername, cfg.ExpectedPassword) {
		_ = writeAdmissionFailure(tlsConn, admissionAuthFail)
		return nil, ErrAdmissionAuth
	}
	if cfg.ValidateRequest != nil {
		if err := cfg.ValidateRequest(req); err != nil {
			_ = writeAdmissionFailure(tlsConn, admissionParamFail)
			return nil, errors.Join(ErrAdmissionParams, err)
		}
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
		LaneID:           req.LaneID,
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
		return nil, candidateError(ctx, err)
	}
	early, err := assoc.DetachTransition()
	if err != nil {
		assoc.AbortTransition()
		return nil, err
	}
	// DetachTransition has transferred sequence-space ownership. Retain only
	// immutable handshake results; the old tls.Conn writer is intentionally not
	// reachable from the returned session, so it cannot emit close_notify,
	// KeyUpdate, or another ticket after handoff.
	return &ServerAdmissionSession{
		TLS:          &ServerSession{Hello: hello, Keys: keys},
		Negotiated:   result,
		Boundary:     boundary,
		EarlyRecords: early,
	}, nil
}

func marshalAdmissionRequest(req AdmissionRequest) ([]byte, error) {
	if req.RecordVersion != RecordVersionV1 {
		return nil, ErrAdmissionVersion
	}
	if req.LaneID == 0 {
		req.LaneID = 1
	}
	if !validAdmissionLaneID(req.LaneID) || !validRecordLimit(req.ClientLimit) || len(req.TunnelID) != tunnelIDLen ||
		len(req.Username) == 0 || len(req.Username) > maxAdmissionUserLen ||
		len(req.Password) == 0 || len(req.Password) > maxAdmissionPassLen {
		return nil, ErrAdmissionParams
	}
	out := make([]byte, admissionRequestLen+len(req.TunnelID)+len(req.Username)+len(req.Password))
	copy(out[:4], admissionMagic)
	binary.BigEndian.PutUint16(out[4:6], req.RecordVersion)
	binary.BigEndian.PutUint16(out[6:8], req.ClientLimit)
	out[8] = req.LaneID
	binary.BigEndian.PutUint16(out[9:11], uint16(len(req.TunnelID)))
	binary.BigEndian.PutUint16(out[11:13], uint16(len(req.Username)))
	binary.BigEndian.PutUint16(out[13:15], uint16(len(req.Password)))
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
	out.LaneID = hdr[8]
	tunnelLen := int(binary.BigEndian.Uint16(hdr[9:11]))
	userLen := int(binary.BigEndian.Uint16(hdr[11:13]))
	passLen := int(binary.BigEndian.Uint16(hdr[13:15]))
	if !validAdmissionLaneID(out.LaneID) || !validRecordLimit(out.ClientLimit) || tunnelLen != tunnelIDLen ||
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
	if result.LaneID == 0 {
		result.LaneID = 1
	}
	if result.RecordVersion != RecordVersionV1 || !validAdmissionLaneID(result.LaneID) ||
		!validRecordLimit(result.ClientLimit) || !validRecordLimit(result.ServerLimit) ||
		len(result.TunnelID) != tunnelIDLen {
		return nil, ErrAdmissionParams
	}
	out := make([]byte, admissionReplyLen+len(result.TunnelID))
	out[0] = admissionOK
	binary.BigEndian.PutUint16(out[1:3], result.RecordVersion)
	copy(out[3:19], result.IncarnationNonce[:])
	binary.BigEndian.PutUint16(out[19:21], result.ClientLimit)
	binary.BigEndian.PutUint16(out[21:23], result.ServerLimit)
	out[23] = result.LaneID
	binary.BigEndian.PutUint16(out[24:26], uint16(len(result.TunnelID)))
	copy(out[26:], result.TunnelID)
	return out, nil
}

func readAdmissionReply(r io.Reader, req AdmissionRequest) (AdmissionResult, error) {
	var out AdmissionResult
	if req.LaneID == 0 {
		req.LaneID = 1
	}
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
	out.LaneID = rest[22]
	tunnelLen := int(binary.BigEndian.Uint16(rest[23:25]))
	if out.RecordVersion != RecordVersionV1 || out.RecordVersion != req.RecordVersion ||
		out.LaneID != req.LaneID || !validAdmissionLaneID(out.LaneID) ||
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

func validAdmissionLaneID(id uint8) bool {
	return id >= 1 && id <= 4
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
