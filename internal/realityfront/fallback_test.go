package realityfront

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

)

func TestUnrecognizedHelloFallsBackOnSameAssociationWithExactReplay(t *testing.T) {
	cert := makeServerCert(t)
	serverRouteKey := []byte("0123456789abcdef0123456789abcdef")
	clientRouteKey := []byte("fedcba9876543210fedcba9876543210")
	assoc, peer := newAssociationPeer(t)
	defer assoc.Close()
	defer peer.Close()

	targetDial, targetServer := net.Pipe()
	var dialCalls atomic.Int32
	capturedHello := make(chan []byte, 1)
	targetDone := make(chan error, 1)
	go func() {
		defer targetServer.Close()
		info, raw, err := readClientHello(targetServer, 64<<10, 3*time.Second)
		if err != nil {
			targetDone <- err
			return
		}
		if info.ServerName != "target.test" {
			targetDone <- errors.New("decoy saw wrong SNI")
			return
		}
		capturedHello <- append([]byte(nil), raw...)

		tlsConn := tls.Server(replay(targetServer, raw), &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS13,
			MaxVersion:   tls.VersionTLS13,
		})
		if err := tlsConn.HandshakeContext(context.Background()); err != nil {
			targetDone <- err
			return
		}
		var request [4]byte
		if _, err := io.ReadFull(tlsConn, request[:]); err != nil {
			targetDone <- err
			return
		}
		if string(request[:]) != "ping" {
			targetDone <- errors.New("decoy application request mismatch")
			return
		}
		if _, err := tlsConn.Write([]byte("pong")); err != nil {
			targetDone <- err
			return
		}
		targetDone <- tlsConn.Close()
	}()

	type serverResult struct {
		result ServerAssociationResult
		err    error
	}
	serverDone := make(chan serverResult, 1)
	go func() {
		result, err := HandleServerAssociation(
			context.Background(),
			assoc,
			ServerAdmissionConfig{
				TLS: ServerConfig{
					ServerName: "target.test",
					RouteKey:   serverRouteKey,
					TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
					Timeout:    3 * time.Second,
				},
				ExpectedUsername: "solo",
				ExpectedPassword: "correct-password",
				ServerLimit:      1450,
			},
			FallbackConfig{
				Target:         "decoy.invalid:443",
				ServerName:     "target.test",
				SessionTimeout: 5 * time.Second,
				MaxBytes:       1 << 20,
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					dialCalls.Add(1)
					return targetDial, nil
				},
			},
		)
		serverDone <- serverResult{result: result, err: err}
	}()

	clientTLS, err := newFirefox120Client(peer, ClientConfig{
		ServerName: "target.test",
		RouteKey:   clientRouteKey,
		Timeout:    3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := clientTLS.HandshakeContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := clientTLS.Write([]byte("ping")); err != nil {
		t.Fatal(err)
	}
	var response [4]byte
	if _, err := io.ReadFull(clientTLS, response[:]); err != nil {
		t.Fatal(err)
	}
	if string(response[:]) != "pong" {
		t.Fatalf("response=%q", response)
	}
	// The decoy closes first after pong. Let the TLS client consume its
	// close_notify before closing the underlying test transport; this Read also
	// drives the FakeTCP peer's cumulative ACK exactly as a kernel TCP stack
	// would continue doing while the TLS session shuts down.
	_ = clientTLS.SetReadDeadline(time.Now().Add(time.Second))
	var afterClose [1]byte
	if n, err := clientTLS.Read(afterClose[:]); n != 0 || !errors.Is(err, io.EOF) {
		t.Fatalf("fallback TLS shutdown: n=%d err=%v want EOF", n, err)
	}
	_ = clientTLS.SetReadDeadline(time.Time{})
	// The application has consumed the decoy's TLS close and now half-closes its
	// TCP send side. This lets the opposite fallback copy drain without relying
	// on the old full-close workaround.
	if err := peer.CloseWrite(); err != nil {
		t.Fatal(err)
	}

	if err := <-targetDone; err != nil && !errors.Is(err, net.ErrClosed) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatal(err)
	}
	var sr serverResult
	select {
	case sr = <-serverDone:
	case <-time.After(2 * time.Second):
		t.Fatal("fallback splice did not terminate after target close")
	}
	_ = clientTLS.Close()
	_ = peer.Close()
	if sr.err != nil && !benignFallbackCopyError(sr.err) {
		t.Fatal(sr.err)
	}
	if sr.result.Branch != "fallback" || sr.result.Fallback == nil || sr.result.Admission != nil {
		t.Fatalf("server result=%#v", sr.result)
	}
	if dialCalls.Load() != 1 {
		t.Fatalf("dial calls=%d want=1", dialCalls.Load())
	}
	gotTargetHello := <-capturedHello
	if !bytes.Equal(gotTargetHello, sr.result.Fallback.Hello.Raw) {
		t.Fatal("decoy did not receive byte-identical replayed ClientHello")
	}
	if sr.result.Fallback.Hello.Recognized {
		t.Fatal("fallback result marked recognized")
	}
	if sr.result.Fallback.UpBytes < int64(len(gotTargetHello)) || sr.result.Fallback.DownBytes == 0 {
		t.Fatalf("fallback bytes up=%d down=%d", sr.result.Fallback.UpBytes, sr.result.Fallback.DownBytes)
	}
	if _, ok := assoc.TransitionState(); ok {
		t.Fatal("fallback path prepared a record transition")
	}
}

func TestRecognizedAssociationNeverDialsFallback(t *testing.T) {
	cert := makeServerCert(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	assoc, peer := newAssociationPeer(t)
	defer assoc.Close()
	defer peer.Close()

	nonce := [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	var dialCalls atomic.Int32
	type serverResult struct {
		result ServerAssociationResult
		err    error
	}
	serverDone := make(chan serverResult, 1)
	go func() {
		result, err := HandleServerAssociation(
			context.Background(),
			assoc,
			ServerAdmissionConfig{
				TLS: ServerConfig{
					ServerName: "target.test",
					RouteKey:   routeKey,
					TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
					Timeout:    3 * time.Second,
				},
				ExpectedUsername: "solo",
				ExpectedPassword: "correct-password",
				ServerLimit:      1450,
				Random:           bytes.NewReader(nonce[:]),
			},
			FallbackConfig{
				Target:     "decoy.invalid:443",
				ServerName: "target.test",
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					dialCalls.Add(1)
					return nil, errors.New("fallback dial must not occur")
				},
			},
		)
		serverDone <- serverResult{result: result, err: err}
	}()

	client, err := EstablishClient(context.Background(), peer, ClientAdmissionConfig{
		TLS: ClientConfig{
			ServerName: "target.test",
			RouteKey:   routeKey,
			Timeout:    3 * time.Second,
		},
		Username:    "solo",
		Password:    "correct-password",
		TunnelID:    []byte("0123456789abcdef"),
		ClientLimit: 1500,
	})
	if err != nil {
		t.Fatal(err)
	}
	sr := <-serverDone
	if sr.err != nil {
		t.Fatal(sr.err)
	}
	if sr.result.Branch != "wbd" || sr.result.Admission == nil || sr.result.Fallback != nil {
		t.Fatalf("server result=%#v", sr.result)
	}
	if dialCalls.Load() != 0 {
		t.Fatalf("recognized WBD session dialed fallback %d times", dialCalls.Load())
	}
	if client.Negotiated.Keys != sr.result.Admission.Negotiated.Keys {
		t.Fatal("recognized branch admission keys mismatch")
	}
}

func TestFallbackSNIMismatchDoesNotDialTarget(t *testing.T) {
	assoc, peer := newAssociationPeer(t)
	defer assoc.Close()
	defer peer.Close()

	serverRouteKey := []byte("0123456789abcdef0123456789abcdef")
	clientRouteKey := []byte("fedcba9876543210fedcba9876543210")
	var dialCalls atomic.Int32
	serverDone := make(chan error, 1)
	go func() {
		_, err := HandleServerAssociation(
			context.Background(),
			assoc,
			ServerAdmissionConfig{
				TLS: ServerConfig{
					ServerName: "target.test",
					RouteKey:   serverRouteKey,
					TLSConfig:  &tls.Config{},
					Timeout:    time.Second,
				},
				ExpectedUsername: "solo",
				ExpectedPassword: "correct-password",
				ServerLimit:      1450,
			},
			FallbackConfig{
				Target:     "decoy.invalid:443",
				ServerName: "target.test",
				DialContext: func(context.Context, string, string) (net.Conn, error) {
					dialCalls.Add(1)
					return nil, errors.New("must not dial mismatched SNI")
				},
			},
		)
		serverDone <- err
	}()

	client, err := newFirefox120Client(peer, ClientConfig{
		ServerName: "wrong.test",
		RouteKey:   clientRouteKey,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	clientErr := make(chan error, 1)
	go func() { clientErr <- client.HandshakeContext(context.Background()) }()

	if err := <-serverDone; err == nil {
		t.Fatal("SNI mismatch unexpectedly accepted")
	}
	assoc.Close()
	_ = peer.Close()
	select {
	case <-clientErr:
	case <-time.After(time.Second):
		t.Fatal("client handshake did not stop after fallback SNI rejection")
	}
	if dialCalls.Load() != 0 {
		t.Fatalf("SNI mismatch dialed target %d times", dialCalls.Load())
	}
}

