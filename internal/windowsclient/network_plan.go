package windowsclient

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	StateSchema          = "wbd-windows-client-state/v1"
	NRPTDisplayName      = "WBD Runtime DNS"
	NRPTComment          = "wbd-owned-runtime-dns/v1"
	IPv6FirewallGroup    = "WBD Runtime IPv6 Kill Switch"
	IPv6FirewallComment  = "wbd-owned-runtime-ipv6-killswitch/v1"
)

var ErrNetworkPlan = errors.New("windowsclient: invalid network plan")

type PhysicalPath struct {
	InterfaceIndex uint32
	NextHop4       netip.Addr
}

type Config struct {
	AdapterAlias string
	Lease4       netip.Prefix
	Server4      netip.Addr
	Physical     PhysicalPath
	DNSServers4  []netip.Addr
	Direct4      []netip.Prefix
	StatePath    string
}

type OwnedRoute struct {
	Kind           string
	Prefix         netip.Prefix
	InterfaceIndex uint32
	NextHop        netip.Addr
	AdapterAlias   string
}

type NetworkPlan struct {
	Schema string

	AdapterAlias string
	Lease4       netip.Prefix
	Server4      netip.Addr
	Physical     PhysicalPath
	StatePath    string

	UnderlayRoute OwnedRoute
	DirectRoutes  []OwnedRoute
	CaptureRoutes []OwnedRoute
	DNSServers4   []netip.Addr

	NRPTNamespace string
	NRPTDisplay   string
	NRPTComment   string

	IPv6BlockPrefixes []netip.Prefix
	IPv6FirewallGroup string
	IPv6Comment       string
}

func BuildNetworkPlan(cfg Config) (NetworkPlan, error) {
	cfg.AdapterAlias = strings.TrimSpace(cfg.AdapterAlias)
	if cfg.AdapterAlias == "" || strings.ContainsAny(cfg.AdapterAlias, "\r\n") {
		return NetworkPlan{}, fmt.Errorf("%w: adapter alias", ErrNetworkPlan)
	}
	if !cfg.Lease4.IsValid() || !cfg.Lease4.Addr().Is4() || cfg.Lease4.Bits() != 32 {
		return NetworkPlan{}, fmt.Errorf("%w: lease must be IPv4 /32", ErrNetworkPlan)
	}
	if !cfg.Server4.IsValid() || !cfg.Server4.Is4() {
		return NetworkPlan{}, fmt.Errorf("%w: server IPv4", ErrNetworkPlan)
	}
	if cfg.Physical.InterfaceIndex == 0 || !cfg.Physical.NextHop4.IsValid() || !cfg.Physical.NextHop4.Is4() {
		return NetworkPlan{}, fmt.Errorf("%w: physical path", ErrNetworkPlan)
	}
	if strings.TrimSpace(cfg.StatePath) == "" {
		return NetworkPlan{}, fmt.Errorf("%w: state path", ErrNetworkPlan)
	}

	lease := netip.PrefixFrom(cfg.Lease4.Addr().Unmap(), 32)
	server := cfg.Server4.Unmap()
	nextHop := cfg.Physical.NextHop4.Unmap()
	underlay := OwnedRoute{
		Kind:           "underlay",
		Prefix:         netip.PrefixFrom(server, 32),
		InterfaceIndex: cfg.Physical.InterfaceIndex,
		NextHop:        nextHop,
	}

	directPrefixes, err := normalizePrefixes(cfg.Direct4)
	if err != nil {
		return NetworkPlan{}, err
	}
	direct := make([]OwnedRoute, 0, len(directPrefixes))
	for _, prefix := range directPrefixes {
		direct = append(direct, OwnedRoute{
			Kind:           "direct",
			Prefix:         prefix,
			InterfaceIndex: cfg.Physical.InterfaceIndex,
			NextHop:        nextHop,
		})
	}

	dns, err := normalizeAddrs(cfg.DNSServers4)
	if err != nil {
		return NetworkPlan{}, err
	}
	capturePrefixes := []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/1"),
		netip.MustParsePrefix("128.0.0.0/1"),
	}
	for _, addr := range dns {
		capturePrefixes = append(capturePrefixes, netip.PrefixFrom(addr, 32))
	}
	capturePrefixes, _ = normalizePrefixes(capturePrefixes)
	capture := make([]OwnedRoute, 0, len(capturePrefixes))
	for _, prefix := range capturePrefixes {
		capture = append(capture, OwnedRoute{
			Kind:         "capture",
			Prefix:       prefix,
			AdapterAlias: cfg.AdapterAlias,
		})
	}

	return NetworkPlan{
		Schema:            StateSchema,
		AdapterAlias:      cfg.AdapterAlias,
		Lease4:            lease,
		Server4:           server,
		Physical:          PhysicalPath{InterfaceIndex: cfg.Physical.InterfaceIndex, NextHop4: nextHop},
		StatePath:         filepath.Clean(cfg.StatePath),
		UnderlayRoute:     underlay,
		DirectRoutes:      direct,
		CaptureRoutes:     capture,
		DNSServers4:       dns,
		NRPTNamespace:     ".",
		NRPTDisplay:       NRPTDisplayName,
		NRPTComment:       NRPTComment,
		IPv6BlockPrefixes: []netip.Prefix{netip.MustParsePrefix("::/1"), netip.MustParsePrefix("8000::/1")},
		IPv6FirewallGroup: IPv6FirewallGroup,
		IPv6Comment:       IPv6FirewallComment,
	}, nil
}

func (p NetworkPlan) PowerShellArgs(action, scriptPath string) ([]string, error) {
	switch action {
	case "Render", "Apply", "Cleanup":
	default:
		return nil, ErrNetworkPlan
	}
	if strings.TrimSpace(scriptPath) == "" {
		return nil, ErrNetworkPlan
	}
	args := []string{
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", scriptPath,
		"-Action", action,
		"-AdapterAlias", p.AdapterAlias,
		"-TunnelAddress4", p.Lease4.String(),
		"-Underlay4", p.Server4.String(),
		"-PhysicalInterfaceIndex", strconv.FormatUint(uint64(p.Physical.InterfaceIndex), 10),
		"-PhysicalNextHop4", p.Physical.NextHop4.String(),
		"-StatePath", p.StatePath,
	}
	if len(p.DNSServers4) > 0 {
		values := make([]string, 0, len(p.DNSServers4))
		for _, addr := range p.DNSServers4 {
			values = append(values, addr.String())
		}
		args = append(args, "-DNSServer", strings.Join(values, ","))
	}
	if len(p.DirectRoutes) > 0 {
		values := make([]string, 0, len(p.DirectRoutes))
		for _, route := range p.DirectRoutes {
			values = append(values, route.Prefix.String())
		}
		args = append(args, "-DirectPrefix4", strings.Join(values, ","))
	}
	return args, nil
}

func normalizeAddrs(in []netip.Addr) ([]netip.Addr, error) {
	set := map[string]netip.Addr{}
	for _, addr := range in {
		if !addr.IsValid() || !addr.Is4() {
			return nil, fmt.Errorf("%w: non-IPv4 address", ErrNetworkPlan)
		}
		addr = addr.Unmap()
		set[addr.String()] = addr
	}
	keys := make([]string, 0, len(set))
	for k := range set { keys = append(keys, k) }
	sort.Strings(keys)
	out := make([]netip.Addr, 0, len(keys))
	for _, k := range keys { out = append(out, set[k]) }
	return out, nil
}

func normalizePrefixes(in []netip.Prefix) ([]netip.Prefix, error) {
	set := map[string]netip.Prefix{}
	for _, prefix := range in {
		if !prefix.IsValid() || !prefix.Addr().Is4() {
			return nil, fmt.Errorf("%w: non-IPv4 prefix", ErrNetworkPlan)
		}
		prefix = prefix.Masked()
		set[prefix.String()] = prefix
	}
	keys := make([]string, 0, len(set))
	for k := range set { keys = append(keys, k) }
	sort.Strings(keys)
	out := make([]netip.Prefix, 0, len(keys))
	for _, k := range keys { out = append(out, set[k]) }
	return out, nil
}
