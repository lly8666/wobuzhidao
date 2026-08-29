package windowsruntime

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/lly8666/wobuzhidao/internal/ipset"
)

const (
	defaultFakeTCPLocalPort  = 45101
	defaultFakeTCPSourcePort = 41001 // compatibility fallback; product Connect assigns a dynamic port
	windowsDynamicPortMin    = 49152
	windowsDynamicPortCount  = 16384
	defaultDTLSPlainPort     = 46101
	defaultLinkListenPort    = 47101
	defaultMTU               = 1400

	RouteFull    = "Full"
	RouteForeign = "Foreign"
	RouteChina   = "China"

	DNSAuto       = "Auto"
	DNSSystem     = "System"
	DNSCloudflare = "Cloudflare"
	DNSCustom     = "Custom"
)

var (
	fakeTCPPortSeedOnce sync.Once
	fakeTCPPortSeed     uint32
	fakeTCPPortCounter  atomic.Uint32
)

type Profile struct {
	BinDir       string
	// ServerFront is retained for config compatibility but V2.3 requires it to
	// equal ServerRaw. There is only one public endpoint/4-tuple lineage.
	ServerFront  string
	ServerName   string
	RouteKey     string
	Username     string
	Password     string
	ServerRaw    string
	VerifyServer bool
	FEC          string
	IfName       string
	MTU          int
	RouteMode    string
	Prefix4      []string
	CNSetDir     string
	DNSMode      string
	DNSServer    string
	TunnelIPv4   string
	TicketPath   string
	RouteState   string
}

type Underlay struct {
	SourceIP     string
	PacketDevice string
	SourceMAC    string
	NextHopMAC   string
	// SourcePort is a per-Connect TCP-shaped ephemeral source port. Zero is
	// accepted only for builders/tests that need the historical deterministic
	// fallback; product Controller.Connect always assigns a dynamic-range port.
	SourcePort uint16
}

type Command struct {
	Name string
	Path string
	Args []string
}

type Plan struct {
	// Bootstrap is diagnostic compatibility only. Product Connect never runs it;
	// TLS/Reality admission is performed by FakeTCP on the same public flow.
	Bootstrap    Command
	FakeTCP      Command
	DTLS         Command
	Link         Command
	TUN          Command
	IPv6Apply    Command
	RouteApply   Command
	RouteCleanup Command
	IPv6Cleanup  Command
	TicketPath   string
}

func (p Plan) ProcessSequence() []Command { return []Command{p.FakeTCP, p.DTLS, p.Link, p.TUN} }
func (p Plan) StartSequence() []Command { return []Command{p.FakeTCP, p.DTLS, p.Link, p.TUN, p.IPv6Apply, p.RouteApply} }

func (p Plan) StopSequence() []Command {
	return []Command{p.RouteCleanup, p.IPv6Cleanup, p.TUN, p.Link, p.DTLS, p.FakeTCP}
}

func (p Profile) normalized() Profile {
	if p.FEC == "" { p.FEC = "off" }
	if p.IfName == "" { p.IfName = "WBD" }
	if p.MTU == 0 { p.MTU = defaultMTU }
	if p.RouteMode == "" { p.RouteMode = RouteFull }
	if p.DNSMode == "" { p.DNSMode = DNSAuto }
	if p.TunnelIPv4 == "" { p.TunnelIPv4 = "10.66.0.2/30" }
	return p
}

func (p Profile) Validate() error {
	p = p.normalized()
	if strings.TrimSpace(p.BinDir) == "" { return errors.New("bin directory is required") }
	front, err := netip.ParseAddrPort(p.ServerFront)
	if err != nil || !front.Addr().Is4() { return errors.New("server front must be an IPv4 address:port") }
	if strings.TrimSpace(p.ServerName) == "" { return errors.New("server name is required") }
	if len(p.RouteKey) < 16 { return errors.New("route key must be at least 16 bytes") }
	if p.Username == "" || p.Password == "" { return errors.New("username and password are required") }
	raw, err := netip.ParseAddrPort(p.ServerRaw)
	if err != nil || !raw.Addr().Is4() { return errors.New("server raw must be an IPv4 address:port") }
	if front != raw { return errors.New("V2.3 single-flow requires server front and raw endpoints to be identical") }
	if p.FEC != "off" && p.FEC != "20:20" { return errors.New("FEC must be off or 20:20") }
	if p.MTU < 576 || p.MTU > 9000 { return errors.New("MTU must be 576..9000") }
	if p.RouteMode != RouteFull && p.RouteMode != RouteForeign && p.RouteMode != RouteChina { return errors.New("route mode must be Full, Foreign, or China") }
	if (p.RouteMode == RouteForeign || p.RouteMode == RouteChina) && strings.TrimSpace(p.CNSetDir) == "" { return errors.New("China/Foreign route mode requires the WBD CN ipset directory") }
	for _, prefix := range p.Prefix4 {
		px, err := netip.ParsePrefix(prefix)
		if err != nil || !px.Addr().Is4() { return fmt.Errorf("invalid IPv4 capture prefix %q", prefix) }
	}
	if p.DNSMode != DNSAuto && p.DNSMode != DNSSystem && p.DNSMode != DNSCloudflare && p.DNSMode != DNSCustom { return errors.New("DNS mode must be Auto, System, Cloudflare, or Custom") }
	if p.DNSMode == DNSCustom {
		ip, err := netip.ParseAddr(strings.TrimSpace(p.DNSServer))
		if err != nil || !ip.Is4() { return errors.New("custom DNS server must be one IPv4 address") }
	}
	if px, err := netip.ParsePrefix(p.TunnelIPv4); err != nil || !px.Addr().Is4() { return errors.New("tunnel IPv4 must be an IPv4 CIDR") }
	if strings.TrimSpace(p.TicketPath) == "" || strings.TrimSpace(p.RouteState) == "" { return errors.New("ticket and route-state paths are required") }
	return nil
}

func ValidateRoutingAssets(profile Profile) error {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil { return err }
	if profile.RouteMode != RouteForeign && profile.RouteMode != RouteChina { return nil }
	if _, err := ipset.VerifyCNBundle(profile.CNSetDir); err != nil { return fmt.Errorf("verify WBD CN ipset: %w", err) }
	return nil
}

func (u Underlay) Validate() error {
	if ip, err := netip.ParseAddr(u.SourceIP); err != nil || !ip.Is4() { return errors.New("underlay source IP must be IPv4") }
	if !strings.HasPrefix(u.PacketDevice, `\Device\NPF_{`) || !strings.HasSuffix(u.PacketDevice, "}") { return errors.New("underlay packet device must be an Npcap device") }
	if !validMAC(u.SourceMAC) || !validMAC(u.NextHopMAC) { return errors.New("underlay source and next-hop MACs are required") }
	if u.SourcePort != 0 && (u.SourcePort < windowsDynamicPortMin || int(u.SourcePort) >= windowsDynamicPortMin+windowsDynamicPortCount) {
		return errors.New("underlay FakeTCP source port must be in the Windows dynamic TCP port range")
	}
	return nil
}

// nextFakeTCPSourcePort returns a normal Windows dynamic-range source port and
// guarantees no immediate reuse inside one wbd.exe process until all 16384
// values have been cycled. The random seed changes the starting point after a
// process restart. The port is connection metadata only; it does not change the
// frozen FakeTCP recovery/FEC semantics.
func nextFakeTCPSourcePort() uint16 {
	fakeTCPPortSeedOnce.Do(func() {
		var b [2]byte
		if _, err := rand.Read(b[:]); err == nil {
			fakeTCPPortSeed = uint32(binary.BigEndian.Uint16(b[:]) & (windowsDynamicPortCount - 1))
		}
	})
	n := fakeTCPPortCounter.Add(1) - 1
	return uint16(windowsDynamicPortMin + int((fakeTCPPortSeed+n)&(windowsDynamicPortCount-1)))
}

// BuildBootstrap remains available only for diagnostics/backward-compatible
// tooling. Product Controller.Connect must never invoke it.
func BuildBootstrap(profile Profile) (Command, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil { return Command{}, err }
	return buildBootstrapCommand(profile), nil
}

func buildBootstrapCommand(profile Profile) Command {
	return Command{
		Name: "reality-bootstrap-diagnostic",
		Path: filepath.Join(profile.BinDir, "wbd-reality-front.exe"),
		Args: []string{"client", "-addr", profile.ServerFront, "-server-name", profile.ServerName, "-route-key", profile.RouteKey, "-username", profile.Username, "-password", profile.Password, "-verify-server=" + strconv.FormatBool(profile.VerifyServer), "-ticket-out", profile.TicketPath},
	}
}

// BuildFakeTCPCommand creates the unique public transport process. Reality-like
// TLS/auth parameters are consumed inside this process after its raw handshake,
// so no other public connection is created to obtain the ticket.
func BuildFakeTCPCommand(profile Profile, underlay Underlay) (Command, error) {
	profile = profile.normalized()
	if err := ValidateRoutingAssets(profile); err != nil { return Command{}, err }
	if err := underlay.Validate(); err != nil { return Command{}, err }
	raw, _ := netip.ParseAddrPort(profile.ServerRaw)
	bin := func(name string) string { return filepath.Join(profile.BinDir, name) }
	loop := func(port int) string { return "127.0.0.1:" + strconv.Itoa(port) }
	sourcePort := underlay.SourcePort
	if sourcePort == 0 { sourcePort = defaultFakeTCPSourcePort }
	args := []string{
		"client",
		"--local-udp", loop(defaultFakeTCPLocalPort),
		"--source", netip.AddrPortFrom(netip.MustParseAddr(underlay.SourceIP), sourcePort).String(),
		"--remote", raw.String(),
		"--shadow-recovery", "legacy",
		"--packet-device", underlay.PacketDevice,
		"--source-mac", underlay.SourceMAC,
		"--next-hop-mac", underlay.NextHopMAC,
		"--reality-server-name", profile.ServerName,
		"--reality-route-key", profile.RouteKey,
		"--reality-username", profile.Username,
		"--reality-password", profile.Password,
		"--reality-ticket-out", profile.TicketPath,
		"--reality-verify-server=" + strconv.FormatBool(profile.VerifyServer),
	}
	return Command{Name: "faketcp", Path: bin("wbd-faketcp.exe"), Args: args}, nil
}

func BuildPlan(profile Profile, underlay Underlay, ticket string) (Plan, error) {
	profile = profile.normalized()
	if err := ValidateRoutingAssets(profile); err != nil { return Plan{}, err }
	if err := underlay.Validate(); err != nil { return Plan{}, err }
	if len(strings.TrimSpace(ticket)) != 64 { return Plan{}, errors.New("Reality ticket must be 64 hex characters") }
	for _, c := range ticket {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) { return Plan{}, errors.New("Reality ticket must be hexadecimal") }
	}

	raw, _ := netip.ParseAddrPort(profile.ServerRaw)
	bin := func(name string) string { return filepath.Join(profile.BinDir, name) }
	loop := func(port int) string { return "127.0.0.1:" + strconv.Itoa(port) }
	psScript := func(name, script, action string) Command { return Command{Name: name, Path: "powershell.exe", Args: []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", bin(script), "-Action", action}} }

	fake, err := BuildFakeTCPCommand(profile, underlay)
	if err != nil { return Plan{}, err }
	dtlsArgs := []string{"client", strconv.Itoa(defaultDTLSPlainPort), "127.0.0.1", strconv.Itoa(defaultFakeTCPLocalPort), "none", "none"}
	linkArgs := []string{"-mode", "client", "-listen", loop(defaultLinkListenPort), "-dtls", loop(defaultDTLSPlainPort), "-fec", profile.FEC, "-mtu", strconv.Itoa(profile.MTU), "-lanes", "1", "-demo-reality-ticket", strings.TrimSpace(ticket)}
	tunArgs := []string{"-mode", "client", "-ifname", profile.IfName, "-mtu", strconv.Itoa(profile.MTU), "-transport", loop(defaultLinkListenPort)}

	psMode := "Full"
	var prefixFile, directFile string
	switch profile.RouteMode {
	case RouteForeign:
		directFile = filepath.Join(profile.CNSetDir, ipset.CNIPv4File)
	case RouteChina:
		psMode = "Split"
		prefixFile = filepath.Join(profile.CNSetDir, ipset.CNIPv4File)
	}
	dnsServers := resolvedDNSServers(profile)

	routeArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", bin("windows_tun_route.ps1"), "-Action", "Apply", "-Mode", psMode, "-AdapterAlias", profile.IfName, "-TunnelAddress4", profile.TunnelIPv4, "-Underlay4", raw.Addr().String(), "-MTU", strconv.Itoa(profile.MTU), "-StatePath", profile.RouteState}
	if prefixFile != "" { routeArgs = append(routeArgs, "-PrefixFile4", prefixFile) }
	if directFile != "" { routeArgs = append(routeArgs, "-DirectPrefixFile4", directFile) }
	if len(dnsServers) > 0 { routeArgs = append(routeArgs, "-DNSServer", strings.Join(dnsServers, ",")) }
	cleanupArgs := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", bin("windows_tun_route.ps1"), "-Action", "Cleanup", "-StatePath", profile.RouteState}

	return Plan{
		Bootstrap:    buildBootstrapCommand(profile),
		FakeTCP:      fake,
		DTLS:         Command{Name: "dtls", Path: bin("wbd_dtls_shim.exe"), Args: dtlsArgs},
		Link:         Command{Name: "link", Path: bin("wbd-link-proxy.exe"), Args: linkArgs},
		TUN:          Command{Name: "tun", Path: bin("wbd-tun.exe"), Args: tunArgs},
		IPv6Apply:    psScript("ipv6-apply", "windows_ipv6_killswitch.ps1", "Apply"),
		RouteApply:   Command{Name: "route-apply", Path: "powershell.exe", Args: routeArgs},
		RouteCleanup: Command{Name: "route-cleanup", Path: "powershell.exe", Args: cleanupArgs},
		IPv6Cleanup:  psScript("ipv6-cleanup", "windows_ipv6_killswitch.ps1", "Cleanup"),
		TicketPath:   profile.TicketPath,
	}, nil
}

func resolvedDNSServers(profile Profile) []string {
	profile = profile.normalized()
	switch profile.DNSMode {
	case DNSSystem: return nil
	case DNSCloudflare: return []string{"1.1.1.1", "1.0.0.1"}
	case DNSCustom: return []string{strings.TrimSpace(profile.DNSServer)}
	case DNSAuto:
		if profile.RouteMode == RouteChina { return nil }
		return []string{"1.1.1.1", "1.0.0.1"}
	default: return nil
	}
}

func validMAC(s string) bool {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), ":")
	if len(parts) != 6 { return false }
	for _, part := range parts {
		if len(part) != 2 { return false }
		for _, c := range part { if !strings.ContainsRune("0123456789abcdef", c) { return false } }
	}
	return true
}