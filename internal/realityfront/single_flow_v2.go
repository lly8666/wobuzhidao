package realityfront

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

type SingleFlowClientV2Config struct {
	SingleFlowClientConfig
	InstallationID logicaltunnel.InstallationID
}

type SingleFlowServerV2Config struct {
	SingleFlowServerConfig
	LeaseProvider TunnelLeaseProvider
}

type SingleFlowBootstrapV2Result struct {
	AuthenticatedTunnel
	TLSState tls.ConnectionState
}

func BootstrapClientSingleFlowV2(ctx context.Context, conn net.Conn, cfg SingleFlowClientV2Config) (SingleFlowBootstrapV2Result, error) {
	var zero SingleFlowBootstrapV2Result
	base := cfg.SingleFlowClientConfig
	if conn == nil || strings.TrimSpace(base.ServerName) == "" || len(base.RouteKey) < 16 || base.Username == "" || base.Password == "" {
		return zero, errors.New("realityfront: incomplete single-flow v2 client config")
	}
	installation, err := logicaltunnel.ParseInstallationID(string(cfg.InstallationID))
	if err != nil {
		return zero, err
	}
	timeout := base.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	uconn, err := newSingleFlowFirefox120Client(conn, base)
	if err != nil {
		return zero, err
	}
	if err := uconn.HandshakeContext(ctx); err != nil {
		return zero, err
	}
	auth, err := BootstrapClientSimpleV2(uconn, base.Username, base.Password, installation)
	state := standardTLSState(uconn.ConnectionState())
	if err != nil {
		return zero, err
	}
	_ = conn.SetDeadline(time.Time{})
	return SingleFlowBootstrapV2Result{AuthenticatedTunnel: auth, TLSState: state}, nil
}

func BootstrapServerRecognizedSingleFlowV2(ctx context.Context, conn net.Conn, rawHello []byte, cfg SingleFlowServerV2Config) (AuthenticatedTunnel, error) {
	var zero AuthenticatedTunnel
	base := cfg.SingleFlowServerConfig
	if conn == nil || base.TLSConfig == nil || strings.TrimSpace(base.ServerName) == "" || len(base.RouteKey) < 16 || base.ExpectedUsername == "" || base.ExpectedPassword == "" || strings.TrimSpace(base.TicketDir) == "" || len(rawHello) == 0 || cfg.LeaseProvider == nil {
		return zero, errors.New("realityfront: incomplete recognized single-flow v2 server config")
	}
	timeout := base.Timeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	_ = conn.SetDeadline(time.Now().Add(timeout))
	tlsCfg := base.TLSConfig.Clone()
	tlsCfg.MinVersion = tls.VersionTLS13
	tlsCfg.MaxVersion = tls.VersionTLS13
	tlsConn := tls.Server(ReplayConn(conn, rawHello), tlsCfg)
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return zero, err
	}
	result, err := BootstrapServerSimpleV2(tlsConn, base.ExpectedUsername, base.ExpectedPassword, base.TicketDir, cfg.LeaseProvider, time.Now())
	if err != nil {
		return zero, err
	}
	_ = conn.SetDeadline(time.Time{})
	return result, nil
}

func BootstrapServerSingleFlowV2(ctx context.Context, conn net.Conn, cfg SingleFlowServerV2Config) (AuthenticatedTunnel, error) {
	var zero AuthenticatedTunnel
	base := cfg.SingleFlowServerConfig
	if conn == nil || base.TLSConfig == nil || strings.TrimSpace(base.ServerName) == "" || len(base.RouteKey) < 16 || base.ExpectedUsername == "" || base.ExpectedPassword == "" || strings.TrimSpace(base.TicketDir) == "" || cfg.LeaseProvider == nil {
		return zero, errors.New("realityfront: incomplete single-flow v2 server config")
	}
	hello, err := ReadSingleFlowHello(conn, base.ServerName, base.RouteKey, base.Timeout)
	if err != nil {
		return zero, err
	}
	if !hello.Recognized {
		return zero, ErrMarker
	}
	return BootstrapServerRecognizedSingleFlowV2(ctx, conn, hello.Raw, cfg)
}
