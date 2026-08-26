package windowsruntime

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	defaultFakeTCPLocalPort = 45101
	defaultFakeTCPSourcePort = 41001
	defaultDTLSPlainPort     = 46101
	defaultLinkListenPort    = 47101
	defaultMTU               = 1400
)

type Profile struct {
	BinDir       string
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
	TunnelIPv4   string
	TicketPath   string
	RouteState   string
}

type Underlay struct {
	SourceIP    string
	PacketDevice string
	SourceMAC   string
	NextHopMAC  string
}

type Command struct {
	Name string
	Path string
	Args []string
}

type Plan struct {
	Bootstrap Command
	FakeTCP   Command
	DTLS      Command
	Link      Command
	TUN       Command
	RouteApply Command
	RouteCleanup Command
	TicketPath string
}

func (p Profile) normalized() Profile {
	if p.FEC == "" {
		p.FEC = "off"
	}
	if p.IfName == "" {
		p.IfName = "WBD"
	}
	if p.MTU == 0 {
		p.MTU = defaultMTU
	}
	if p.RouteMode == "" {
		p.RouteMode = "Full"
	}
	if p.TunnelIPv4 == "" {
		p.TunnelIPv4 = "10.66.0.2/30"
	}
	return p
}

func (p Profile) Validate() error {
	p = p.normalized()
	if strings.TrimSpace(p.BinDir) == "" {
		return errors.New("bin directory is required")
	}
	if _, err := netip.ParseAddrPort(p.ServerFront); err != nil {
		return fmt.Errorf("server front: %w", err)
	}
	if strings.TrimSpace(p.ServerName) == "" {
		return errors.New("server name is required")
	}
	if len(p.RouteKey) < 16 {
		return errors.New("route key must be at least 16 bytes")
	}
	if p.Username == "" || p.Password == "" {
		return errors.New("username and password are required")
	}
	raw, err := netip.ParseAddrPort(p.ServerRaw)
	if err != nil || !raw.Addr().Is4() {
		return errors.New("server raw must be an IPv4 address:port")
	}
	if p.FEC != "off" && p.FEC != "20:20" {
		return errors.New("FEC must be off or 20:20")
	}
	if p.MTU < 576 || p.MTU > 9000 {
		return errors.New("MTU must be 576..9000")
	}
	if p.RouteMode != "Full" && p.RouteMode != "Split" {
		return errors.New("route mode must be Full or Split")
	}
	if p.RouteMode == "Split" && len(p.Prefix4) == 0 {
		return errors.New("Split route mode requires at least one IPv4 prefix")
	}
	for _, prefix := range p.Prefix4 {
		px, err := netip.ParsePrefix(prefix)
		if err != nil || !px.Addr().Is4() {
			return fmt.Errorf("invalid IPv4 capture prefix %q", prefix)
		}
	}
	if px, err := netip.ParsePrefix(p.TunnelIPv4); err != nil || !px.Addr().Is4() {
		return errors.New("tunnel IPv4 must be an IPv4 CIDR")
	}
	if strings.TrimSpace(p.TicketPath) == "" || strings.TrimSpace(p.RouteState) == "" {
		return errors.New("ticket and route-state paths are required")
	}
	return nil
}

func (u Underlay) Validate() error {
	if ip, err := netip.ParseAddr(u.SourceIP); err != nil || !ip.Is4() {
		return errors.New("underlay source IP must be IPv4")
	}
	if !strings.HasPrefix(u.PacketDevice, `\Device\NPF_{`) || !strings.HasSuffix(u.PacketDevice, "}") {
		return errors.New("underlay packet device must be an Npcap device")
	}
	if !validMAC(u.SourceMAC) || !validMAC(u.NextHopMAC) {
		return errors.New("underlay source and next-hop MACs are required")
	}
	return nil
}

func BuildPlan(profile Profile, underlay Underlay, ticket string) (Plan, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil {
		return Plan{}, err
	}
	if err := underlay.Validate(); err != nil {
		return Plan{}, err
	}
	if len(strings.TrimSpace(ticket)) != 64 {
		return Plan{}, errors.New("Reality ticket must be 64 hex characters")
	}
	for _, c := range ticket {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return Plan{}, errors.New("Reality ticket must be hexadecimal")
		}
	}

	raw, _ := netip.ParseAddrPort(profile.ServerRaw)
	bin := func(name string) string { return filepath.Join(profile.BinDir, name) }
	loop := func(port int) string { return "127.0.0.1:" + strconv.Itoa(port) }

	bootstrapArgs := []string{
		"client",
		"-addr", profile.ServerFront,
		"-server-name", profile.ServerName,
		"-route-key", profile.RouteKey,
		"-username", profile.Username,
		"-password", profile.Password,
		"-verify-server=" + strconv.FormatBool(profile.VerifyServer),
		"-ticket-out", profile.TicketPath,
	}

	fakeArgs := []string{
		"client",
		"--local-udp", loop(defaultFakeTCPLocalPort),
		"--source", netip.AddrPortFrom(netip.MustParseAddr(underlay.SourceIP), defaultFakeTCPSourcePort).String(),
		"--remote", raw.String(),
		"--shadow-recovery", "legacy",
		"--packet-device", underlay.PacketDevice,
		"--source-mac", underlay.SourceMAC,
		"--next-hop-mac", underlay.NextHopMAC,
	}

	dtlsArgs := []string{
		"client",
		strconv.Itoa(defaultDTLSPlainPort),
		"127.0.0.1",
		strconv.Itoa(defaultFakeTCPLocalPort),
		"none",
		"none",
	}

	linkArgs := []string{
		"-mode", "client",
		"-listen", loop(defaultLinkListenPort),
		"-dtls", loop(defaultDTLSPlainPort),
		"-fec", profile.FEC,
		"-mtu", strconv.Itoa(profile.MTU),
		"-lanes", "1",
		"-demo-reality-ticket", strings.TrimSpace(ticket),
	}

	tunArgs := []string{
		"-mode", "client",
		"-ifname", profile.IfName,
		"-mtu", strconv.Itoa(profile.MTU),
		"-transport", loop(defaultLinkListenPort),
	}

	routeArgs := []string{
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", bin("windows_tun_route.ps1"),
		"-Action", "Apply",
		"-Mode", profile.RouteMode,
		"-AdapterAlias", profile.IfName,
		"-TunnelAddress4", profile.TunnelIPv4,
		"-Underlay4", raw.Addr().String(),
		"-MTU", strconv.Itoa(profile.MTU),
		"-StatePath", profile.RouteState,
	}
	if profile.RouteMode == "Split" {
		for _, prefix := range profile.Prefix4 {
			routeArgs = append(routeArgs, "-Prefix4", prefix)
		}
	}
	cleanupArgs := []string{
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", bin("windows_tun_route.ps1"),
		"-Action", "Cleanup",
		"-StatePath", profile.RouteState,
	}

	return Plan{
		Bootstrap: Command{Name: "reality-bootstrap", Path: bin("wbd-reality-front.exe"), Args: bootstrapArgs},
		FakeTCP: Command{Name: "faketcp", Path: bin("wbd-faketcp.exe"), Args: fakeArgs},
		DTLS: Command{Name: "dtls", Path: bin("wbd_dtls_shim.exe"), Args: dtlsArgs},
		Link: Command{Name: "link", Path: bin("wbd-link-proxy.exe"), Args: linkArgs},
		TUN: Command{Name: "tun", Path: bin("wbd-tun.exe"), Args: tunArgs},
		RouteApply: Command{Name: "route-apply", Path: "powershell.exe", Args: routeArgs},
		RouteCleanup: Command{Name: "route-cleanup", Path: "powershell.exe", Args: cleanupArgs},
		TicketPath: profile.TicketPath,
	}, nil
}

func validMAC(s string) bool {
	parts := strings.Split(strings.ToLower(strings.TrimSpace(s)), ":")
	if len(parts) != 6 {
		return false
	}
	for _, part := range parts {
		if len(part) != 2 {
			return false
		}
		for _, c := range part {
			if !strings.ContainsRune("0123456789abcdef", c) {
				return false
			}
		}
	}
	return true
}
