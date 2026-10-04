package realitymirror

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestReadClientHelloExtractsSNIAndALPN(t *testing.T) {
	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer client.Close()
		c := tls.Client(client, &tls.Config{
			ServerName:         "target.test",
			InsecureSkipVerify: true,
			NextProtos:         []string{"h2", "http/1.1"},
			MinVersion:         tls.VersionTLS12,
		})
		done <- c.Handshake()
	}()

	info, raw, err := ReadClientHello(server, 64<<10, 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if info.ServerName != "target.test" {
		t.Fatalf("server name=%q", info.ServerName)
	}
	if len(info.ALPN) != 2 || info.ALPN[0] != "h2" || info.ALPN[1] != "http/1.1" {
		t.Fatalf("ALPN=%v", info.ALPN)
	}
	if len(raw) < 5 || raw[0] != 22 || !bytes.Contains(raw, []byte("target.test")) {
		t.Fatalf("unexpected raw ClientHello len=%d", len(raw))
	}
	_ = server.Close()
	<-done
}

func TestMirrorReturnsRealTargetCertificate(t *testing.T) {
	cert, roots, leafDER := makeTargetCertificate(t, "target.test")
	targetLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer targetLn.Close()
	tlsLn := tls.NewListener(targetLn, &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"http/1.1"}})
	defer tlsLn.Close()
	targetDone := make(chan error, 1)
	go func() {
		conn, err := tlsLn.Accept()
		if err != nil {
			targetDone <- err
			return
		}
		defer conn.Close()
		req, err := http.ReadRequest(bufio.NewReader(conn))
		if err != nil {
			targetDone <- err
			return
		}
		_ = req.Body.Close()
		_, err = io.WriteString(conn, "HTTP/1.1 204 No Content\r\nContent-Length: 0\r\nConnection: close\r\n\r\n")
		targetDone <- err
	}()

	mirrorLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer mirrorLn.Close()
	mirrorDone := make(chan error, 1)
	go func() {
		conn, err := mirrorLn.Accept()
		if err != nil {
			mirrorDone <- err
			return
		}
		defer conn.Close()
		_, err = Handle(context.Background(), conn, Config{
			Target:         targetLn.Addr().String(),
			ServerName:     "target.test",
			HelloTimeout:   2 * time.Second,
			DialTimeout:    2 * time.Second,
			SessionTimeout: 5 * time.Second,
			MaxHelloBytes:  64 << 10,
			MaxBytes:       1 << 20,
		})
		mirrorDone <- err
	}()

	raw, err := net.DialTimeout("tcp", mirrorLn.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	client := tls.Client(raw, &tls.Config{
		ServerName: "target.test",
		RootCAs:    roots,
		NextProtos: []string{"http/1.1"},
		MinVersion: tls.VersionTLS12,
	})
	if err := client.Handshake(); err != nil {
		t.Fatal(err)
	}
	state := client.ConnectionState()
	if len(state.PeerCertificates) == 0 || !bytes.Equal(state.PeerCertificates[0].Raw, leafDER) {
		t.Fatal("mirror did not return the real target leaf certificate")
	}
	req, _ := http.NewRequest(http.MethodHead, "https://target.test/", nil)
	req.Close = true
	if err := req.Write(client); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(client), req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	_ = client.Close()
	if err := <-targetDone; err != nil {
		t.Fatal(err)
	}
	if err := <-mirrorDone; err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatal(err)
	}
}

func TestMirrorRejectsUnexpectedSNIWithoutDialingTarget(t *testing.T) {
	client, server := net.Pipe()
	done := make(chan error, 1)
	go func() {
		defer client.Close()
		c := tls.Client(client, &tls.Config{ServerName: "wrong.test", InsecureSkipVerify: true})
		done <- c.Handshake()
	}()
	_, err := Handle(context.Background(), server, Config{
		Target:         "127.0.0.1:1",
		ServerName:     "target.test",
		HelloTimeout:   time.Second,
		DialTimeout:    time.Second,
		SessionTimeout: time.Second,
		MaxHelloBytes:  64 << 10,
		MaxBytes:       1 << 20,
	})
	if !errors.Is(err, ErrSNIMismatch) {
		t.Fatalf("err=%v", err)
	}
	_ = server.Close()
	<-done
}

func makeTargetCertificate(t *testing.T, dnsName string) (tls.Certificate, *x509.CertPool, []byte) {
	t.Helper()
	now := time.Now()
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "WBD reality mirror test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPub, caPriv)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	leafPub, leafPriv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, leafPub, caPriv)
	if err != nil {
		t.Fatal(err)
	}
	cert := tls.Certificate{Certificate: [][]byte{leafDER, caDER}, PrivateKey: leafPriv}
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	return cert, roots, leafDER
}
