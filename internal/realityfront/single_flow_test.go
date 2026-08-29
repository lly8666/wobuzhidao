package realityfront

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"
)

func TestSingleFlowTLSAdmissionUsesProvidedConnection(t *testing.T) {
	cert := makeServerCert(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	ticketDir := t.TempDir()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		_, err := BootstrapServerSingleFlow(context.Background(), serverConn, SingleFlowServerConfig{
			ServerName: "target.test", RouteKey: key,
			ExpectedUsername: "solo", ExpectedPassword: "single-flow-password",
			TicketDir: ticketDir, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}, Timeout: 2 * time.Second,
		})
		serverDone <- err
	}()

	ticket, state, err := BootstrapClientSingleFlow(context.Background(), clientConn, SingleFlowClientConfig{
		ServerName: "target.test", RouteKey: key, Username: "solo", Password: "single-flow-password",
		VerifyServer: false, Timeout: 2 * time.Second,
	})
	if err != nil { t.Fatal(err) }
	if state.Version != tls.VersionTLS13 { t.Fatalf("TLS version=%x", state.Version) }
	if err := <-serverDone; err != nil { t.Fatal(err) }
	if err := ConsumeTicket(ticketDir, ticket, time.Now(), time.Minute); err != nil { t.Fatalf("consume ticket: %v", err) }
}

func TestSingleFlowWrongMarkerIsRejectedBeforeAdmission(t *testing.T) {
	cert := makeServerCert(t)
	serverKey := []byte("0123456789abcdef0123456789abcdef")
	clientKey := []byte("fedcba9876543210fedcba9876543210")
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		_, err := BootstrapServerSingleFlow(context.Background(), serverConn, SingleFlowServerConfig{
			ServerName: "target.test", RouteKey: serverKey,
			ExpectedUsername: "solo", ExpectedPassword: "single-flow-password", TicketDir: t.TempDir(),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}, Timeout: time.Second,
		})
		serverDone <- err
	}()
	_, _, _ = BootstrapClientSingleFlow(context.Background(), clientConn, SingleFlowClientConfig{
		ServerName: "target.test", RouteKey: clientKey, Username: "solo", Password: "single-flow-password", Timeout: time.Second,
	})
	if err := <-serverDone; err != ErrMarker { t.Fatalf("server err=%v want ErrMarker", err) }
}
