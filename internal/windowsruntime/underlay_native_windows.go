//go:build windows

package windowsruntime

import (
	"context"
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
	"time"
	"unsafe"

	"github.com/lly8666/wobuzhidao/internal/ipset"
	"golang.org/x/sys/windows"
)

// NativeWindowsUnderlayDiscoverer keeps the production Windows discovery and
// dependency preflight independent of PowerShell. All persistent state remains
// inside the portable WBD directory.
type NativeWindowsUnderlayDiscoverer struct{}

func (NativeWindowsUnderlayDiscoverer) Preflight(profile Profile) error {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil {
		return err
	}
	if profile.RequiresCNSet() {
		manifest, installed, err := ipset.EnsureEmbeddedCNBaseline(profile.CNSetDir)
		if err != nil {
			return fmt.Errorf("prepare offline mainland-China IP ranges: %w", err)
		}
		fmt.Printf("WBD_WINDOWS_CN_BASELINE_READY ipv4=%d ipv6=%d installed=%d source=%s\n", manifest.IPv4Count, manifest.IPv6Count, boolToInt(installed), manifest.Source)
		if profile.AutoUpdateCN {
			refreshCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			refreshed, refreshErr := ipset.EnsureCNBundle(refreshCtx, profile.CNSetDir)
			cancel()
			if refreshErr != nil {
				fmt.Printf("WBD_WINDOWS_CN_REFRESH_WARN baseline_kept=1 error=%q\n", refreshErr)
			} else {
				fmt.Printf("WBD_WINDOWS_CN_REFRESH_READY refreshed=%d stale=%d ipv4=%d ipv6=%d\n", boolToInt(refreshed.Refreshed), boolToInt(refreshed.UsedStale), refreshed.Manifest.IPv4Count, refreshed.Manifest.IPv6Count)
			}
		}
	}
	if err := ValidateRoutingAssets(profile); err != nil {
		return err
	}
	state, err := ensureNpcapReady()
	if err != nil {
		return fmt.Errorf("Npcap runtime is not ready: %w; use wbd.exe -install-npcap", err)
	}
	fmt.Printf("WBD_WINDOWS_NPCAP_READY version=%s service=%s control=program\n", npcapVersion, state)
	return nil
}

func (NativeWindowsUnderlayDiscoverer) Discover(profile Profile) (Underlay, error) {
	observed, err := nativeUnderlayObservation(profile, false)
	if err != nil {
		return Underlay{}, err
	}
	return observed.Underlay, nil
}

func (NativeWindowsUnderlayDiscoverer) DiscoverPath(profile Profile) (Underlay, error) {
	observed, err := nativeUnderlayObservation(profile, true)
	if err != nil {
		return Underlay{}, err
	}
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
	prefix  netip.Prefix
	nextHop netip.Addr
	ifIndex uint32
	metric  uint64
	adapter nativeAdapterInfo
}

type nativeRouteState struct {
	AdapterInterfaceIndex uint32 `json:"AdapterInterfaceIndex"`
	UnderlayRoutes        []struct {
		DestinationPrefix string `json:"DestinationPrefix"`
		InterfaceIndex    uint32 `json:"InterfaceIndex"`
		NextHop           string `json:"NextHop"`
	} `json:"UnderlayRoutes"`
}

func nativeUnderlayObservation(profile Profile, monitor bool) (underlayPathObservation, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil {
		return underlayPathObservation{}, err
	}
	raw, _ := netip.ParseAddrPort(profile.ServerRaw)
	remote := raw.Addr()

	adapters, err := nativeIPv4Adapters()
	if err != nil {
		return underlayPathObservation{}, fmt.Errorf("enumerate Windows adapters: %w", err)
	}
	excludedIf := uint32(0)
	owned := map[string]bool{}
	if monitor {
		state, err := readNativeRouteState(profile.RouteState)
		if err != nil {
			return underlayPathObservation{}, err
		}
		excludedIf = state.AdapterInterfaceIndex
		for _, item := range state.UnderlayRoutes {
			prefix, err := netip.ParsePrefix(strings.TrimSpace(item.DestinationPrefix))
			if err != nil || !prefix.Addr().Is4() {
				continue
			}
			nextHop := normalizeRouteNextHop(item.NextHop)
			owned[nativeRouteKey(prefix.Masked(), item.InterfaceIndex, nextHop)] = true
		}
	}

	routes, err := nativeIPv4Routes(adapters, remote, excludedIf, owned)
	if err != nil {
		return underlayPathObservation{}, err
	}
	selected, err := selectNativeRoute(routes)
	if err != nil {
		return underlayPathObservation{}, fmt.Errorf("select physical IPv4 route for %s: %w", remote, err)
	}

	nextHop := selected.nextHop
	if !nextHop.IsValid() || nextHop.IsUnspecified() {
		nextHop = remote
	}
	nextHopMAC, err := sendARP(nextHop, selected.adapter.source)
	if err != nil {
		return underlayPathObservation{}, fmt.Errorf("resolve next-hop MAC for %s on interface %d: %w", nextHop, selected.ifIndex, err)
	}
	packetDevice, err := npcapDeviceForAdapter(selected.adapter.adapterName)
	if err != nil {
		return underlayPathObservation{}, err
	}
	observed := underlayPathObservation{
		Underlay: Underlay{
			SourceIP:       selected.adapter.source.String(),
			InterfaceIndex: selected.ifIndex,
			PacketDevice:   packetDevice,
			SourceMAC:      selected.adapter.sourceMAC,
			NextHopIP:      nextHop.String(),
			NextHopMAC:     nextHopMAC,
		},
		InterfaceIndex: selected.ifIndex,
		NextHopIP:      nextHop.String(),
	}
	if err := observed.Validate(); err != nil {
		return underlayPathObservation{}, err
	}
	return observed, nil
}

func readNativeRouteState(path string) (nativeRouteState, error) {
	var state nativeRouteState
	raw, err := os.ReadFile(path)
	if err != nil {
		return state, fmt.Errorf("connected physical-path monitoring requires WBD route state %q: %w", path, err)
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, fmt.Errorf("decode WBD route state: %w", err)
	}
	return state, nil
}

func nativeIPv4Adapters() (map[uint32]nativeAdapterInfo, error) {
	size := uint32(16 * 1024)
	for attempt := 0; attempt < 3; attempt++ {
		buf := make([]byte, size)
		first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
		err := windows.GetAdaptersAddresses(uint32(windows.AF_INET), windows.GAA_FLAG_INCLUDE_PREFIX, 0, first, &size)
		if errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result := make(map[uint32]nativeAdapterInfo)
		for a := first; a != nil; a = a.Next {
			if a.IfIndex == 0 || a.OperStatus != windows.IfOperStatusUp || a.IfType == windows.IF_TYPE_SOFTWARE_LOOPBACK {
				continue
			}
			source, bits := preferredAdapterIPv4(a)
			if !source.IsValid() {
				continue
			}
			macLen := int(a.PhysicalAddressLength)
			if macLen < 6 || macLen > len(a.PhysicalAddress) {
				continue
			}
			sourceMAC := net.HardwareAddr(a.PhysicalAddress[:macLen]).String()
			result[a.IfIndex] = nativeAdapterInfo{
				index:        a.IfIndex,
				metric:       a.Ipv4Metric,
				adapterName:  windows.BytePtrToString(a.AdapterName),
				source:       source,
				sourcePrefix: bits,
				sourceMAC:    strings.ToLower(sourceMAC),
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
		if v4 == nil || v4[0] == 169 && v4[1] == 254 {
			continue
		}
		addr, ok := netip.AddrFromSlice(v4)
		if !ok {
			continue
		}
		addr = addr.Unmap()
		bits := int(u.OnLinkPrefixLength)
		if !best.IsValid() || bits > bestBits {
			best, bestBits = addr, bits
		}
	}
	return best, bestBits
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

func selectNativeRoute(candidates []nativeRouteCandidate) (nativeRouteCandidate, error) {
	if len(candidates) == 0 {
		return nativeRouteCandidate{}, errors.New("no usable connected physical route")
	}
	sort.Slice(candidates, func(i, j int) bool {
		ib, jb := candidates[i].prefix.Bits(), candidates[j].prefix.Bits()
		if ib != jb {
			return ib > jb
		}
		if candidates[i].metric != candidates[j].metric {
			return candidates[i].metric < candidates[j].metric
		}
		return candidates[i].ifIndex < candidates[j].ifIndex
	})
	return candidates[0], nil
}

func normalizeRouteNextHop(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "0.0.0.0"
	}
	if ip, err := netip.ParseAddr(value); err == nil && ip.Is4() {
		return ip.String()
	}
	return strings.ToLower(value)
}

func nativeRouteKey(prefix netip.Prefix, ifIndex uint32, nextHop string) string {
	return strings.ToLower(prefix.Masked().String()) + "|" + fmt.Sprint(ifIndex) + "|" + normalizeRouteNextHop(nextHop)
}

func npcapDeviceForAdapter(name string) (string, error) {
	guid := strings.TrimSpace(name)
	upper := strings.ToUpper(guid)
	const tcpipPrefix = `\\DEVICE\\TCPIP_`
	if strings.HasPrefix(upper, tcpipPrefix) {
		guid = guid[len(tcpipPrefix):]
	}
	if len(guid) == 36 && !strings.HasPrefix(guid, "{") {
		guid = "{" + guid + "}"
	}
	if len(guid) != 38 || !strings.HasPrefix(guid, "{") || !strings.HasSuffix(guid, "}") {
		return "", fmt.Errorf("Windows adapter name is not a GUID: %q", name)
	}
	return `\Device\NPF_` + strings.ToUpper(guid), nil
}

var sendARPProc = windows.NewLazySystemDLL("iphlpapi.dll").NewProc("SendARP")

func sendARP(destination, source netip.Addr) (string, error) {
	if !destination.Is4() || !source.Is4() {
		return "", errors.New("SendARP requires IPv4 addresses")
	}
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
	if r1 != 0 {
		return "", syscall.Errno(r1)
	}
	if length < 6 || int(length) > len(mac) {
		return "", fmt.Errorf("SendARP returned invalid MAC length %d", length)
	}
	return strings.ToLower(net.HardwareAddr(mac[:length]).String()), nil
}
