package platformproxy

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"sync"
	"testing"
	"time"
)

func testTCPRelayConfig() TCPRelayConfig {
	cfg := DefaultTCPRelayConfig()
	cfg.Reliability = TCPReliabilityConfig{
		ChunkSize: 4, MaxInFlight: 3,
		RTO: 20 * time.Millisecond, MaxRetransmits: 2,
		MaxReorderBytes: 12,
	}
	cfg.IdleTimeout = time.Second
	cfg.DialTimeout = time.Second
	return cfg
}

func listenRelayUDP(t *testing.T) *net.UDPConn {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func readRelayFrame(t *testing.T, conn *net.UDPConn) Frame {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 65535)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	frame, err := Unmarshal(buf[:n])
	if err != nil {
		t.Fatal(err)
	}
	return frame
}

func TestTCPRelayKeysFlowByServicePeerAndOpenIsIdempotent(t *testing.T) {
	relayConn := listenRelayUDP(t)
	serviceA := listenRelayUDP(t)
	serviceB := listenRelayUDP(t)
	relay, err := NewTCPRelay(relayConn, testTCPRelayConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()

	var mu sync.Mutex
	var dialCount int
	var peerEnds []net.Conn
	relay.dial = func(context.Context, string, string) (net.Conn, error) {
		left, right := net.Pipe()
		mu.Lock()
		dialCount++
		peerEnds = append(peerEnds, right)
		mu.Unlock()
		return left, nil
	}
	defer func() {
		mu.Lock()
		defer mu.Unlock()
		for _, conn := range peerEnds {
			_ = conn.Close()
		}
	}()

	targetA := netip.MustParseAddrPort("198.51.100.10:443")
	targetB := netip.MustParseAddrPort("203.0.113.20:443")
	openA := Frame{Kind: KindTCPOpen, FlowID: 1, Peer: targetA}
	if err := relay.HandleFrame(serviceA.LocalAddr().(*net.UDPAddr), openA, time.Now()); err != nil {
		t.Fatal(err)
	}
	if ack := readRelayFrame(t, serviceA); ack.Kind != KindTCPAck || ack.FlowID != 1 || ack.Offset != 0 {
		t.Fatalf("open ack A=%+v", ack)
	}
	if err := relay.HandleFrame(serviceA.LocalAddr().(*net.UDPAddr), openA, time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = readRelayFrame(t, serviceA)

	openB := Frame{Kind: KindTCPOpen, FlowID: 1, Peer: targetB}
	if err := relay.HandleFrame(serviceB.LocalAddr().(*net.UDPAddr), openB, time.Now()); err != nil {
		t.Fatal(err)
	}
	if ack := readRelayFrame(t, serviceB); ack.FlowID != 1 || ack.Kind != KindTCPAck {
		t.Fatalf("open ack B=%+v", ack)
	}

	mu.Lock()
	gotDials := dialCount
	mu.Unlock()
	if gotDials != 2 || relay.Len() != 2 {
		t.Fatalf("dials=%d flows=%d want 2/2", gotDials, relay.Len())
	}
	if err := relay.HandleFrame(serviceA.LocalAddr().(*net.UDPAddr), Frame{Kind: KindTCPOpen, FlowID: 1, Peer: targetB}, time.Now()); !errors.Is(err, ErrMalformed) {
		t.Fatalf("changed target err=%v", err)
	}

	if err := relay.HandleFrame(serviceA.LocalAddr().(*net.UDPAddr), Frame{Kind: KindTCPClose, FlowID: 1}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if relay.Len() != 1 {
		t.Fatalf("flows after close A=%d want=1", relay.Len())
	}
	if err := relay.HandleFrame(serviceA.LocalAddr().(*net.UDPAddr), Frame{Kind: KindTCPClose, FlowID: 1}, time.Now()); err != nil {
		t.Fatalf("duplicate close err=%v", err)
	}
	if err := relay.HandleFrame(serviceB.LocalAddr().(*net.UDPAddr), Frame{Kind: KindTCPClose, FlowID: 1}, time.Now()); err != nil {
		t.Fatal(err)
	}
}

func TestTCPRelayDataAckResponseAndRetransmit(t *testing.T) {
	relayConn := listenRelayUDP(t)
	service := listenRelayUDP(t)
	relay, err := NewTCPRelay(relayConn, testTCPRelayConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer relay.Close()

	var upstreamPeer net.Conn
	relay.dial = func(context.Context, string, string) (net.Conn, error) {
		left, right := net.Pipe()
		upstreamPeer = right
		return left, nil
	}
	defer func() {
		if upstreamPeer != nil {
			_ = upstreamPeer.Close()
		}
	}()

	servicePeer := service.LocalAddr().(*net.UDPAddr)
	if err := relay.HandleFrame(servicePeer, Frame{
		Kind: KindTCPOpen, FlowID: 9, Peer: netip.MustParseAddrPort("198.51.100.9:80"),
	}, time.Now()); err != nil {
		t.Fatal(err)
	}
	_ = readRelayFrame(t, service) // OPEN ACK(0)

	writeDone := make(chan error, 1)
	go func() {
		writeDone <- relay.HandleFrame(servicePeer, Frame{
			Kind: KindTCPData, FlowID: 9, Offset: 0, Payload: []byte("ping"),
		}, time.Now())
	}()
	buf := make([]byte, 4)
	if err := upstreamPeer.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := upstreamPeer.Read(buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "ping" {
		t.Fatalf("upstream got=%q", buf)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}
	if ack := readRelayFrame(t, service); ack.Kind != KindTCPAck || ack.Offset != 4 {
		t.Fatalf("client data ack=%+v", ack)
	}

	peerWrite := make(chan error, 1)
	go func() {
		_, err := upstreamPeer.Write([]byte("pong"))
		peerWrite <- err
	}()
	response := readRelayFrame(t, service)
	if err := <-peerWrite; err != nil {
		t.Fatal(err)
	}
	if response.Kind != KindTCPData || response.FlowID != 9 || response.Offset != 0 || string(response.Payload) != "pong" {
		t.Fatalf("response=%+v", response)
	}

	// Do not ACK the first copy. Tick must retransmit the same unacknowledged
	// range and no already-ACKed client->server data is involved.
	relay.Tick(time.Now().Add(2 * testTCPRelayConfig().Reliability.RTO))
	retry := readRelayFrame(t, service)
	if retry.Kind != KindTCPData || retry.Offset != response.Offset || string(retry.Payload) != "pong" {
		t.Fatalf("retry=%+v", retry)
	}
	if err := relay.HandleFrame(servicePeer, Frame{Kind: KindTCPAck, FlowID: 9, Offset: 4}, time.Now()); err != nil {
		t.Fatal(err)
	}
}
