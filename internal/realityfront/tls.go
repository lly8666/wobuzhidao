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
	// Timeout is the absolute candidate establishment budget when used by
	// EstablishClient/HandleServerAssociation. Handshake-only helpers use it as
	// their phase timeout.
	Timeout time.Duration
}

type ServerConfig struct {
	ServerName string
	RouteKey   []byte
	TLSConfig  *tls.Config
	// Timeout is the absolute candidate establishment budget when used by the
	// admission/server-association entry points.
	Timeout time.Duration
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

// recognizedServerTLSConfig fixes the local WBD server-side TLS policy. This is
// intentionally not presented as a target-site fingerprint clone:
//
//   - TLS version is exactly 1.3.
//   - certificate selection comes from the configured Certificates/GetCertificate.
//   - ALPN is deliberately empty because WBD does not implement h2/http/1.1 on
//     this recognized path.
//   - Go TLS 1.3 session tickets are enabled so the real TLS implementation emits
//     its normal post-handshake ticket in the handshake flight, but resumption is
//     explicitly rejected. Every WBD lane therefore still performs a full TLS
//     handshake, protected admission, fresh exporter context and fresh nonce.
//   - 0-RTT is not enabled; crypto/tls TCP rejects TLS 1.3 early_data.
//
// Go 1.23.12 sends at most one automatic TLS 1.3 ticket from
// serverHandshakeStateTLS13.sendSessionTickets before Handshake returns. Keeping
// ticket generation inside that ownership interval means the later
// PrepareTransition/final admission reply/DetachTransition sequence never relies
// on sleeps and never retains a TLS writer for a deferred ticket.
func recognizedServerTLSConfig(base *tls.Config) *tls.Config {
	cfg := base.Clone()
	cfg.MinVersion = tls.VersionTLS13
	cfg.MaxVersion = tls.VersionTLS13
	cfg.Renegotiation = tls.RenegotiateNever
	cfg.NextProtos = nil
	cfg.SessionTicketsDisabled = false
	// A callback could otherwise swap in an unrestricted config after the policy
	// above. Certificates/GetCertificate remain available for the configured SNI.
	cfg.GetConfigForClient = nil
	cfg.UnwrapSession = func([]byte, tls.ConnectionState) (*tls.SessionState, error) {
		return nil, nil
	}
	return cfg
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

	// HelloFirefox_120 intentionally includes the renegotiation_info extension.
	// In uTLS v1.6.5 that extension also mutates Config.Renegotiation to
	// RenegotiateOnceAsClient during ApplyConfig, which disables RFC 5705/8446
	// exporters even on a TLS 1.3 connection. Keep the extension (and therefore
	// the Firefox wire persona) but make its internal acceptance policy Never.
	// uTLS documents that RenegotiationInfoExtension is still serialized when
	// this field is RenegotiateNever.
	for _, ext := range uconn.Extensions {
		if reneg, ok := ext.(*utls.RenegotiationInfoExtension); ok {
			reneg.Renegotiation = utls.RenegotiateNever
		}
	}
	if err := uconn.BuildHandshakeState(); err != nil {
		return nil, err
	}
	return uconn, nil
}

// handshakeClientConn performs the real uTLS handshake but deliberately does
// not derive record keys. Admission needs to negotiate the exporter context
// inside this already-protected connection first.
func handshakeClientConn(ctx context.Context, conn net.Conn, cfg ClientConfig) (*utls.UConn, error) {
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
	_ = conn.SetDeadline(time.Time{})
	return uconn, nil
}

// HandshakeClient performs real TLS over the provided already-established
// transport and derives record keys when all exporter parameters are already
// known. Admission-oriented callers use handshakeClientConn first instead.
func HandshakeClient(ctx context.Context, conn net.Conn, cfg ClientConfig, params ExporterParams) (*ClientSession, error) {
	uconn, err := handshakeClientConn(ctx, conn, cfg)
	if err != nil {
		return nil, err
	}
	state := uconn.ConnectionState()
	keys, err := deriveRecordKeys(&state, params)
	if err != nil {
		return nil, err
	}
	return &ClientSession{Conn: uconn, Keys: keys}, nil
}

// handshakeServerRecognizedConn replays exactly the classified ClientHello into
// a real crypto/tls server but waits to derive record keys until admission has
// fixed the exporter context.
func handshakeServerRecognizedConn(ctx context.Context, conn net.Conn, hello Hello, cfg ServerConfig) (*tls.Conn, error) {
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

	tlsCfg := recognizedServerTLSConfig(cfg.TLSConfig)
	tlsConn := tls.Server(replay(conn, hello.Raw), tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	state := tlsConn.ConnectionState()
	if state.Version != tls.VersionTLS13 || !state.HandshakeComplete {
		return nil, errors.New("realityfront: TLS 1.3 server handshake not complete")
	}
	_ = conn.SetDeadline(time.Time{})
	return tlsConn, nil
}

// HandshakeServerRecognized is the compatibility helper for callers that
// already know the exporter context before the TLS takeover.
func HandshakeServerRecognized(ctx context.Context, conn net.Conn, hello Hello, cfg ServerConfig, params ExporterParams) (*ServerSession, error) {
	tlsConn, err := handshakeServerRecognizedConn(ctx, conn, hello, cfg)
	if err != nil {
		return nil, err
	}
	state := tlsConn.ConnectionState()
	keys, err := deriveRecordKeys(&state, params)
	if err != nil {
		return nil, err
	}
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
