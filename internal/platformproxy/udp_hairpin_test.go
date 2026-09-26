package platformproxy

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestUDPRelayHairpinLoopbackBetweenMappings(t *testing.T) {
	echo := startUDPEcho(t)

	relayConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer relayConn.Close()
	relay, err := NewUDPRelay(relayConn, time.Minute)
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

	service := dialRelay(t, relayConn.LocalAddr().(*net.UDPAddr))
	defer service.Close()
	const flowA uint64 = 31
	const flowB uint64 = 32
	target := echo.LocalAddr().(*net.UDPAddr).AddrPort()

	// Prime two independent internal mappings through an ordinary external peer.
	sendUDPFrame(t, service, Frame{Kind: KindUDPDatagram, FlowID: flowA, Peer: target, Payload: []byte("prime-a")})
	if got := readUDPFrame(t, service); got.FlowID != flowA || string(got.Payload) != "prime-a" {
		t.Fatalf("prime A got %+v payload=%q", got, got.Payload)
	}
	sendUDPFrame(t, service, Frame{Kind: KindUDPDatagram, FlowID: flowB, Peer: target, Payload: []byte("prime-b")})
	if got := readUDPFrame(t, service); got.FlowID != flowB || string(got.Payload) != "prime-b" {
		t.Fatalf("prime B got %+v payload=%q", got, got.Payload)
	}

	keyA := udpFlowKey{servicePeer: service.LocalAddr().String(), flowID: flowA}
	keyB := udpFlowKey{servicePeer: service.LocalAddr().String(), flowID: flowB}
	relay.mu.Lock()
	a := relay.flows[keyA]
	b := relay.flows[keyB]
	relay.mu.Unlock()
	if a == nil || b == nil {
		t.Fatalf("missing mappings: a=%v b=%v", a != nil, b != nil)
	}
	portA := a.upstream.LocalAddr().(*net.UDPAddr).Port
	portB := b.upstream.LocalAddr().(*net.UDPAddr).Port
	if portA == 0 || portB == 0 || portA == portB {
		t.Fatalf("unexpected mapped ports: a=%d b=%d", portA, portB)
	}

	// A addresses B by B's external mapped endpoint. The packet must be delivered
	// to flow B and must appear to B as coming from A's external mapped endpoint.
	hairpinB := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: portB}
	sendUDPFrame(t, service, Frame{Kind: KindUDPDatagram, FlowID: flowA, Peer: hairpinB.AddrPort(), Payload: []byte("a-to-b")})
	toB := readUDPFrame(t, service)
	if toB.FlowID != flowB || toB.Peer.Port() != uint16(portA) || !toB.Peer.Addr().IsLoopback() || string(toB.Payload) != "a-to-b" {
		t.Fatalf("hairpin A->B got %+v payload=%q", toB, toB.Payload)
	}

	// B replies to the observed mapped endpoint. The reverse loopback must land
	// on A and expose B's mapped endpoint as the source.
	sendUDPFrame(t, service, Frame{Kind: KindUDPDatagram, FlowID: flowB, Peer: toB.Peer, Payload: []byte("b-to-a")})
	toA := readUDPFrame(t, service)
	if toA.FlowID != flowA || toA.Peer.Port() != uint16(portB) || !toA.Peer.Addr().IsLoopback() || string(toA.Payload) != "b-to-a" {
		t.Fatalf("hairpin B->A got %+v payload=%q", toA, toA.Payload)
	}
}
