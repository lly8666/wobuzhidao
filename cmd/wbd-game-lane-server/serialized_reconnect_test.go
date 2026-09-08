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

func TestSerializedReconnectAdvancesLostLeaveAcrossLogicalLanes(t *testing.T) {
	var sid gamelane.SessionID
	for i := range sid {
		sid[i] = byte(0x40 + i)
	}
	meta := rawipbackend.TunnelMeta{
		TunnelID: logicaltunnel.TunnelID(hex.EncodeToString(sid[:])),
		Address4: netip.MustParseAddr("10.66.0.19"),
	}
	gs := &gameSession{
		id: sid, meta: meta,
		lanes: map[uint8]*net.UDPAddr{}, overlap: map[uint8]*net.UDPAddr{}, peerLane: map[string]uint8{},
		last: time.Now(),
	}
	s := &server{
		maxLanes: 4,
		sessions: map[gamelane.SessionID]*gameSession{sid: gs},
		peerSession: map[string]gamelane.SessionID{},
		peerMeta: map[string]rawipbackend.TunnelMeta{},
	}

	oldPeers := map[uint8]*net.UDPAddr{}
	newPeers := map[uint8]*net.UDPAddr{}
	for laneID := uint8(1); laneID <= 4; laneID++ {
		oldPeers[laneID] = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 52000 + int(laneID)}
		newPeers[laneID] = &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 52100 + int(laneID)}
		s.peerMeta[oldPeers[laneID].String()] = meta
		s.peerMeta[newPeers[laneID].String()] = meta
		if _, err := s.bindLane(sid, laneID, oldPeers[laneID], meta, time.Now()); err != nil {
			t.Fatalf("initial lane %d: %v", laneID, err)
		}
	}

	// Model all four authoritative client transports dying at once. Recovery is
	// serialized by the Windows controller: lane 1 replacement first, then lane
	// 2, and so on. Each retiring transport is already dead, so its CLIENT_LEAVE
	// can be lost. A stale overlap from the previous lane must not block the next
	// lane's authenticated replacement.
	for laneID := uint8(1); laneID <= 4; laneID++ {
		if _, err := s.bindLane(sid, laneID, newPeers[laneID], meta, time.Now()); err != nil {
			t.Fatalf("serialized reconnect lane %d: %v", laneID, err)
		}
		if got := gs.overlap[laneID]; got == nil || got.String() != newPeers[laneID].String() {
			t.Fatalf("lane %d overlap=%v want=%s", laneID, got, newPeers[laneID])
		}
		if laneID > 1 {
			prev := laneID - 1
			if got := gs.lanes[prev]; got == nil || got.String() != newPeers[prev].String() {
				t.Fatalf("lane %d stale overlap did not roll forward: primary=%v want=%s", prev, got, newPeers[prev])
			}
			if _, ok := gs.overlap[prev]; ok {
				t.Fatalf("lane %d overlap survived serialized roll-forward", prev)
			}
			if _, ok := gs.peerLane[oldPeers[prev].String()]; ok {
				t.Fatalf("lane %d stale primary peerLane survived", prev)
			}
			if _, ok := s.peerSession[oldPeers[prev].String()]; ok {
				t.Fatalf("lane %d stale primary peerSession survived", prev)
			}
			if _, ok := s.peerMeta[oldPeers[prev].String()]; ok {
				t.Fatalf("lane %d stale primary metadata survived", prev)
			}
		}
	}

	// The final lane still has A+B until either its best-effort LEAVE arrives or
	// a later same-LaneID incarnation proves promotion. Exercise that existing
	// same-lane recovery too so all-lane failure converges without a server-side
	// permanent replacement conflict.
	third := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 52204}
	s.peerMeta[third.String()] = meta
	if _, err := s.bindLane(sid, 4, third, meta, time.Now()); err != nil {
		t.Fatalf("final same-lane convergence: %v", err)
	}
	if got := gs.lanes[4]; got == nil || got.String() != newPeers[4].String() {
		t.Fatalf("lane 4 promoted primary=%v want=%s", got, newPeers[4])
	}
	if got := gs.overlap[4]; got == nil || got.String() != third.String() {
		t.Fatalf("lane 4 next overlap=%v want=%s", got, third)
	}
}
