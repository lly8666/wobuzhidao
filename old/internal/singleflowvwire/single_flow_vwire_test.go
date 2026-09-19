package singleflowvwire

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

// TestSingleFlowVirtualWireAdmissionThenNoHOL is deliberately platform neutral
// and is run natively on windows-latest as well as Linux CI. It exercises the
// protocol portion that sits above Npcap/raw sockets: the actual Firefox120
// uTLS client, TLS 1.3 server/admission, FakeTCP's temporary ordered bootstrap
// adapter, the mode barrier, and then steady-state Receiver semantics on the
// exact sequence lineage that TLS just consumed.
//
// This is not a substitute for physical Npcap/NIC qualification. It exists so
// Windows protocol regressions cannot hide behind a successful cross-compile.
func TestSingleFlowVirtualWireAdmissionThenNoHOL(t *testing.T) {
	cert := makeServerCert(t)
	const clientBase uint32 = 100001
	const serverBase uint32 = 700001

	var mu sync.Mutex
	clientTx := clientBase
	serverTx := serverBase
	var clientStream, serverStream *faketcp.BootstrapStream

	clientSend := func(p []byte) (uint32, error) {
		mu.Lock()
		seq := clientTx
		clientTx += uint32(len(p))
		end := clientTx
		peer := serverStream
		mu.Unlock()
		peer.Feed(seq, append([]byte(nil), p...))
		return end, nil
	}
	serverSend := func(p []byte) (uint32, error) {
		mu.Lock()
		seq := serverTx
		serverTx += uint32(len(p))
		end := serverTx
		peer := clientStream
		mu.Unlock()
		peer.Feed(seq, append([]byte(nil), p...))
		return end, nil
	}
	ackImmediate := func(uint32, time.Time) error { return nil }

	var err error
	clientStream, err = faketcp.NewBootstrapStream(serverBase, clientSend, ackImmediate,
		&net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 51001},
		&net.TCPAddr{IP: net.ParseIP("198.51.100.20"), Port: 443})
	if err != nil { t.Fatal(err) }
	serverStream, err = faketcp.NewBootstrapStream(clientBase, serverSend, ackImmediate,
		&net.TCPAddr{IP: net.ParseIP("198.51.100.20"), Port: 443},
		&net.TCPAddr{IP: net.ParseIP("192.0.2.10"), Port: 51001})
	if err != nil { t.Fatal(err) }
	defer clientStream.Close()
	defer serverStream.Close()

	key := []byte("0123456789abcdef0123456789abcdef")
	ticketDir := t.TempDir()
	serverDone := make(chan error, 1)
	go func() {
		_, err := realityfront.BootstrapServerSingleFlow(context.Background(), serverStream, realityfront.SingleFlowServerConfig{
			ServerName: "target.test", RouteKey: key,
			ExpectedUsername: "solo", ExpectedPassword: "single-flow-password",
			TicketDir: ticketDir,
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
			Timeout: 5 * time.Second,
		})
		serverDone <- err
	}()

	ticket, state, err := realityfront.BootstrapClientSingleFlow(context.Background(), clientStream, realityfront.SingleFlowClientConfig{
		ServerName: "target.test", RouteKey: key,
		Username: "solo", Password: "single-flow-password",
		VerifyServer: false, Timeout: 5 * time.Second,
	})
	if err != nil { t.Fatalf("client single-flow bootstrap: %v", err) }
	if err := <-serverDone; err != nil { t.Fatalf("server single-flow bootstrap: %v", err) }
	if state.Version != tls.VersionTLS13 { t.Fatalf("TLS version=%x want TLS1.3", state.Version) }
	if err := realityfront.ConsumeTicket(ticketDir, ticket, time.Now(), time.Minute); err != nil {
		t.Fatalf("consume issued ticket: %v", err)
	}

	mu.Lock()
	postBarrierSeq := clientTx
	clientTLSBytes := clientTx - clientBase
	serverTLSBytes := serverTx - serverBase
	mu.Unlock()
	if clientTLSBytes < 256 || serverTLSBytes < 256 {
		t.Fatalf("bootstrap did not exercise real TLS records: client=%d server=%d", clientTLSBytes, serverTLSBytes)
	}

	// The ordered stream is discarded here. Steady-state starts exactly at the
	// next sequence byte from the TLS phase; no new SYN/ISN/connection exists.
	if err := clientStream.Close(); err != nil { t.Fatal(err) }
	if err := serverStream.Close(); err != nil { t.Fatal(err) }
	r := faketcp.NewReceiver(postBarrierSeq)

	// Lose the first 100-byte datagram. A later independent datagram must be
	// deliverable immediately rather than waiting for the hole (no TCP HOL).
	if deliver, _ := r.Accept(postBarrierSeq+100, 80); !deliver {
		t.Fatal("post-barrier later datagram was HOL-blocked")
	}
	if got := r.Next(); got != postBarrierSeq {
		t.Fatalf("cumulative ACK advanced across post-barrier hole: got=%d want=%d", got, postBarrierSeq)
	}
	if deliver, _ := r.Accept(postBarrierSeq, 100); !deliver {
		t.Fatal("post-barrier hole repair was not delivered")
	}
	if got := r.Next(); got != postBarrierSeq+180 {
		t.Fatalf("post-barrier sequence lineage reset/broke: got=%d want=%d", got, postBarrierSeq+180)
	}
}

func makeServerCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil { t.Fatal(err) }
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(23),
		Subject: pkix.Name{CommonName: "target.test"},
		NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour),
		DNSNames: []string{"target.test"},
		KeyUsage: x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil { t.Fatal(err) }
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}
