package realityfront

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"

	"github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

type ClientConfig struct {
	ServerName   string
	RouteKey     []byte
	VerifyServer bool
	Timeout      time.Duration
}

type ServerConfig struct {
	ServerName string
	RouteKey   []byte
	TLSConfig  *tls.Config
	Timeout    time.Duration
}

type ExporterParams struct {
	Version          uint16
	IncarnationNonce [16]byte
	TunnelID         []byte
	ClientLimit      uint16
	ServerLimit      uint16
}

type ClientSession struct {
	Conn *utls.UConn
	Keys tlsrecord.KeyPair
}

type ServerSession struct {
	Conn  *tls.Conn
	Hello Hello
	Keys  tlsrecord.KeyPair
}

type keyingMaterialExporter interface {
	ExportKeyingMaterial(label string, context []byte, length int) ([]byte, error)
}

func newFirefox120Client(conn net.Conn, cfg ClientConfig) (*utls.UConn, error) {
	if conn == nil || normalizeName(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 {
		return nil, errors.New("realityfront: incomplete Firefox 120 client config")
	}
	uconn := utls.UClient(conn, &utls.Config{
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: !cfg.VerifyServer,
		Renegotiation:      utls.RenegotiateNever,
	}, utls.HelloFirefox_120)
	if err := uconn.BuildHandshakeState(); err != nil {
		return nil, err
	}
	hello := uconn.HandshakeState.Hello
	if hello == nil || len(hello.Random) != 32 {
		return nil, errors.New("realityfront: Firefox 120 ClientHello has invalid random")
	}
	var random [32]byte
	copy(random[:], hello.Random)
	marker := markerFor(cfg.RouteKey, cfg.ServerName, random)
	hello.SessionId = append([]byte(nil), marker[:]...)
	if err := uconn.BuildHandshakeState(); err != nil {
		return nil, err
	}
	return uconn, nil
}

// HandshakeClient performs real TLS over the provided already-established
// transport. It never dials or replaces the connection. Exporter material is
// derived directly from uTLS's real ConnectionState, not from a copied
// crypto/tls state that would lose the exporter closure.
func HandshakeClient(ctx context.Context, conn net.Conn, cfg ClientConfig, params ExporterParams) (*ClientSession, error) {
	if conn == nil || normalizeName(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 {
		return nil, errors.New("realityfront: incomplete client config")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	uconn, err := newFirefox120Client(conn, cfg)
	if err != nil {
		return nil, err
	}
	if err := uconn.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	state := uconn.ConnectionState()
	if state.Version != utls.VersionTLS13 || !state.HandshakeComplete {
		return nil, errors.New("realityfront: TLS 1.3 handshake not complete")
	}
	keys, err := deriveRecordKeys(&state, params)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return &ClientSession{Conn: uconn, Keys: keys}, nil
}

// HandshakeServerRecognized replays exactly the classified ClientHello into a
// real crypto/tls server on the same transport. Session tickets are disabled so
// no post-handshake TLS writer remains after the explicit transition prepare.
func HandshakeServerRecognized(ctx context.Context, conn net.Conn, hello Hello, cfg ServerConfig, params ExporterParams) (*ServerSession, error) {
	if conn == nil || cfg.TLSConfig == nil || normalizeName(cfg.ServerName) == "" ||
		len(cfg.RouteKey) < 16 || len(hello.Raw) == 0 {
		return nil, errors.New("realityfront: incomplete recognized server config")
	}
	if !hello.Recognized || normalizeName(hello.Info.ServerName) != normalizeName(cfg.ServerName) ||
		!recognized(hello.Raw, cfg.RouteKey, hello.Info.ServerName) {
		return nil, ErrMarker
	}

	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))

	tlsCfg := cfg.TLSConfig.Clone()
	tlsCfg.MinVersion = tls.VersionTLS13
	tlsCfg.MaxVersion = tls.VersionTLS13
	tlsCfg.Renegotiation = tls.RenegotiateNever
	tlsCfg.SessionTicketsDisabled = true
	tlsConn := tls.Server(replay(conn, hello.Raw), tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	state := tlsConn.ConnectionState()
	if state.Version != tls.VersionTLS13 || !state.HandshakeComplete {
		return nil, errors.New("realityfront: TLS 1.3 server handshake not complete")
	}
	keys, err := deriveRecordKeys(&state, params)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return &ServerSession{Conn: tlsConn, Hello: hello, Keys: keys}, nil
}

func HandshakeServer(ctx context.Context, conn net.Conn, cfg ServerConfig, params ExporterParams) (*ServerSession, error) {
	if conn == nil || cfg.TLSConfig == nil || strings.TrimSpace(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 {
		return nil, errors.New("realityfront: incomplete server config")
	}
	hello, err := ReadHello(conn, cfg.ServerName, cfg.RouteKey, cfg.Timeout)
	if err != nil {
		return nil, err
	}
	if !hello.Recognized {
		return nil, ErrMarker
	}
	return HandshakeServerRecognized(ctx, conn, hello, cfg, params)
}

func deriveRecordKeys(exporter keyingMaterialExporter, params ExporterParams) (tlsrecord.KeyPair, error) {
	if exporter == nil {
		return tlsrecord.KeyPair{}, errors.New("realityfront: nil TLS exporter")
	}
	contextHash, err := tlsrecord.ExporterContextHash(
		params.Version,
		params.IncarnationNonce,
		params.TunnelID,
		params.ClientLimit,
		params.ServerLimit,
	)
	if err != nil {
		return tlsrecord.KeyPair{}, err
	}
	master, err := exporter.ExportKeyingMaterial(
		tlsrecord.ExporterLabel,
		contextHash[:],
		32,
	)
	if err != nil {
		return tlsrecord.KeyPair{}, err
	}
	return tlsrecord.DeriveKeys(master, params.IncarnationNonce)
}
