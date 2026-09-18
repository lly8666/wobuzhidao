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
	addressFamilyInet     = 2
	mibIPProtoNetMgmt     = 3
	routeLifetimeInfinite = ^uint32(0)
	errorNotFound         = 1168
)

type rawSockaddrInet struct {
	Family uint16
	Port   uint16
	Data   [6]uint32
}

type ipAddressPrefix struct {
	Prefix       rawSockaddrInet
	PrefixLength uint8
}

type mibIPForwardRow2 struct {
	InterfaceLuid        uint64
	InterfaceIndex       uint32
	DestinationPrefix    ipAddressPrefix
	NextHop              rawSockaddrInet
	SitePrefixLength     uint8
	ValidLifetime        uint32
	PreferredLifetime    uint32
	Metric               uint32
	Protocol             uint32
	Loopback             uint8
	AutoconfigureAddress uint8
	Publish              uint8
	Immortal             uint8
	Age                  uint32
	Origin               uint32
}

var (
	iphlpapiDLL               = syscall.NewLazyDLL("iphlpapi.dll")
	initializeIPForwardEntry   = iphlpapiDLL.NewProc("InitializeIpForwardEntry")
	createIPForwardEntry2Proc  = iphlpapiDLL.NewProc("CreateIpForwardEntry2")
	deleteIPForwardEntry2Proc  = iphlpapiDLL.NewProc("DeleteIpForwardEntry2")
)

func mutateIPv4Route(action string, prefix netip.Prefix, ifIndex uint32, nextHop netip.Addr, metric uint32) error {
	row, err := makeIPv4RouteRow(prefix, ifIndex, nextHop, metric)
	if err != nil {
		return err
	}
	var proc *syscall.LazyProc
	switch action {
	case "add":
		proc = createIPForwardEntry2Proc
	case "delete":
		proc = deleteIPForwardEntry2Proc
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

func makeIPv4RouteRow(prefix netip.Prefix, ifIndex uint32, nextHop netip.Addr, metric uint32) (mibIPForwardRow2, error) {
	if !prefix.IsValid() || !prefix.Addr().Is4() || prefix.Bits() < 0 || prefix.Bits() > 32 {
		return mibIPForwardRow2{}, fmt.Errorf("invalid IPv4 prefix %q", prefix)
	}
	if ifIndex == 0 {
		return mibIPForwardRow2{}, fmt.Errorf("interface index is required")
	}
	if !nextHop.Is4() {
		return mibIPForwardRow2{}, fmt.Errorf("next hop must be IPv4")
	}

	var row mibIPForwardRow2
	// Microsoft requires InitializeIpForwardEntry before CreateIpForwardEntry2.
	// It seeds infinite lifetimes and ABI defaults; explicitly overwrite every
	// policy field we own below so route semantics stay deterministic.
	initializeIPForwardEntry.Call(uintptr(unsafe.Pointer(&row)))

	prefix = prefix.Masked()
	dest4 := prefix.Addr().As4()
	next4 := nextHop.As4()
	row.InterfaceLuid = 0
	row.InterfaceIndex = ifIndex
	row.DestinationPrefix.Prefix = ipv4Sockaddr(dest4)
	row.DestinationPrefix.PrefixLength = uint8(prefix.Bits())
	row.NextHop = ipv4Sockaddr(next4)
	row.SitePrefixLength = uint8(prefix.Bits())
	row.ValidLifetime = routeLifetimeInfinite
	row.PreferredLifetime = routeLifetimeInfinite
	row.Metric = metric
	row.Protocol = mibIPProtoNetMgmt
	row.Loopback = 0
	row.AutoconfigureAddress = 0
	row.Publish = 0
	row.Immortal = 0
	row.Age = 0
	row.Origin = 0
	return row, nil
}

func ipv4Sockaddr(addr [4]byte) rawSockaddrInet {
	return rawSockaddrInet{
		Family: addressFamilyInet,
		Data:   [6]uint32{binary.LittleEndian.Uint32(addr[:])},
	}
}
