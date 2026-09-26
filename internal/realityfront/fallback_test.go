package realityfront

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

func TestUnrecognizedClientHelloFallsBackToGenuineTarget(t *testing.T) {
	targetCert, roots := namedSelfSigned(t, "target.test")
	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil { t.Fatal(err) }
	defer targetLn.Close()
	targetDone := make(chan error, 1)
	go func() {
		c, err := targetLn.Accept()
		if err != nil { targetDone <- err; return }
		defer c.Close()
		t := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{targetCert}, MinVersion: tls.VersionTLS13})
		if err := t.Handshake(); err != nil { targetDone <- err; return }
		var b [1]byte
		_, err = t.Read(b[:])
		targetDone <- err
	}()

	localCert, _ := namedSelfSigned(t, "local.invalid")
	clientRaw, serverRaw := net.Pipe()
	defer clientRaw.Close(); defer serverRaw.Close()
	frontDone := make(chan error, 1)
	go func() {
		res, err := HandleServerConn(context.Background(), serverRaw, ServerConfig{
			RouteKey: []byte("0123456789abcdef0123456789abcdef"), ServerName: "target.test",
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{localCert}},
			ExpectedUsername: "unused", ExpectedPassword: "unused", TicketDir: t.TempDir(),
			Mirror: realitymirror.Config{Target: targetLn.Addr().String(), ServerName: "target.test", HelloTimeout: time.Second, DialTimeout: time.Second, SessionTimeout: 3*time.Second, MaxHelloBytes: 64<<10, MaxBytes: 1<<20},
		})
		if err == nil && res.Branch != "fallback" { err = ErrMarker }
		frontDone <- err
	}()

	// Deliberately use ordinary crypto/tls randomness: no WBD SessionID marker.
	client := tls.Client(clientRaw, &tls.Config{ServerName: "target.test", RootCAs: roots, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13})
	if err := client.Handshake(); err != nil { t.Fatal(err) }
	st := client.ConnectionState()
	if len(st.PeerCertificates) == 0 || st.PeerCertificates[0].Subject.CommonName != "target.test" {
		t.Fatalf("fallback did not expose genuine target certificate: %+v", st.PeerCertificates)
	}
	_, _ = client.Write([]byte{1})
	_ = client.Close()
	if err := <-targetDone; err != nil { t.Fatal(err) }
	if err := <-frontDone; err != nil && !isBenignFrontClose(err) { t.Fatal(err) }
}

func namedSelfSigned(t *testing.T, name string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil { t.Fatal(err) }
	now := time.Now()
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(now.UnixNano()), Subject: pkix.Name{CommonName:name}, DNSNames: []string{name}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage:x509.KeyUsageDigitalSignature, ExtKeyUsage:[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, pub, priv)
	if err != nil { t.Fatal(err) }
	cert, err := x509.ParseCertificate(der)
	if err != nil { t.Fatal(err) }
	pool := x509.NewCertPool(); pool.AddCert(cert)
	return tls.Certificate{Certificate:[][]byte{der}, PrivateKey:priv}, pool
}

func isBenignFrontClose(err error) bool {
	if err == nil { return true }
	if ne, ok := err.(net.Error); ok && !ne.Timeout() { return true }
	return false
}
