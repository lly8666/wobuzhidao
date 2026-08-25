package realityfront

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"os"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

const (
	simpleAuthMagic      = "WBSA"
	simpleAuthV1         = byte(1)
	simpleAuthHeaderLen  = 9
	maxSimplePasswordLen = 1024
)

type accountTicketRecord struct {
	Version        int    `json:"version"`
	IssuedUnixNano int64  `json:"issued_unix_nano"`
	Username       string `json:"username"`
}

// RecordTicketForAccount writes the same one-time ticket format consumed by
// ConsumeTicket, with an additional username field. The old consumer ignores
// unknown JSON fields, so account metadata can be added without another wire
// protocol or another authentication round trip.
func RecordTicketForAccount(dir string, ticket Ticket, username string, now time.Time) error {
	if err := ensureTicketDir(dir); err != nil {
		return err
	}
	if ticket == (Ticket{}) || username == "" {
		return ErrTicket
	}
	body, err := json.Marshal(accountTicketRecord{
		Version:        ticketVersion,
		IssuedUnixNano: now.UnixNano(),
		Username:       username,
	})
	if err != nil {
		return err
	}
	body = append(body, '\n')
	f, err := os.OpenFile(ticketPath(dir, ticket), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(body)
	cerr := f.Close()
	if werr != nil {
		return werr
	}
	return cerr
}

// TicketAccount returns the account attached to a not-yet-consumed ticket.
// The ticket remains one-shot; this function does not remove it. Product bind
// code should prefer ConsumeTicketForAccount so ownership and one-shot consume
// happen in one operation.
func TicketAccount(dir string, ticket Ticket) (string, error) {
	if ticket == (Ticket{}) {
		return "", ErrTicket
	}
	body, err := os.ReadFile(ticketPath(dir, ticket))
	if err != nil {
		return "", ErrTicket
	}
	var rec accountTicketRecord
	if err := json.Unmarshal(body, &rec); err != nil || rec.Version != ticketVersion || rec.IssuedUnixNano <= 0 || rec.Username == "" {
		return "", ErrTicket
	}
	return rec.Username, nil
}

// ConsumeTicketForAccount atomically claims a simple-auth ticket, validates its
// TTL, and returns the account that owns the new live session. The rename is
// the one-shot serialization point: concurrent bind attempts for one ticket
// cannot both succeed even when they race in separate goroutines/processes.
func ConsumeTicketForAccount(dir string, ticket Ticket, now time.Time, ttl time.Duration) (string, error) {
	if ttl <= 0 || ticket == (Ticket{}) {
		return "", ErrTicket
	}
	path := ticketPath(dir, ticket)
	var nonce [8]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", err
	}
	claim := path + ".claim-" + hex.EncodeToString(nonce[:])
	if err := os.Rename(path, claim); err != nil {
		return "", ErrTicket
	}
	defer os.Remove(claim)

	body, err := os.ReadFile(claim)
	if err != nil {
		return "", ErrTicket
	}
	var rec accountTicketRecord
	if err := json.Unmarshal(body, &rec); err != nil || rec.Version != ticketVersion || rec.IssuedUnixNano <= 0 || rec.Username == "" {
		return "", ErrTicket
	}
	issued := time.Unix(0, rec.IssuedUnixNano)
	if now.Before(issued) || now.Sub(issued) > ttl {
		return "", ErrTicket
	}
	return rec.Username, nil
}

// BootstrapClientSimple is the personal-product authentication path. TLS 1.3
// already encrypts the record, so username/password are sent once inside TLS
// and the server immediately returns an independent one-time session ticket.
// There is intentionally no application-layer nonce/HMAC challenge round trip.
func BootstrapClientSimple(conn net.Conn, username, password string) (Ticket, error) {
	var zero Ticket
	if len(username) == 0 || len(username) > maxUsernameLen || len(password) == 0 || len(password) > maxSimplePasswordLen {
		return zero, ErrBootstrapAuth
	}
	request := make([]byte, simpleAuthHeaderLen+len(username)+len(password))
	copy(request[:4], simpleAuthMagic)
	request[4] = simpleAuthV1
	binary.BigEndian.PutUint16(request[5:7], uint16(len(username)))
	binary.BigEndian.PutUint16(request[7:9], uint16(len(password)))
	copy(request[simpleAuthHeaderLen:], username)
	copy(request[simpleAuthHeaderLen+len(username):], password)
	if err := writeFull(conn, request); err != nil {
		return zero, err
	}
	var reply [1 + TicketLen]byte
	if _, err := io.ReadFull(conn, reply[:]); err != nil {
		return zero, err
	}
	if reply[0] != 0 {
		return zero, ErrBootstrapAuth
	}
	copy(zero[:], reply[1:])
	if zero == (Ticket{}) {
		return Ticket{}, ErrTicket
	}
	return zero, nil
}

func BootstrapServerSimple(conn net.Conn, expectedUsername, expectedPassword, ticketDir string, now time.Time) (Ticket, error) {
	var zero Ticket
	var hdr [simpleAuthHeaderLen]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return zero, err
	}
	if string(hdr[:4]) != simpleAuthMagic || hdr[4] != simpleAuthV1 {
		return zero, ErrBootstrapAuth
	}
	userLen := int(binary.BigEndian.Uint16(hdr[5:7]))
	passLen := int(binary.BigEndian.Uint16(hdr[7:9]))
	if userLen <= 0 || userLen > maxUsernameLen || passLen <= 0 || passLen > maxSimplePasswordLen {
		return zero, ErrBootstrapAuth
	}
	user := make([]byte, userLen)
	password := make([]byte, passLen)
	if _, err := io.ReadFull(conn, user); err != nil {
		return zero, err
	}
	if _, err := io.ReadFull(conn, password); err != nil {
		return zero, err
	}
	userOK := subtle.ConstantTimeCompare(user, []byte(expectedUsername))
	passOK := subtle.ConstantTimeCompare(password, []byte(expectedPassword))
	if userOK != 1 || passOK != 1 || expectedUsername == "" || expectedPassword == "" {
		failure := make([]byte, 1+TicketLen)
		failure[0] = 1
		_ = writeFull(conn, failure)
		return zero, ErrBootstrapAuth
	}
	if _, err := io.ReadFull(rand.Reader, zero[:]); err != nil {
		return Ticket{}, err
	}
	if err := RecordTicketForAccount(ticketDir, zero, expectedUsername, now); err != nil {
		return Ticket{}, err
	}
	reply := make([]byte, 1+TicketLen)
	copy(reply[1:], zero[:])
	if err := writeFull(conn, reply); err != nil {
		return Ticket{}, err
	}
	return zero, nil
}

// HandleServerConnSimple keeps the REALITY-like single-listener split but uses
// the one-request shared username/password admission path on recognized WBD
// sessions. Unrecognized ClientHello bytes still fall through unchanged.
func HandleServerConnSimple(ctx context.Context, conn net.Conn, cfg ServerConfig) (ServerResult, error) {
	var out ServerResult
	if conn == nil || cfg.TLSConfig == nil || len(cfg.RouteKey) < 16 || normalizeName(cfg.ServerName) == "" {
		return out, ErrMarker
	}
	maxHello := cfg.Mirror.MaxHelloBytes
	if maxHello <= 0 {
		maxHello = 64 << 10
	}
	helloTimeout := cfg.HelloTimeout
	if helloTimeout <= 0 {
		helloTimeout = 5 * time.Second
	}
	info, raw, err := realitymirror.ReadClientHello(conn, maxHello, helloTimeout)
	if err != nil {
		return out, err
	}
	if !Recognized(raw, cfg.RouteKey, info.ServerName) {
		out.Branch = "fallback"
		_, err := realitymirror.HandleFromHello(ctx, conn, cfg.Mirror, info, raw)
		return out, err
	}
	if normalizeName(info.ServerName) != normalizeName(cfg.ServerName) {
		return out, ErrMarker
	}
	out.Branch = "wbd"
	tlsCfg := cfg.TLSConfig.Clone()
	if tlsCfg.MinVersion == 0 || tlsCfg.MinVersion < tls.VersionTLS13 {
		tlsCfg.MinVersion = tls.VersionTLS13
	}
	tlsCfg.MaxVersion = tls.VersionTLS13
	tlsConn := tls.Server(ReplayConn(conn, raw), tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return out, err
	}
	defer tlsConn.Close()
	ticket, err := BootstrapServerSimple(tlsConn, cfg.ExpectedUsername, cfg.ExpectedPassword, cfg.TicketDir, time.Now())
	if err != nil {
		return out, err
	}
	out.Ticket = ticket
	return out, nil
}
