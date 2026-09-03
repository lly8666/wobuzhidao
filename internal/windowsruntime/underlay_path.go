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

// UnderlayPathDiscoverer is the existing lane-identity observation capability.
// Implementations that also know the physical route should additionally expose
// UnderlayPathObservationDiscoverer; callers never invent missing route data.
type UnderlayPathDiscoverer interface {
	DiscoverPath(Profile) (Underlay, error)
}

type UnderlayPathObservationDiscoverer interface {
	DiscoverPathObservation(Profile) (underlayPathObservation, error)
}

type underlayPathState struct {
	nextProbe     time.Time
	pending       bool
	routeKnown    bool
	route         underlayPathObservation
	rebindPending bool
}

func newUnderlayPathState() *underlayPathState { return &underlayPathState{} }
func (s *underlayPathState) clear() {
	s.nextProbe = time.Time{}
	s.pending = false
	s.routeKnown = false
	s.route = underlayPathObservation{}
	s.rebindPending = false
}

func decodeUnderlayDiscoveryObservation(output []byte) (underlayPathObservation, error) {
	var jsonLine string
	for _, line := range strings.Split(string(output), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "{") {
			jsonLine = line
			break
		}
	}
	if jsonLine == "" {
		return underlayPathObservation{}, fmt.Errorf("underlay discovery returned no JSON: %s", strings.TrimSpace(string(output)))
	}
	var result struct {
		SourceIP       string `json:"source_ip"`
		InterfaceIndex uint32 `json:"interface_index"`
		PacketDevice   string `json:"packet_device"`
		SourceMAC      string `json:"source_mac"`
		NextHopIP      string `json:"next_hop_ip"`
		NextHopMAC     string `json:"next_hop_mac"`
	}
	if err := json.Unmarshal([]byte(jsonLine), &result); err != nil {
		return underlayPathObservation{}, fmt.Errorf("decode underlay discovery: %w", err)
	}
	observed := underlayPathObservation{
		Underlay: Underlay{
			SourceIP:     result.SourceIP,
			PacketDevice: result.PacketDevice,
			SourceMAC:    result.SourceMAC,
			NextHopMAC:   result.NextHopMAC,
		},
		InterfaceIndex: result.InterfaceIndex,
		NextHopIP:      result.NextHopIP,
	}
	if err := observed.Validate(); err != nil {
		return underlayPathObservation{}, err
	}
	return observed, nil
}

func decodeUnderlayDiscoveryOutput(output []byte) (Underlay, error) {
	observed, err := decodeUnderlayDiscoveryObservation(output)
	if err != nil {
		return Underlay{}, err
	}
	return observed.Underlay, nil
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

func (PowerShellUnderlayDiscoverer) DiscoverPathObservation(profile Profile) (underlayPathObservation, error) {
	args, err := underlayPathDiscoveryArgs(profile)
	if err != nil {
		return underlayPathObservation{}, err
	}
	cmd := exec.Command("powershell.exe", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return underlayPathObservation{}, fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return decodeUnderlayDiscoveryObservation(output)
}

func (d PowerShellUnderlayDiscoverer) DiscoverPath(profile Profile) (Underlay, error) {
	observed, err := d.DiscoverPathObservation(profile)
	if err != nil {
		return Underlay{}, err
	}
	return observed.Underlay, nil
}

func discoverCurrentUnderlayObservation(discoverer UnderlayDiscoverer, profile Profile) (underlayPathObservation, error) {
	if pathDiscoverer, ok := discoverer.(UnderlayPathObservationDiscoverer); ok {
		return pathDiscoverer.DiscoverPathObservation(profile)
	}
	if pathDiscoverer, ok := discoverer.(UnderlayPathDiscoverer); ok {
		underlay, err := pathDiscoverer.DiscoverPath(profile)
		if err != nil {
			return underlayPathObservation{}, err
		}
		observed := underlayPathObservation{Underlay: underlay}
		if err := observed.Validate(); err != nil {
			return underlayPathObservation{}, err
		}
		return observed, nil
	}
	underlay, err := discoverer.Discover(profile)
	if err != nil {
		return underlayPathObservation{}, err
	}
	observed := underlayPathObservation{Underlay: underlay}
	if err := observed.Validate(); err != nil {
		return underlayPathObservation{}, err
	}
	return observed, nil
}

func discoverCurrentUnderlay(discoverer UnderlayDiscoverer, profile Profile) (Underlay, error) {
	observed, err := discoverCurrentUnderlayObservation(discoverer, profile)
	if err != nil {
		return Underlay{}, err
	}
	return observed.Underlay, nil
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
	_, hasPath := discoverer.(UnderlayPathDiscoverer)
	_, hasRouteObservation := discoverer.(UnderlayPathObservationDiscoverer)
	if !hasPath && !hasRouteObservation {
		return false
	}
	if now.Before(path.nextProbe) {
		return path.pending
	}

	observed, err := discoverCurrentUnderlayObservation(discoverer, profile)
	if err != nil {
		// Discovery is observability. Fail open and leave every current lane and
		// the last known-good base/route ownership untouched.
		path.pending = false
		if path.rebindPending {
			path.nextProbe = now.Add(underlayPathRetryDelay)
		} else {
			path.nextProbe = now.Add(underlayPathProbeInterval)
		}
		return false
	}
	if err := observed.Validate(); err != nil {
		path.pending = false
		if path.rebindPending {
			path.nextProbe = now.Add(underlayPathRetryDelay)
		} else {
			path.nextProbe = now.Add(underlayPathProbeInterval)
		}
		return false
	}
	discovered := observed.Underlay

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
			if observed.HasPhysicalRoute() {
				path.rebindPending = true
			}
			path.nextProbe = now.Add(underlayPathLaneStagger)
		}
		return true
	}

	path.pending = false
	needsRebind := observed.HasPhysicalRoute() &&
		(!path.routeKnown || path.rebindPending || !samePhysicalRouteObservation(path.route, observed))
	if needsRebind {
		path.nextProbe = now.Add(underlayPathRetryDelay)
		if err := c.rebindPhysicalRoutes(profile, observed); err != nil {
			// The rebind script keeps old WBD-owned routes authoritative on error.
			// Fail open for payload/liveness and retry from a fresh observation.
			path.rebindPending = true
			return false
		}
		path.routeKnown = true
		path.route = observed
		path.rebindPending = false
		path.nextProbe = now.Add(underlayPathProbeInterval)
		return true
	}

	path.nextProbe = now.Add(underlayPathProbeInterval)
	return false
}
