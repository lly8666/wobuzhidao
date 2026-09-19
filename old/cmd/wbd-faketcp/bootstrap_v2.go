package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func runSingleFlowBootstrapV2(stream net.Conn, c config) (tls.ConnectionState, error) {
	var zero tls.ConnectionState
	installation, err := logicaltunnel.ParseInstallationID(c.realityInstallationID)
	if err != nil {
		return zero, fmt.Errorf("parse installation id: %w", err)
	}
	result, err := realityfront.BootstrapClientSingleFlowV2(context.Background(), stream, realityfront.SingleFlowClientV2Config{
		SingleFlowClientConfig: realityfront.SingleFlowClientConfig{
			ServerName: c.realityServerName,
			RouteKey: []byte(c.realityRouteKey),
			Username: c.realityUsername,
			Password: c.realityPassword,
			VerifyServer: c.realityVerify,
			Timeout: c.realityTimeout,
		},
		InstallationID: installation,
	})
	if err != nil {
		return zero, err
	}
	if err := result.Config.Validate(); err != nil {
		return zero, fmt.Errorf("validate authenticated tunnel config: %w", err)
	}
	configJSON, err := json.Marshal(result.Config)
	if err != nil {
		return zero, err
	}
	configJSON = append(configJSON, '\n')
	if err := os.WriteFile(strings.TrimSpace(c.realityTicketOut), []byte(result.Ticket.Hex()+"\n"), 0o600); err != nil {
		return zero, fmt.Errorf("write ticket: %w", err)
	}
	if err := os.WriteFile(strings.TrimSpace(c.realityTunnelConfigOut), configJSON, 0o600); err != nil {
		_ = os.Remove(strings.TrimSpace(c.realityTicketOut))
		return zero, fmt.Errorf("write tunnel config: %w", err)
	}
	return result.TLSState, nil
}
