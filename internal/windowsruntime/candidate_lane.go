package windowsruntime

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
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
	// profile.MTU is the user-visible inner/Wintun MTU. Every Game datagram adds
	// a fixed 32-byte WGL1 envelope before entering LINK, so the immutable LINK
	// plaintext budget must include that header rather than rejecting a legal
	// inner packet as fec.ErrPacketTooLarge even when FEC is disabled.
	gameLinkMTU := profile.MTU + gamelane.HeaderSize

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
			Args: []string{"-mode", "client", "-listen", linkListen, "-dtls", dtlsPlain, "-fec", profile.FEC, "-mtu", strconv.Itoa(gameLinkMTU), "-lanes", "1", "-demo-reality-ticket", strings.TrimSpace(bootstrap.Ticket)},
		},
	}, nil
}

// NextReplacementSlot retains the historical one-lane planning helper. Product
// replacement uses NextReplacementSlotForPlans so the one spare physical slot
// follows the last retired incarnation instead of assuming slot 5 is always free.
func NextReplacementSlot(current LanePlan) int {
	if current.Slot == makeBeforeBreakCandidateSlot { return current.ID }
	return makeBeforeBreakCandidateSlot
}

// NextReplacementSlotForPlans chooses one currently unused physical transport
// slot from the bounded 1..5 runtime space. With four authoritative logical lanes
// there is exactly one spare slot; after promotion the retired old slot becomes
// the spare for the next replacement. This preserves 4 logical lanes + at most
// one overlapping physical candidate without treating slot 5 as a fifth lane.
func NextReplacementSlotForPlans(current LanePlan, plans map[int]LanePlan) (int, error) {
	currentSlot := current.Slot
	if currentSlot == 0 { currentSlot = current.ID }
	if _, err := transportSlotPort(0, currentSlot); err != nil {
		return 0, fmt.Errorf("current replacement slot: %w", err)
	}
	authoritative, ok := plans[current.ID]
	if !ok {
		return 0, fmt.Errorf("logical lane %d is not active", current.ID)
	}
	authoritativeSlot := authoritative.Slot
	if authoritativeSlot == 0 { authoritativeSlot = authoritative.ID }
	if authoritative.ID != current.ID || authoritativeSlot != currentSlot {
		return 0, fmt.Errorf("logical lane %d authoritative slot drift: current=%d plan=%d", current.ID, currentSlot, authoritativeSlot)
	}

	used := make(map[int]int, len(plans))
	for id, plan := range plans {
		if plan.ID != id {
			return 0, fmt.Errorf("logical lane map key=%d contains lane id=%d", id, plan.ID)
		}
		slot := plan.Slot
		if slot == 0 { slot = plan.ID }
		if _, err := transportSlotPort(0, slot); err != nil {
			return 0, fmt.Errorf("logical lane %d slot: %w", id, err)
		}
		if owner, exists := used[slot]; exists {
			return 0, fmt.Errorf("physical transport slot %d is authoritative for lanes %d and %d", slot, owner, id)
		}
		used[slot] = id
	}

	preferences := make([]int, 0, makeBeforeBreakCandidateSlot+2)
	if currentSlot == makeBeforeBreakCandidateSlot {
		preferences = append(preferences, current.ID)
	} else {
		preferences = append(preferences, makeBeforeBreakCandidateSlot)
	}
	for slot := 1; slot <= makeBeforeBreakCandidateSlot; slot++ {
		preferences = append(preferences, slot)
	}
	seen := map[int]bool{}
	for _, slot := range preferences {
		if seen[slot] || slot == currentSlot { continue }
		seen[slot] = true
		if _, occupied := used[slot]; !occupied {
			return slot, nil
		}
	}
	return 0, errors.New("no spare physical transport slot for make-before-break replacement")
}

func LaneGameTarget(plan LanePlan) (string, error) {
	if plan.ID < 1 || plan.ID > 4 { return "", errors.New("logical lane id must be 1..4") }
	if plan.Slot == 0 { plan.Slot = plan.ID }
	return transportSlotLoopback(defaultLinkListenPort, plan.Slot)
}
