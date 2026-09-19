//go:build linux

package main

import (
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
)

func TestCapacityForPrefix(t *testing.T) {
	if got := capacityForPrefix(netip.MustParsePrefix("198.18.240.0/24")); got != 64 {
		t.Fatalf("capacity=%d want 64", got)
	}
	if got := capacityForPrefix(netip.MustParsePrefix("198.18.240.0/30")); got != 1 {
		t.Fatalf("capacity=%d want 1", got)
	}
}

func TestTransitPair(t *testing.T) {
	prefix := netip.MustParsePrefix("198.18.240.0/24")
	cases := []struct {
		slot int
		host string
		edge string
	}{
		{0, "198.18.240.1", "198.18.240.2"},
		{1, "198.18.240.5", "198.18.240.6"},
		{63, "198.18.240.253", "198.18.240.254"},
	}
	for _, tc := range cases {
		host, edge, err := transitPair(prefix, tc.slot)
		if err != nil {
			t.Fatalf("slot %d: %v", tc.slot, err)
		}
		if host.String() != tc.host || edge.String() != tc.edge {
			t.Fatalf("slot %d got %s/%s want %s/%s", tc.slot, host, edge, tc.host, tc.edge)
		}
	}
	if _, _, err := transitPair(prefix, 64); err == nil {
		t.Fatal("slot 64 should be rejected")
	}
}

func TestLogicalTunnelSessionUsesLeasePrefix(t *testing.T) {
	id, err := logicaltunnel.ParseTunnelID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	s := &gatewaySession{
		logicalTunnel: true,
		tunnelID:      id,
		lease:         netip.MustParseAddr("10.66.42.17"),
	}
	fallback := netip.MustParsePrefix("10.66.0.0/30")
	if got := s.innerPrefix(fallback).String(); got != "10.66.42.17/32" {
		t.Fatalf("inner prefix=%s want lease /32", got)
	}
	if got := s.marker(); got != "tunnel_id_prefix=00112233" {
		t.Fatalf("marker=%q", got)
	}
}

func TestLegacySessionKeepsConfiguredInnerPrefix(t *testing.T) {
	s := &gatewaySession{sid: "abcdef"}
	fallback := netip.MustParsePrefix("10.66.0.0/30")
	if got := s.innerPrefix(fallback); got != fallback {
		t.Fatalf("inner prefix=%s want %s", got, fallback)
	}
	if got := s.marker(); got != "sid=abcdef" {
		t.Fatalf("marker=%q", got)
	}
}

func TestHandleFrameStoresTunnelMeta(t *testing.T) {
	id, err := logicaltunnel.ParseTunnelID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	lease := netip.MustParseAddr("10.66.42.17")
	wire, err := rawipbackend.MarshalTunnelMeta(id, lease)
	if err != nil {
		t.Fatal(err)
	}
	g := &gateway{
		sessions:      make(map[string]*gatewaySession),
		pendingSID:    make(map[string]string),
		pendingTunnel: make(map[string]rawipbackend.TunnelMeta),
	}
	peer := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41000}
	if err := g.handleFrame(peer, wire, time.Now()); err != nil {
		t.Fatal(err)
	}
	got, ok := g.pendingTunnel[peer.String()]
	if !ok {
		t.Fatal("TunnelMeta was not retained for first raw-IP datagram")
	}
	if got.TunnelID != id || got.Address4 != lease {
		t.Fatalf("metadata=%+v want tunnel=%s lease=%s", got, id, lease)
	}
}

func TestHandleFrameRejectsTunnelMetaChangeOnActivePeer(t *testing.T) {
	idA, _ := logicaltunnel.ParseTunnelID("00112233445566778899aabbccddeeff")
	idB, _ := logicaltunnel.ParseTunnelID("ffeeddccbbaa99887766554433221100")
	leaseA := netip.MustParseAddr("10.66.42.17")
	wire, err := rawipbackend.MarshalTunnelMeta(idB, netip.MustParseAddr("10.66.42.18"))
	if err != nil {
		t.Fatal(err)
	}
	peer := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 41000}
	g := &gateway{
		sessions: map[string]*gatewaySession{
			peer.String(): {logicalTunnel: true, tunnelID: idA, lease: leaseA},
		},
		pendingSID:    make(map[string]string),
		pendingTunnel: make(map[string]rawipbackend.TunnelMeta),
	}
	if err := g.handleFrame(peer, wire, time.Now()); err == nil {
		t.Fatal("active backend peer accepted changed Logical Tunnel metadata")
	}
}
