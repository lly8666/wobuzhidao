//go:build windows

package main

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"syscall"
	"unsafe"
)

const (
	mibIPRouteTypeDirect   = 3
	mibIPRouteTypeIndirect = 4
	mibIPProtoNetMgmt      = 3
	mibMetricUnused        = ^uint32(0)
	errorNotFound          = 1168
)

type mibIPForwardRow struct {
	ForwardDest      uint32
	ForwardMask      uint32
	ForwardPolicy    uint32
	ForwardNextHop   uint32
	ForwardIfIndex   uint32
	ForwardType      uint32
	ForwardProto     uint32
	ForwardAge       uint32
	ForwardNextHopAS uint32
	ForwardMetric1   uint32
	ForwardMetric2   uint32
	ForwardMetric3   uint32
	ForwardMetric4   uint32
	ForwardMetric5   uint32
}

var (
	iphlpapiDLL             = syscall.NewLazyDLL("iphlpapi.dll")
	createIPForwardEntryProc = iphlpapiDLL.NewProc("CreateIpForwardEntry")
	deleteIPForwardEntryProc = iphlpapiDLL.NewProc("DeleteIpForwardEntry")
)

func mutateIPv4Route(action string, prefix netip.Prefix, ifIndex uint32, nextHop netip.Addr, metric uint32) error {
	row, err := makeIPv4RouteRow(prefix, ifIndex, nextHop, metric)
	if err != nil {
		return err
	}
	var proc *syscall.LazyProc
	switch action {
	case "add":
		proc = createIPForwardEntryProc
	case "delete":
		proc = deleteIPForwardEntryProc
	default:
		return fmt.Errorf("unsupported route action %q", action)
	}
	r1, _, _ := proc.Call(uintptr(unsafe.Pointer(&row)))
	code := uint32(r1)
	if code == 0 || (action == "delete" && code == errorNotFound) {
		return nil
	}
	return syscall.Errno(code)
}

func makeIPv4RouteRow(prefix netip.Prefix, ifIndex uint32, nextHop netip.Addr, metric uint32) (mibIPForwardRow, error) {
	if !prefix.IsValid() || !prefix.Addr().Is4() || prefix.Bits() < 0 || prefix.Bits() > 32 {
		return mibIPForwardRow{}, fmt.Errorf("invalid IPv4 prefix %q", prefix)
	}
	if ifIndex == 0 {
		return mibIPForwardRow{}, fmt.Errorf("interface index is required")
	}
	if !nextHop.Is4() {
		return mibIPForwardRow{}, fmt.Errorf("next hop must be IPv4")
	}
	prefix = prefix.Masked()
	dest4 := prefix.Addr().As4()
	next4 := nextHop.As4()
	var mask4 [4]byte
	bits := prefix.Bits()
	for i := 0; i < bits; i++ {
		mask4[i/8] |= 1 << uint(7-(i%8))
	}
	routeType := uint32(mibIPRouteTypeIndirect)
	if nextHop == netip.IPv4Unspecified() {
		routeType = mibIPRouteTypeDirect
	}
	return mibIPForwardRow{
		ForwardDest:      binary.LittleEndian.Uint32(dest4[:]),
		ForwardMask:      binary.LittleEndian.Uint32(mask4[:]),
		ForwardNextHop:   binary.LittleEndian.Uint32(next4[:]),
		ForwardIfIndex:   ifIndex,
		ForwardType:      routeType,
		ForwardProto:     mibIPProtoNetMgmt,
		ForwardMetric1:   metric,
		ForwardMetric2:   mibMetricUnused,
		ForwardMetric3:   mibMetricUnused,
		ForwardMetric4:   mibMetricUnused,
		ForwardMetric5:   mibMetricUnused,
	}, nil
}
