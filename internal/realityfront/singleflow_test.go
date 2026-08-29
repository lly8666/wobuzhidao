package realityfront

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
	"github.com/lly8666/wobuzhidao/internal/singleflow"
)

type recordingConn struct {
	net.Conn
	mu sync.Mutex
	b  bytes.Buffer
}

func (c *recordingConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	_, _ = c.b.Write(p)
	c.mu.Unlock()
	return c.Conn.Write(p)
}

func (c *recordingConn) bytes() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]byte(nil), c.b.Bytes()...)
}

func TestSingleFlowBootstrapKeepsCarrierOpenAndEncryptsSwitch(t *testing.T) {
	cert := makeServerCert(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	ticketDir := t.TempDir()
	clientPipe, serverPipe := net.Pipe()
	defer clientPipe.Close()
	defer serverPipe.Close()
	clientRaw := &recordingConn{Conn: clientPipe}
	serverRaw := &recordingConn{Conn: serverPipe}

	type serverResult struct {
		ticket Ticket
		tls    *tls.Conn
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
		serverDone <- serverResult{ticket: res.Ticket, tls: tlsConn, err: err}
	}()

	ticket, clientTLS, err := BootstrapClientSingleFlow(context.Background(), clientRaw, SingleFlowClientConfig{
		ServerName: "target.test", RouteKey: key,
		Username: "solo", Password: "correct-password", VerifyServer: false, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if clientTLS == nil || clientTLS.ConnectionState().Version != tls.VersionTLS13 {
		t.Fatal("client did not negotiate TLS 1.3")
	}
	server := <-serverDone
	if server.err != nil {
		t.Fatal(server.err)
	}
	if ticket != server.ticket || ticket == (Ticket{}) {
		t.Fatal("client/server ticket mismatch")
	}

	req := singleflow.SwitchRequest(ticket[:])
	ack := singleflow.SwitchAck(ticket[:])
	serverSwitch := make(chan error, 1)
	go func() {
		got := make([]byte, singleflow.SwitchFrameLen)
		if _, err := io.ReadFull(server.tls, got); err != nil {
			serverSwitch <- err
			return
		}
		if !singleflow.IsSwitchRequest(got, ticket[:]) {
			serverSwitch <- singleflow.ErrBadSwitchFrame
			return
		}
		_, err := server.tls.Write(ack)
		serverSwitch <- err
	}()

	if _, err := clientTLS.Write(req); err != nil {
		t.Fatal(err)
	}
	gotAck := make([]byte, singleflow.SwitchFrameLen)
	if _, err := io.ReadFull(clientTLS, gotAck); err != nil {
		t.Fatal(err)
	}
	if !singleflow.IsSwitchAck(gotAck, ticket[:]) {
		t.Fatal("client did not receive valid encrypted switch ACK")
	}
	if err := <-serverSwitch; err != nil {
		t.Fatal(err)
	}

	// The switch controls are application plaintext before tls.Conn.Write, but
	// must never appear verbatim on the caller-owned public carrier.
	if bytes.Contains(clientRaw.bytes(), req) {
		t.Fatal("single-flow switch request leaked as plaintext on public carrier")
	}
	if bytes.Contains(serverRaw.bytes(), ack) {
		t.Fatal("single-flow switch ACK leaked as plaintext on public carrier")
	}
}
