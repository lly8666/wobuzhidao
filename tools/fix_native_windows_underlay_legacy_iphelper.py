from pathlib import Path

p = Path('internal/windowsruntime/underlay_native_windows.go')
s = p.read_text(encoding='utf-8')
start_marker = 'func nativeIPv4Routes('
end_marker = 'func selectNativeRoute('
start = s.find(start_marker)
end = s.find(end_marker, start)
if start < 0 or end < 0 or end <= start:
    raise SystemExit('native IPv4 route block boundaries not found')
new = r'''type mibIPForwardRow struct {
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

var getIPForwardTableProc = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("GetIpForwardTable")

func nativeIPv4Routes(adapters map[uint32]nativeAdapterInfo, remote netip.Addr, excludedIf uint32, owned map[string]bool) ([]nativeRouteCandidate, error) {
    // WBD's FakeTCP underlay is IPv4-only. Use the long-established IPv4 IP
    // Helper table directly so runtime discovery does not depend on PowerShell
    // or on newer x/sys wrappers.
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
s = s[:start] + new + s[end:]
p.write_text(s, encoding='utf-8')
print('native underlay switched to compatible IPv4 GetIpForwardTable API')
