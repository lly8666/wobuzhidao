package runtimeentry

import (
	"testing"
	"time"
)

func TestQualifiedEgressMaskIsNotFullLifecycleBarrier(t *testing.T) {
	s := &LifecycleServer{}
	g := &serverLifecycleTunnel{desired: 4, lanes: map[uint8]*serverLifecycleLane{
		1: {qualified: true}, 2: {qualified: false}, 3: {qualified: true, retiring: true},
	}}
	if mask := s.egressLanesLocked(g); mask != 1 || s.groupReadyLocked(g) {
		t.Fatalf("mask=%x full=%v", mask, s.groupReadyLocked(g))
	}
	g.dormant = true
	if s.egressLanesLocked(g) != 0 || s.egressLanesLocked(nil) != 0 {
		t.Fatal("dormant/missing tunnel became eligible")
	}
}

func TestLifecycleReadySiblingReturnsBeforeMissingLanes(t *testing.T) {
	h := newLifecycleAuditHarness(t, 4, 0, 0)
	h.sendForward(t, [4]byte{8, 8, 8, 8})
	// Model the server publication boundary during sequential wake. The owner
	// deliberately still has all four lanes: excluded peers must not even seal
	// a business copy while only lane1 is qualified for this egress snapshot.
	h.server.mu.Lock()
	g := h.server.byTunnel[h.tunnelID]
	saved := g.lanes
	g.lanes = map[uint8]*serverLifecycleLane{1: saved[1]}
	h.server.mu.Unlock()
	defer func() { h.server.mu.Lock(); g.lanes = saved; h.server.mu.Unlock() }()
	before, _ := h.server.TunnelStats(h.tunnelID)
	packet := ipv4Packet([4]byte{1, 1, 1, 1}, [4]byte{10, 66, 0, 31}, 17)
	if err := h.server.RoutePacket(packet, time.Now()); err != nil {
		t.Fatalf("ready sibling blocked by absent peers: %v", err)
	}
	select {
	case got := <-h.clientDeliver:
		if string(got) != string(packet) {
			t.Fatal("reply payload changed")
		}
	case <-time.After(time.Second):
		t.Fatal("ready sibling did not deliver")
	}
	after, _ := h.server.TunnelStats(h.tunnelID)
	if after.GameLogicalOutbound != before.GameLogicalOutbound+1 || after.GameLaneCopies != before.GameLaneCopies+1 {
		t.Fatalf("unqualified siblings encoded copies: before=%+v after=%+v", before, after)
	}
	if h.server.TunnelQualified(h.tunnelID) {
		t.Fatal("partial readiness falsely declared full qualification")
	}
}
