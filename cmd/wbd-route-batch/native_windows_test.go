//go:build windows

package main

import (
	"net/netip"
	"testing"
)

func TestMakeIPv4RouteRow(t *testing.T) {
	row, err := makeIPv4RouteRow(netip.MustParsePrefix("203.0.113.0/24"), 29, netip.MustParseAddr("192.0.2.1"), 7)
	if err != nil {
		t.Fatal(err)
	}
	if row.ForwardDest != 0x007100cb || row.ForwardMask != 0x00ffffff || row.ForwardNextHop != 0x010200c0 {
		t.Fatalf("route network fields dest=%#x mask=%#x next=%#x", row.ForwardDest, row.ForwardMask, row.ForwardNextHop)
	}
	if row.ForwardIfIndex != 29 || row.ForwardType != mibIPRouteTypeIndirect || row.ForwardProto != mibIPProtoNetMgmt || row.ForwardMetric1 != 7 {
		t.Fatalf("route metadata=%+v", row)
	}
}

func TestMakeIPv4OnLinkRouteRow(t *testing.T) {
	row, err := makeIPv4RouteRow(netip.MustParsePrefix("10.0.0.0/8"), 31, netip.IPv4Unspecified(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if row.ForwardType != mibIPRouteTypeDirect || row.ForwardNextHop != 0 {
		t.Fatalf("on-link route=%+v", row)
	}
}
