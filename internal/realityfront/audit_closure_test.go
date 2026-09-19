package realityfront

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func auditOrdinarySYN(port uint16, seq uint32, mss uint16, mssSet, sack, wsSet bool, ws uint8) faketcp.Segment {
	return faketcp.Segment{
		SrcIP:          [4]byte{10, 3, 0, byte(port%200 + 1)},
		DstIP:          [4]byte{10, 3, 1, 1},
		SrcPort:        port,
		DstPort:        443,
		Seq:            seq,
		Flags:          faketcp.FlagSYN,
		Window:         64240,
		MSS:            mss,
		MSSSet:         mssSet,
		SACKPermitted:  sack,
		WindowScale:    ws,
		WindowScaleSet: wsSet,
	}
}

func newAssociationPeerWithSYNForAudit(t *testing.T, syn faketcp.Segment) (*faketcp.ServerAssociation, *associationPeerConn) {
	t.Helper()
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
	finalACK.Seq = syn.Seq + 1
	finalACK.Ack = 5001
	finalACK.Payload = nil
	if _, err := assoc.HandleSegment(finalACK, time.Now()); err != nil {
		t.Fatal(err)
	}
	return assoc, &associationPeerConn{
		assoc:   assoc,
		base:    syn,
		emitted: emitted,
		sendSeq: syn.Seq + 1,
		recvSeq: 5001,
		done:    make(chan struct{}),
	}
}

func TestOrdinarySYNProfilesReachClientHelloFallbackClassification(t *testing.T) {
	serverKey := []byte("0123456789abcdef0123456789abcdef")
	clientKey := []byte("fedcba9876543210fedcba9876543210")
	dialErr := errors.New("audit: ordinary SYN reached fallback dial")
	cases := []struct {
		name string
		syn  faketcp.Segment
	}{
		{"mss1460-ws7-sack", auditOrdinarySYN(42001, 1000, 1460, true, true, true, 7)},
		{"no-options", auditOrdinarySYN(42002, 2000, 0, false, false, false, 0)},
		{"mss1200-ws4-no-sack", auditOrdinarySYN(42003, 3000, 1200, true, false, true, 4)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if faketcp.IsWBDHandshakeSegment(tc.syn) {
				t.Fatal("audit ordinary SYN unexpectedly matches WBD presentation")
			}
			assoc, peer := newAssociationPeerWithSYNForAudit(t, tc.syn)
			defer assoc.Close()
			defer peer.Close()

			var dialCalls atomic.Int32
			serverDone := make(chan error, 1)
			go func() {
				_, err := HandleServerAssociation(
					context.Background(),
					assoc,
					ServerAdmissionConfig{
						TLS: ServerConfig{
							ServerName: "target.test",
							RouteKey:   serverKey,
							TLSConfig:  &tls.Config{},
							Timeout:    500 * time.Millisecond,
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
							return nil, dialErr
						},
					},
				)
				serverDone <- err
			}()

			client, err := newFirefox120Client(peer, ClientConfig{
				ServerName: "target.test",
				RouteKey:   clientKey,
				Timeout:    500 * time.Millisecond,
			})
			if err != nil {
				t.Fatal(err)
			}
			clientDone := make(chan error, 1)
			go func() { clientDone <- client.HandshakeContext(context.Background()) }()

			select {
			case err := <-serverDone:
				if !errors.Is(err, dialErr) {
					t.Fatalf("server err=%v want fallback dial sentinel", err)
				}
			case <-time.After(time.Second):
				t.Fatal("ordinary SYN did not reach ClientHello fallback classification")
			}
			if dialCalls.Load() != 1 {
				t.Fatalf("fallback dial calls=%d want=1", dialCalls.Load())
			}
			if _, ok := assoc.TransitionState(); ok {
				t.Fatal("ordinary fallback candidate prepared WBD transition")
			}
			_ = peer.Close()
			select {
			case <-clientDone:
			case <-time.After(time.Second):
				t.Fatal("client handshake did not exit after fallback dial failure")
			}
		})
	}
}

func TestClientCandidateDeadlineCoversPostTLSAdmissionSilence(t *testing.T) {
	cert := makeServerCert(t)
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		tlsConn := tls.Server(serverConn, &tls.Config{
			Certificates:           []tls.Certificate{cert},
			MinVersion:             tls.VersionTLS13,
			MaxVersion:             tls.VersionTLS13,
			SessionTicketsDisabled: true,
		})
		if err := tlsConn.HandshakeContext(context.Background()); err != nil {
			serverDone <- err
			return
		}
		if _, err := readAdmissionRequest(tlsConn); err != nil {
			serverDone <- err
			return
		}
		var one [1]byte
		_, err := tlsConn.Read(one[:])
		serverDone <- err
	}()

	start := time.Now()
	_, err := EstablishClient(context.Background(), clientConn, ClientAdmissionConfig{
		TLS: ClientConfig{
			ServerName: "target.test",
			RouteKey:   []byte("0123456789abcdef0123456789abcdef"),
			Timeout:    150 * time.Millisecond,
		},
		Username: "solo", Password: "correct-password",
		TunnelID: []byte("0123456789abcdef"), ClientLimit: 1450,
	})
	if err == nil {
		t.Fatal("post-TLS admission silence did not time out")
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("candidate deadline took too long: %v", elapsed)
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("client timeout did not release underlying connection")
	}
}

func TestClientCandidateContextCancelCoversAdmissionWait(t *testing.T) {
	cert := makeServerCert(t)
	clientConn, serverConn := net.Pipe()
	defer serverConn.Close()

	requestSeen := make(chan struct{})
	serverDone := make(chan error, 1)
	go func() {
		tlsConn := tls.Server(serverConn, &tls.Config{
			Certificates:           []tls.Certificate{cert},
			MinVersion:             tls.VersionTLS13,
			MaxVersion:             tls.VersionTLS13,
			SessionTicketsDisabled: true,
		})
		if err := tlsConn.HandshakeContext(context.Background()); err != nil {
			serverDone <- err
			return
		}
		if _, err := readAdmissionRequest(tlsConn); err != nil {
			serverDone <- err
			return
		}
		close(requestSeen)
		var one [1]byte
		_, err := tlsConn.Read(one[:])
		serverDone <- err
	}()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		<-requestSeen
		cancel()
	}()
	start := time.Now()
	_, err := EstablishClient(ctx, clientConn, ClientAdmissionConfig{
		TLS: ClientConfig{
			ServerName: "target.test",
			RouteKey:   []byte("0123456789abcdef0123456789abcdef"),
			Timeout:    5 * time.Second,
		},
		Username: "solo", Password: "correct-password",
		TunnelID: []byte("0123456789abcdef"), ClientLimit: 1450,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("client err=%v want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("context cancellation was not prompt: %v", elapsed)
	}
	select {
	case <-serverDone:
	case <-time.After(time.Second):
		t.Fatal("client cancellation did not release connection")
	}
}

func TestServerCandidateDeadlineRejectsPartialAdmissionAndClosesCandidate(t *testing.T) {
	cert := makeServerCert(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	assoc, peer := newAssociationPeer(t)
	defer peer.Close()

	serverDone := make(chan error, 1)
	go func() {
		_, err := EstablishServer(context.Background(), assoc, ServerAdmissionConfig{
			TLS: ServerConfig{
				ServerName: "target.test",
				RouteKey:   routeKey,
				TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout:    180 * time.Millisecond,
			},
			ExpectedUsername: "solo",
			ExpectedPassword: "correct-password",
			ServerLimit:      1450,
		})
		serverDone <- err
	}()

	clientTLS, err := handshakeClientConn(context.Background(), peer, ClientConfig{
		ServerName: "target.test",
		RouteKey:   routeKey,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := marshalAdmissionRequest(AdmissionRequest{
		RecordVersion: RecordVersionV1,
		TunnelID:      []byte("0123456789abcdef"),
		ClientLimit:   1450,
		Username:      "solo",
		Password:      "correct-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clientTLS.Write(wire[:len(wire)/2]); err != nil {
		t.Fatal(err)
	}

	select {
	case err := <-serverDone:
		if err == nil {
			t.Fatal("partial admission unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("partial admission did not obey absolute candidate deadline")
	}
	if assoc.State() != faketcp.ServerAssociationClosed {
		t.Fatalf("association state=%v want closed", assoc.State())
	}
	if _, ok := assoc.TransitionState(); ok {
		t.Fatal("partial admission prepared transition")
	}
}

func TestServerFinalAdmissionReplyWithoutACKAbortsCandidate(t *testing.T) {
	cert := makeServerCert(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	assoc, peer := newAssociationPeer(t)
	defer peer.Close()

	serverDone := make(chan error, 1)
	go func() {
		_, err := EstablishServer(context.Background(), assoc, ServerAdmissionConfig{
			TLS: ServerConfig{
				ServerName: "target.test",
				RouteKey:   routeKey,
				TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout:    250 * time.Millisecond,
			},
			ExpectedUsername: "solo",
			ExpectedPassword: "correct-password",
			ServerLimit:      1450,
			Random:           bytes.NewReader(bytes.Repeat([]byte{7}, 16)),
		})
		serverDone <- err
	}()

	clientTLS, err := handshakeClientConn(context.Background(), peer, ClientConfig{
		ServerName: "target.test",
		RouteKey:   routeKey,
		Timeout:    time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	wire, err := marshalAdmissionRequest(AdmissionRequest{
		RecordVersion: RecordVersionV1,
		TunnelID:      []byte("0123456789abcdef"),
		ClientLimit:   1450,
		Username:      "solo",
		Password:      "correct-password",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := clientTLS.Write(wire); err != nil {
		t.Fatal(err)
	}

	// Consume the server's final TLS application record directly from the test
	// emitter and intentionally do not generate its FakeTCP ACK.
	select {
	case seg := <-peer.emitted:
		if len(seg.Payload) == 0 {
			t.Fatal("final admission reply segment had no payload")
		}
	case <-time.After(time.Second):
		t.Fatal("server never emitted final admission reply")
	}

	select {
	case err := <-serverDone:
		if err == nil {
			t.Fatal("unacknowledged final admission reply unexpectedly succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("server did not abandon unacknowledged final admission reply")
	}
	if assoc.State() != faketcp.ServerAssociationClosed {
		t.Fatalf("association state=%v want closed", assoc.State())
	}
	if state, ok := assoc.TransitionState(); !ok || state != faketcp.TransitionAborted {
		t.Fatalf("transition state=%v ok=%v want aborted", state, ok)
	}
}

func TestCandidateDeadlineDoesNotLeakAfterSuccessfulHandoff(t *testing.T) {
	cert := makeServerCert(t)
	routeKey := []byte("0123456789abcdef0123456789abcdef")
	assoc, peer := newAssociationPeer(t)
	defer assoc.Close()
	defer peer.Close()

	serverDone := make(chan error, 1)
	go func() {
		_, err := EstablishServer(context.Background(), assoc, ServerAdmissionConfig{
			TLS: ServerConfig{
				ServerName: "target.test",
				RouteKey:   routeKey,
				TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout:    500 * time.Millisecond,
			},
			ExpectedUsername: "solo",
			ExpectedPassword: "correct-password",
			ServerLimit:      1450,
			Random:           bytes.NewReader(bytes.Repeat([]byte{8}, 16)),
		})
		serverDone <- err
	}()

	_, err := EstablishClient(context.Background(), peer, ClientAdmissionConfig{
		TLS: ClientConfig{
			ServerName: "target.test",
			RouteKey:   routeKey,
			Timeout:    500 * time.Millisecond,
		},
		Username: "solo", Password: "correct-password",
		TunnelID: []byte("0123456789abcdef"), ClientLimit: 1450,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := <-serverDone; err != nil {
		t.Fatal(err)
	}

	// After successful handoff the absolute candidate deadline must be cleared
	// on the underlying client transport rather than leaking into steady state.
	peer.mu.Lock()
	readDeadline := peer.readDeadline
	writeDeadline := peer.writeDeadline
	peer.mu.Unlock()
	if !readDeadline.IsZero() || !writeDeadline.IsZero() {
		t.Fatalf("successful handoff retained deadlines read=%v write=%v", readDeadline, writeDeadline)
	}
}
