//go:build linux

package realityfront

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

const (
	p2KernelFallbackPort = 24443
	p2KernelFakeIP       = "127.0.0.2"
)

type kernelFallbackResult struct {
	result ServerAssociationResult
	err    error
}

type kernelRawFallbackServer struct {
	endpoint *faketcp.RawIPv4Endpoint
	table    *faketcp.ServerAssociationTable
	ctx      context.Context
	cancel   context.CancelFunc

	targetAddr string
	cert       tls.Certificate
	routeKey   []byte

	wg       sync.WaitGroup
	errCh    chan error
	done     chan kernelFallbackResult

	mu             sync.Mutex
	payloadBySeq   map[uint32][]byte
	firstPayloadAt map[uint32]time.Time
	holdACK        bool
	holdStarted    bool
	retransSeen    bool
}

func newKernelRawFallbackServer(endpoint *faketcp.RawIPv4Endpoint, targetAddr string, cert tls.Certificate) (*kernelRawFallbackServer, error) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &kernelRawFallbackServer{
		endpoint: endpoint,
		ctx: ctx, cancel: cancel,
		targetAddr: targetAddr,
		cert: cert,
		routeKey: []byte("p2-kernel-route-key-0123456789ab"),
		errCh: make(chan error, 8),
		done: make(chan kernelFallbackResult, 1),
		payloadBySeq: make(map[uint32][]byte),
		firstPayloadAt: make(map[uint32]time.Time),
	}
	table, err := faketcp.NewServerAssociationTable(8, s.emit)
	if err != nil {
		cancel()
		return nil, err
	}
	s.table = table
	return s, nil
}

func (s *kernelRawFallbackServer) Start() {
	s.wg.Add(1)
	go s.readLoop()
}

func (s *kernelRawFallbackServer) Close() {
	s.cancel()
	_ = s.endpoint.Close()
	s.wg.Wait()
}

func (s *kernelRawFallbackServer) fail(err error) {
	if err == nil {
		return
	}
	select {
	case s.errCh <- err:
	default:
	}
}

func (s *kernelRawFallbackServer) readLoop() {
	defer s.wg.Done()
	for {
		seg, _, err := s.endpoint.ReadSegment()
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			s.fail(err)
			return
		}
		if seg.DstPort != p2KernelFallbackPort {
			continue
		}
		if err := s.handleSegment(seg); err != nil {
			s.fail(err)
			return
		}
	}
}

func (s *kernelRawFallbackServer) handleSegment(seg faketcp.Segment) error {
	if faketcp.IsInitialSYN(seg) {
		flow := faketcp.ServerFlowFromSegment(seg)
		_, existed := s.table.Get(flow)
		assoc, err := s.table.AddSYN(seg, 0x5a000000+uint32(seg.SrcPort), 200*time.Millisecond)
		if err != nil {
			return err
		}
		synack, err := assoc.SYNACKSegment()
		if err != nil {
			return err
		}
		if _, err := s.endpoint.WriteSegment(synack); err != nil {
			return err
		}
		if !existed {
			s.startAssociation(assoc)
		}
		return nil
	}

	assoc, ok := s.table.GetSegment(seg)
	if !ok {
		return nil
	}

	s.mu.Lock()
	holdACK := s.holdACK && seg.Flags&faketcp.FlagACK != 0
	s.mu.Unlock()
	if holdACK {
		// Hosted qualification deliberately withholds cumulative ACK ownership
		// from the userspace FakeTCP sender until one RTO retransmission is
		// observed. The real kernel still sends/captures the ACK; only this test
		// adapter suppresses its delivery to the association.
		seg.Flags &^= faketcp.FlagACK
		seg.Ack = 0
	}

	res, err := assoc.HandleSegment(seg, time.Now())
	if err != nil {
		return err
	}
	if res.AckNeeded {
		if _, err := s.endpoint.WriteSegment(assoc.ACKSegment(res.Ack)); err != nil {
			return err
		}
	}
	if assoc.State() == faketcp.ServerAssociationClosed {
		s.table.Remove(assoc.Flow())
	}
	return nil
}

func (s *kernelRawFallbackServer) startAssociation(assoc *faketcp.ServerAssociation) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(25 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.ctx.Done():
				return
			case now := <-ticker.C:
				if assoc.State() == faketcp.ServerAssociationClosed {
					return
				}
				if _, err := assoc.EmitRetransmitDue(now); err != nil {
					s.fail(err)
					return
				}
			}
		}
	}()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		result, err := HandleServerAssociation(s.ctx, assoc, ServerAdmissionConfig{
			TLS: ServerConfig{
				ServerName: "target.test",
				RouteKey:   s.routeKey,
				TLSConfig:  &tls.Config{Certificates: []tls.Certificate{s.cert}},
				Timeout:    5 * time.Second,
			},
			ExpectedUsername: "not-used-for-fallback",
			ExpectedPassword: "not-used-for-fallback",
			ServerLimit:      1450,
		}, FallbackConfig{
			Target:         s.targetAddr,
			ServerName:     "target.test",
			DialTimeout:    2 * time.Second,
			SessionTimeout: 10 * time.Second,
			MaxBytes:       1 << 20,
		})
		select {
		case s.done <- kernelFallbackResult{result: result, err: err}:
		default:
		}
	}()
}

func (s *kernelRawFallbackServer) emit(seg faketcp.Segment) error {
	if len(seg.Payload) != 0 {
		s.mu.Lock()
		if previous, ok := s.payloadBySeq[seg.Seq]; ok {
			if !bytes.Equal(previous, seg.Payload) {
				s.mu.Unlock()
				return errors.New("kernel fallback: same TCP sequence changed retransmit payload")
			}
			// A repeated emitter call, unlike a loopback capture duplicate, is
			// an actual FakeTCP retransmission. Release ACK suppression before
			// injecting it so the real kernel duplicate ACK can complete RTO.
			s.retransSeen = true
			s.holdACK = false
		} else {
			s.payloadBySeq[seg.Seq] = append([]byte(nil), seg.Payload...)
			s.firstPayloadAt[seg.Seq] = time.Now()
			if !s.holdStarted {
				s.holdStarted = true
				s.holdACK = true
			}
		}
		s.mu.Unlock()
	}
	_, err := s.endpoint.WriteSegment(seg)
	return err
}

func (s *kernelRawFallbackServer) RetransmissionSeen() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.retransSeen
}

func TestKernelTLSFallbackVerifiedHTTPAndNormalClose(t *testing.T) {
	if os.Getenv("WBD_P2_KERNEL_NET") != "1" {
		t.Skip("set WBD_P2_KERNEL_NET=1 in privileged Actions qualification job")
	}
	if os.Geteuid() != 0 {
		t.Fatal("P2 kernel fallback qualification requires root raw-socket capability")
	}

	cert, roots := makeKernelFallbackPKI(t)
	targetAddr, targetDone, responseBody := startKernelDecoy(t, cert)

	endpoint, err := faketcp.OpenRawIPv4Endpoint("lo", [4]byte{127, 0, 0, 2}, faketcp.PacketPersonaLegacy)
	if err != nil {
		t.Fatal(err)
	}
	server, err := newKernelRawFallbackServer(endpoint, targetAddr, cert)
	if err != nil {
		_ = endpoint.Close()
		t.Fatal(err)
	}
	server.Start()
	defer server.Close()

	dialer := &net.Dialer{Timeout: 8 * time.Second}
	client, err := tls.DialWithDialer(dialer, "tcp4", fmt.Sprintf("%s:%d", p2KernelFakeIP, p2KernelFallbackPort), &tls.Config{
		ServerName: "target.test",
		RootCAs:    roots,
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"http/1.1"},
	})
	if err != nil {
		select {
		case serverErr := <-server.errCh:
			t.Fatalf("kernel TLS dial failed: %v; raw server: %v", err, serverErr)
		default:
			t.Fatal(err)
		}
	}
	state := client.ConnectionState()
	if len(state.VerifiedChains) == 0 || state.PeerCertificates[0].DNSNames[0] != "target.test" {
		t.Fatal("ordinary kernel TLS client did not verify controlled target certificate")
	}
	if state.NegotiatedProtocol != "http/1.1" {
		t.Fatalf("fallback target ALPN=%q want http/1.1", state.NegotiatedProtocol)
	}

	req, err := http.NewRequest("GET", "https://target.test/p2", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "target.test"
	req.Close = true
	if err := req.Write(client); err != nil {
		t.Fatal(err)
	}
	resp, err := http.ReadResponse(bufio.NewReader(client), req)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != responseBody {
		t.Fatalf("HTTP response status=%d bodyLen=%d want status=200 bodyLen=%d", resp.StatusCode, len(body), len(responseBody))
	}

	// Consume the target close_notify/EOF before closing the client side so the
	// pcap contains a normal bidirectional FIN lifecycle rather than an abort.
	var one [1]byte
	if n, err := client.Read(one[:]); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("post-response TLS read n=%d err=%v want EOF", n, err)
	}
	if err := client.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		t.Fatalf("client close: %v", err)
	}

	select {
	case err := <-targetDone:
		if err != nil {
			t.Fatalf("controlled decoy: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("controlled decoy did not finish")
	}

	select {
	case got := <-server.done:
		if got.err != nil {
			t.Fatalf("fallback handler: %v", got.err)
		}
		if got.result.Branch != "fallback" || got.result.Fallback == nil {
			t.Fatalf("branch=%q result=%#v", got.result.Branch, got.result)
		}
		if got.result.Fallback.UpBytes <= int64(len(got.result.Fallback.Hello.Raw)) || got.result.Fallback.DownBytes == 0 {
			t.Fatalf("fallback byte counts up=%d down=%d", got.result.Fallback.UpBytes, got.result.Fallback.DownBytes)
		}
	case err := <-server.errCh:
		t.Fatalf("raw FakeTCP server: %v", err)
	case <-time.After(8 * time.Second):
		t.Fatal("fallback handler did not finish after normal close")
	}

	if !server.RetransmissionSeen() {
		t.Fatal("qualification did not force a real same-sequence FakeTCP retransmission")
	}

	deadline := time.Now().Add(2 * time.Second)
	for server.table.Len() != 0 && time.Now().Before(deadline) {
		server.table.Sweep(time.Now())
		time.Sleep(10 * time.Millisecond)
	}
	if server.table.Len() != 0 {
		t.Fatalf("association table retained %d entries after normal close", server.table.Len())
	}

	closeStart := time.Now()
	server.Close()
	if elapsed := time.Since(closeStart); elapsed > time.Second {
		t.Fatalf("raw endpoint/server close took %v want <=1s", elapsed)
	}
}

func makeKernelFallbackPKI(t *testing.T) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1001),
		Subject:               pkix.Name{CommonName: "WBD P2 Actions Test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}

	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1002),
		Subject:      pkix.Name{CommonName: "target.test"},
		DNSNames:     []string{"target.test"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	leafDER, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(caCert)
	return tls.Certificate{
		Certificate: [][]byte{leafDER},
		PrivateKey:  leafKey,
	}, roots
}

func startKernelDecoy(t *testing.T, cert tls.Certificate) (string, <-chan error, string) {
	t.Helper()
	ln, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	body := strings.Repeat("controlled-decoy-body-", 160)
	done := make(chan error, 1)
	go func() {
		defer close(done)
		defer ln.Close()
		raw, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		tlsConn := tls.Server(raw, &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
			MaxVersion:   tls.VersionTLS13,
			NextProtos:   []string{"http/1.1"},
		})
		defer tlsConn.Close()
		if err := tlsConn.Handshake(); err != nil {
			done <- err
			return
		}
		req, err := http.ReadRequest(bufio.NewReader(tlsConn))
		if err != nil {
			done <- err
			return
		}
		_ = req.Body.Close()
		if req.Host != "target.test" || req.URL.Path != "/p2" {
			done <- fmt.Errorf("unexpected HTTP request host=%q path=%q", req.Host, req.URL.Path)
			return
		}
		if _, err := fmt.Fprintf(tlsConn,
			"HTTP/1.1 200 OK\r\nContent-Length: %d\r\nContent-Type: text/plain\r\nConnection: close\r\n\r\n%s",
			len(body), body); err != nil {
			done <- err
			return
		}
		if err := tlsConn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			done <- err
			return
		}
		done <- nil
	}()
	return ln.Addr().String(), done, body
}
