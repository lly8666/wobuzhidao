package main

import (
	"net"
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
		_ = lanes[i].SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	}

	var sid gamelane.SessionID
	copy(sid[:], []byte("max-lanes-real-udp"))
	enc, err := gamelane.NewEncoder(sid, 1)
	if err != nil { t.Fatal(err) }

	// Packet 1 binds lane 1 and lane 2 to two distinct real UDP peers. Both
	// copies carry the same logical packet ID; downstream must see it once and
	// the echoed logical response must race back over exactly those two lanes.
	_, first, err := enc.WrapCopies([]byte("first"), []uint8{1, 2})
	if err != nil { t.Fatal(err) }
	if _, err := lanes[0].Write(first[0].Wire); err != nil { t.Fatal(err) }
	if _, err := lanes[1].Write(first[1].Wire); err != nil { t.Fatal(err) }
	readPayload := func(c *net.UDPConn) []byte {
		buf := make([]byte, 65535)
		n, err := c.Read(buf)
		if err != nil { t.Fatal(err) }
		_, payload, err := gamelane.Parse(buf[:n])
		if err != nil { t.Fatal(err) }
		return payload
	}
	if got := string(readPayload(lanes[0])); got != "first" { t.Fatalf("lane1 first=%q", got) }
	if got := string(readPayload(lanes[1])); got != "first" { t.Fatalf("lane2 first=%q", got) }

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
	_ = lanes[0].SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_ = lanes[1].SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	_, second, err := enc.WrapCopies([]byte("still-healthy"), []uint8{1, 2})
	if err != nil { t.Fatal(err) }
	if _, err := lanes[0].Write(second[0].Wire); err != nil { t.Fatal(err) }
	if _, err := lanes[1].Write(second[1].Wire); err != nil { t.Fatal(err) }
	if got := string(readPayload(lanes[0])); got != "still-healthy" { t.Fatalf("lane1 after reject=%q", got) }
	if got := string(readPayload(lanes[1])); got != "still-healthy" { t.Fatalf("lane2 after reject=%q", got) }

	s.mu.Lock()
	if len(s.sessions) != 1 { t.Fatalf("sessions=%d", len(s.sessions)) }
	var gs *gameSession
	for _, x := range s.sessions { gs = x }
	s.mu.Unlock()
	gs.mu.Lock()
	bound := len(gs.lanes)
	inFirst, inDup := gs.inFirst, gs.inDup
	gs.mu.Unlock()
	if bound != 2 { t.Fatalf("bound lanes=%d, want 2", bound) }
	if inFirst != 2 || inDup != 2 { t.Fatalf("in_first=%d in_dup=%d, want 2/2", inFirst, inDup) }

	_ = conn.Close()
	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("server Run did not stop after UDP close")
	}
}
