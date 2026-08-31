//go:build linux

package main

import (
	"encoding/binary"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
)

func mustTunnelID(t *testing.T, s string) logicaltunnel.TunnelID {
	t.Helper()
	id, err := logicaltunnel.ParseTunnelID(s)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func testGateway() *sharedGateway {
	return &sharedGateway{
		cfg: config{maxSessions: 4},
		leases: netip.MustParsePrefix("10.66.0.0/16"),
		byPeer: make(map[string]*sharedSession),
		byLease: make(map[netip.Addr]*sharedSession),
		byTunnel: make(map[logicaltunnel.TunnelID]*sharedSession),
	}
}

func TestRegisterDistinctLeasesDemuxIndependently(t *testing.T) {
	g := testGateway()
	now := time.Unix(1, 0)
	peerA := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41001}
	peerB := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41002}
	metaA := rawipbackend.TunnelMeta{TunnelID: mustTunnelID(t, "00112233445566778899aabbccddeeff"), Address4: netip.MustParseAddr("10.66.0.1")}
	metaB := rawipbackend.TunnelMeta{TunnelID: mustTunnelID(t, "ffeeddccbbaa99887766554433221100"), Address4: netip.MustParseAddr("10.66.0.2")}
	if err := g.register(peerA.String(), peerA, metaA, now); err != nil {
		t.Fatal(err)
	}
	if err := g.register(peerB.String(), peerB, metaB, now); err != nil {
		t.Fatal(err)
	}
	if got := g.byLease[metaA.Address4]; got == nil || got.peer.Port != peerA.Port {
		t.Fatalf("lease A routed to wrong peer: %#v", got)
	}
	if got := g.byLease[metaB.Address4]; got == nil || got.peer.Port != peerB.Port {
		t.Fatalf("lease B routed to wrong peer: %#v", got)
	}
	if len(g.byPeer) != 2 || len(g.byLease) != 2 || len(g.byTunnel) != 2 {
		t.Fatalf("unexpected registry sizes peers=%d leases=%d tunnels=%d", len(g.byPeer), len(g.byLease), len(g.byTunnel))
	}
}

func TestRegisterRejectsDuplicateLeaseOrTunnel(t *testing.T) {
	g := testGateway()
	now := time.Unix(1, 0)
	peerA := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41001}
	peerB := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41002}
	idA := mustTunnelID(t, "00112233445566778899aabbccddeeff")
	idB := mustTunnelID(t, "ffeeddccbbaa99887766554433221100")
	leaseA := netip.MustParseAddr("10.66.0.1")
	if err := g.register(peerA.String(), peerA, rawipbackend.TunnelMeta{TunnelID: idA, Address4: leaseA}, now); err != nil {
		t.Fatal(err)
	}
	if err := g.register(peerB.String(), peerB, rawipbackend.TunnelMeta{TunnelID: idB, Address4: leaseA}, now); err == nil {
		t.Fatal("duplicate lease unexpectedly accepted")
	}
	if err := g.register(peerB.String(), peerB, rawipbackend.TunnelMeta{TunnelID: idA, Address4: netip.MustParseAddr("10.66.0.2")}, now); err == nil {
		t.Fatal("duplicate tunnel id unexpectedly accepted")
	}
}

func TestRegisterRejectsLeaseOutsidePool(t *testing.T) {
	g := testGateway()
	peer := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41001}
	meta := rawipbackend.TunnelMeta{TunnelID: mustTunnelID(t, "00112233445566778899aabbccddeeff"), Address4: netip.MustParseAddr("10.67.0.1")}
	if err := g.register(peer.String(), peer, meta, time.Now()); err == nil {
		t.Fatal("out-of-pool lease unexpectedly accepted")
	}
}

func TestIPv4SourceDest(t *testing.T) {
	p := make([]byte, 28)
	p[0] = 0x45
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)))
	copy(p[12:16], net.IPv4(10, 66, 0, 2).To4())
	copy(p[16:20], net.IPv4(1, 1, 1, 1).To4())
	src, dst, err := ipv4SourceDest(p)
	if err != nil {
		t.Fatal(err)
	}
	if src.String() != "10.66.0.2" || dst.String() != "1.1.1.1" {
		t.Fatalf("unexpected addresses src=%s dst=%s", src, dst)
	}
	p[0] = 0x60
	if _, _, err := ipv4SourceDest(p); err == nil {
		t.Fatal("IPv6 packet unexpectedly accepted")
	}
}

func TestDropSessionKeepsOtherLease(t *testing.T) {
	g := testGateway()
	now := time.Unix(1, 0)
	peerA := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41001}
	peerB := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41002}
	metaA := rawipbackend.TunnelMeta{TunnelID: mustTunnelID(t, "00112233445566778899aabbccddeeff"), Address4: netip.MustParseAddr("10.66.0.1")}
	metaB := rawipbackend.TunnelMeta{TunnelID: mustTunnelID(t, "ffeeddccbbaa99887766554433221100"), Address4: netip.MustParseAddr("10.66.0.2")}
	_ = g.register(peerA.String(), peerA, metaA, now)
	_ = g.register(peerB.String(), peerB, metaB, now)
	g.dropSession(peerA.String(), "test")
	if g.byLease[metaA.Address4] != nil || g.byTunnel[metaA.TunnelID] != nil {
		t.Fatal("dropped session still registered")
	}
	if got := g.byLease[metaB.Address4]; got == nil || got.peer.Port != peerB.Port {
		t.Fatal("dropping A damaged B")
	}
}
