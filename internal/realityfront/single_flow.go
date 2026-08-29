package realityfront

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

type SingleFlowClientConfig struct {
	ServerName   string
	RouteKey     []byte
	Username     string
	Password     string
	VerifyServer bool
	Timeout      time.Duration
}

type SingleFlowServerConfig struct {
	ServerName       string
	RouteKey         []byte
	ExpectedUsername string
	ExpectedPassword string
	TicketDir        string
	TLSConfig        *tls.Config
	Timeout          time.Duration
}

// BootstrapClientSingleFlow performs real TLS 1.3 and WBD admission on an
// already-established FakeTCP bootstrap stream. It never dials, closes or
// replaces the public transport association.
func BootstrapClientSingleFlow(ctx context.Context, conn net.Conn, cfg SingleFlowClientConfig) (Ticket, tls.ConnectionState, error) {
	var zero Ticket
	var state tls.ConnectionState
	if conn == nil || strings.TrimSpace(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 || cfg.Username == "" || cfg.Password == "" {
		return zero, state, errors.New("realityfront: incomplete single-flow client config")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	mr, err := NewMarkerRand(rand.Reader, cfg.RouteKey, cfg.ServerName)
	if err != nil {
		return zero, state, err
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	tlsConn := tls.Client(conn, &tls.Config{
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: !cfg.VerifyServer,
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		Rand:               mr,
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return zero, state, err
	}
	ticket, err := BootstrapClientSimple(tlsConn, cfg.Username, cfg.Password)
	state = tlsConn.ConnectionState()
	if err != nil {
		return zero, state, err
	}
	_ = conn.SetDeadline(time.Time{})
	return ticket, state, nil
}

// BootstrapServerSingleFlow recognizes and authenticates a WBD TLS ClientHello
// on an existing FakeTCP bootstrap stream. Unrecognized ClientHello is returned
// as ErrMarker; the raw-listener caller may later route that stream to the
// configured decoy proxy without creating WBD DTLS state.
func BootstrapServerSingleFlow(ctx context.Context, conn net.Conn, cfg SingleFlowServerConfig) (Ticket, error) {
	var zero Ticket
	if conn == nil || cfg.TLSConfig == nil || strings.TrimSpace(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 || cfg.ExpectedUsername == "" || cfg.ExpectedPassword == "" || strings.TrimSpace(cfg.TicketDir) == "" {
		return zero, errors.New("realityfront: incomplete single-flow server config")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	info, raw, err := realitymirror.ReadClientHello(conn, 64<<10, timeout)
	if err != nil {
		return zero, err
	}
	if normalizeName(info.ServerName) != normalizeName(cfg.ServerName) || !Recognized(raw, cfg.RouteKey, info.ServerName) {
		return zero, ErrMarker
	}
	tlsCfg := cfg.TLSConfig.Clone()
	tlsCfg.MinVersion = tls.VersionTLS13
	tlsCfg.MaxVersion = tls.VersionTLS13
	tlsConn := tls.Server(ReplayConn(conn, raw), tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return zero, err
	}
	ticket, err := BootstrapServerSimple(tlsConn, cfg.ExpectedUsername, cfg.ExpectedPassword, cfg.TicketDir, time.Now())
	if err != nil {
		return zero, err
	}
	_ = conn.SetDeadline(time.Time{})
	return ticket, nil
}
