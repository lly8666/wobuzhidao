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

func TestSameLogicalLaneAllowsOneReplacementPeerAndPromotesOnLeave(t *testing.T) {
	var sid gamelane.SessionID
	for i := range sid { sid[i] = byte(i + 1) }
	meta := rawipbackend.TunnelMeta{TunnelID:logicaltunnel.TunnelID(hex.EncodeToString(sid[:])), Address4:netip.MustParseAddr("10.66.0.9")}
	gs := &gameSession{id:sid, meta:meta, lanes:map[uint8]*net.UDPAddr{}, overlap:map[uint8]*net.UDPAddr{}, peerLane:map[string]uint8{}, last:time.Now()}
	s := &server{maxLanes:1, sessions:map[gamelane.SessionID]*gameSession{sid:gs}, peerSession:map[string]gamelane.SessionID{}, peerMeta:map[string]rawipbackend.TunnelMeta{}}

	oldPeer := &net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:51001}
	candidatePeer := &net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:51002}
	thirdPeer := &net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:51003}
	otherLanePeer := &net.UDPAddr{IP:net.IPv4(127,0,0,1),Port:51004}
	for _, peer := range []*net.UDPAddr{oldPeer,candidatePeer,thirdPeer,otherLanePeer} { s.peerMeta[peer.String()] = meta }

	if _, err := s.bindLane(sid, 1, oldPeer, meta, time.Now()); err != nil { t.Fatal(err) }
	if _, err := s.bindLane(sid, 1, candidatePeer, meta, time.Now()); err != nil { t.Fatal(err) }
	if got := gs.lanes[1]; got == nil || got.String() != oldPeer.String() { t.Fatalf("primary=%v want=%s", got, oldPeer) }
	if got := gs.overlap[1]; got == nil || got.String() != candidatePeer.String() { t.Fatalf("candidate=%v want=%s", got, candidatePeer) }
	if len(gs.lanes) != 1 || len(gs.overlap) != 1 { t.Fatalf("logical=%d overlap=%d", len(gs.lanes), len(gs.overlap)) }

	if _, err := s.bindLane(sid, 1, thirdPeer, meta, time.Now()); err == nil { t.Fatal("third transport incarnation for one LaneID was accepted") }
	if _, err := s.bindLane(sid, 2, otherLanePeer, meta, time.Now()); err == nil { t.Fatal("second logical LaneID bypassed maxLanes=1") }

	if err := s.unbindLane(sid, 1, oldPeer, time.Now(), "test_cutover"); err != nil { t.Fatal(err) }
	if got := gs.lanes[1]; got == nil || got.String() != candidatePeer.String() { t.Fatalf("candidate was not promoted: %v", got) }
	if len(gs.overlap) != 0 { t.Fatalf("overlap remained after old leave: %v", gs.overlap) }
	if got := gs.peerLane[candidatePeer.String()]; got != 1 { t.Fatalf("candidate peer lane=%d", got) }
	if _, ok := s.peerSession[candidatePeer.String()]; !ok { t.Fatal("candidate session mapping was lost") }
	if _, ok := s.peerMeta[candidatePeer.String()]; !ok { t.Fatal("candidate metadata was lost") }
	if _, ok := s.peerSession[oldPeer.String()]; ok { t.Fatal("old peer session mapping survived leave") }
}
