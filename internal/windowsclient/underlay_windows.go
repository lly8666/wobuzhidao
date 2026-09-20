//go:build windows

package windowsclient

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type nativeAdapterInfo struct {
	interfaceIndex uint32
	adapterName    string
	source4        netip.Addr
	sourceMAC      [6]byte
}

type mibIPForwardRow struct {
	Dest      uint32
	Mask      uint32
	Policy    uint32
	NextHop   uint32
	IfIndex   uint32
	Type      uint32
	Proto     uint32
	Age       uint32
	NextHopAS uint32
	Metric1   uint32
	Metric2   uint32
	Metric3   uint32
	Metric4   uint32
	Metric5   uint32
}

type mibIPForwardTable struct {
	NumEntries uint32
	Table      [1]mibIPForwardRow
}

var (
	getIPForwardTableProc = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetIpForwardTable")
	sendARPProc            = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("SendARP")
)

func DiscoverPhysicalUnderlay(server4 netip.Addr, excludedInterfaceIndex uint32) (PhysicalUnderlay, error) {
	if !server4.IsValid() || !server4.Is4() {
		return PhysicalUnderlay{}, ErrPhysicalUnderlay
	}
	server4 = server4.Unmap()
	adapters, err := nativeIPv4Adapters()
	if err != nil {
		return PhysicalUnderlay{}, fmt.Errorf("enumerate Windows adapters: %w", err)
	}
	routes, err := nativeIPv4Routes(adapters, server4, excludedInterfaceIndex)
	if err != nil {
		return PhysicalUnderlay{}, fmt.Errorf("enumerate Windows routes: %w", err)
	}
	selected, err := selectPhysicalRoute(routes)
	if err != nil {
		return PhysicalUnderlay{}, fmt.Errorf("select physical route for %s: %w", server4, err)
	}
	nextHop := selected.nextHop
	if !nextHop.IsValid() || nextHop.IsUnspecified() {
		nextHop = server4
	}
	nextHopMAC, err := sendARP(nextHop, selected.source4)
	if err != nil {
		return PhysicalUnderlay{}, fmt.Errorf(
			"resolve next-hop MAC for %s on interface %d: %w",
			nextHop,
			selected.interfaceIndex,
			err,
		)
	}
	packetDevice, err := npcapDeviceForAdapter(selected.adapterName)
	if err != nil {
		return PhysicalUnderlay{}, err
	}
	out := PhysicalUnderlay{
		Peer4:          server4,
		Source4:        selected.source4,
		InterfaceIndex: selected.interfaceIndex,
		PacketDevice:   packetDevice,
		SourceMAC:      selected.sourceMAC,
		NextHop4:       nextHop,
		NextHopMAC:     nextHopMAC,
	}
	if err := out.Validate(); err != nil {
		return PhysicalUnderlay{}, err
	}
	return out, nil
}

func nativeIPv4Adapters() (map[uint32]nativeAdapterInfo, error) {
	size := uint32(16 * 1024)
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(
			uint32(windows.AF_INET),
			windows.GAA_FLAG_INCLUDE_PREFIX,
			0,
			first,
			&size,
		)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result := make(map[uint32]nativeAdapterInfo)
		for a := first; a != nil; a = a.Next {
			if a.IfIndex == 0 ||
				a.OperStatus != windows.IfOperStatusUp ||
				a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
				continue
			}
			source := preferredAdapterIPv4(a)
			if !source.IsValid() {
				continue
			}
			macLen := int(a.PhysicalAddressLength)
			if macLen < 6 || macLen > len(a.PhysicalAddress) {
				continue
			}
			var mac [6]byte
			copy(mac[:], a.PhysicalAddress[:6])
			result[a.IfIndex] = nativeAdapterInfo{
				interfaceIndex: a.IfIndex,
				adapterName:    windows.BytePtrToString(a.AdapterName),
				source4:        source,
				sourceMAC:      mac,
			}
		}
		return result, nil
	}
	return nil, errors.New("GetAdaptersAddresses buffer size changed repeatedly")
}

func preferredAdapterIPv4(a *windows.IpAdapterAddresses) netip.Addr {
	bestBits := -1
	var best netip.Addr
	for u := a.FirstUnicastAddress; u != nil; u = u.Next {
		v4 := u.Address.IP().To4()
		if v4 == nil || (v4[0] == 169 && v4[1] == 254) {
			continue
		}
		addr, ok := netip.AddrFromSlice(v4)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		bits := int(u.OnLinkPrefixLength)
		if !best.IsValid() || bits > bestBits {
			best = addr
			bestBits = bits
		}
	}
	return best
}

func nativeIPv4Routes(adapters map[uint32]nativeAdapterInfo, remote netip.Addr, excludedInterfaceIndex uint32) ([]physicalRouteCandidate, error) {
	var size uint32
	r1, _, _ := getIPForwardTableProc.Call(
		0,
		uintptr(unsafe.Pointer(&size)),
		0,
	)
	if r1 != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) && r1 != 0 {
		return nil, syscall.Errno(r1)
	}
	if size < uint32(unsafe.Sizeof(mibIPForwardTable{})) {
		size = uint32(unsafe.Sizeof(mibIPForwardTable{}))
	}
	buf := make([]byte, size)
	r1, _, _ = getIPForwardTableProc.Call(
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
		0,
	)
	if r1 != 0 {
		return nil, syscall.Errno(r1)
	}
	table := (*mibIPForwardTable)(unsafe.Pointer(&buf[0]))
	rowSize := unsafe.Sizeof(mibIPForwardRow{})
	rowOffset := unsafe.Offsetof(table.Table)
	if rowOffset+uintptr(table.NumEntries)*rowSize > uintptr(len(buf)) {
		return nil, errors.New("GetIpForwardTable returned truncated table")
	}

	out := make([]physicalRouteCandidate, 0, int(table.NumEntries))
	for i := uint32(0); i < table.NumEntries; i++ {
		row := (*mibIPForwardRow)(unsafe.Pointer(
			uintptr(unsafe.Pointer(table)) +
				rowOffset +
				uintptr(i)*rowSize,
		))
		if row.IfIndex == 0 || row.IfIndex == excludedInterfaceIndex {
			continue
		}
		adapter, ok := adapters[row.IfIndex]
		if !ok {
			continue
		}
		dest := ipv4FromNetworkDWORD(row.Dest)
		bits, ok := prefixBitsFromNetworkMask(row.Mask)
		if !ok {
			continue
		}
		prefix := netip.PrefixFrom(dest, bits).Masked()
		if !prefix.Contains(remote) {
			continue
		}
		out = append(out, physicalRouteCandidate{
			prefix:         prefix,
			nextHop:        ipv4FromNetworkDWORD(row.NextHop),
			interfaceIndex: row.IfIndex,
			metric:         uint64(row.Metric1),
			source4:        adapter.source4,
			adapterName:    adapter.adapterName,
			sourceMAC:      adapter.sourceMAC,
		})
	}
	return out, nil
}

func ipv4FromNetworkDWORD(v uint32) netip.Addr {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	return netip.AddrFrom4(b)
}

func prefixBitsFromNetworkMask(v uint32) (int, bool) {
	var b [4]byte
	binary.LittleEndian.PutUint32(b[:], v)
	ones, bits := net.IPMask(b[:]).Size()
	return ones, bits == 32 && ones >= 0
}

func sendARP(destination, source netip.Addr) ([6]byte, error) {
	var out [6]byte
	if !destination.Is4() || !source.Is4() {
		return out, ErrPhysicalUnderlay
	}
	dst4 := destination.As4()
	src4 := source.As4()
	dst := binary.LittleEndian.Uint32(dst4[:])
	src := binary.LittleEndian.Uint32(src4[:])
	var mac [8]byte
	length := uint32(len(mac))
	r1, _, _ := sendARPProc.Call(
		uintptr(dst),
		uintptr(src),
		uintptr(unsafe.Pointer(&mac[0])),
		uintptr(unsafe.Pointer(&length)),
	)
	if r1 != 0 {
		return out, syscall.Errno(r1)
	}
	if length < 6 {
		return out, fmt.Errorf(
			"%w: SendARP returned MAC length %d",
			ErrPhysicalUnderlay,
			length,
		)
	}
	copy(out[:], mac[:6])
	return out, nil
}
