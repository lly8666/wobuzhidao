package windowsruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

const (
	underlayPathProbeInterval = 5 * time.Second
	underlayPathRetryDelay    = time.Second
	underlayPathLaneStagger   = time.Second
)

// UnderlayPathDiscoverer is an optional connected-state capability. Discover
// remains the authoritative pre-connect route lookup; DiscoverPath must ignore
// WBD's own pinned server escape route so a NIC/default-route change can be
// observed after capture routes are installed.
type UnderlayPathDiscoverer interface {
	DiscoverPath(Profile) (Underlay, error)
}

type underlayPathState struct {
	nextProbe time.Time
	pending   bool
}

func newUnderlayPathState() *underlayPathState { return &underlayPathState{} }
func (s *underlayPathState) clear()            { s.nextProbe = time.Time{}; s.pending = false }

func decodeUnderlayDiscoveryOutput(output []byte) (Underlay, error) {
	var jsonLine string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			jsonLine = line
			break
		}
	}
	if jsonLine == "" {
		return Underlay{}, fmt.Errorf("underlay discovery returned no JSON: %s", strings.TrimSpace(string(output)))
	}
	var result struct {
		SourceIP     string `json:"source_ip"`
		PacketDevice string `json:"packet_device"`
		SourceMAC    string `json:"source_mac"`
		NextHopMAC   string `json:"next_hop_mac"`
	}
	if err := json.Unmarshal([]byte(jsonLine), &result); err != nil {
		return Underlay{}, fmt.Errorf("decode underlay discovery: %w", err)
	}
	underlay := Underlay{
		SourceIP:     result.SourceIP,
		PacketDevice: result.PacketDevice,
		SourceMAC:    result.SourceMAC,
		NextHopMAC:   result.NextHopMAC,
	}
	if err := underlay.Validate(); err != nil {
		return Underlay{}, err
	}
	return underlay, nil
}

func underlayPathDiscoveryArgs(profile Profile) ([]string, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil {
		return nil, err
	}
	raw, _ := netip.ParseAddrPort(profile.ServerRaw)
	return []string{
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(profile.BinDir, "windows_faketcp_underlay.ps1"),
		"-RemoteIPAddress", raw.Addr().String(),
		"-MonitorPhysicalPath",
		"-StatePath", profile.RouteState,
	}, nil
}

func (PowerShellUnderlayDiscoverer) DiscoverPath(profile Profile) (Underlay, error) {
	args, err := underlayPathDiscoveryArgs(profile)
	if err != nil {
		return Underlay{}, err
	}
	cmd := exec.Command("powershell.exe", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return Underlay{}, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return decodeUnderlayDiscoveryOutput(output)
}

func discoverCurrentUnderlay(discoverer UnderlayDiscoverer, profile Profile) (Underlay, error) {
	if pathDiscoverer, ok := discoverer.(UnderlayPathDiscoverer); ok {
		return pathDiscoverer.DiscoverPath(profile)
	}
	return discoverer.Discover(profile)
}

func sameUnderlayPath(a, b Underlay) bool {
	aIP, aErr := netip.ParseAddr(strings.TrimSpace(a.SourceIP))
	bIP, bErr := netip.ParseAddr(strings.TrimSpace(b.SourceIP))
	if aErr != nil || bErr != nil || aIP != bIP {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(a.PacketDevice), strings.TrimSpace(b.PacketDevice)) &&
		strings.EqualFold(strings.TrimSpace(a.SourceMAC), strings.TrimSpace(b.SourceMAC)) &&
		strings.EqualFold(strings.TrimSpace(a.NextHopMAC), strings.TrimSpace(b.NextHopMAC))
}

func commandFlagValue(args []string, flag string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1], true
		}
	}
	return "", false
}

func lanePlanUnderlay(plan LanePlan) (Underlay, error) {
	source, ok := commandFlagValue(plan.FakeTCP.Args, "--source")
	if !ok {
		return Underlay{}, errors.New("lane FakeTCP command has no --source")
	}
	sourceAddr, err := netip.ParseAddrPort(source)
	if err != nil {
		return Underlay{}, fmt.Errorf("parse lane FakeTCP source: %w", err)
	}
	packetDevice, ok := commandFlagValue(plan.FakeTCP.Args, "--packet-device")
	if !ok {
		return Underlay{}, errors.New("lane FakeTCP command has no --packet-device")
	}
	sourceMAC, ok := commandFlagValue(plan.FakeTCP.Args, "--source-mac")
	if !ok {
		return Underlay{}, errors.New("lane FakeTCP command has no --source-mac")
	}
	nextHopMAC, ok := commandFlagValue(plan.FakeTCP.Args, "--next-hop-mac")
	if !ok {
		return Underlay{}, errors.New("lane FakeTCP command has no --next-hop-mac")
	}
	underlay := Underlay{
		SourceIP:     sourceAddr.Addr().String(),
		PacketDevice: packetDevice,
		SourceMAC:    sourceMAC,
		NextHopMAC:   nextHopMAC,
		SourcePort:   sourceAddr.Port(),
	}
	if err := underlay.Validate(); err != nil {
		return Underlay{}, err
	}
	return underlay, nil
}

// runUnderlayPathTick returns true while path convergence owns this lifecycle
// tick. That priority prevents an exited old child from immediately starting a
// replacement on the stale underlay between staggered path migrations.
func (c *Controller) runUnderlayPathTick(path *underlayPathState, now time.Time) bool {
	c.mu.Lock()
	state := c.state
	plans := cloneLanePlans(c.lanePlans)
	profile := c.profile
	discoverer := c.discoverer
	refs := make(map[int]logicaltunnel.LaneRef, len(plans))
	if state == RuntimeConnected && c.lifecycle != nil {
		for _, snap := range c.lifecycle.Snapshot() {
			refs[int(snap.Ref.ID)] = snap.Ref
		}
	}
	c.mu.Unlock()

	if state == RuntimeDormant {
		path.clear()
		return false
	}
	if state != RuntimeConnected {
		return false
	}
	pathDiscoverer, ok := discoverer.(UnderlayPathDiscoverer)
	if !ok {
		return false
	}
	if now.Before(path.nextProbe) {
		return path.pending
	}

	discovered, err := pathDiscoverer.DiscoverPath(profile)
	if err != nil {
		// Discovery is observability. Fail open and leave every current lane and
		// the last known-good base underlay untouched.
		path.pending = false
		path.nextProbe = now.Add(underlayPathProbeInterval)
		return false
	}
	if err := discovered.Validate(); err != nil {
		path.pending = false
		path.nextProbe = now.Add(underlayPathProbeInterval)
		return false
	}

	ids := make([]int, 0, len(plans))
	for id := range plans {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, laneID := range ids {
		current, err := lanePlanUnderlay(plans[laneID])
		if err != nil {
			continue
		}
		if sameUnderlayPath(current, discovered) {
			continue
		}
		expected, ok := refs[laneID]
		if !ok {
			continue
		}
		path.pending = true
		path.nextProbe = now.Add(underlayPathRetryDelay)
		if err := c.replaceLaneOnUnderlay(laneID, expected, discovered); err == nil {
			path.nextProbe = now.Add(underlayPathLaneStagger)
		}
		return true
	}

	path.pending = false
	path.nextProbe = now.Add(underlayPathProbeInterval)
	return false
}
