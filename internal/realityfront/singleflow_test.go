package realityfront

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

func TestSingleFlowBootstrapKeepsCarrierOpen(t *testing.T) {
	cert := makeServerCert(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	ticketDir := t.TempDir()
	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()

	type serverResult struct {
		ticket Ticket
		err    error
	}
	serverDone := make(chan serverResult, 1)
	go func() {
		res, tlsConn, err := HandleServerConnSimpleSingleFlow(context.Background(), serverRaw, ServerConfig{
			RouteKey: key, ServerName: "target.test",
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13},
			ExpectedUsername: "solo", ExpectedPassword: "correct-password", TicketDir: ticketDir,
			HelloTimeout: time.Second,
			Mirror: realitymirror.Config{MaxHelloBytes: 64 << 10},
		})
		if err == nil && tlsConn == nil {
			err = errors.New("server did not return live TLS handle")
		}
		serverDone <- serverResult{ticket: res.Ticket, err: err}
	}()

	ticket, tlsConn, err := BootstrapClientSingleFlow(context.Background(), clientRaw, SingleFlowClientConfig{
		ServerName: "target.test", RouteKey: key,
		Username: "solo", Password: "correct-password", VerifyServer: false, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if tlsConn == nil || tlsConn.ConnectionState().Version != tls.VersionTLS13 {
		t.Fatal("client did not negotiate TLS 1.3")
	}
	server := <-serverDone
	if server.err != nil {
		t.Fatal(server.err)
	}
	if ticket != server.ticket || ticket == (Ticket{}) {
		t.Fatal("client/server ticket mismatch")
	}

	// Neither helper sends close_notify or closes the underlying carrier. This
	// byte models the following transport-level SWITCH_REQ on the same flow.
	go func() { _, _ = clientRaw.Write([]byte("S")) }()
	var b [1]byte
	if _, err := serverRaw.Read(b[:]); err != nil || b[0] != 'S' {
		t.Fatalf("single flow was not reusable after TLS bootstrap: b=%q err=%v", b[:], err)
	}
}
