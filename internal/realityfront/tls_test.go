package realityfront

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"math/big"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestFirefox120PersonaKeepsPresetAndRouteMarker(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	clientConn, peerConn := net.Pipe()
	defer clientConn.Close()
	defer peerConn.Close()

	cfg := ClientConfig{ServerName: "target.test", RouteKey: key}
	got, err := newFirefox120Client(clientConn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientHelloID != utls.HelloFirefox_120 {
		t.Fatalf("ClientHelloID=%v want Firefox120", got.ClientHelloID)
	}
	if got.HandshakeState.Hello == nil ||
		len(got.HandshakeState.Hello.Random) != 32 ||
		len(got.HandshakeState.Hello.SessionId) != 32 {
		t.Fatal("invalid prepared ClientHello state")
	}
	var random [32]byte
	copy(random[:], got.HandshakeState.Hello.Random)
	wantMarker := markerFor(key, cfg.ServerName, random)
	if !bytes.Equal(got.HandshakeState.Hello.SessionId, wantMarker[:]) {
		t.Fatal("compatibility SessionID is not route marker")
	}

	refClient, refPeer := net.Pipe()
	defer refClient.Close()
	defer refPeer.Close()
	ref := utls.UClient(
		refClient,
		&utls.Config{ServerName: cfg.ServerName, InsecureSkipVerify: true},
		utls.HelloFirefox_120,
	)
	if err := ref.BuildHandshakeState(); err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(got.HandshakeState.Hello.CipherSuites, ref.HandshakeState.Hello.CipherSuites) ||
		!reflect.DeepEqual(got.HandshakeState.Hello.SupportedVersions, ref.HandshakeState.Hello.SupportedVersions) ||
		!reflect.DeepEqual(got.HandshakeState.Hello.SupportedCurves, ref.HandshakeState.Hello.SupportedCurves) ||
		!reflect.DeepEqual(got.HandshakeState.Hello.SupportedSignatureAlgorithms, ref.HandshakeState.Hello.SupportedSignatureAlgorithms) ||
		!reflect.DeepEqual(got.HandshakeState.Hello.AlpnProtocols, ref.HandshakeState.Hello.AlpnProtocols) {
		t.Fatal("Firefox120 preset fields changed beyond SessionID marker")
	}
	if len(got.Extensions) != len(ref.Extensions) {
		t.Fatalf("extension count=%d want=%d", len(got.Extensions), len(ref.Extensions))
	}
	for i := range got.Extensions {
		if reflect.TypeOf(got.Extensions[i]) != reflect.TypeOf(ref.Extensions[i]) {
			t.Fatalf("extension[%d]=%T want %T", i, got.Extensions[i], ref.Extensions[i])
		}
	}
}

func TestRealTLSExporterOverServerAssociationAndTransition(t *testing.T) {
	cert := makeServerCert(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	assoc, peer := newAssociationPeer(t)
	defer peer.Close()
	defer assoc.Close()

	params := ExporterParams{
		Version:          1,
		IncarnationNonce: [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		TunnelID:         []byte("tunnel-A"),
		ClientLimit:      1400,
		ServerLimit:      1360,
	}

	type serverResult struct {
		session  *ServerSession
		boundary uint32
		err      error
	}
	serverDone := make(chan serverResult, 1)
	go func() {
		session, err := HandshakeServer(
			context.Background(),
			assoc.BootstrapConn(),
			ServerConfig{
				ServerName: "target.test",
				RouteKey:   routeKey,
				TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout:    3 * time.Second,
			},
			params,
		)
		if err != nil {
			serverDone <- serverResult{err: err}
			return
		}
		boundary, err := assoc.PrepareTransition(2048)
		serverDone <- serverResult{session: session, boundary: boundary, err: err}
	}()

	clientSession, err := HandshakeClient(
		context.Background(),
		peer,
		ClientConfig{
			ServerName: "target.test",
			RouteKey:   routeKey,
			Timeout:    3 * time.Second,
		},
		params,
	)
	if err != nil {
		t.Fatal(err)
	}
	sr := <-serverDone
	if sr.err != nil {
		t.Fatal(sr.err)
	}
	if clientSession.Keys != sr.session.Keys {
		t.Fatal("client/server exporter-derived key pairs differ")
	}
	if sr.session.Hello.Info.ServerName != "target.test" || !sr.session.Hello.Recognized {
		t.Fatalf("classified hello=%#v", sr.session.Hello)
	}
	if got := peer.NextSendSeq(); got != sr.boundary {
		t.Fatalf("transition boundary=%d client next=%d", sr.boundary, got)
	}

	// Exporter context must bind negotiated parameters. Re-derive from the
	// original uTLS connection state with a changed tunnel id and require a
	// different result.
	changed := params
	changed.TunnelID = []byte("tunnel-B")
	clientState := clientSession.Conn.ConnectionState()
	otherKeys, err := deriveRecordKeys(&clientState, changed)
	if err != nil {
		t.Fatal(err)
	}
	if otherKeys == clientSession.Keys {
		t.Fatal("exporter keys did not change with context")
	}

	// Simulate the first TLS-like record arriving after prepare but before
	// detach. It must not be consumed by the TLS reader.
	record := []byte{0x17, 0x03, 0x03, 0, 4, 1, 2, 3, 4}
	recordSeq, err := peer.WriteRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	queued, err := assoc.DetachTransition()
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 1 || queued[0].Seq != recordSeq || !bytes.Equal(queued[0].Payload, record) {
		t.Fatalf("queued early record=%#v", queued)
	}
}

func TestReadHelloRetainsUnrecognizedRawForFallback(t *testing.T) {
	serverKey := []byte("0123456789abcdef0123456789abcdef")
	clientKey := []byte("fedcba9876543210fedcba9876543210")
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	uconn, err := newFirefox120Client(clientConn, ClientConfig{
		ServerName: "target.test",
		RouteKey:   clientKey,
	})
	if err != nil {
		t.Fatal(err)
	}
	clientDone := make(chan error, 1)
	go func() {
		clientDone <- uconn.HandshakeContext(context.Background())
	}()

	hello, err := ReadHello(serverConn, "target.test", serverKey, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if hello.Recognized {
		t.Fatal("wrong route key was recognized")
	}
	if hello.Info.ServerName != "target.test" || len(hello.Raw) == 0 {
		t.Fatalf("fallback hello=%#v", hello)
	}
	_ = serverConn.Close()
	select {
	case <-clientDone:
	case <-time.After(time.Second):
		t.Fatal("client handshake did not stop after fallback-side close")
	}
}

type associationPeerConn struct {
	assoc   *faketcp.ServerAssociation
	base    faketcp.Segment
	emitted <-chan faketcp.Segment

	mu           sync.Mutex
	sendSeq      uint32
	recvSeq      uint32
	readBuf      bytes.Buffer
	closed       bool
	readEOF      bool
	writeClosed  bool
	readDeadline time.Time
	writeDeadline time.Time
	onServerPayload func(faketcp.Segment)
	onClientPayload func(faketcp.Segment)
	done            chan struct{}
	closeOnce       sync.Once
}

func newAssociationPeer(t *testing.T) (*faketcp.ServerAssociation, *associationPeerConn) {
	t.Helper()
	syn := faketcp.Segment{
		SrcIP:          [4]byte{10, 0, 0, 2},
		DstIP:          [4]byte{10, 0, 1, 1},
		SrcPort:        41001,
		DstPort:        443,
		Seq:            1000,
		Flags:          faketcp.FlagSYN,
		Window:         65535,
		MSS:            faketcp.DefaultMSS,
		MSSSet:         true,
		SACKPermitted:  true,
		WindowScale:    faketcp.DefaultWindowScale,
		WindowScaleSet: true,
	}
	emitted := make(chan faketcp.Segment, 64)
	assoc, err := faketcp.NewServerAssociation(
		syn,
		5000,
		2*time.Second,
		func(seg faketcp.Segment) error {
			emitted <- seg
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	finalACK := syn
	finalACK.Flags = faketcp.FlagACK
	finalACK.Seq = 1001
	finalACK.Ack = 5001
	if _, err := assoc.HandleSegment(finalACK, time.Now()); err != nil {
		t.Fatal(err)
	}
	return assoc, &associationPeerConn{
		assoc:   assoc,
		base:    syn,
		emitted: emitted,
		sendSeq: 1001,
		recvSeq: 5001,
		done:    make(chan struct{}),
	}
}

func (c *associationPeerConn) Read(p []byte) (int, error) {
	for {
		c.mu.Lock()
		if c.readBuf.Len() != 0 {
			n, err := c.readBuf.Read(p)
			c.mu.Unlock()
			return n, err
		}
		if c.closed || c.readEOF {
			c.mu.Unlock()
			return 0, io.EOF
		}
		deadline := c.readDeadline
		c.mu.Unlock()

		var (
			seg faketcp.Segment
			ok  bool
		)
		if deadline.IsZero() {
			select {
			case seg, ok = <-c.emitted:
			case <-c.done:
				return 0, io.EOF
			}
		} else {
			d := time.Until(deadline)
			if d <= 0 {
				return 0, timeoutError{}
			}
			timer := time.NewTimer(d)
			select {
			case seg, ok = <-c.emitted:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
			case <-c.done:
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				return 0, io.EOF
			case <-timer.C:
				return 0, timeoutError{}
			}
		}
		if !ok {
			return 0, io.EOF
		}
		if len(seg.Payload) == 0 && seg.Flags&faketcp.FlagFIN == 0 {
			continue
		}

		c.mu.Lock()
		if seg.Seq != c.recvSeq {
			c.mu.Unlock()
			return 0, errors.New("test peer: unexpected server sequence")
		}
		c.recvSeq += uint32(len(seg.Payload))
		if len(seg.Payload) != 0 {
			_, _ = c.readBuf.Write(seg.Payload)
		}
		if seg.Flags&faketcp.FlagFIN != 0 {
			c.recvSeq++
			c.readEOF = true
		}
		ack := c.recvSeq
		seq := c.sendSeq
		hook := c.onServerPayload
		c.mu.Unlock()

		if hook != nil && len(seg.Payload) != 0 {
			hook(seg)
		}

		ackSeg := c.base
		ackSeg.Flags = faketcp.FlagACK
		ackSeg.Seq = seq
		ackSeg.Ack = ack
		ackSeg.Payload = nil
		if _, err := c.assoc.HandleSegment(ackSeg, time.Now()); err != nil &&
			c.assoc.State() != faketcp.ServerAssociationClosed {
			return 0, err
		}
	}
}

func (c *associationPeerConn) Write(p []byte) (int, error) {
	return c.writePayload(p, true)
}

func (c *associationPeerConn) WriteRecord(p []byte) (uint32, error) {
	c.mu.Lock()
	start := c.sendSeq
	c.mu.Unlock()
	_, err := c.writePayload(p, false)
	return start, err
}

func (c *associationPeerConn) writePayload(p []byte, requireAckAdvance bool) (int, error) {
	written := 0
	for len(p) != 0 {
		chunk := len(p)
		if chunk > faketcp.DefaultBootstrapChunk {
			chunk = faketcp.DefaultBootstrapChunk
		}

		c.mu.Lock()
		if c.closed || c.writeClosed {
			c.mu.Unlock()
			return written, net.ErrClosed
		}
		if !c.writeDeadline.IsZero() && !time.Now().Before(c.writeDeadline) {
			c.mu.Unlock()
			return written, timeoutError{}
		}
		seq := c.sendSeq
		ack := c.recvSeq
		payload := append([]byte(nil), p[:chunk]...)
		c.mu.Unlock()

		seg := c.base
		seg.Flags = faketcp.FlagACK | faketcp.FlagPSH
		seg.Seq = seq
		seg.Ack = ack
		seg.Payload = payload
		c.mu.Lock()
		hook := c.onClientPayload
		c.mu.Unlock()
		if hook != nil {
			hook(seg)
		}
		res, err := c.assoc.HandleSegment(seg, time.Now())
		if err != nil {
			return written, err
		}
		next := seq + uint32(chunk)
		if requireAckAdvance && (!res.AckNeeded || res.Ack != next) {
			return written, errors.New("test peer: bootstrap cumulative ACK did not advance")
		}

		c.mu.Lock()
		c.sendSeq = next
		c.mu.Unlock()
		written += chunk
		p = p[chunk:]
	}
	return written, nil
}

func (c *associationPeerConn) NextSendSeq() uint32 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.sendSeq
}

func (c *associationPeerConn) CloseWrite() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return net.ErrClosed
	}
	if c.writeClosed {
		c.mu.Unlock()
		return nil
	}
	seq := c.sendSeq
	ack := c.recvSeq
	c.writeClosed = true
	c.sendSeq++
	c.mu.Unlock()

	seg := c.base
	seg.Flags = faketcp.FlagACK | faketcp.FlagFIN
	seg.Seq = seq
	seg.Ack = ack
	seg.Payload = nil
	res, err := c.assoc.HandleSegment(seg, time.Now())
	if err != nil {
		return err
	}
	if !res.AckNeeded || res.Ack != seq+1 {
		return errors.New("test peer: FIN cumulative ACK did not advance")
	}
	return nil
}

func (c *associationPeerConn) Close() error {
	_ = c.CloseWrite()
	c.mu.Lock()
	c.closed = true
	c.mu.Unlock()
	c.closeOnce.Do(func() { close(c.done) })
	return nil
}

func (c *associationPeerConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(10, 0, 0, 2), Port: 41001}
}

func (c *associationPeerConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(10, 0, 1, 1), Port: 443}
}

func (c *associationPeerConn) SetDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline = t
	c.writeDeadline = t
	c.mu.Unlock()
	return nil
}

func (c *associationPeerConn) SetReadDeadline(t time.Time) error {
	c.mu.Lock()
	c.readDeadline = t
	c.mu.Unlock()
	return nil
}

func (c *associationPeerConn) SetWriteDeadline(t time.Time) error {
	c.mu.Lock()
	c.writeDeadline = t
	c.mu.Unlock()
	return nil
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "test peer: deadline exceeded" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func makeServerCert(t *testing.T) tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "target.test"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		DNSNames:     []string{"target.test"},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}
