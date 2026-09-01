package main

import (
	"net"
	"runtime"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestMaxLanesRejectsThirdRealUDPPeerAndKeepsExistingLanesHealthy(t *testing.T) {
	echo, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil { t.Fatal(err) }
	defer echo.Close()
	go func() {
		buf := make([]byte, 65535)
		for {
			n, peer, err := echo.ReadFromUDP(buf)
			if err != nil { return }
			_, _ = echo.WriteToUDP(buf[:n], peer)
		}
	}()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil { t.Fatal(err) }
	s := &server{
		conn: conn,
		serviceAddr: echo.LocalAddr().(*net.UDPAddr),
		replayWindow: 4096,
		maxSessions: 4,
		maxLanes: 2,
		idle: time.Minute,
		sessions: make(map[gamelane.SessionID]*gameSession),
		peerSession: make(map[string]gamelane.SessionID),
	}
	defer s.Close()
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run() }()

	serverAddr := conn.LocalAddr().(*net.UDPAddr)
	lanes := make([]*net.UDPConn, 3)
	for i := range lanes {
		lanes[i], err = net.DialUDP("udp4", nil, serverAddr)
		if err != nil { t.Fatal(err) }
		defer lanes[i].Close()
	}

	var sid gamelane.SessionID
	copy(sid[:], []byte("max-lanes-real"))
	enc, err := gamelane.NewEncoder(sid, 1)
	if err != nil { t.Fatal(err) }

	// Packet 1 exists to bind two distinct real UDP peers and establish the
	// dedupe relation. Return timing for this first packet is intentionally not
	// asserted because the echo may come back before the second bind completes.
	_, first, err := enc.WrapCopies([]byte("bind"), []uint8{1, 2})
	if err != nil { t.Fatal(err) }
	if _, err := lanes[0].Write(first[0].Wire); err != nil { t.Fatal(err) }
	if _, err := lanes[1].Write(first[1].Wire); err != nil { t.Fatal(err) }

	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock()
		gs := s.sessions[sid]
		s.mu.Unlock()
		bound := 0
		if gs != nil {
			gs.mu.Lock(); bound = len(gs.lanes); gs.mu.Unlock()
		}
		if bound == 2 { break }
		if time.Now().After(deadline) { t.Fatalf("only %d lanes bound before timeout", bound) }
		time.Sleep(time.Millisecond)
	}

	// Drain any race return from the binding packet so later reads identify the
	// exact logical packet under test rather than an earlier queued datagram.
	for _, c := range lanes[:2] {
		_ = c.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		buf := make([]byte, 65535)
		for {
			if _, err := c.Read(buf); err != nil { break }
		}
	}

	readPayload := func(c *net.UDPConn) []byte {
		_ = c.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		buf := make([]byte, 65535)
		n, err := c.Read(buf)
		if err != nil { t.Fatal(err) }
		_, payload, err := gamelane.Parse(buf[:n])
		if err != nil { t.Fatal(err) }
		return payload
	}

	// With both lanes admitted, one logical packet must be delivered once to
	// the echo and raced back over both real UDP associations.
	_, both, err := enc.WrapCopies([]byte("both-ready"), []uint8{1, 2})
	if err != nil { t.Fatal(err) }
	if _, err := lanes[0].Write(both[0].Wire); err != nil { t.Fatal(err) }
	if _, err := lanes[1].Write(both[1].Wire); err != nil { t.Fatal(err) }
	if got := string(readPayload(lanes[0])); got != "both-ready" { t.Fatalf("lane1 ready=%q", got) }
	if got := string(readPayload(lanes[1])); got != "both-ready" { t.Fatalf("lane2 ready=%q", got) }

	// A third authenticated service peer for the same logical session tries to
	// bind lane ID 3. It must not receive an echoed response because max-lanes=2.
	_, thirdCopy, err := enc.WrapCopies([]byte("third-must-drop"), []uint8{3})
	if err != nil { t.Fatal(err) }
	_ = lanes[2].SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, err := lanes[2].Write(thirdCopy[0].Wire); err != nil { t.Fatal(err) }
	buf := make([]byte, 65535)
	if n, err := lanes[2].Read(buf); err == nil {
		t.Fatalf("third lane unexpectedly received %d bytes", n)
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("third lane read err=%v, want timeout", err)
	}

	// The over-cap attempt must not poison or evict the two admitted lanes.
	_, healthy, err := enc.WrapCopies([]byte("still-healthy"), []uint8{1, 2})
	if err != nil { t.Fatal(err) }
	if _, err := lanes[0].Write(healthy[0].Wire); err != nil { t.Fatal(err) }
	if _, err := lanes[1].Write(healthy[1].Wire); err != nil { t.Fatal(err) }
	if got := string(readPayload(lanes[0])); got != "still-healthy" { t.Fatalf("lane1 after reject=%q", got) }
	if got := string(readPayload(lanes[1])); got != "still-healthy" { t.Fatalf("lane2 after reject=%q", got) }

	s.mu.Lock()
	if len(s.sessions) != 1 { t.Fatalf("sessions=%d", len(s.sessions)) }
	gs := s.sessions[sid]
	s.mu.Unlock()
	if gs == nil { t.Fatal("logical session disappeared") }

	// The two healthy echoes prove the logical session and admitted lanes still
	// work, but the final duplicate datagram can still be queued in Run's UDP
	// goroutine when the client-side reads complete. Wait for the exact 3/3
	// accounting state with a strict deadline instead of betting on a fixed
	// sleep or weakening the original assertion.
	counterDeadline := time.Now().Add(time.Second)
	for {
		gs.mu.Lock()
		inFirst, inDup := gs.inFirst, gs.inDup
		gs.mu.Unlock()
		if inFirst == 3 && inDup == 3 { break }
		if time.Now().After(counterDeadline) {
			t.Fatalf("inbound counters did not settle before timeout: in_first=%d in_dup=%d, want 3/3", inFirst, inDup)
		}
		runtime.Gosched()
	}

	gs.mu.Lock()
	bound := len(gs.lanes)
	inFirst, inDup := gs.inFirst, gs.inDup
	gs.mu.Unlock()
	if bound != 2 { t.Fatalf("bound lanes=%d, want 2", bound) }
	if inFirst != 3 || inDup != 3 { t.Fatalf("in_first=%d in_dup=%d, want 3/3", inFirst, inDup) }

	_ = conn.Close()
	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("server Run did not stop after UDP close")
	}
}
