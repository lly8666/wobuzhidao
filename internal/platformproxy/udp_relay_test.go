package platformproxy

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestUDPRelayIsolatesSameFlowIDByServicePeer(t *testing.T) {
	echoA := startUDPEcho(t)
	echoB := startUDPEcho(t)

	relayConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer relayConn.Close()
	relay, err := NewUDPRelay(relayConn, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relay.Serve(ctx) }()
	defer func() {
		cancel()
		_ = relayConn.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
				t.Errorf("relay exit: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("relay did not stop")
		}
	}()

	serviceA := dialRelay(t, relayConn.LocalAddr().(*net.UDPAddr))
	defer serviceA.Close()
	serviceB := dialRelay(t, relayConn.LocalAddr().(*net.UDPAddr))
	defer serviceB.Close()

	const flowID = 1
	sendUDPFrame(t, serviceA, Frame{Kind: KindUDPDatagram, FlowID: flowID, Peer: echoA.LocalAddr().(*net.UDPAddr).AddrPort(), Payload: []byte("from-a")})
	sendUDPFrame(t, serviceB, Frame{Kind: KindUDPDatagram, FlowID: flowID, Peer: echoB.LocalAddr().(*net.UDPAddr).AddrPort(), Payload: []byte("from-b")})

	gotA := readUDPFrame(t, serviceA)
	gotB := readUDPFrame(t, serviceB)
	if gotA.FlowID != flowID || gotA.Peer != echoA.LocalAddr().(*net.UDPAddr).AddrPort() || string(gotA.Payload) != "from-a" {
		t.Fatalf("service A got %+v payload=%q", gotA, gotA.Payload)
	}
	if gotB.FlowID != flowID || gotB.Peer != echoB.LocalAddr().(*net.UDPAddr).AddrPort() || string(gotB.Payload) != "from-b" {
		t.Fatalf("service B got %+v payload=%q", gotB, gotB.Payload)
	}
}

func TestUDPRelayRejectsTargetChangeWithinFlow(t *testing.T) {
	echoA := startUDPEcho(t)
	echoB := startUDPEcho(t)
	relayConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer relayConn.Close()
	relay, err := NewUDPRelay(relayConn, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	service := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 30001}
	now := time.Now()

	first, err := Marshal(Frame{Kind: KindUDPDatagram, FlowID: 9, Peer: echoA.LocalAddr().(*net.UDPAddr).AddrPort(), Payload: []byte("one")})
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.handle(service, first, now); err != nil {
		t.Fatal(err)
	}
	defer relay.closeAll()

	changed, err := Marshal(Frame{Kind: KindUDPDatagram, FlowID: 9, Peer: echoB.LocalAddr().(*net.UDPAddr).AddrPort(), Payload: []byte("two")})
	if err != nil {
		t.Fatal(err)
	}
	if err := relay.handle(service, changed, now); !errors.Is(err, ErrMalformed) {
		t.Fatalf("target-change err=%v", err)
	}
}

func startUDPEcho(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			n, peer, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			_, _ = conn.WriteToUDP(buf[:n], peer)
		}
	}()
	return conn
}

func dialRelay(t *testing.T, relay *net.UDPAddr) *net.UDPConn {
	t.Helper()
	conn, err := net.DialUDP("udp4", nil, relay)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func sendUDPFrame(t *testing.T, conn *net.UDPConn, frame Frame) {
	t.Helper()
	wire, err := Marshal(frame)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Write(wire); err != nil {
		t.Fatal(err)
	}
}

func readUDPFrame(t *testing.T, conn *net.UDPConn) Frame {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 2048)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := Unmarshal(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	return frame
}
