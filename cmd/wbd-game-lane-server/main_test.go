package main

import (
	"encoding/hex"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
)

func TestMaxLanesRejectsThirdRealUDPPeerAndKeepsExistingLanesHealthy(t *testing.T) {
	echo, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil { t.Fatal(err) }
	defer echo.Close()
	metaSeen := make(chan rawipbackend.TunnelMeta, 1)
	go func() {
		buf := make([]byte, 65535)
		for {
			n, peer, err := echo.ReadFromUDP(buf)
			if err != nil { return }
			if meta, ok := rawipbackend.UnmarshalTunnelMeta(buf[:n]); ok {
				select { case metaSeen <- meta: default: }
				continue
			}
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
		peerMeta: make(map[string]rawipbackend.TunnelMeta),
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
	for i := range sid { sid[i] = byte(i + 1) }
	tunnelID := logicaltunnel.TunnelID(hex.EncodeToString(sid[:]))
	lease := netip.MustParseAddr("10.66.0.1")
	metaWire, err := rawipbackend.MarshalTunnelMeta(tunnelID, lease)
	if err != nil { t.Fatal(err) }
	for _, lane := range lanes {
		if _, err := lane.Write(metaWire); err != nil { t.Fatal(err) }
	}
	enc, err := gamelane.NewEncoder(sid, 1)
	if err != nil { t.Fatal(err) }

	_, first, err := enc.WrapCopies([]byte("bind"), []uint8{1, 2})
	if err != nil { t.Fatal(err) }
	if _, err := lanes[0].Write(first[0].Wire); err != nil { t.Fatal(err) }
	if _, err := lanes[1].Write(first[1].Wire); err != nil { t.Fatal(err) }

	select {
	case got := <-metaSeen:
		if got.TunnelID != tunnelID || got.Address4 != lease { t.Fatalf("downstream meta=%+v", got) }
	case <-time.After(time.Second):
		t.Fatal("Game server did not forward authenticated tunnel metadata downstream")
	}

	deadline := time.Now().Add(time.Second)
	for {
		s.mu.Lock(); gs := s.sessions[sid]; s.mu.Unlock()
		bound := 0
		if gs != nil { gs.mu.Lock(); bound = len(gs.lanes); gs.mu.Unlock() }
		if bound == 2 { break }
		if time.Now().After(deadline) { t.Fatalf("only %d lanes bound before timeout", bound) }
		time.Sleep(time.Millisecond)
	}

	for _, c := range lanes[:2] {
		_ = c.SetReadDeadline(time.Now().Add(20 * time.Millisecond))
		buf := make([]byte, 65535)
		for { if _, err := c.Read(buf); err != nil { break } }
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

	_, both, err := enc.WrapCopies([]byte("both-ready"), []uint8{1, 2})
	if err != nil { t.Fatal(err) }
	if _, err := lanes[0].Write(both[0].Wire); err != nil { t.Fatal(err) }
	if _, err := lanes[1].Write(both[1].Wire); err != nil { t.Fatal(err) }
	if got := string(readPayload(lanes[0])); got != "both-ready" { t.Fatalf("lane1 ready=%q", got) }
	if got := string(readPayload(lanes[1])); got != "both-ready" { t.Fatalf("lane2 ready=%q", got) }

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

	_, healthy, err := enc.WrapCopies([]byte("still-healthy"), []uint8{1, 2})
	if err != nil { t.Fatal(err) }
	if _, err := lanes[0].Write(healthy[0].Wire); err != nil { t.Fatal(err) }
	if _, err := lanes[1].Write(healthy[1].Wire); err != nil { t.Fatal(err) }
	if got := string(readPayload(lanes[0])); got != "still-healthy" { t.Fatalf("lane1 after reject=%q", got) }
	if got := string(readPayload(lanes[1])); got != "still-healthy" { t.Fatalf("lane2 after reject=%q", got) }

	deadline = time.Now().Add(500 * time.Millisecond)
	for {
		s.mu.Lock()
		if len(s.sessions) != 1 { n := len(s.sessions); s.mu.Unlock(); t.Fatalf("sessions=%d", n) }
		gs := s.sessions[sid]
		s.mu.Unlock()
		if gs == nil { t.Fatal("logical session disappeared") }
		gs.mu.Lock(); bound := len(gs.lanes); inFirst, inDup := gs.inFirst, gs.inDup; gs.mu.Unlock()
		if bound != 2 { t.Fatalf("bound lanes=%d, want 2", bound) }
		if inFirst == 3 && inDup == 3 { break }
		if time.Now().After(deadline) { t.Fatalf("in_first=%d in_dup=%d, want 3/3", inFirst, inDup) }
		time.Sleep(time.Millisecond)
	}

	_ = conn.Close()
	select { case <-errCh: case <-time.After(time.Second): t.Fatal("server Run did not stop after UDP close") }
}

func TestGameRejectsPayloadWithoutAuthenticatedTunnelMetadata(t *testing.T) {
	s := &server{sessions: map[gamelane.SessionID]*gameSession{}, peerSession: map[string]gamelane.SessionID{}, peerMeta: map[string]rawipbackend.TunnelMeta{}}
	var sid gamelane.SessionID
	sid[0] = 1
	enc, _ := gamelane.NewEncoder(sid, 1)
	_, copies, _ := enc.WrapCopies([]byte("x"), []uint8{1})
	err := s.handle(&net.UDPAddr{IP: net.IPv4(127,0,0,1), Port: 12345}, copies[0].Wire, time.Now())
	if err == nil { t.Fatal("unauthenticated Game lane payload accepted") }
}
