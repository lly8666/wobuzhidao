package main

import (
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestReplacementCandidateDoesNotRacePayloadBeforeMembershipReady(t *testing.T) {
	oldTarget := loopbackUDP(t)
	candidateTarget := loopbackUDP(t)
	app := loopbackUDP(t)

	var sid gamelane.SessionID
	sid[0] = 0x5a
	enc, err := gamelane.NewEncoder(sid, 1)
	if err != nil { t.Fatal(err) }
	pacer, err := gamelane.NewInnerPacer(0)
	if err != nil { t.Fatal(err) }
	c := &client{app: app, enc: enc, pacer: pacer, lanes: make(map[uint8]*laneConn, gamelane.MaxLanes), overlap: make(map[uint8]*laneConn, 1)}
	t.Cleanup(c.closeAllLanes)

	oldLane := gamelane.LaneTarget{ID: 1, Address: oldTarget.LocalAddr().String()}
	candidateLane := gamelane.LaneTarget{ID: 1, Address: candidateTarget.LocalAddr().String()}
	if _, err := c.setLaneTargets([]gamelane.LaneTarget{oldLane}); err != nil { t.Fatal(err) }

	appErr := make(chan error, 1)
	go func() { appErr <- c.appLoop() }()

	candidatePeer := make(chan *net.UDPAddr, 1)
	probeErr := make(chan error, 1)
	go func() {
		_ = candidateTarget.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 256)
		n, peer, err := candidateTarget.ReadFromUDP(buf)
		if err != nil { probeErr <- err; return }
		control, err := gamelane.ParseMembershipControl(buf[:n])
		if err != nil { probeErr <- err; return }
		if control.SessionID != sid || control.LaneID != 1 || control.Op != gamelane.MembershipProbe {
			probeErr <- gamelane.ErrMalformed
			return
		}
		candidatePeer <- peer
		probeErr <- nil
	}()

	setErr := make(chan error, 1)
	go func() {
		_, err := c.setLaneTargets([]gamelane.LaneTarget{oldLane, candidateLane})
		setErr <- err
	}()

	var peer *net.UDPAddr
	select {
	case peer = <-candidatePeer:
	case <-time.After(time.Second):
		t.Fatal("candidate membership Probe was not sent")
	}
	if err := <-probeErr; err != nil { t.Fatal(err) }

	platformPeer, err := net.DialUDP("udp4", nil, app.LocalAddr().(*net.UDPAddr))
	if err != nil { t.Fatal(err) }
	defer platformPeer.Close()
	if _, err := platformPeer.Write([]byte("before-ready")); err != nil { t.Fatal(err) }

	readPayload := func(conn *net.UDPConn, timeout time.Duration) string {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(timeout))
		buf := make([]byte, 65535)
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil { t.Fatal(err) }
		_, payload, err := gamelane.Parse(buf[:n])
		if err != nil { t.Fatal(err) }
		return string(payload)
	}
	if got := readPayload(oldTarget, time.Second); got != "before-ready" {
		t.Fatalf("old lane payload=%q", got)
	}

	_ = candidateTarget.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	buf := make([]byte, 65535)
	if n, _, err := candidateTarget.ReadFromUDP(buf); err == nil {
		t.Fatalf("unqualified replacement received %d bytes of payload before MembershipReady", n)
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("candidate pre-ready read: %v", err)
	}

	ready, err := gamelane.MarshalLaneReady(sid, 1)
	if err != nil { t.Fatal(err) }
	if _, err := candidateTarget.WriteToUDP(ready, peer); err != nil { t.Fatal(err) }
	select {
	case err := <-setErr:
		if err != nil { t.Fatal(err) }
	case <-time.After(time.Second):
		t.Fatal("replacement target update did not complete after MembershipReady")
	}

	if _, err := platformPeer.Write([]byte("after-ready")); err != nil { t.Fatal(err) }
	if got := readPayload(candidateTarget, time.Second); got != "after-ready" {
		t.Fatalf("qualified replacement payload=%q", got)
	}

	_ = app.Close()
	select {
	case <-appErr:
	case <-time.After(time.Second):
		t.Fatal("appLoop did not stop")
	}
}
