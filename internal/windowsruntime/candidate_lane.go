package windowsruntime

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"
)

const makeBeforeBreakCandidateSlot = 5

// ErrOverlappingPublicFlow is temporarily retained for still-unrepaired
// dynamic-lane/runtime guards. Candidate construction itself is ADR-0012
// make-before-break capable again; the next atomic phase repairs execution.
var ErrOverlappingPublicFlow = errors.New("windowsruntime: dynamic public-flow overlap is not yet restored")

func BuildCandidateLaneBootstrap(profile Profile, base Underlay, laneID int) (LaneBootstrap, error) {
	return BuildCandidateLaneBootstrapSlot(profile, base, laneID, makeBeforeBreakCandidateSlot)
}

func BuildCandidateLaneBootstrapSlot(profile Profile, base Underlay, laneID, slot int) (LaneBootstrap, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil { return LaneBootstrap{}, err }
	if _, err := lanePort(0, laneID); err != nil { return LaneBootstrap{}, err }
	if _, err := transportSlotPort(0, slot); err != nil { return LaneBootstrap{}, err }
	if err := base.Validate(); err != nil { return LaneBootstrap{}, err }
	if base.SourcePort == 0 { return LaneBootstrap{}, errors.New("candidate lane requires an assigned dynamic FakeTCP source port") }

	localUDP, err := transportSlotLoopback(defaultFakeTCPLocalPort, slot)
	if err != nil { return LaneBootstrap{}, err }
	raw, _ := netip.ParseAddrPort(profile.ServerRaw)
	ticketPath := fmt.Sprintf("%s.lane%d.candidate.slot%d", profile.TicketPath, laneID, slot)
	configPath := fmt.Sprintf("%s.lane%d.candidate.slot%d", profile.TunnelConfigPath, laneID, slot)
	args := []string{
		"client", "--local-udp", localUDP,
		"--source", netip.AddrPortFrom(netip.MustParseAddr(base.SourceIP), base.SourcePort).String(),
		"--remote", raw.String(), "--shadow-recovery", "legacy",
		"--packet-device", base.PacketDevice, "--source-mac", base.SourceMAC, "--next-hop-mac", base.NextHopMAC,
		"--reality-server-name", profile.ServerName, "--reality-route-key", profile.RouteKey,
		"--reality-username", profile.Username, "--reality-password", profile.Password,
		"--reality-ticket-out", ticketPath, "--reality-installation-id", profile.InstallationID,
		"--reality-tunnel-config-out", configPath, "--reality-verify-server=" + strconv.FormatBool(profile.VerifyServer),
	}
	return LaneBootstrap{
		ID: laneID,
		Underlay: base,
		FakeTCP: Command{
			Name: fmt.Sprintf("faketcp-%d-candidate-s%d", laneID, slot),
			Path: filepath.Join(profile.BinDir, "wbd-faketcp.exe"),
			Args: args,
		},
		TicketPath: ticketPath,
		TunnelConfigPath: configPath,
	}, nil
}

func BuildAuthenticatedLanePlan(profile Profile, bootstrap LaneBootstrap) (LanePlan, error) {
	return buildLanePlanForSlot(profile, bootstrap, bootstrap.ID, false)
}

func BuildCandidateLanePlan(profile Profile, bootstrap LaneBootstrap) (LanePlan, error) {
	return BuildCandidateLanePlanSlot(profile, bootstrap, makeBeforeBreakCandidateSlot)
}

func BuildCandidateLanePlanSlot(profile Profile, bootstrap LaneBootstrap, slot int) (LanePlan, error) {
	return buildLanePlanForSlot(profile, bootstrap, slot, true)
}

func buildLanePlanForSlot(profile Profile, bootstrap LaneBootstrap, slot int, candidate bool) (LanePlan, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil { return LanePlan{}, err }
	if err := bootstrap.ValidateAuthenticated(nil); err != nil { return LanePlan{}, err }
	if _, err := transportSlotPort(0, slot); err != nil { return LanePlan{}, err }

	fakePort, err := transportSlotPort(defaultFakeTCPLocalPort, slot)
	if err != nil { return LanePlan{}, err }
	dtlsPort, err := transportSlotPort(defaultDTLSPlainPort, slot)
	if err != nil { return LanePlan{}, err }
	dtlsPlain, err := transportSlotLoopback(defaultDTLSPlainPort, slot)
	if err != nil { return LanePlan{}, err }
	linkListen, err := transportSlotLoopback(defaultLinkListenPort, slot)
	if err != nil { return LanePlan{}, err }

	bin := func(name string) string { return filepath.Join(profile.BinDir, name) }
	suffix := strconv.Itoa(bootstrap.ID)
	if candidate { suffix += "-candidate-s" + strconv.Itoa(slot) }
	fake := bootstrap.FakeTCP
	if candidate { fake.Name = "faketcp-" + suffix }

	return LanePlan{
		ID: bootstrap.ID,
		Slot: slot,
		FakeTCP: fake,
		DTLS: Command{
			Name: "dtls-" + suffix,
			Path: bin("wbd_dtls_shim.exe"),
			Args: []string{"client", strconv.Itoa(dtlsPort), "127.0.0.1", strconv.Itoa(fakePort), "none", "none"},
		},
		Link: Command{
			Name: "link-" + suffix,
			Path: bin("wbd-link-proxy.exe"),
			Args: []string{"-mode", "client", "-listen", linkListen, "-dtls", dtlsPlain, "-fec", profile.FEC, "-mtu", strconv.Itoa(profile.MTU), "-lanes", "1", "-demo-reality-ticket", strings.TrimSpace(bootstrap.Ticket)},
		},
	}, nil
}

func NextReplacementSlot(current LanePlan) int {
	if current.Slot == makeBeforeBreakCandidateSlot { return current.ID }
	return makeBeforeBreakCandidateSlot
}

func LaneGameTarget(plan LanePlan) (string, error) {
	if plan.ID < 1 || plan.ID > 4 { return "", errors.New("logical lane id must be 1..4") }
	if plan.Slot == 0 { plan.Slot = plan.ID }
	return transportSlotLoopback(defaultLinkListenPort, plan.Slot)
}
