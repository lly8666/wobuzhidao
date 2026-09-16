from pathlib import Path


def replace_once(path: str, old: str, new: str):
    p = Path(path)
    text = p.read_text(encoding='utf-8')
    n = text.count(old)
    if n != 1:
        raise SystemExit(f'{path}: expected one occurrence, got {n}: {old[:120]!r}')
    p.write_text(text.replace(old, new, 1), encoding='utf-8')


replace_once('internal/windowsruntime/controller.go',
'''\tif discoverer == nil {
\t\tdiscoverer = PowerShellUnderlayDiscoverer{}
\t}''',
'''\tif discoverer == nil {
\t\tdiscoverer = defaultUnderlayDiscoverer()
\t}''')

# The legacy discoverer stays available for diagnostic fallback, but even that
# path must not surface a console window.
replace_once('internal/windowsruntime/underlay_path.go',
'''\tcmd := exec.Command("powershell.exe", args...)
\toutput, err := cmd.CombinedOutput()''',
'''\tcmd := exec.Command("powershell.exe", args...)
\tconfigureHiddenProcess(cmd)
\toutput, err := cmd.CombinedOutput()''')

Path('internal/windowsruntime/underlay_default_windows.go').write_text(r'''//go:build windows

package windowsruntime

func defaultUnderlayDiscoverer() UnderlayDiscoverer { return NativeWindowsUnderlayDiscoverer{} }
''', encoding='utf-8')

Path('internal/windowsruntime/underlay_default_other.go').write_text(r'''//go:build !windows

package windowsruntime

func defaultUnderlayDiscoverer() UnderlayDiscoverer { return PowerShellUnderlayDiscoverer{} }
''', encoding='utf-8')

Path('internal/windowsruntime/underlay_native_windows.go').write_text(r'''//go:build windows

package windowsruntime

import (
    "encoding/binary"
    "encoding/json"
    "errors"
    "fmt"
    "net"
    "net/netip"
    "os"
    "sort"
    "strings"
    "syscall"
    "unsafe"

    "golang.org/x/sys/windows"
)

// NativeWindowsUnderlayDiscoverer replaces steady-state NetTCPIP PowerShell
// discovery with IP Helper APIs. PowerShell is retained only by the dependency
// preflight/installer compatibility path while that separate surface is migrated.
type NativeWindowsUnderlayDiscoverer struct{}

func (NativeWindowsUnderlayDiscoverer) Preflight(profile Profile) error {
    return PowerShellUnderlayDiscoverer{}.Preflight(profile)
}

func (NativeWindowsUnderlayDiscoverer) Discover(profile Profile) (Underlay, error) {
    observed, err := nativeUnderlayObservation(profile, false)
    if err != nil { return Underlay{}, err }
    return observed.Underlay, nil
}

func (NativeWindowsUnderlayDiscoverer) DiscoverPath(profile Profile) (Underlay, error) {
    observed, err := nativeUnderlayObservation(profile, true)
    if err != nil { return Underlay{}, err }
    return observed.Underlay, nil
}

func (NativeWindowsUnderlayDiscoverer) DiscoverPathObservation(profile Profile) (underlayPathObservation, error) {
    return nativeUnderlayObservation(profile, true)
}

type nativeAdapterInfo struct {
    index        uint32
    metric       uint32
    adapterName  string
    source       netip.Addr
    sourcePrefix int
    sourceMAC    string
}

type nativeRouteCandidate struct {
    prefix    netip.Prefix
    nextHop   netip.Addr
    ifIndex   uint32
    metric    uint64
    adapter   nativeAdapterInfo
}

type nativeRouteState struct {
    AdapterInterfaceIndex uint32 `json:"AdapterInterfaceIndex"`
    UnderlayRoutes []struct {
        DestinationPrefix string `json:"DestinationPrefix"`
        InterfaceIndex uint32 `json:"InterfaceIndex"`
        NextHop string `json:"NextHop"`
    } `json:"UnderlayRoutes"`
}

func nativeUnderlayObservation(profile Profile, monitor bool) (underlayPathObservation, error) {
    profile = profile.normalized()
    if err := profile.Validate(); err != nil { return underlayPathObservation{}, err }
    raw, _ := netip.ParseAddrPort(profile.ServerRaw)
    remote := raw.Addr()

    adapters, err := nativeIPv4Adapters()
    if err != nil { return underlayPathObservation{}, fmt.Errorf("enumerate Windows adapters: %w", err) }
    excludedIf := uint32(0)
    owned := map[string]bool{}
    if monitor {
        state, err := readNativeRouteState(profile.RouteState)
        if err != nil { return underlayPathObservation{}, err }
        excludedIf = state.AdapterInterfaceIndex
        for _, item := range state.UnderlayRoutes {
            prefix, err := netip.ParsePrefix(strings.TrimSpace(item.DestinationPrefix))
            if err != nil || !prefix.Addr().Is4() { continue }
            nextHop := normalizeRouteNextHop(item.NextHop)
            owned[nativeRouteKey(prefix.Masked(), item.InterfaceIndex, nextHop)] = true
        }
    }

    routes, err := nativeIPv4Routes(adapters, remote, excludedIf, owned)
    if err != nil { return underlayPathObservation{}, err }
    selected, err := selectNativeRoute(routes)
    if err != nil { return underlayPathObservation{}, fmt.Errorf("select physical IPv4 route for %s: %w", remote, err) }

    nextHop := selected.nextHop
    if !nextHop.IsValid() || nextHop.IsUnspecified() { nextHop = remote }
    nextHopMAC, err := sendARP(nextHop, selected.adapter.source)
    if err != nil {
        return underlayPathObservation{}, fmt.Errorf("resolve next-hop MAC for %s on interface %d: %w", nextHop, selected.ifIndex, err)
    }
    packetDevice, err := npcapDeviceForAdapter(selected.adapter.adapterName)
    if err != nil { return underlayPathObservation{}, err }
    observed := underlayPathObservation{
        Underlay: Underlay{
            SourceIP: selected.adapter.source.String(),
            PacketDevice: packetDevice,
            SourceMAC: selected.adapter.sourceMAC,
            NextHopMAC: nextHopMAC,
        },
        InterfaceIndex: selected.ifIndex,
        NextHopIP: nextHop.String(),
    }
    if err := observed.Validate(); err != nil { return underlayPathObservation{}, err }
    return observed, nil
}

func readNativeRouteState(path string) (nativeRouteState, error) {
    var state nativeRouteState
    raw, err := os.ReadFile(path)
    if err != nil { return state, fmt.Errorf("connected physical-path monitoring requires WBD route state %q: %w", path, err) }
    if err := json.Unmarshal(raw, &state); err != nil { return state, fmt.Errorf("decode WBD route state: %w", err) }
    return state, nil
}

func nativeIPv4Adapters() (map[uint32]nativeAdapterInfo, error) {
    size := uint32(16 * 1024)
    for attempt := 0; attempt < 3; attempt++ {
        buf := make([]byte, size)
        first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
        err := windows.GetAdaptersAddresses(uint32(windows.AF_INET), windows.GAA_FLAG_INCLUDE_PREFIX, 0, first, &size)
        if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) { continue }
        if err != nil { return nil, err }
        result := make(map[uint32]nativeAdapterInfo)
        for a := first; a != nil; a = a.Next {
            if a.IfIndex == 0 || a.OperStatus != windows.IfOperStatusUp || a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK { continue }
            source, bits := preferredAdapterIPv4(a)
            if !source.IsValid() { continue }
            macLen := int(a.PhysicalAddressLength)
            if macLen < 6 || macLen > len(a.PhysicalAddress) { continue }
            sourceMAC := net.HardwareAddr(a.PhysicalAddress[:macLen]).String()
            result[a.IfIndex] = nativeAdapterInfo{
                index: a.IfIndex,
                metric: a.Ipv4Metric,
                adapterName: windows.BytePtrToString(a.AdapterName),
                source: source,
                sourcePrefix: bits,
                sourceMAC: strings.ToLower(sourceMAC),
            }
        }
        return result, nil
    }
    return nil, errors.New("GetAdaptersAddresses buffer size changed repeatedly")
}

func preferredAdapterIPv4(a *windows.IpAdapterAddresses) (netip.Addr, int) {
    bestBits := -1
    var best netip.Addr
    for u := a.FirstUnicastAddress; u != nil; u = u.Next {
        ip := u.Address.IP()
        v4 := ip.To4()
        if v4 == nil || v4[0] == 169 && v4[1] == 254 { continue }
        addr, ok := netip.AddrFromSlice(v4)
        if !ok { continue }
        addr = addr.Unmap()
        bits := int(u.OnLinkPrefixLength)
        if !best.IsValid() || bits > bestBits {
            best, bestBits = addr, bits
        }
    }
    return best, bestBits
}

func nativeIPv4Routes(adapters map[uint32]nativeAdapterInfo, remote netip.Addr, excludedIf uint32, owned map[string]bool) ([]nativeRouteCandidate, error) {
    var table *windows.MibIpForwardTable2
    if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil { return nil, err }
    if table == nil { return nil, errors.New("GetIpForwardTable2 returned nil table") }
    defer windows.FreeMibTable(unsafe.Pointer(table))
    rows := unsafe.Slice(&table.Table[0], int(table.NumEntries))
    candidates := make([]nativeRouteCandidate, 0, len(rows))
    for i := range rows {
        row := &rows[i]
        if row.InterfaceIndex == 0 || row.InterfaceIndex == excludedIf { continue }
        adapter, ok := adapters[row.InterfaceIndex]
        if !ok { continue }
        prefixAddr, ok := ipv4FromRawSockaddr(&row.DestinationPrefix.Prefix)
        if !ok { continue }
        bits := int(row.DestinationPrefix.PrefixLength)
        if bits < 0 || bits > 32 { continue }
        prefix := netip.PrefixFrom(prefixAddr, bits).Masked()
        if !prefix.Contains(remote) { continue }
        nextHop, _ := ipv4FromRawSockaddr(&row.NextHop)
        key := nativeRouteKey(prefix, row.InterfaceIndex, normalizeRouteNextHop(nextHop.String()))
        if owned[key] { continue }
        candidates = append(candidates, nativeRouteCandidate{
            prefix: prefix,
            nextHop: nextHop,
            ifIndex: row.InterfaceIndex,
            metric: uint64(row.Metric) + uint64(adapter.metric),
            adapter: adapter,
        })
    }
    return candidates, nil
}

func ipv4FromRawSockaddr(raw *windows.RawSockaddrInet) (netip.Addr, bool) {
    if raw == nil || raw.Family != windows.AF_INET { return netip.Addr{}, false }
    v4 := (*syscall.RawSockaddrInet4)(unsafe.Pointer(raw))
    addr := netip.AddrFrom4(v4.Addr)
    return addr, true
}

func selectNativeRoute(candidates []nativeRouteCandidate) (nativeRouteCandidate, error) {
    if len(candidates) == 0 { return nativeRouteCandidate{}, errors.New("no usable connected physical route") }
    sort.Slice(candidates, func(i, j int) bool {
        ib, jb := candidates[i].prefix.Bits(), candidates[j].prefix.Bits()
        if ib != jb { return ib > jb }
        if candidates[i].metric != candidates[j].metric { return candidates[i].metric < candidates[j].metric }
        return candidates[i].ifIndex < candidates[j].ifIndex
    })
    return candidates[0], nil
}

func normalizeRouteNextHop(value string) string {
    value = strings.TrimSpace(value)
    if value == "" { return "0.0.0.0" }
    if ip, err := netip.ParseAddr(value); err == nil && ip.Is4() { return ip.String() }
    return strings.ToLower(value)
}

func nativeRouteKey(prefix netip.Prefix, ifIndex uint32, nextHop string) string {
    return strings.ToLower(prefix.Masked().String()) + "|" + fmt.Sprint(ifIndex) + "|" + normalizeRouteNextHop(nextHop)
}

func npcapDeviceForAdapter(name string) (string, error) {
    guid := strings.TrimSpace(name)
    upper := strings.ToUpper(guid)
    const tcpipPrefix = `\\DEVICE\\TCPIP_`
    if strings.HasPrefix(upper, tcpipPrefix) { guid = guid[len(tcpipPrefix):] }
    if len(guid) == 36 && !strings.HasPrefix(guid, "{") { guid = "{" + guid + "}" }
    if len(guid) != 38 || !strings.HasPrefix(guid, "{") || !strings.HasSuffix(guid, "}") {
        return "", fmt.Errorf("Windows adapter name is not a GUID: %q", name)
    }
    return `\Device\NPF_` + strings.ToUpper(guid), nil
}

var sendARPProc = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("SendARP")

func sendARP(destination, source netip.Addr) (string, error) {
    if !destination.Is4() || !source.Is4() { return "", errors.New("SendARP requires IPv4 addresses") }
    dst4 := destination.As4()
    src4 := source.As4()
    dst := binary.LittleEndian.Uint32(dst4[:])
    src := binary.LittleEndian.Uint32(src4[:])
    var mac [8]byte
    length := uint32(len(mac))
    r1, _, _ := sendARPProc.Call(
        uintptr(dst), uintptr(src),
        uintptr(unsafe.Pointer(&mac[0])), uintptr(unsafe.Pointer(&length)),
    )
    if r1 != 0 { return "", syscall.Errno(r1) }
    if length < 6 || int(length) > len(mac) { return "", fmt.Errorf("SendARP returned invalid MAC length %d", length) }
    return strings.ToLower(net.HardwareAddr(mac[:length]).String()), nil
}
''', encoding='utf-8')

Path('internal/windowsruntime/underlay_native_windows_test.go').write_text(r'''//go:build windows

package windowsruntime

import (
    "net/netip"
    "testing"
)

func TestWindowsDefaultUnderlayDiscovererIsNative(t *testing.T) {
    if _, ok := defaultUnderlayDiscoverer().(NativeWindowsUnderlayDiscoverer); !ok {
        t.Fatalf("default discoverer=%T want NativeWindowsUnderlayDiscoverer", defaultUnderlayDiscoverer())
    }
}

func TestNativeRouteSelectionMatchesPowerShellPolicy(t *testing.T) {
    mk := func(prefix string, metric uint64, ifIndex uint32) nativeRouteCandidate {
        return nativeRouteCandidate{prefix: netip.MustParsePrefix(prefix), metric: metric, ifIndex: ifIndex}
    }
    got, err := selectNativeRoute([]nativeRouteCandidate{
        mk("0.0.0.0/0", 5, 9),
        mk("203.0.113.0/24", 40, 12),
        mk("203.0.113.0/24", 20, 11),
        mk("203.0.113.0/24", 20, 10),
    })
    if err != nil { t.Fatal(err) }
    if got.prefix.Bits() != 24 || got.metric != 20 || got.ifIndex != 10 {
        t.Fatalf("selected=%+v", got)
    }
}

func TestNpcapDeviceUsesAdapterGuid(t *testing.T) {
    got, err := npcapDeviceForAdapter("{11111111-2222-3333-4444-555555555555}")
    if err != nil { t.Fatal(err) }
    want := `\Device\NPF_{11111111-2222-3333-4444-555555555555}`
    if got != want { t.Fatalf("got=%q want=%q", got, want) }
}
''', encoding='utf-8')

print('native Windows underlay patch applied')
