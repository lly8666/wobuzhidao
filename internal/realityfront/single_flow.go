package realityfront

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"time"

	utls "github.com/refraction-networking/utls"

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

type SingleFlowHello struct {
	Info       realitymirror.HelloInfo
	Raw        []byte
	Recognized bool
}

// newSingleFlowFirefox120Client prepares a pinned Firefox 120 ClientHello on
// the already-established FakeTCP bootstrap stream. WBD changes only the TLS
// 1.3 compatibility SessionID: it is derived from the preset's own random and
// remains the route classifier marker recognized by the server. The browser
// cipher/extension/GREASE/group/ALPN persona otherwise stays owned by uTLS.
func newSingleFlowFirefox120Client(conn net.Conn, cfg SingleFlowClientConfig) (*utls.UConn, error) {
	if conn == nil || strings.TrimSpace(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 {
		return nil, errors.New("realityfront: incomplete Firefox 120 client config")
	}
	uconn := utls.UClient(conn, &utls.Config{
		ServerName:         cfg.ServerName,
		InsecureSkipVerify: !cfg.VerifyServer,
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
	// A second BuildHandshakeState remarshal preserves the selected Firefox
	// preset while incorporating the WBD compatibility SessionID marker.
	if err := uconn.BuildHandshakeState(); err != nil {
		return nil, err
	}
	return uconn, nil
}

func standardTLSState(state utls.ConnectionState) tls.ConnectionState {
	return tls.ConnectionState{
		Version:                     state.Version,
		HandshakeComplete:           state.HandshakeComplete,
		DidResume:                   state.DidResume,
		CipherSuite:                 state.CipherSuite,
		NegotiatedProtocol:          state.NegotiatedProtocol,
		NegotiatedProtocolIsMutual:  state.NegotiatedProtocolIsMutual,
		ServerName:                  state.ServerName,
		PeerCertificates:            state.PeerCertificates,
		VerifiedChains:              state.VerifiedChains,
		SignedCertificateTimestamps: state.SignedCertificateTimestamps,
		OCSPResponse:                state.OCSPResponse,
		TLSUnique:                   state.TLSUnique,
	}
}

// BootstrapClientSingleFlow performs real TLS 1.3 and WBD admission on an
// already-established FakeTCP bootstrap stream. It never dials, closes or
// replaces the public transport association. The ClientHello uses the pinned
// Firefox 120 uTLS persona while the WBD route marker occupies only the normal
// 32-byte TLS 1.3 compatibility SessionID.
func BootstrapClientSingleFlow(ctx context.Context, conn net.Conn, cfg SingleFlowClientConfig) (Ticket, tls.ConnectionState, error) {
	var zero Ticket
	var state tls.ConnectionState
	if conn == nil || strings.TrimSpace(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 || cfg.Username == "" || cfg.Password == "" {
		return zero, state, errors.New("realityfront: incomplete single-flow client config")
	}
	timeout := cfg.Timeout
	if timeout <= 0 { timeout = 10 * time.Second }
	_ = conn.SetDeadline(time.Now().Add(timeout))
	uconn, err := newSingleFlowFirefox120Client(conn, cfg)
	if err != nil { return zero, state, err }
	if err := uconn.HandshakeContext(ctx); err != nil { return zero, state, err }
	ticket, err := BootstrapClientSimple(uconn, cfg.Username, cfg.Password)
	state = standardTLSState(uconn.ConnectionState())
	if err != nil { return zero, state, err }
	_ = conn.SetDeadline(time.Time{})
	return ticket, state, nil
}

// ReadSingleFlowHello consumes exactly the records required for ClientHello and
// returns their byte-for-byte wire representation. This lets the raw mux choose
// between the WBD TLS takeover and transparent decoy forwarding without needing
// a second public TCP listener.
func ReadSingleFlowHello(conn net.Conn, serverName string, routeKey []byte, timeout time.Duration) (SingleFlowHello, error) {
	var out SingleFlowHello
	if conn == nil || strings.TrimSpace(serverName) == "" || len(routeKey) < 16 {
		return out, errors.New("realityfront: incomplete single-flow classifier config")
	}
	if timeout <= 0 { timeout = 10 * time.Second }
	info, raw, err := realitymirror.ReadClientHello(conn, 64<<10, timeout)
	if err != nil { return out, err }
	out.Info = info
	out.Raw = raw
	out.Recognized = normalizeName(info.ServerName) == normalizeName(serverName) && Recognized(raw, routeKey, info.ServerName)
	return out, nil
}

// BootstrapServerRecognizedSingleFlow performs the TLS takeover after the mux
// has already classified and retained the ClientHello bytes. The bytes are
// replayed into crypto/tls so the handshake transcript remains exact.
func BootstrapServerRecognizedSingleFlow(ctx context.Context, conn net.Conn, rawHello []byte, cfg SingleFlowServerConfig) (Ticket, error) {
	var zero Ticket
	if conn == nil || cfg.TLSConfig == nil || strings.TrimSpace(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 || cfg.ExpectedUsername == "" || cfg.ExpectedPassword == "" || strings.TrimSpace(cfg.TicketDir) == "" || len(rawHello) == 0 {
		return zero, errors.New("realityfront: incomplete recognized single-flow server config")
	}
	timeout := cfg.Timeout
	if timeout <= 0 { timeout = 10 * time.Second }
	_ = conn.SetDeadline(time.Now().Add(timeout))
	tlsCfg := cfg.TLSConfig.Clone()
	tlsCfg.MinVersion = tls.VersionTLS13
	tlsCfg.MaxVersion = tls.VersionTLS13
	tlsConn := tls.Server(ReplayConn(conn, rawHello), tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil { return zero, err }
	ticket, err := BootstrapServerSimple(tlsConn, cfg.ExpectedUsername, cfg.ExpectedPassword, cfg.TicketDir, time.Now())
	if err != nil { return zero, err }
	_ = conn.SetDeadline(time.Time{})
	return ticket, nil
}

// BootstrapServerSingleFlow is the direct recognized-product helper used by
// focused tests. Product mux code normally calls ReadSingleFlowHello first so
// unrecognized probes can be forwarded to the decoy instead of being dropped.
func BootstrapServerSingleFlow(ctx context.Context, conn net.Conn, cfg SingleFlowServerConfig) (Ticket, error) {
	var zero Ticket
	if conn == nil || cfg.TLSConfig == nil || strings.TrimSpace(cfg.ServerName) == "" || len(cfg.RouteKey) < 16 || cfg.ExpectedUsername == "" || cfg.ExpectedPassword == "" || strings.TrimSpace(cfg.TicketDir) == "" {
		return zero, errors.New("realityfront: incomplete single-flow server config")
	}
	hello, err := ReadSingleFlowHello(conn, cfg.ServerName, cfg.RouteKey, cfg.Timeout)
	if err != nil { return zero, err }
	if !hello.Recognized { return zero, ErrMarker }
	return BootstrapServerRecognizedSingleFlow(ctx, conn, hello.Raw, cfg)
}
