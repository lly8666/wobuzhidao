//go:build linux

package main

import (
	"net/netip"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
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
