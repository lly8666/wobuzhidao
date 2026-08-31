package windowsruntime

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

type LaneBootstrap struct {
	ID               int
	Underlay         Underlay
	FakeTCP          Command
	TicketPath       string
	TunnelConfigPath string
	Ticket           string
	TunnelConfig     logicaltunnel.TunnelConfig
}

type LanePlan struct {
	ID      int
	FakeTCP Command
	DTLS    Command
	Link    Command
}

type MultiLanePlan struct {
	Lanes          []LanePlan
	Game           Command
	TUN            Command
	IPv6Apply      Command
	RouteApply     Command
	RouteCleanup   Command
	IPv6Cleanup    Command
	TunnelConfig   logicaltunnel.TunnelConfig
}

func lanePort(base, laneID int) (int, error) {
	if laneID < 1 || laneID > logicaltunnel.MaxProductPublicTransportLanes {
		return 0, logicaltunnel.ErrTransportLanes
	}
	return base + laneID - 1, nil
}

func laneStatePath(base string, laneID int) (string, error) {
	if _, err := lanePort(0, laneID); err != nil {
		return "", err
	}
	if strings.TrimSpace(base) == "" {
		return "", errors.New("lane state path base is required")
	}
	return fmt.Sprintf("%s.lane%d", base, laneID), nil
}

func laneLoopback(base, laneID int) (string, error) {
	port, err := lanePort(base, laneID)
	if err != nil {
		return "", err
	}
	return "127.0.0.1:" + strconv.Itoa(port), nil
}

func BuildLaneBootstrap(profile Profile, base Underlay, laneID int) (LaneBootstrap, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil {
		return LaneBootstrap{}, err
	}
	if err := base.Validate(); err != nil {
		return LaneBootstrap{}, err
	}
	localUDP, err := laneLoopback(defaultFakeTCPLocalPort, laneID)
	if err != nil {
		return LaneBootstrap{}, err
	}
	ticketPath, err := laneStatePath(profile.TicketPath, laneID)
	if err != nil {
		return LaneBootstrap{}, err
	}
	configPath, err := laneStatePath(profile.TunnelConfigPath, laneID)
	if err != nil {
		return LaneBootstrap{}, err
	}
	if base.SourcePort == 0 {
		return LaneBootstrap{}, errors.New("lane bootstrap requires an assigned dynamic FakeTCP source port")
	}
	raw, _ := netip.ParseAddrPort(profile.ServerRaw)
	args := []string{
		"client",
		"--local-udp", localUDP,
		"--source", netip.AddrPortFrom(netip.MustParseAddr(base.SourceIP), base.SourcePort).String(),
		"--remote", raw.String(),
		"--shadow-recovery", "legacy",
		"--packet-device", base.PacketDevice,
		"--source-mac", base.SourceMAC,
		"--next-hop-mac", base.NextHopMAC,
		"--reality-server-name", profile.ServerName,
		"--reality-route-key", profile.RouteKey,
		"--reality-username", profile.Username,
		"--reality-password", profile.Password,
		"--reality-ticket-out", ticketPath,
		"--reality-installation-id", profile.InstallationID,
		"--reality-tunnel-config-out", configPath,
		"--reality-verify-server=" + strconv.FormatBool(profile.VerifyServer),
	}
	return LaneBootstrap{
		ID:               laneID,
		Underlay:         base,
		FakeTCP:          Command{Name: fmt.Sprintf("faketcp-%d", laneID), Path: filepath.Join(profile.BinDir, "wbd-faketcp.exe"), Args: args},
		TicketPath:       ticketPath,
		TunnelConfigPath: configPath,
	}, nil
}

func (b LaneBootstrap) ValidateAuthenticated(expected *logicaltunnel.TunnelConfig) error {
	if b.ID < 1 || b.ID > logicaltunnel.MaxProductPublicTransportLanes {
		return logicaltunnel.ErrTransportLanes
	}
	if err := b.Underlay.Validate(); err != nil {
		return err
	}
	if len(strings.TrimSpace(b.Ticket)) != 64 {
		return errors.New("lane Reality ticket must be 64 hex characters")
	}
	for _, c := range b.Ticket {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return errors.New("lane Reality ticket must be hexadecimal")
		}
	}
	if err := b.TunnelConfig.Validate(); err != nil {
		return err
	}
	if expected != nil && (b.TunnelConfig.TunnelID != expected.TunnelID || b.TunnelConfig.Address4 != expected.Address4 || !sameStringSet(b.TunnelConfig.Routes4, expected.Routes4)) {
		return fmt.Errorf("lane %d authenticated Logical Tunnel mismatch", b.ID)
	}
	return nil
}

func sameStringSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	counts := make(map[string]int, len(a))
	for _, s := range a {
		counts[s]++
	}
	for _, s := range b {
		if counts[s] == 0 {
			return false
		}
		counts[s]--
	}
	for _, n := range counts {
		if n != 0 {
			return false
		}
	}
	return true
}

func BuildMultiLanePlan(profile Profile, bootstraps []LaneBootstrap) (MultiLanePlan, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil {
		return MultiLanePlan{}, err
	}
	if len(bootstraps) != profile.Lanes {
		return MultiLanePlan{}, fmt.Errorf("authenticated lane count=%d want=%d", len(bootstraps), profile.Lanes)
	}
	if err := logicaltunnel.ValidateProductTransportLaneCount(len(bootstraps)); err != nil {
		return MultiLanePlan{}, err
	}

	var tunnel logicaltunnel.TunnelConfig
	for i := range bootstraps {
		b := bootstraps[i]
		if b.ID != i+1 {
			return MultiLanePlan{}, fmt.Errorf("lane ordering mismatch: index=%d id=%d", i, b.ID)
		}
		if i == 0 {
			if err := b.ValidateAuthenticated(nil); err != nil {
				return MultiLanePlan{}, fmt.Errorf("lane %d: %w", b.ID, err)
			}
			tunnel = b.TunnelConfig
			continue
		}
		if err := b.ValidateAuthenticated(&tunnel); err != nil {
			return MultiLanePlan{}, err
		}
	}

	profile.TunnelIPv4 = tunnel.Address4
	common, err := BuildPlan(profile, bootstraps[0].Underlay, bootstraps[0].Ticket)
	if err != nil {
		return MultiLanePlan{}, err
	}
	bin := func(name string) string { return filepath.Join(profile.BinDir, name) }
	lanes := make([]LanePlan, 0, len(bootstraps))
	linkAddresses := make([]string, 0, len(bootstraps))
	for _, b := range bootstraps {
		dtlsPlain, _ := laneLoopback(defaultDTLSPlainPort, b.ID)
		linkListen, _ := laneLoopback(defaultLinkListenPort, b.ID)
		dtlsPort, _ := lanePort(defaultDTLSPlainPort, b.ID)
		fakePort, _ := lanePort(defaultFakeTCPLocalPort, b.ID)
		lanes = append(lanes, LanePlan{
			ID:      b.ID,
			FakeTCP: b.FakeTCP,
			DTLS: Command{
				Name: fmt.Sprintf("dtls-%d", b.ID),
				Path: bin("wbd_dtls_shim.exe"),
				Args: []string{"client", strconv.Itoa(dtlsPort), "127.0.0.1", strconv.Itoa(fakePort), "none", "none"},
			},
			Link: Command{
				Name: fmt.Sprintf("link-%d", b.ID),
				Path: bin("wbd-link-proxy.exe"),
				Args: []string{"-mode", "client", "-listen", linkListen, "-dtls", dtlsPlain, "-fec", profile.FEC, "-mtu", strconv.Itoa(profile.MTU), "-lanes", "1", "-demo-reality-ticket", strings.TrimSpace(b.Ticket)},
			},
		})
		linkAddresses = append(linkAddresses, linkListen)
	}

	gameListen := "127.0.0.1:" + strconv.Itoa(defaultGameListenPort)
	game := Command{
		Name: "game",
		Path: bin("wbd-game-lane-client.exe"),
		Args: []string{"-listen", gameListen, "-lanes", strings.Join(linkAddresses, ","), "-session-id", string(tunnel.TunnelID), "-replay-window", "4096"},
	}
	tun := common.TUN
	for i := 0; i+1 < len(tun.Args); i++ {
		if tun.Args[i] == "-transport" {
			tun.Args[i+1] = gameListen
			break
		}
	}
	return MultiLanePlan{
		Lanes:        lanes,
		Game:         game,
		TUN:          tun,
		IPv6Apply:    common.IPv6Apply,
		RouteApply:   common.RouteApply,
		RouteCleanup: common.RouteCleanup,
		IPv6Cleanup:  common.IPv6Cleanup,
		TunnelConfig: tunnel,
	}, nil
}

func (p MultiLanePlan) ProcessCommands() []Command {
	out := make([]Command, 0, len(p.Lanes)*3+2)
	for _, lane := range p.Lanes {
		out = append(out, lane.FakeTCP, lane.DTLS, lane.Link)
	}
	out = append(out, p.Game, p.TUN)
	return out
}
