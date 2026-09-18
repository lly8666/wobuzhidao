//go:build windows

package windowsruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	wbdNRPTRootService = `SYSTEM\CurrentControlSet\Services\Dnscache\Parameters\DnsPolicyConfig`
	wbdNRPTRootPolicy  = `SOFTWARE\Policies\Microsoft\Windows NT\DNSClient\DnsPolicyConfig`
	wbdNRPTKeyName     = "{A7D4A733-BE89-44B2-9E0A-3D489F9F09C2}"
	wbdNRPTDisplayName = "WBD Runtime DNS"
	wbdNRPTComment     = "wbd-owned-runtime-dns/v2"

	ipv6RuleOutbound = "WBD Block IPv6 Outbound"
	ipv6RuleInbound  = "WBD Block IPv6 Inbound"

	npcapVersion = "1.88"
	npcapURL     = "https://npcap.com/dist/npcap-1.88.exe"
	npcapSHA256  = "a2f4ec1e5ea353ff67efd24b2ebf081ba44532410fae8d5e146af0310aa4f56b"
)

type portableRouteSpec struct {
	DestinationPrefix string `json:"DestinationPrefix"`
	InterfaceIndex    uint32 `json:"InterfaceIndex"`
	NextHop           string `json:"NextHop"`
	Metric            uint32 `json:"Metric,omitempty"`
}

type portableAddressSpec struct {
	InterfaceIndex uint32 `json:"InterfaceIndex"`
	AdapterAlias   string `json:"AdapterAlias,omitempty"`
	IPAddress      string `json:"IPAddress"`
}

type portableRouteState struct {
	Schema                string                `json:"Schema"`
	AdapterAlias          string                `json:"AdapterAlias"`
	AdapterInterfaceIndex uint32                `json:"AdapterInterfaceIndex"`
	NRPTRuleKey           string                `json:"NRPTRuleKey,omitempty"`
	Addresses             []portableAddressSpec `json:"Addresses"`
	UnderlayRoutes        []portableRouteSpec   `json:"UnderlayRoutes"`
	DirectRoutes          []portableRouteSpec   `json:"DirectRoutes"`
	CaptureRoutes         []portableRouteSpec   `json:"CaptureRoutes"`
}

func RunWindowsNetControl(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: wbd-win-net <route|rebind|ipv6|npcap> ...")
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "route":
		return runPortableRoute(args[1:])
	case "rebind":
		return runPortableRebind(args[1:])
	case "ipv6":
		return runPortableIPv6(args[1:])
	case "npcap":
		return runPortableNpcap(args[1:])
	default:
		return fmt.Errorf("unknown wbd-win-net command %q", args[0])
	}
}

func requireWindowsAdmin() error {
	token := windows.GetCurrentProcessToken()
	if !token.IsElevated() {
		return errors.New("administrator privileges are required")
	}
	return nil
}

func hiddenCommand(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	return cmd
}

func runSystemCommand(name string, args ...string) error {
	cmd := hiddenCommand(name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func waitInterfaceByName(name string, timeout time.Duration) (*net.Interface, error) {
	deadline := time.Now().Add(timeout)
	for {
		iface, err := net.InterfaceByName(name)
		if err == nil && iface != nil && iface.Index > 0 {
			return iface, nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("adapter %q did not appear within %s", name, timeout)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func parseIPv4Prefix(value, label string) (netip.Prefix, error) {
	p, err := netip.ParsePrefix(strings.TrimSpace(value))
	if err != nil || !p.Addr().Is4() {
		return netip.Prefix{}, fmt.Errorf("%s must be IPv4 CIDR: %q", label, value)
	}
	return p.Masked(), nil
}

func parseIPv4(value, label string) (netip.Addr, error) {
	ip, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || !ip.Is4() {
		return netip.Addr{}, fmt.Errorf("%s must be IPv4: %q", label, value)
	}
	return ip.Unmap(), nil
}

func ipv4Mask(bits int) string {
	var v uint32
	if bits > 0 {
		v = ^uint32(0) << uint(32-bits)
	}
	return fmt.Sprintf("%d.%d.%d.%d", byte(v>>24), byte(v>>16), byte(v>>8), byte(v))
}

func addIPv4Route(spec portableRouteSpec) error {
	if _, err := parseIPv4Prefix(spec.DestinationPrefix, "route prefix"); err != nil {
		return err
	}
	if _, err := parseIPv4(spec.NextHop, "route next hop"); err != nil {
		return err
	}
	args := []string{"interface", "ipv4", "add", "route",
		"prefix=" + spec.DestinationPrefix,
		"interface=" + strconv.FormatUint(uint64(spec.InterfaceIndex), 10),
		"nexthop=" + spec.NextHop,
		"metric=" + strconv.FormatUint(uint64(spec.Metric), 10),
		"store=active",
	}
	return runSystemCommand("netsh.exe", args...)
}

func deleteIPv4Route(spec portableRouteSpec) error {
	args := []string{"interface", "ipv4", "delete", "route",
		"prefix=" + spec.DestinationPrefix,
		"interface=" + strconv.FormatUint(uint64(spec.InterfaceIndex), 10),
		"nexthop=" + spec.NextHop,
		"store=active",
	}
	cmd := hiddenCommand("netsh.exe", args...)
	_, _ = cmd.CombinedOutput() // cleanup is deliberately idempotent
	return nil
}

func configureTunnelIPv4(alias string, prefix netip.Prefix) error {
	args := []string{"interface", "ipv4", "set", "address",
		"name=" + alias,
		"source=static",
		"address=" + prefix.Addr().String(),
		"mask=" + ipv4Mask(prefix.Bits()),
		"gateway=none",
		"store=active",
	}
	if err := runSystemCommand("netsh.exe", args...); err != nil {
		return err
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		iface, err := net.InterfaceByName(alias)
		if err == nil {
			addrs, _ := iface.Addrs()
			for _, a := range addrs {
				if p, err := netip.ParsePrefix(a.String()); err == nil && p.Addr().Unmap() == prefix.Addr() {
					fmt.Printf("WBD_WINDOWS_TUN_ADDRESS_READY family=IPv4 ip=%s state=Preferred\n", prefix.Addr())
					fmt.Printf("WBD_WINDOWS_TUN_ADDRESS_EXCLUSIVE ifindex=%d address4=%s dhcp=disabled\n", iface.Index, prefix.Addr())
					return nil
				}
			}
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("WBD tunnel address %s did not become visible within 10s", prefix.Addr())
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func removeTunnelIPv4(alias, ip string) {
	if strings.TrimSpace(alias) == "" || strings.TrimSpace(ip) == "" {
		return
	}
	cmd := hiddenCommand("netsh.exe", "interface", "ipv4", "delete", "address", "name="+alias, "address="+ip, "store=active")
	_, _ = cmd.CombinedOutput()
}

func savePortableRouteState(path string, state portableRouteState) error {
	if strings.TrimSpace(path) == "" {
		return errors.New("route state path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

func loadPortableRouteState(path string) (portableRouteState, error) {
	var state portableRouteState
	raw, err := os.ReadFile(path)
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, err
	}
	return state, nil
}

func cleanupPortableRouteState(path string) error {
	state, err := loadPortableRouteState(path)
	if errors.Is(err, os.ErrNotExist) {
		_ = removeWBDNRPTRules()
		fmt.Println("WBD_WINDOWS_TUN_CLEAN state=absent stale_nrpt_removed=1")
		return nil
	}
	if err != nil {
		return fmt.Errorf("decode route state: %w", err)
	}
	if state.NRPTRuleKey != "" {
		_ = deleteNRPTRuleKey(wbdNRPTRootService, state.NRPTRuleKey)
	}
	_ = removeWBDNRPTRules()
	for i := len(state.CaptureRoutes) - 1; i >= 0; i-- {
		_ = deleteIPv4Route(state.CaptureRoutes[i])
	}
	for i := len(state.DirectRoutes) - 1; i >= 0; i-- {
		_ = deleteIPv4Route(state.DirectRoutes[i])
	}
	for i := len(state.UnderlayRoutes) - 1; i >= 0; i-- {
		_ = deleteIPv4Route(state.UnderlayRoutes[i])
	}
	for _, addr := range state.Addresses {
		removeTunnelIPv4(addr.AdapterAlias, addr.IPAddress)
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	fmt.Println("WBD_WINDOWS_TUN_CLEANUP_PASS")
	return nil
}

func runPortableRoute(args []string) error {
	if len(args) == 0 {
		return errors.New("route action is required")
	}
	action := strings.ToLower(args[0])
	fs := flag.NewFlagSet("route "+action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	adapter := fs.String("adapter", "WBD", "Wintun adapter alias")
	tunnel4 := fs.String("tunnel-address4", "", "server-assigned Wintun IPv4 CIDR")
	underlay4 := fs.String("underlay4", "", "public WBD server IPv4")
	physicalIf := fs.Uint("physical-ifindex", 0, "physical interface index")
	physicalNext := fs.String("physical-next-hop4", "", "physical IPv4 next hop")
	captureLAN := fs.Bool("capture-lan", true, "capture RFC1918 into Wintun for in-TUN classification")
	dnsText := fs.String("dns-server", "", "comma/semicolon-separated NRPT resolver list")
	statePath := fs.String("state", "", "portable route state path")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if action == "render" {
		fmt.Printf("WBD_WINDOWS_TUN_PLAN mode=Full adapter=%s capture_lan=%d control=program\n", *adapter, boolToInt(*captureLAN))
		return nil
	}
	if err := requireWindowsAdmin(); err != nil {
		return err
	}
	if action == "cleanup" {
		return cleanupPortableRouteState(*statePath)
	}
	if action != "apply" {
		return fmt.Errorf("unsupported route action %q", action)
	}
	if *physicalIf == 0 {
		return errors.New("physical-ifindex is required")
	}
	tunnelPrefix, err := parseIPv4Prefix(*tunnel4, "tunnel-address4")
	if err != nil {
		return err
	}
	serverIP, err := parseIPv4(*underlay4, "underlay4")
	if err != nil {
		return err
	}
	nextHop, err := parseIPv4(*physicalNext, "physical-next-hop4")
	if err != nil {
		return err
	}
	_ = cleanupPortableRouteState(*statePath)
	iface, err := waitInterfaceByName(*adapter, 10*time.Second)
	if err != nil {
		return err
	}
	if uint64(iface.Index) > uint64(^uint32(0)) {
		return errors.New("Wintun interface index out of range")
	}
	state := portableRouteState{
		Schema: "wbd-windows-route-state/v5-program",
		AdapterAlias: *adapter,
		AdapterInterfaceIndex: uint32(iface.Index),
	}
	if err := savePortableRouteState(*statePath, state); err != nil {
		return err
	}
	fmt.Printf("WBD_WINDOWS_TUN_ADAPTER_READY adapter=%s ifindex=%d\n", *adapter, iface.Index)
	fmt.Printf("WBD_WINDOWS_TUN_ROUTE_STATE_READY path=%s ifindex=%d\n", *statePath, iface.Index)

	underlay := portableRouteSpec{
		DestinationPrefix: serverIP.String() + "/32",
		InterfaceIndex: uint32(*physicalIf),
		NextHop: nextHop.String(),
		Metric: 1,
	}
	state.UnderlayRoutes = append(state.UnderlayRoutes, underlay)
	if err := savePortableRouteState(*statePath, state); err != nil {
		return err
	}
	if err := addIPv4Route(underlay); err != nil {
		return err
	}
	fmt.Println("WBD_WINDOWS_TUN_UNDERLAY_ROUTES_READY underlay4=1 underlay6=0")

	if err := configureTunnelIPv4(*adapter, tunnelPrefix); err != nil {
		return err
	}
	state.Addresses = append(state.Addresses, portableAddressSpec{InterfaceIndex: uint32(iface.Index), AdapterAlias: *adapter, IPAddress: tunnelPrefix.Addr().String()})
	if err := savePortableRouteState(*statePath, state); err != nil {
		return err
	}

	capture := []string{"0.0.0.0/1", "128.0.0.0/1"}
	if *captureLAN {
		capture = append(capture, "10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16")
	}
	for _, p := range capture {
		spec := portableRouteSpec{DestinationPrefix: p, InterfaceIndex: uint32(iface.Index), NextHop: "0.0.0.0", Metric: 5}
		state.CaptureRoutes = append(state.CaptureRoutes, spec)
		if err := savePortableRouteState(*statePath, state); err != nil {
			return err
		}
		if err := addIPv4Route(spec); err != nil {
			return err
		}
	}
	fmt.Printf("WBD_WINDOWS_TUN_CAPTURE_ROUTES_READY total=%d\n", len(state.CaptureRoutes))

	dns := splitDNSList(*dnsText)
	if len(dns) > 0 {
		for _, s := range dns {
			if _, err := parseIPv4(s, "dns-server"); err != nil {
				return err
			}
		}
		if err := installWBDNRPTRule(dns); err != nil {
			return err
		}
		state.NRPTRuleKey = wbdNRPTKeyName
		if err := savePortableRouteState(*statePath, state); err != nil {
			return err
		}
		fmt.Printf("WBD_WINDOWS_TUN_DNS_READY nrpt=1 servers=%s\n", strings.Join(dns, ","))
	}
	fmt.Printf("WBD_WINDOWS_TUN_READY adapter=%s ifindex=%d mode=Full capture_lan=%d control=program\n", *adapter, iface.Index, boolToInt(*captureLAN))
	return nil
}

func splitDNSList(value string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ';' }) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

func installWBDNRPTRule(servers []string) error {
	if err := removeWBDNRPTRules(); err != nil {
		return err
	}
	full := wbdNRPTRootService + `\` + wbdNRPTKeyName
	k, _, err := registry.CreateKey(registry.LOCAL_MACHINE, full, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringsValue("Name", []string{"."}); err != nil { return err }
	if err := k.SetDWordValue("Version", 1); err != nil { return err }
	if err := k.SetDWordValue("ConfigOptions", 8); err != nil { return err }
	if err := k.SetStringValue("GenericDNSServers", strings.Join(servers, ";")); err != nil { return err }
	if err := k.SetDWordValue("VpnRequired", 1); err != nil { return err }
	_ = k.SetStringValue("DisplayName", wbdNRPTDisplayName)
	_ = k.SetStringValue("Comment", wbdNRPTComment)
	_ = hiddenCommand("ipconfig.exe", "/flushdns").Run()
	return nil
}

func removeWBDNRPTRules() error {
	for _, root := range []string{wbdNRPTRootService, wbdNRPTRootPolicy} {
		parent, err := registry.OpenKey(registry.LOCAL_MACHINE, root, registry.ENUMERATE_SUB_KEYS|registry.QUERY_VALUE|registry.SET_VALUE)
		if err != nil {
			continue
		}
		names, _ := parent.ReadSubKeyNames(-1)
		parent.Close()
		for _, name := range names {
			if strings.EqualFold(name, wbdNRPTKeyName) {
				_ = deleteNRPTRuleKey(root, name)
				continue
			}
			k, err := registry.OpenKey(registry.LOCAL_MACHINE, root+`\`+name, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			display, _, _ := k.GetStringValue("DisplayName")
			comment, _, _ := k.GetStringValue("Comment")
			k.Close()
			if display == wbdNRPTDisplayName || strings.HasPrefix(comment, "wbd-owned-runtime-dns/") {
				_ = deleteNRPTRuleKey(root, name)
			}
		}
	}
	_ = hiddenCommand("ipconfig.exe", "/flushdns").Run()
	return nil
}

func deleteNRPTRuleKey(root, name string) error {
	err := registry.DeleteKey(registry.LOCAL_MACHINE, root+`\`+name)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return nil
	}
	return err
}

func runPortableRebind(args []string) error {
	fs := flag.NewFlagSet("rebind", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	underlay4 := fs.String("underlay4", "", "WBD server IPv4")
	physicalIf := fs.Uint("physical-ifindex", 0, "new physical interface index")
	nextHop4 := fs.String("physical-next-hop4", "", "new physical next hop")
	statePath := fs.String("state", "", "route state path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := requireWindowsAdmin(); err != nil {
		return err
	}
	serverIP, err := parseIPv4(*underlay4, "underlay4")
	if err != nil { return err }
	nextHop, err := parseIPv4(*nextHop4, "physical-next-hop4")
	if err != nil { return err }
	if *physicalIf == 0 { return errors.New("physical-ifindex is required") }
	state, err := loadPortableRouteState(*statePath)
	if err != nil { return err }

	target := portableRouteSpec{
		DestinationPrefix: serverIP.String()+"/32",
		InterfaceIndex: uint32(*physicalIf),
		NextHop: nextHop.String(),
		Metric: 1,
	}
	if len(state.UnderlayRoutes) == 1 &&
		state.UnderlayRoutes[0].DestinationPrefix == target.DestinationPrefix &&
		state.UnderlayRoutes[0].InterfaceIndex == target.InterfaceIndex &&
		state.UnderlayRoutes[0].NextHop == target.NextHop {
		fmt.Printf("WBD_WINDOWS_TUN_REBIND_PASS underlay4=%s ifindex=%d next_hop=%s created=0 retired=0\n", serverIP, *physicalIf, nextHop)
		return nil
	}

	old := append([]portableRouteSpec(nil), state.UnderlayRoutes...)
	state.UnderlayRoutes = append(append([]portableRouteSpec(nil), old...), target)
	if err := savePortableRouteState(*statePath, state); err != nil { return err }
	if err := addIPv4Route(target); err != nil {
		state.UnderlayRoutes = old
		_ = savePortableRouteState(*statePath, state)
		return err
	}
	for _, spec := range old {
		_ = deleteIPv4Route(spec)
	}
	state.UnderlayRoutes = []portableRouteSpec{target}
	if err := savePortableRouteState(*statePath, state); err != nil { return err }
	fmt.Printf("WBD_WINDOWS_TUN_REBIND_PASS underlay4=%s ifindex=%d next_hop=%s direct4=0 created=1 retired=%d\n", serverIP, *physicalIf, nextHop, len(old))
	return nil
}

func runPortableIPv6(args []string) error {
	if len(args) == 0 {
		return errors.New("ipv6 action is required")
	}
	action := strings.ToLower(args[0])
	if action == "render" {
		fmt.Println("WBD_WINDOWS_IPV6_KILLSWITCH_PLAN scope=device directions=inbound,outbound range=::/1,8000::/1 restore=remove_wbd_owned_rules control=program")
		return nil
	}
	if err := requireWindowsAdmin(); err != nil { return err }
	if action == "cleanup" {
		removeIPv6FirewallRules()
		fmt.Println("WBD_WINDOWS_IPV6_KILLSWITCH_CLEANUP_PASS")
		return nil
	}
	if action != "apply" {
		return fmt.Errorf("unsupported ipv6 action %q", action)
	}
	if err := requireFirewallProfilesEnabled(); err != nil { return err }
	removeIPv6FirewallRules()
	if err := runSystemCommand("netsh.exe", "advfirewall", "firewall", "add", "rule",
		"name="+ipv6RuleOutbound, "dir=out", "action=block", "enable=yes", "profile=any",
		"protocol=any", "remoteip=::/1,8000::/1"); err != nil {
		removeIPv6FirewallRules()
		return err
	}
	if err := runSystemCommand("netsh.exe", "advfirewall", "firewall", "add", "rule",
		"name="+ipv6RuleInbound, "dir=in", "action=block", "enable=yes", "profile=any",
		"protocol=any", "localip=::/1,8000::/1"); err != nil {
		removeIPv6FirewallRules()
		return err
	}
	fmt.Println("WBD_WINDOWS_IPV6_KILLSWITCH_READY scope=device ipv6=blocked directions=inbound,outbound control=program")
	return nil
}

func removeIPv6FirewallRules() {
	for _, name := range []string{ipv6RuleOutbound, ipv6RuleInbound} {
		cmd := hiddenCommand("netsh.exe", "advfirewall", "firewall", "delete", "rule", "name="+name)
		_, _ = cmd.CombinedOutput()
	}
}

func requireFirewallProfilesEnabled() error {
	base := `SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy`
	for _, profile := range []string{"DomainProfile", "StandardProfile", "PublicProfile"} {
		k, err := registry.OpenKey(registry.LOCAL_MACHINE, base+`\`+profile, registry.QUERY_VALUE)
		if err != nil {
			return fmt.Errorf("read Windows Firewall %s state: %w", profile, err)
		}
		enabled, _, err := k.GetIntegerValue("EnableFirewall")
		k.Close()
		if err != nil {
			return fmt.Errorf("read Windows Firewall %s EnableFirewall: %w", profile, err)
		}
		if enabled == 0 {
			return fmt.Errorf("Windows Firewall profile %s is disabled; refusing to connect because IPv6 could bypass WBD", profile)
		}
	}
	return nil
}

func runPortableNpcap(args []string) error {
	if len(args) == 0 {
		return errors.New("npcap action is required")
	}
	action := strings.ToLower(args[0])
	fs := flag.NewFlagSet("npcap "+action, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	downloadDir := fs.String("download-dir", "", "portable download directory")
	if err := fs.Parse(args[1:]); err != nil { return err }
	switch action {
	case "status":
		state, err := ensureNpcapReady()
		if err != nil { return err }
		fmt.Printf("WBD_WINDOWS_NPCAP_READY version=%s service=%s wpcap=%s control=program\n", npcapVersion, state, npcapRuntimePaths()[0])
		return nil
	case "fetch":
		path, hash, err := fetchNpcapInstaller(*downloadDir)
		if err != nil { return err }
		fmt.Printf("WBD_WINDOWS_NPCAP_FETCH_PASS version=%s signer=authenticode-trusted sha256=%s path=%s\n", npcapVersion, hash, path)
		return nil
	case "install":
		path, hash, err := fetchNpcapInstaller(*downloadDir)
		if err != nil { return err }
		fmt.Printf("WBD_WINDOWS_NPCAP_FETCH_PASS version=%s signer=authenticode-trusted sha256=%s path=%s\n", npcapVersion, hash, path)
		cmd := exec.Command(path)
		err = cmd.Run()
		exitCode := 0
		if err != nil {
			var ee *exec.ExitError
			if errors.As(err, &ee) {
				exitCode = ee.ExitCode()
			} else {
				return err
			}
		}
		if exitCode != 0 && exitCode != 3010 {
			return fmt.Errorf("Npcap installer exited %d", exitCode)
		}
		state, err := ensureNpcapReady()
		if err != nil { return err }
		fmt.Printf("WBD_WINDOWS_NPCAP_READY version=%s service=%s wpcap=%s control=program\n", npcapVersion, state, npcapRuntimePaths()[0])
		if exitCode == 3010 { fmt.Println("WBD_WINDOWS_NPCAP_REBOOT_REQUIRED") }
		fmt.Printf("WBD_WINDOWS_NPCAP_INSTALL_PASS version=%s\n", npcapVersion)
		return nil
	default:
		return fmt.Errorf("unsupported npcap action %q", action)
	}
}

func npcapRuntimePaths() []string {
	root := os.Getenv("SystemRoot")
	if root == "" { root = `C:\Windows` }
	dir := filepath.Join(root, "System32", "Npcap")
	return []string{filepath.Join(dir, "wpcap.dll"), filepath.Join(dir, "Packet.dll")}
}

func ensureNpcapReady() (string, error) {
	for _, path := range npcapRuntimePaths() {
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("Npcap %s runtime is not ready: %s: %w", npcapVersion, path, err)
		}
		if err := verifyAuthenticode(path); err != nil {
			return "", fmt.Errorf("verify Npcap runtime %s: %w", path, err)
		}
	}
	m, err := mgr.Connect()
	if err != nil { return "", err }
	defer m.Disconnect()
	svc, err := m.OpenService("npcap")
	if err != nil {
		return "", fmt.Errorf("Npcap driver service is missing: %w", err)
	}
	defer svc.Close()
	status, err := svc.Query()
	if err != nil { return "", err }
	return fmt.Sprint(status.State), nil
}

func fetchNpcapInstaller(downloadDir string) (string, string, error) {
	if strings.TrimSpace(downloadDir) == "" {
		return "", "", errors.New("portable Npcap download directory is required")
	}
	if err := os.MkdirAll(downloadDir, 0o700); err != nil {
		return "", "", err
	}
	path := filepath.Join(downloadDir, "npcap-"+npcapVersion+".exe")
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Get(npcapURL)
	if err != nil { return "", "", err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", "", fmt.Errorf("Npcap download HTTP status %s", resp.Status)
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil { return "", "", err }
	h := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(f, h), resp.Body)
	closeErr := f.Close()
	if copyErr != nil { _ = os.Remove(tmp); return "", "", copyErr }
	if closeErr != nil { _ = os.Remove(tmp); return "", "", closeErr }
	got := hex.EncodeToString(h.Sum(nil))
	if got != npcapSHA256 {
		_ = os.Remove(tmp)
		return "", "", fmt.Errorf("Npcap %s installer SHA256 mismatch: got=%s want=%s", npcapVersion, got, npcapSHA256)
	}
	if err := verifyAuthenticode(tmp); err != nil {
		_ = os.Remove(tmp)
		return "", "", fmt.Errorf("Npcap installer Authenticode: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", "", err
	}
	return path, got, nil
}

func verifyAuthenticode(path string) error {
	path16, err := windows.UTF16PtrFromString(path)
	if err != nil { return err }
	fileInfo := &windows.WinTrustFileInfo{Size: uint32(unsafe.Sizeof(windows.WinTrustFileInfo{})), FilePath: path16}
	data := &windows.WinTrustData{
		Size: uint32(unsafe.Sizeof(windows.WinTrustData{})),
		UIChoice: windows.WTD_UI_NONE,
		RevocationChecks: windows.WTD_REVOKE_NONE,
		UnionChoice: windows.WTD_CHOICE_FILE,
		StateAction: windows.WTD_STATEACTION_VERIFY,
		FileOrCatalogOrBlobOrSgnrOrCert: unsafe.Pointer(fileInfo),
	}
	verifyErr := windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	data.StateAction = windows.WTD_STATEACTION_CLOSE
	_ = windows.WinVerifyTrustEx(windows.InvalidHWND, &windows.WINTRUST_ACTION_GENERIC_VERIFY_V2, data)
	return verifyErr
}
