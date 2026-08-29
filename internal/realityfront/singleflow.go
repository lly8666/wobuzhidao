package realityfront

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

// SingleFlowClientConfig drives the Reality-like setup that runs inside the
// initial ordered phase of one FakeTCP association. The underlying net.Conn is
// supplied by the raw carrier; this package never dials or closes a public TCP
// socket for the single-flow path.
type SingleFlowClientConfig struct {
	ServerName   string
	RouteKey     []byte
	Username     string
	Password     string
	VerifyServer bool
	Timeout      time.Duration
}

// BootstrapClientSingleFlow performs a real TLS 1.3 ClientHello/handshake and
// the existing simple username/password admission over an already-established
// FakeTCP bootstrap stream. It deliberately does not call tls.Conn.Close: a
// TLS close_notify would look like end-of-stream, while V3 must continue the
// same TCP-shaped sequence space into its explicit DTLS mode-switch barrier.
func BootstrapClientSingleFlow(ctx context.Context, raw net.Conn, cfg SingleFlowClientConfig) (Ticket, *tls.Conn, error) {
	var zero Ticket
	if raw == nil || normalizeName(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 || cfg.Username == "" || cfg.Password == "" {
		return zero, nil, errors.New("realityfront: incomplete single-flow client config")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	mr, err := NewMarkerRand(rand.Reader, cfg.RouteKey, cfg.ServerName)
	if err != nil {
		return zero, nil, err
	}
	deadline := time.Now().Add(cfg.Timeout)
	_ = raw.SetDeadline(deadline)
	tlsConn := tls.Client(raw, &tls.Config{
		ServerName: cfg.ServerName,
		InsecureSkipVerify: !cfg.VerifyServer,
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
		Rand: mr,
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return zero, nil, err
	}
	ticket, err := BootstrapClientSimple(tlsConn, cfg.Username, cfg.Password)
	if err != nil {
		return zero, nil, err
	}
	_ = raw.SetDeadline(time.Time{})
	return ticket, tlsConn, nil
}

// HandleServerConnSimpleSingleFlow is the WBD-only counterpart to
// HandleServerConnSimple. The raw FakeTCP SYN signature has already selected a
// WBD association, so there is intentionally no fallback branch here. The
// function still parses the real TLS ClientHello and validates the same
// Reality-like marker/SNI before TLS takeover and account admission.
//
// The returned tls.Conn is intentionally left open. The caller abandons the
// TLS stream only after its transport-level SWITCH_REQ/SWITCH_ACK barrier has
// cumulatively acknowledged all bootstrap bytes in the same FakeTCP sequence
// space.
func HandleServerConnSimpleSingleFlow(ctx context.Context, conn net.Conn, cfg ServerConfig) (ServerResult, *tls.Conn, error) {
	var out ServerResult
	if conn == nil || cfg.TLSConfig == nil || len(cfg.RouteKey) < 16 || normalizeName(cfg.ServerName) == "" {
		return out, nil, ErrMarker
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
		return out, nil, err
	}
	if !Recognized(raw, cfg.RouteKey, info.ServerName) || normalizeName(info.ServerName) != normalizeName(cfg.ServerName) {
		return out, nil, ErrMarker
	}
	out.Branch = "wbd"
	tlsCfg := cfg.TLSConfig.Clone()
	if tlsCfg.MinVersion == 0 || tlsCfg.MinVersion < tls.VersionTLS13 {
		tlsCfg.MinVersion = tls.VersionTLS13
	}
	tlsCfg.MaxVersion = tls.VersionTLS13
	tlsConn := tls.Server(ReplayConn(conn, raw), tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return out, nil, err
	}
	ticket, err := BootstrapServerSimple(tlsConn, cfg.ExpectedUsername, cfg.ExpectedPassword, cfg.TicketDir, time.Now())
	if err != nil {
		return out, nil, err
	}
	out.Ticket = ticket
	return out, tlsConn, nil
}
