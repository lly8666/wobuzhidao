package realityfront

import (
	"context"
	"crypto/tls"
	"net"
	"reflect"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

func TestSingleFlowTLSAdmissionUsesProvidedConnection(t *testing.T) {
	cert := makeServerCert(t)
	key := []byte("0123456789abcdef0123456789abcdef")
	ticketDir := t.TempDir()
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		_, err := BootstrapServerSingleFlow(context.Background(), serverConn, SingleFlowServerConfig{
			ServerName: "target.test", RouteKey: key,
			ExpectedUsername: "solo", ExpectedPassword: "single-flow-password",
			TicketDir: ticketDir, TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}, Timeout: 2 * time.Second,
		})
		serverDone <- err
	}()

	ticket, state, err := BootstrapClientSingleFlow(context.Background(), clientConn, SingleFlowClientConfig{
		ServerName: "target.test", RouteKey: key, Username: "solo", Password: "single-flow-password",
		VerifyServer: false, Timeout: 2 * time.Second,
	})
	if err != nil { t.Fatal(err) }
	if state.Version != tls.VersionTLS13 { t.Fatalf("TLS version=%x", state.Version) }
	if err := <-serverDone; err != nil { t.Fatal(err) }
	if err := ConsumeTicket(ticketDir, ticket, time.Now(), time.Minute); err != nil { t.Fatalf("consume ticket: %v", err) }
}

func TestSingleFlowFirefox120PersonaKeepsPresetAndRouteMarker(t *testing.T) {
	key := []byte("0123456789abcdef0123456789abcdef")
	clientConn, peerConn := net.Pipe()
	defer clientConn.Close()
	defer peerConn.Close()

	cfg := SingleFlowClientConfig{ServerName: "target.test", RouteKey: key}
	got, err := newSingleFlowFirefox120Client(clientConn, cfg)
	if err != nil { t.Fatal(err) }
	if got.ClientHelloID != utls.HelloFirefox_120 {
		t.Fatalf("ClientHelloID=%v want Firefox120", got.ClientHelloID)
	}
	if got.HandshakeState.Hello == nil || len(got.HandshakeState.Hello.Random) != 32 || len(got.HandshakeState.Hello.SessionId) != 32 {
		t.Fatalf("invalid prepared ClientHello state")
	}
	var random [32]byte
	copy(random[:], got.HandshakeState.Hello.Random)
	wantMarker := markerFor(key, cfg.ServerName, random)
	if !reflect.DeepEqual(got.HandshakeState.Hello.SessionId, wantMarker[:]) {
		t.Fatalf("compatibility SessionID is not the WBD route marker")
	}

	refClient, refPeer := net.Pipe()
	defer refClient.Close()
	defer refPeer.Close()
	ref := utls.UClient(refClient, &utls.Config{ServerName: cfg.ServerName, InsecureSkipVerify: true}, utls.HelloFirefox_120)
	if err := ref.BuildHandshakeState(); err != nil { t.Fatal(err) }

	if !reflect.DeepEqual(got.HandshakeState.Hello.CipherSuites, ref.HandshakeState.Hello.CipherSuites) {
		t.Fatalf("Firefox120 cipher suites changed")
	}
	if !reflect.DeepEqual(got.HandshakeState.Hello.SupportedVersions, ref.HandshakeState.Hello.SupportedVersions) {
		t.Fatalf("Firefox120 supported versions changed")
	}
	if !reflect.DeepEqual(got.HandshakeState.Hello.SupportedCurves, ref.HandshakeState.Hello.SupportedCurves) {
		t.Fatalf("Firefox120 supported groups changed")
	}
	if !reflect.DeepEqual(got.HandshakeState.Hello.SupportedSignatureAlgorithms, ref.HandshakeState.Hello.SupportedSignatureAlgorithms) {
		t.Fatalf("Firefox120 signature algorithms changed")
	}
	if !reflect.DeepEqual(got.HandshakeState.Hello.AlpnProtocols, ref.HandshakeState.Hello.AlpnProtocols) {
		t.Fatalf("Firefox120 ALPN changed")
	}
	if len(got.Extensions) != len(ref.Extensions) {
		t.Fatalf("Firefox120 extension count=%d want=%d", len(got.Extensions), len(ref.Extensions))
	}
	for i := range got.Extensions {
		if reflect.TypeOf(got.Extensions[i]) != reflect.TypeOf(ref.Extensions[i]) {
			t.Fatalf("Firefox120 extension[%d]=%T want %T", i, got.Extensions[i], ref.Extensions[i])
		}
	}
}

func TestSingleFlowWrongMarkerIsRejectedBeforeAdmission(t *testing.T) {
	cert := makeServerCert(t)
	serverKey := []byte("0123456789abcdef0123456789abcdef")
	clientKey := []byte("fedcba9876543210fedcba9876543210")
	clientConn, serverConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	serverDone := make(chan error, 1)
	go func() {
		_, err := BootstrapServerSingleFlow(context.Background(), serverConn, SingleFlowServerConfig{
			ServerName: "target.test", RouteKey: serverKey,
			ExpectedUsername: "solo", ExpectedPassword: "single-flow-password", TicketDir: t.TempDir(),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}}, Timeout: time.Second,
		})
		serverDone <- err
	}()
	_, _, _ = BootstrapClientSingleFlow(context.Background(), clientConn, SingleFlowClientConfig{
		ServerName: "target.test", RouteKey: clientKey, Username: "solo", Password: "single-flow-password", Timeout: time.Second,
	})
	if err := <-serverDone; err != ErrMarker { t.Fatalf("server err=%v want ErrMarker", err) }
}
