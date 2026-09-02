package main

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/persona"
	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

func TestClientHelloCaptureMatchesMirrorWitness(t *testing.T) {
	clientRaw, serverRaw := netPipe(t)
	defer clientRaw.Close()
	defer serverRaw.Close()

	captured := newHelloCaptureConn(clientRaw)
	client := tls.Client(captured, &tls.Config{
		ServerName:         "target.example",
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: true, // test never reaches certificate verification
	})

	clientDone := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		clientDone <- client.HandshakeContext(ctx)
	}()

	info, rawHello, err := realitymirror.ReadClientHello(serverRaw, 64<<10, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if info.ServerName != "target.example" {
		t.Fatalf("server name=%q", info.ServerName)
	}
	want := persona.WitnessFromClientHello(rawHello).Hex()
	got := captured.SHA256Hex()
	if got != want {
		t.Fatalf("client hello hash mismatch: client=%s mirror=%s", got, want)
	}

	_ = serverRaw.Close()
	select {
	case <-clientDone:
	case <-time.After(2 * time.Second):
		t.Fatal("TLS client did not stop after test server close")
	}
}

// netPipe is isolated only so the test's intent reads clearly; the production
// diagnostic still uses a real TCP connection.
func netPipe(t *testing.T) (client, server net.Conn) {
	t.Helper()
	return net.Pipe()
}
