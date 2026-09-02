package main

import (
	"bytes"
	"errors"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func loopbackUDP(t *testing.T) *net.UDPConn {
	t.Helper()
	c, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127,0,0,1), Port:0})
	if err != nil { t.Fatal(err) }
	t.Cleanup(func(){ _ = c.Close() })
	return c
}

func TestDynamicLaneMembershipDormantWake(t *testing.T) {
	a := loopbackUDP(t)
	b := loopbackUDP(t)
	c := &client{lanes: make(map[uint8]*laneConn, gamelane.MaxLanes)}
	t.Cleanup(c.closeAllLanes)

	lane1 := gamelane.LaneTarget{ID:1, Address:a.LocalAddr().String()}
	lane2 := gamelane.LaneTarget{ID:2, Address:b.LocalAddr().String()}
	if got, err := c.setLaneTargets([]gamelane.LaneTarget{lane1}); err != nil || !reflect.DeepEqual(got, []uint8{1}) {
		t.Fatalf("lane1 got=%v err=%v", got, err)
	}
	if got, err := c.setLaneTargets([]gamelane.LaneTarget{lane1,lane2}); err != nil || !reflect.DeepEqual(got, []uint8{1,2}) {
		t.Fatalf("race got=%v err=%v", got, err)
	}
	if got, err := c.setLaneTargets(nil); err != nil || len(got) != 0 {
		t.Fatalf("dormant got=%v err=%v", got, err)
	}
	if got, err := c.setLaneTargets([]gamelane.LaneTarget{lane2}); err != nil || !reflect.DeepEqual(got, []uint8{2}) {
		t.Fatalf("wake got=%v err=%v", got, err)
	}
}

func TestLaneFailureDoesNotKillOtherMembership(t *testing.T) {
	a := loopbackUDP(t)
	b := loopbackUDP(t)
	c := &client{lanes: make(map[uint8]*laneConn, gamelane.MaxLanes)}
	t.Cleanup(c.closeAllLanes)
	_, err := c.setLaneTargets([]gamelane.LaneTarget{{ID:1,Address:a.LocalAddr().String()},{ID:2,Address:b.LocalAddr().String()}})
	if err != nil { t.Fatal(err) }
	lanes := c.activeLanes()
	if len(lanes) != 2 { t.Fatalf("lanes=%d", len(lanes)) }
	c.failLane(lanes[0], errors.New("synthetic lane failure"))
	if got := c.activeIDs(); !reflect.DeepEqual(got, []uint8{2}) {
		t.Fatalf("surviving lanes=%v", got)
	}
}

func TestReplacementOverlapFansOutIdenticalLogicalFrame(t *testing.T) {
	oldTarget := loopbackUDP(t)
	candidateTarget := loopbackUDP(t)
	app := loopbackUDP(t)
	var sid gamelane.SessionID
	sid[0] = 7
	enc, err := gamelane.NewEncoder(sid, 1)
	if err != nil { t.Fatal(err) }
	pacer, err := gamelane.NewInnerPacer(0)
	if err != nil { t.Fatal(err) }
	c := &client{app:app, enc:enc, pacer:pacer, lanes:make(map[uint8]*laneConn, gamelane.MaxLanes)}
	t.Cleanup(c.closeAllLanes)
	if got, err := c.setLaneTargets([]gamelane.LaneTarget{{ID:1,Address:oldTarget.LocalAddr().String()},{ID:1,Address:candidateTarget.LocalAddr().String()}}); err != nil || !reflect.DeepEqual(got, []uint8{1}) {
		t.Fatalf("overlap active=%v err=%v", got, err)
	}
	if groups := c.activeRaceGroups(); len(groups) != 1 || groups[0].primary == nil || groups[0].overlap == nil {
		t.Fatalf("overlap groups=%+v", groups)
	}

	errCh := make(chan error, 1)
	go func(){ errCh <- c.appLoop() }()
	peer, err := net.DialUDP("udp4", nil, app.LocalAddr().(*net.UDPAddr))
	if err != nil { t.Fatal(err) }
	defer peer.Close()
	if _, err := peer.Write([]byte("same-packet-id")); err != nil { t.Fatal(err) }

	readWire := func(conn *net.UDPConn) []byte {
		t.Helper()
		_ = conn.SetReadDeadline(time.Now().Add(time.Second))
		buf := make([]byte, 65535)
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil { t.Fatal(err) }
		return append([]byte(nil), buf[:n]...)
	}
	oldWire := readWire(oldTarget)
	candidateWire := readWire(candidateTarget)
	if !bytes.Equal(oldWire, candidateWire) {
		t.Fatalf("replacement targets received different Game frames\nold=%x\ncandidate=%x", oldWire, candidateWire)
	}
	h, payload, err := gamelane.Parse(oldWire)
	if err != nil { t.Fatal(err) }
	if h.LaneID != 1 || string(payload) != "same-packet-id" {
		t.Fatalf("header=%+v payload=%q", h, payload)
	}
	_ = app.Close()
	select {
	case <-errCh:
	case <-time.After(time.Second):
		t.Fatal("appLoop did not stop")
	}
}

func TestPrimaryFailurePromotesReplacementIncarnation(t *testing.T) {
	oldTarget := loopbackUDP(t)
	candidateTarget := loopbackUDP(t)
	c := &client{lanes:make(map[uint8]*laneConn, gamelane.MaxLanes)}
	t.Cleanup(c.closeAllLanes)
	if _, err := c.setLaneTargets([]gamelane.LaneTarget{{ID:1,Address:oldTarget.LocalAddr().String()},{ID:1,Address:candidateTarget.LocalAddr().String()}}); err != nil { t.Fatal(err) }
	group := c.activeRaceGroups()[0]
	old := group.primary
	candidate := group.overlap
	c.failLane(old, errors.New("synthetic primary failure"))
	groups := c.activeRaceGroups()
	if len(groups) != 1 || groups[0].primary != candidate || groups[0].overlap != nil {
		t.Fatalf("replacement was not promoted after primary failure: %+v", groups)
	}
}
