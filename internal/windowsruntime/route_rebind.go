package windowsruntime

import (
	"errors"
	"fmt"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/lly8666/wobuzhidao/internal/ipset"
)

// underlayPathObservation keeps kernel-route identity separate from the raw
// FakeTCP lane identity. InterfaceIndex/NextHopIP are Windows orchestration
// metadata only; they do not enter any Transport Lane wire format.
type underlayPathObservation struct {
	Underlay       Underlay
	InterfaceIndex uint32
	NextHopIP      string
}

func (o underlayPathObservation) HasPhysicalRoute() bool {
	return o.InterfaceIndex != 0 && strings.TrimSpace(o.NextHopIP) != ""
}

func (o underlayPathObservation) Validate() error {
	if err := o.Underlay.Validate(); err != nil {
		return err
	}
	if o.InterfaceIndex == 0 && strings.TrimSpace(o.NextHopIP) == "" {
		// Non-PowerShell test/discovery implementations may expose only the lane
		// identity. Route rebind stays disabled rather than guessing.
		return nil
	}
	if o.InterfaceIndex == 0 || strings.TrimSpace(o.NextHopIP) == "" {
		return errors.New("physical route observation requires both interface index and IPv4 next hop")
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(o.NextHopIP))
	if err != nil || !ip.Is4() {
		return errors.New("physical route next hop must be IPv4")
	}
	return nil
}

func samePhysicalRouteObservation(a, b underlayPathObservation) bool {
	if !a.HasPhysicalRoute() || !b.HasPhysicalRoute() {
		return false
	}
	aIP, aErr := netip.ParseAddr(strings.TrimSpace(a.NextHopIP))
	bIP, bErr := netip.ParseAddr(strings.TrimSpace(b.NextHopIP))
	return aErr == nil && bErr == nil && a.InterfaceIndex == b.InterfaceIndex && aIP == bIP
}

func buildRouteRebindCommand(profile Profile, observed underlayPathObservation) (Command, error) {
	profile = profile.normalized()
	if err := profile.Validate(); err != nil {
		return Command{}, err
	}
	if err := observed.Validate(); err != nil {
		return Command{}, err
	}
	if !observed.HasPhysicalRoute() {
		return Command{}, errors.New("physical route metadata is unavailable")
	}
	raw, _ := netip.ParseAddrPort(profile.ServerRaw)
	args := []string{
		"-NoProfile", "-ExecutionPolicy", "Bypass",
		"-File", filepath.Join(profile.BinDir, "windows_tun_rebind.ps1"),
		"-Underlay4", raw.Addr().String(),
		"-ExpectedPhysicalInterfaceIndex", strconv.FormatUint(uint64(observed.InterfaceIndex), 10),
		"-ExpectedPhysicalNextHop4", strings.TrimSpace(observed.NextHopIP),
		"-StatePath", profile.RouteState,
	}
	if profile.RouteMode == RouteForeign {
		args = append(args, "-DirectPrefixFile4", filepath.Join(profile.CNSetDir, ipset.CNIPv4File))
	}
	return Command{Name: "route-rebind", Path: "powershell.exe", Args: args}, nil
}

func (c *Controller) rebindPhysicalRoutes(profile Profile, observed underlayPathObservation) error {
	command, err := buildRouteRebindCommand(profile, observed)
	if err != nil {
		return fmt.Errorf("build Windows physical-route rebind: %w", err)
	}
	return c.executor.RebindRoutes(command)
}
