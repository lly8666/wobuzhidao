package realityfront

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

func TestMarkerRandIsVisibleToServerWithoutPostBuildPatch(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	mr, err := NewMarkerRand(rand.Reader, key, "target.test")
	if err != nil {
		t.Fatal(err)
	}
	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()
	client := tls.Client(clientRaw, &tls.Config{
		ServerName:         "target.test",
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		Rand:               mr,
	})
	done := make(chan error, 1)
	go func() { done <- client.Handshake() }()
	info, raw, err := realitymirror.ReadClientHello(serverRaw, 64<<10, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if info.ServerName != "target.test" {
		t.Fatalf("SNI=%q", info.ServerName)
	}
	if !Recognized(raw, key, info.ServerName) {
		t.Fatal("server did not recognize TLS-generated SessionID marker")
	}
	_ = serverRaw.Close()
	<-done
}

func TestRecognizedBranchTakesOverSameTCPAndIssuesOneTimeTicket(t *testing.T) {
	cert := makeServerCert(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	ticketDir := t.TempDir()
	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()

	serverDone := make(chan error, 1)
	go func() {
		res, err := HandleServerConn(context.Background(), serverRaw, ServerConfig{
			RouteKey:         key,
			ServerName:       "target.test",
			TLSConfig:        &tls.Config{Certificates: []tls.Certificate{cert}},
			ExpectedUsername: "solo",
			ExpectedPassword: "high-entropy-test-password",
			TicketDir:        ticketDir,
			HelloTimeout:     time.Second,
			Mirror: realitymirror.Config{
				Target: "127.0.0.1:1", ServerName: "target.test",
				HelloTimeout: time.Second, DialTimeout: time.Second,
				SessionTimeout: 2 * time.Second, MaxHelloBytes: 64 << 10, MaxBytes: 1 << 20,
			},
		})
		if err == nil && res.Branch != "wbd" {
			err = io.ErrUnexpectedEOF
		}
		serverDone <- err
	}()

	mr, err := NewMarkerRand(rand.Reader, key, "target.test")
	if err != nil {
		t.Fatal(err)
	}
	client := tls.Client(clientRaw, &tls.Config{
		ServerName:         "target.test",
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
		MaxVersion:         tls.VersionTLS13,
		Rand:               mr,
	})
	if err := client.Handshake(); err != nil {
		t.Fatal(err)
	}
	ticket, err := BootstrapClient(client, "solo", "high-entropy-test-password")
	if err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}
	if err := ConsumeTicket(ticketDir, ticket, time.Now(), time.Minute); err != nil {
		t.Fatalf("consume ticket: %v", err)
	}
	if err := ConsumeTicket(ticketDir, ticket, time.Now(), time.Minute); err == nil {
		t.Fatal("one-time ticket was reusable")
	}
}

func TestWrongPasswordDoesNotIssueTicket(t *testing.T) {
	cert := makeServerCert(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close()
	defer serverRaw.Close()
	serverDone := make(chan error, 1)
	go func() {
		_, err := HandleServerConn(context.Background(), serverRaw, ServerConfig{
			RouteKey: key, ServerName: "target.test",
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
			ExpectedUsername: "solo", ExpectedPassword: "correct-password",
			TicketDir: t.TempDir(), HelloTimeout: time.Second,
			Mirror: realitymirror.Config{Target: "127.0.0.1:1", ServerName: "target.test", HelloTimeout: time.Second, DialTimeout: time.Second, MaxHelloBytes: 64 << 10, MaxBytes: 1 << 20},
		})
		serverDone <- err
	}()
	mr, _ := NewMarkerRand(rand.Reader, key, "target.test")
	client := tls.Client(clientRaw, &tls.Config{ServerName: "target.test", InsecureSkipVerify: true, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13, Rand: mr})
	if err := client.Handshake(); err != nil {
		t.Fatal(err)
	}
	if _, err := BootstrapClient(client, "solo", "wrong-password"); err == nil {
		t.Fatal("wrong password unexpectedly authenticated")
	}
	if err := <-serverDone; err == nil {
		t.Fatal("server accepted wrong password")
	}
}

func makeServerCert(t *testing.T) tls.Certificate {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "unrelated.self.signed"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		DNSNames: []string{"unrelated.self.signed"}, KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}
