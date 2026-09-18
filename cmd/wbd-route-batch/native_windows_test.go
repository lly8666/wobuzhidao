//go:build windows

package main

import (
	"net/netip"
	"testing"
	"unsafe"
)

func TestMibIpForwardRow2ABILayout(t *testing.T) {
	if got := unsafe.Sizeof(mibIPForwardRow2{}); got != 104 {
		t.Fatalf("MIB_IPFORWARD_ROW2 ABI size=%d want=104", got)
	}
}

func TestMakeIPv4RouteRow(t *testing.T) {
	row, err := makeIPv4RouteRow(netip.MustParsePrefix("203.0.113.0/24"), 29, netip.MustParseAddr("192.0.2.1"), 7)
	if err != nil {
		t.Fatal(err)
	}
	if row.DestinationPrefix.Prefix.Data[0] != 0x007100cb || row.DestinationPrefix.PrefixLength != 24 || row.NextHop.Data[0] != 0x010200c0 {
		t.Fatalf("route network fields dest=%#x/%d next=%#x", row.DestinationPrefix.Prefix.Data[0], row.DestinationPrefix.PrefixLength, row.NextHop.Data[0])
	}
	if row.InterfaceIndex != 29 || row.DestinationPrefix.Prefix.Family != addressFamilyInet || row.NextHop.Family != addressFamilyInet || row.Protocol != mibIPProtoNetMgmt || row.Metric != 7 {
		t.Fatalf("route metadata=%+v", row)
	}
}

func TestMakeIPv4OnLinkRouteRow(t *testing.T) {
	row, err := makeIPv4RouteRow(netip.MustParsePrefix("10.0.0.0/8"), 31, netip.IPv4Unspecified(), 5)
	if err != nil {
		t.Fatal(err)
	}
	if row.NextHop.Family != addressFamilyInet || row.NextHop.Data[0] != 0 || row.Metric != 5 {
		t.Fatalf("on-link route=%+v", row)
	}
}
