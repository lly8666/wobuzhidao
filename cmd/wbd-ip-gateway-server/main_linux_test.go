//go:build linux

package main

import (
	"net/netip"
	"testing"
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
