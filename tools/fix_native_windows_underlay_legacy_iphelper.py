from pathlib import Path

p = Path('internal/windowsruntime/underlay_native_windows.go')
s = p.read_text(encoding='utf-8')
old = r'''func nativeIPv4Routes(adapters map[uint32]nativeAdapterInfo, remote netip.Addr, excludedIf uint32, owned map[string]bool) ([]nativeRouteCandidate, error) {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return nil, err
	}
	if table == nil {
		return nil, errors.New("GetIpForwardTable2 returned nil table")
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))
	rows := unsafe.Slice(&table.Table[0], int(table.NumEntries))
	candidates := make([]nativeRouteCandidate, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		if row.InterfaceIndex == 0 || row.InterfaceIndex == excludedIf {
			continue
		}
		adapter, ok := adapters[row.InterfaceIndex]
		if !ok {
			continue
		}
		prefixAddr, ok := ipv4FromRawSockaddr(&row.DestinationPrefix.Prefix)
		if !ok {
			continue
		}
		bits := int(row.DestinationPrefix.PrefixLength)
		if bits < 0 || bits > 32 {
			continue
		}
		prefix := netip.PrefixFrom(prefixAddr, bits).Masked()
		if !prefix.Contains(remote) {
			continue
		}
		nextHop, _ := ipv4FromRawSockaddr(&row.NextHop)
		key := nativeRouteKey(prefix, row.InterfaceIndex, normalizeRouteNextHop(nextHop.String()))
		if owned[key] {
			continue
		}
		candidates = append(candidates, nativeRouteCandidate{
			prefix:  prefix,
			nextHop: nextHop,
			ifIndex: row.InterfaceIndex,
			metric:  uint64(row.Metric) + uint64(adapter.metric),
			adapter: adapter,
		})
	}
	return candidates, nil
}

func ipv4FromRawSockaddr(raw *windows.RawSockaddrInet) (netip.Addr, bool) {
	if raw == nil || raw.Family != windows.AF_INET {
		return netip.Addr{}, false
	}
	v4 := (*syscall.RawSockaddrInet4)(unsafe.Pointer(raw))
	addr := netip.AddrFrom4(v4.Addr)
	return addr, true
}
'''
new = r'''type mibIPForwardRow struct {
	Dest       uint32
	Mask       uint32
	Policy     uint32
	NextHop    uint32
	IfIndex    uint32
	Type       uint32
	Proto      uint32
	Age        uint32
	NextHopAS  uint32
	Metric1    uint32
	Metric2    uint32
	Metric3    uint32
	Metric4    uint32
	Metric5    uint32
}

type mibIPForwardTable struct {
	NumEntries uint32
	Table      [1]mibIPForwardRow
}

var getIPForwardTableProc = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetIpForwardTable")

func nativeIPv4Routes(adapters map[uint32]nativeAdapterInfo, remote netip.Addr, excludedIf uint32, owned map[string]bool) ([]nativeRouteCandidate, error) {
	// GetIpForwardTable is IPv4-only and has existed since Windows 2000. That is
	// sufficient here because WBD's current FakeTCP underlay is IPv4-only, and it
	// avoids depending on newer x/sys wrappers while preserving native IP Helper.
	var size uint32
	r1, _, _ := getIPForwardTableProc.Call(0, uintptr(unsafe.Pointer(&size)), 0)
	if r1 != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) && r1 != 0 {
		return nil, syscall.Errno(r1)
	}
	if size < uint32(unsafe.Sizeof(mibIPForwardTable{})) {
		size = uint32(unsafe.Sizeof(mibIPForwardTable{}))
	}
	buf := make([]byte, size)
	r1, _, _ = getIPForwardTableProc.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0)
	if r1 != 0 {
		return nil, syscall.Errno(r1)
	}
	table := (*mibIPForwardTable)(unsafe.Pointer(&buf[0]))
	rowSize := unsafe.Sizeof(mibIPForwardRow{})
	rowOffset := unsafe.Offsetof(table.Table)
	if rowOffset+uintptr(table.NumEntries)*rowSize > uintptr(len(buf)) {
		return nil, errors.New("GetIpForwardTable returned truncated table")
	}
	candidates := make([]nativeRouteCandidate, 0, int(table.NumEntries))
	for i := uint32(0); i < table.NumEntries; i++ {
		row := (*mibIPForwardRow)(unsafe.Pointer(uintptr(unsafe.Pointer(table)) + rowOffset + uintptr(i)*rowSize))
		if row.IfIndex == 0 || row.IfIndex == excludedIf {
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
		nextHop := ipv4FromNetworkDWORD(row.NextHop)
		key := nativeRouteKey(prefix, row.IfIndex, normalizeRouteNextHop(nextHop.String()))
		if owned[key] {
			continue
		}
		// On Vista and later Metric1 is already the route+interface combined
		// metric, matching the policy previously implemented with NetTCPIP.
		candidates = append(candidates, nativeRouteCandidate{
			prefix: prefix, nextHop: nextHop, ifIndex: row.IfIndex,
			metric: uint64(row.Metric1), adapter: adapter,
		})
	}
	return candidates, nil
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
'''
if old not in s:
    raise SystemExit('expected GetIpForwardTable2 block not found')
s = s.replace(old, new, 1)
p.write_text(s, encoding='utf-8')
print('native underlay switched to compatible IPv4 GetIpForwardTable API')
