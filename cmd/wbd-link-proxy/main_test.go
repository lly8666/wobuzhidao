package main

import (
	"bytes"
	"net"
	"os"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
	"github.com/lly8666/wobuzhidao/internal/persona"
)

func udp4(t *testing.T) *net.UDPConn {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func addr(c *net.UDPConn) *net.UDPAddr { return c.LocalAddr().(*net.UDPAddr) }

func TestImmutableLinkProxyOffNoAuth(t *testing.T) {
	runProxyIntegration(t, "off", false, false)
}

func TestImmutableLinkProxyFixed20x20WithAuth(t *testing.T) {
	runProxyIntegration(t, "20:20", true, false)
}

func TestImmutableLinkProxyRealityDemoGateThenEncryptedData(t *testing.T) {
	runProxyIntegration(t, "off", true, true)
}

type establishedStartup struct{}

func (establishedStartup) Established() bool { return true }
func (establishedStartup) RetryWire() ([]byte, error) { return nil, nil }
func (establishedStartup) HandleWire([]byte) ([]byte, error) { return nil, nil }
func (establishedStartup) Accept() (control.LinkAccept, bool) { return control.LinkAccept{}, true }

func TestClientDataLoopHeartbeatAndGracefulClose(t *testing.T) {
	client := udp4(t)
	dtls := udp4(t)
	defer client.Close()
	defer dtls.Close()

	path, err := linkdata.New(control.LinkConfig{
		FECMode: control.FECOff, Scheduler: control.FECSchedulerNone,
		MTU: 1400, LaneCount: 1,
	}, maxBlocks)
	if err != nil {
		t.Fatal(err)
	}
	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	go func() {
		done <- clientDataLoop(client, addr(dtls), path, establishedStartup{}, 20*time.Millisecond, stop)
	}()

	buf := make([]byte, 65535)
	_ = dtls.SetReadDeadline(time.Now().Add(time.Second))
	n, peer, err := dtls.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read PING: %v", err)
	}
	frame, err := control.UnmarshalLink(buf[:n])
	if err != nil {
		t.Fatalf("decode PING: %v", err)
	}
	ping, ok := frame.(control.Ping)
	if !ok || ping.Nonce == 0 {
		t.Fatalf("got %T %#v want nonzero PING", frame, frame)
	}
	pong, err := control.MarshalLink(control.Pong{Nonce: ping.Nonce})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dtls.WriteToUDP(pong, peer); err != nil {
		t.Fatal(err)
	}

	stop <- os.Interrupt
	deadline := time.Now().Add(time.Second)
	for {
		_ = dtls.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err = dtls.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() && time.Now().Before(deadline) {
				continue
			}
			t.Fatalf("read CLOSE: %v", err)
		}
		frame, err = control.UnmarshalLink(buf[:n])
		if err != nil {
			continue
		}
		closeFrame, ok := frame.(control.Close)
		if !ok {
			continue
		}
		if closeFrame.Reason != control.CloseNormal {
			t.Fatalf("close reason=%d want=%d", closeFrame.Reason, control.CloseNormal)
		}
		break
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("client loop: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("client loop did not stop")
	}
}

func runProxyIntegration(t *testing.T, fecMode string, auth, demo bool) {
	t.Helper()
	clientProxy := udp4(t)
	serverProxy := udp4(t)
	dtlsClient := udp4(t)
	dtlsServer := udp4(t)
	service := udp4(t)
	app := udp4(t)
	defer clientProxy.Close()
	defer serverProxy.Close()
	defer dtlsClient.Close()
	defer dtlsServer.Close()
	defer service.Close()
	defer app.Close()

	stopClient := make(chan os.Signal, 1)
	stopServer := make(chan os.Signal, 1)
	clientDone := make(chan error, 1)
	serverDone := make(chan error, 1)

	// Fake-DTLS plaintext relay. Direction C->S must arrive at serverProxy from
	// dtlsServer's source port, while S->C must arrive at clientProxy from the
	// configured dtlsClient source port. The real product replaces this relay
	// with the qualified DTLS 1.3 shim; this test targets WBD startup/data demux.
	relayStop := make(chan struct{})
	defer close(relayStop)
	go relayUDP(dtlsClient, dtlsServer, addr(serverProxy), relayStop)
	go relayUDP(dtlsServer, dtlsClient, addr(clientProxy), relayStop)

	// Local service echo behind the server-side WBD proxy.
	serviceStop := make(chan struct{})
	defer close(serviceStop)
	go func() {
		buf := make([]byte, 4096)
		for {
			_ = service.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
			n, peer, err := service.ReadFromUDP(buf)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					select {
					case <-serviceStop:
						return
					default:
						continue
					}
				}
				return
			}
			_, _ = service.WriteToUDP(buf[:n], peer)
		}
	}()

	expected, token := "", ""
	if auth {
		expected, token = "test-secret", "test-secret"
	}
	serverOpts := options{
		mode: "server", service: addr(service).String(), expectedToken: expected,
		setupTimeout: 3 * time.Second,
	}
	clientOpts := options{
		mode: "client", dtls: addr(dtlsClient).String(), fec: fecMode,
		mtu: 1400, flushMS: 8, lanes: 1, token: token,
		setupTimeout: 3 * time.Second,
	}
	if demo {
		dir := t.TempDir()
		id := persona.WitnessFromClientHello([]byte("demo-real-target-clienthello"))
		if err := persona.RecordWitness(dir, id, "target.example", time.Now()); err != nil {
			t.Fatal(err)
		}
		serverOpts.demoRealityWitnessDir = dir
		serverOpts.demoRealityServerName = "target.example"
		serverOpts.demoRealityTTL = 15 * time.Second
		clientOpts.demoRealityWitness = id.Hex()
	}
	go func() { serverDone <- runServer(serverProxy, serverOpts, stopServer) }()
	go func() { clientDone <- runClient(clientProxy, clientOpts, stopClient) }()

	payload := []byte("immutable-link-product-datagram")
	buf := make([]byte, 4096)
	deadline := time.Now().Add(4 * time.Second)
	for {
		if time.Now().After(deadline) {
			t.Fatalf("%s demo=%v: timed out waiting for echo", fecMode, demo)
		}
		_, _ = app.WriteToUDP(payload, addr(clientProxy))
		_ = app.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err := app.ReadFromUDP(buf)
		if err == nil {
			if !bytes.Equal(buf[:n], payload) {
				t.Fatalf("%s demo=%v: got %q want %q", fecMode, demo, buf[:n], payload)
			}
			break
		}
		if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
			t.Fatal(err)
		}
	}

	stopClient <- os.Interrupt
	stopServer <- os.Interrupt
	for name, ch := range map[string]<-chan error{"client": clientDone, "server": serverDone} {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("%s demo=%v %s: %v", fecMode, demo, name, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s demo=%v %s did not stop", fecMode, demo, name)
		}
	}
}

func relayUDP(readFrom, writeFrom *net.UDPConn, dst *net.UDPAddr, stop <-chan struct{}) {
	buf := make([]byte, 65535)
	for {
		_ = readFrom.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
		n, _, err := readFrom.ReadFromUDP(buf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				select {
				case <-stop:
					return
				default:
					continue
				}
			}
			return
		}
		_, _ = writeFrom.WriteToUDP(buf[:n], dst)
	}
}
