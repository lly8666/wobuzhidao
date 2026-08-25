package realityfront

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

const (
	markerLen       = 32
	TicketLen       = 32
	maxUsernameLen  = 255
	bootstrapMagic  = "WBRA"
	bootstrapV1     = byte(1)
	markerLabel     = "WBD-REALITY-FRONT-MARKER-v1\x00"
	authLabel       = "WBD-REALITY-FRONT-AUTH-v1\x00"
	ticketVersion   = 1
)

var (
	ErrMarker            = errors.New("realityfront: invalid ClientHello marker")
	ErrBootstrapAuth     = errors.New("realityfront: username/password authentication failed")
	ErrTicket            = errors.New("realityfront: invalid or expired one-time ticket")
	ErrUnsupportedHello  = errors.New("realityfront: unsupported ClientHello layout")
)

type Ticket [TicketLen]byte

func (t Ticket) Hex() string { return hex.EncodeToString(t[:]) }

func ParseTicketHex(s string) (Ticket, error) {
	var out Ticket
	b, err := hex.DecodeString(strings.TrimSpace(s))
	if err != nil || len(b) != TicketLen {
		return out, ErrTicket
	}
	copy(out[:], b)
	return out, nil
}

// MarkerRand lets crypto/tls itself generate a REALITY-like compatibility
// SessionID marker. It does not patch bytes after TLS has built the transcript:
// the first 32-byte request is the TLS ClientHello random and the second is the
// TLS 1.3 compatibility SessionID. Later randomness is delegated unchanged.
//
// The route key is a classifier secret, not account authentication. Account
// authentication still happens inside the recognized TLS branch.
type MarkerRand struct {
	source     io.Reader
	key        []byte
	serverName string
	stage      int
	random     [32]byte
}

func NewMarkerRand(source io.Reader, routeKey []byte, serverName string) (*MarkerRand, error) {
	if source == nil {
		source = rand.Reader
	}
	if len(routeKey) < 16 {
		return nil, errors.New("realityfront: route key must be at least 16 bytes")
	}
	if normalizeName(serverName) == "" {
		return nil, errors.New("realityfront: server name is required")
	}
	return &MarkerRand{source: source, key: append([]byte(nil), routeKey...), serverName: normalizeName(serverName)}, nil
}

func (r *MarkerRand) Read(p []byte) (int, error) {
	if len(p) == 32 && r.stage == 0 {
		if _, err := io.ReadFull(r.source, p); err != nil {
			return 0, err
		}
		copy(r.random[:], p)
		r.stage = 1
		return len(p), nil
	}
	if len(p) == 32 && r.stage == 1 {
		m := markerFor(r.key, r.serverName, r.random)
		copy(p, m[:])
		r.stage = 2
		return len(p), nil
	}
	return r.source.Read(p)
}

func markerFor(key []byte, serverName string, random [32]byte) [markerLen]byte {
	mac := hmac.New(sha256.New, key)
	_, _ = io.WriteString(mac, markerLabel)
	_, _ = mac.Write(random[:])
	_, _ = io.WriteString(mac, normalizeName(serverName))
	var out [markerLen]byte
	copy(out[:], mac.Sum(nil))
	return out
}

type HelloIdentity struct {
	Random    [32]byte
	SessionID [32]byte
}

func ParseHelloIdentity(raw []byte) (HelloIdentity, error) {
	var out HelloIdentity
	handshake := make([]byte, 0, len(raw))
	for off := 0; off < len(raw); {
		if off+5 > len(raw) || raw[off] != 22 {
			return out, ErrUnsupportedHello
		}
		n := int(binary.BigEndian.Uint16(raw[off+3 : off+5]))
		off += 5
		if n <= 0 || off+n > len(raw) {
			return out, ErrUnsupportedHello
		}
		handshake = append(handshake, raw[off:off+n]...)
		off += n
	}
	if len(handshake) < 4 || handshake[0] != 1 {
		return out, ErrUnsupportedHello
	}
	msgLen := int(handshake[1])<<16 | int(handshake[2])<<8 | int(handshake[3])
	if msgLen <= 0 || 4+msgLen > len(handshake) {
		return out, ErrUnsupportedHello
	}
	b := handshake[4 : 4+msgLen]
	// legacy_version(2), random(32), legacy_session_id_len(1)
	if len(b) < 35 {
		return out, ErrUnsupportedHello
	}
	copy(out.Random[:], b[2:34])
	sidLen := int(b[34])
	if sidLen != 32 || len(b) < 35+sidLen {
		return out, ErrUnsupportedHello
	}
	copy(out.SessionID[:], b[35:67])
	return out, nil
}

func Recognized(raw, routeKey []byte, serverName string) bool {
	if len(routeKey) < 16 {
		return false
	}
	id, err := ParseHelloIdentity(raw)
	if err != nil {
		return false
	}
	want := markerFor(routeKey, serverName, id.Random)
	return subtle.ConstantTimeCompare(id.SessionID[:], want[:]) == 1
}

type replayConn struct {
	net.Conn
	prefix *bytes.Reader
}

func (c *replayConn) Read(p []byte) (int, error) {
	if c.prefix != nil && c.prefix.Len() != 0 {
		return c.prefix.Read(p)
	}
	return c.Conn.Read(p)
}

func ReplayConn(conn net.Conn, alreadyRead []byte) net.Conn {
	return &replayConn{Conn: conn, prefix: bytes.NewReader(append([]byte(nil), alreadyRead...))}
}

type ticketRecord struct {
	Version        int   `json:"version"`
	IssuedUnixNano int64 `json:"issued_unix_nano"`
}

func ensureTicketDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return errors.New("realityfront: ticket directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return os.Chmod(dir, 0o700)
}

func ticketPath(dir string, ticket Ticket) string {
	return filepath.Join(dir, ticket.Hex()+".json")
}

func RecordTicket(dir string, ticket Ticket, now time.Time) error {
	if err := ensureTicketDir(dir); err != nil {
		return err
	}
	if ticket == (Ticket{}) {
		return ErrTicket
	}
	body, err := json.Marshal(ticketRecord{Version: ticketVersion, IssuedUnixNano: now.UnixNano()})
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

func ConsumeTicket(dir string, ticket Ticket, now time.Time, ttl time.Duration) error {
	if ttl <= 0 || ticket == (Ticket{}) {
		return ErrTicket
	}
	path := ticketPath(dir, ticket)
	body, err := os.ReadFile(path)
	if err != nil {
		return ErrTicket
	}
	// Remove before checking contents so a malformed/stale ticket is still
	// one-shot and cannot be hammered repeatedly.
	_ = os.Remove(path)
	var rec ticketRecord
	if err := json.Unmarshal(body, &rec); err != nil || rec.Version != ticketVersion || rec.IssuedUnixNano <= 0 {
		return ErrTicket
	}
	issued := time.Unix(0, rec.IssuedUnixNano)
	if now.Before(issued) || now.Sub(issued) > ttl {
		return ErrTicket
	}
	return nil
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

func authMAC(password string, username []byte, clientNonce, serverNonce [32]byte) [32]byte {
	mac := hmac.New(sha256.New, []byte(password))
	_, _ = io.WriteString(mac, authLabel)
	_, _ = mac.Write(username)
	_, _ = mac.Write(clientNonce[:])
	_, _ = mac.Write(serverNonce[:])
	var out [32]byte
	copy(out[:], mac.Sum(nil))
	return out
}

// BootstrapClient authenticates inside the already-established recognized TLS
// branch. The password itself is never transmitted; only an HMAC challenge
// response is sent. The returned one-time ticket binds the later DTLS/WBD
// association without keeping VPN data in this ordinary TCP/TLS stream.
func BootstrapClient(conn net.Conn, username, password string) (Ticket, error) {
	var zero Ticket
	if len(username) == 0 || len(username) > maxUsernameLen || password == "" {
		return zero, ErrBootstrapAuth
	}
	var clientNonce [32]byte
	if _, err := io.ReadFull(rand.Reader, clientNonce[:]); err != nil {
		return zero, err
	}
	hello := make([]byte, 7+len(username)+32)
	copy(hello[:4], bootstrapMagic)
	hello[4] = bootstrapV1
	binary.BigEndian.PutUint16(hello[5:7], uint16(len(username)))
	copy(hello[7:7+len(username)], username)
	copy(hello[7+len(username):], clientNonce[:])
	if err := writeFull(conn, hello); err != nil {
		return zero, err
	}
	var serverNonce [32]byte
	if _, err := io.ReadFull(conn, serverNonce[:]); err != nil {
		return zero, err
	}
	proof := authMAC(password, []byte(username), clientNonce, serverNonce)
	if err := writeFull(conn, proof[:]); err != nil {
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

func BootstrapServer(conn net.Conn, expectedUsername, expectedPassword, ticketDir string, now time.Time) (Ticket, error) {
	var zero Ticket
	var hdr [7]byte
	if _, err := io.ReadFull(conn, hdr[:]); err != nil {
		return zero, err
	}
	if string(hdr[:4]) != bootstrapMagic || hdr[4] != bootstrapV1 {
		return zero, ErrBootstrapAuth
	}
	n := int(binary.BigEndian.Uint16(hdr[5:7]))
	if n <= 0 || n > maxUsernameLen {
		return zero, ErrBootstrapAuth
	}
	user := make([]byte, n)
	if _, err := io.ReadFull(conn, user); err != nil {
		return zero, err
	}
	var clientNonce [32]byte
	if _, err := io.ReadFull(conn, clientNonce[:]); err != nil {
		return zero, err
	}
	var serverNonce [32]byte
	if _, err := io.ReadFull(rand.Reader, serverNonce[:]); err != nil {
		return zero, err
	}
	if err := writeFull(conn, serverNonce[:]); err != nil {
		return zero, err
	}
	var got [32]byte
	if _, err := io.ReadFull(conn, got[:]); err != nil {
		return zero, err
	}
	want := authMAC(expectedPassword, []byte(expectedUsername), clientNonce, serverNonce)
	userOK := subtle.ConstantTimeCompare([]byte(string(user)), []byte(expectedUsername)) == 1
	proofOK := subtle.ConstantTimeCompare(got[:], want[:]) == 1
	if userOK != 1 || proofOK != 1 || expectedUsername == "" || expectedPassword == "" {
		_ = writeFull(conn, make([]byte, 1+TicketLen))
		return zero, ErrBootstrapAuth
	}
	if _, err := io.ReadFull(rand.Reader, zero[:]); err != nil {
		return Ticket{}, err
	}
	if err := RecordTicket(ticketDir, zero, now); err != nil {
		return Ticket{}, err
	}
	reply := make([]byte, 1+TicketLen)
	copy(reply[1:], zero[:])
	if err := writeFull(conn, reply); err != nil {
		return Ticket{}, err
	}
	return zero, nil
}

type ServerConfig struct {
	RouteKey         []byte
	ServerName       string
	TLSConfig        *tls.Config
	ExpectedUsername string
	ExpectedPassword string
	TicketDir        string
	Mirror           realitymirror.Config
	HelloTimeout     time.Duration
}

type ServerResult struct {
	Branch string
	Ticket Ticket
}

// HandleServerConn implements the useful REALITY-style split on one TCP
// listener: read one ClientHello, classify it, and either take over that same
// connection locally or pass the exact already-read bytes to the genuine
// fallback target. Sustained WBD payload is deliberately not carried here.
func HandleServerConn(ctx context.Context, conn net.Conn, cfg ServerConfig) (ServerResult, error) {
	var out ServerResult
	if conn == nil || cfg.TLSConfig == nil || len(cfg.RouteKey) < 16 || normalizeName(cfg.ServerName) == "" {
		return out, errors.New("realityfront: incomplete server config")
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
	ticket, err := BootstrapServer(tlsConn, cfg.ExpectedUsername, cfg.ExpectedPassword, cfg.TicketDir, time.Now())
	if err != nil {
		return out, err
	}
	out.Ticket = ticket
	return out, nil
}

func normalizeName(s string) string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s)), ".")
}

func (t Ticket) String() string { return t.Hex() }
func (r ServerResult) String() string { return fmt.Sprintf("branch=%s ticket=%s", r.Branch, r.Ticket.Hex()) }
