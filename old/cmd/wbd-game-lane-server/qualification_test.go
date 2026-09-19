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

func TestMembershipProbeBindsLaneBeforeReady(t *testing.T) {
	downstream, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127,0,0,1)})
	if err != nil { t.Fatal(err) }
	defer downstream.Close()
	serverConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127,0,0,1)})
	if err != nil { t.Fatal(err) }
	clientConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127,0,0,1)})
	if err != nil { t.Fatal(err) }
	defer clientConn.Close()

	s := &server{conn: serverConn, serviceAddr: downstream.LocalAddr().(*net.UDPAddr), replayWindow:4096, maxSessions:4, maxLanes:4, idle:time.Minute, sessions:map[gamelane.SessionID]*gameSession{}, peerSession:map[string]gamelane.SessionID{}, peerMeta:map[string]rawipbackend.TunnelMeta{}}
	defer s.Close()
	var sid gamelane.SessionID
	for i := range sid { sid[i] = byte(i+1) }
	meta := rawipbackend.TunnelMeta{TunnelID:logicaltunnel.TunnelID(hex.EncodeToString(sid[:])), Address4:netip.MustParseAddr("10.66.0.1")}
	peer := clientConn.LocalAddr().(*net.UDPAddr)
	if err := s.registerPeerMeta(peer, meta, time.Now()); err != nil { t.Fatal(err) }
	wire, err := gamelane.MarshalLaneProbe(sid, 3)
	if err != nil { t.Fatal(err) }
	probe, err := gamelane.ParseMembershipControl(wire)
	if err != nil { t.Fatal(err) }
	if err := s.handleMembership(peer, probe, time.Now()); err != nil { t.Fatal(err) }

	s.mu.Lock(); gs := s.sessions[sid]; s.mu.Unlock()
	if gs == nil { t.Fatal("probe acknowledged before Game session existed") }
	gs.mu.Lock(); bound := gs.lanes[3]; gs.mu.Unlock()
	if bound == nil || bound.String() != peer.String() { t.Fatalf("candidate not bound before ready: %v", bound) }

	_ = clientConn.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 256)
	n, _, err := clientConn.ReadFromUDP(buf)
	if err != nil { t.Fatal(err) }
	ready, err := gamelane.ParseMembershipControl(buf[:n])
	if err != nil { t.Fatal(err) }
	if ready.SessionID != sid || ready.LaneID != 3 || ready.Op != gamelane.MembershipReady { t.Fatalf("ready=%+v", ready) }
}
