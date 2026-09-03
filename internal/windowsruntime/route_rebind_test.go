package windowsruntime

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/ipset"
)

type observedPathDiscoverer struct {
	mu       sync.Mutex
	initial  Underlay
	observed underlayPathObservation
	err      error
}

func (d *observedPathDiscoverer) Discover(Profile) (Underlay, error) { return d.initial, nil }
func (d *observedPathDiscoverer) DiscoverPath(Profile) (Underlay, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return Underlay{}, d.err
	}
	return d.observed.Underlay, nil
}
func (d *observedPathDiscoverer) DiscoverPathObservation(Profile) (underlayPathObservation, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return underlayPathObservation{}, d.err
	}
	return d.observed, nil
}
func (d *observedPathDiscoverer) set(observed underlayPathObservation, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.observed = observed
	d.err = err
}

func controllerWithObservedPath(r *recordingRunner, d *observedPathDiscoverer) *Controller {
	return NewController(r, d, &recordingTicketStore{runner: r, ticket: strings.Repeat("ab", 32), tunnelConfig: testTunnelConfigJSON})
}

func observedPath(underlay Underlay, ifIndex uint32, nextHop string) underlayPathObservation {
	return underlayPathObservation{Underlay: underlay, InterfaceIndex: ifIndex, NextHopIP: nextHop}
}

func eventIndex(events []string, want string) int {
	for i, event := range events {
		if event == want {
			return i
		}
	}
	return -1
}

func TestDecodeUnderlayDiscoveryObservationKeepsPhysicalRouteMetadata(t *testing.T) {
	output := []byte(`{"source_ip":"192.0.2.20","interface_index":7,"packet_device":"\\Device\\NPF_{01234567-89AB-CDEF-0123-456789ABCDEF}","source_mac":"00:11:22:33:44:55","next_hop_ip":"192.0.2.1","next_hop_mac":"66:77:88:99:aa:bb"}` + "\nWBD_WINDOWS_FAKETCP_UNDERLAY_PASS\n")
	got, err := decodeUnderlayDiscoveryObservation(output)
	if err != nil {
		t.Fatal(err)
	}
	if got.InterfaceIndex != 7 || got.NextHopIP != "192.0.2.1" || !sameUnderlayPath(got.Underlay, testUnderlay()) {
		t.Fatalf("decoded observation=%+v", got)
	}
}

func TestBuildRouteRebindCommandUsesObservedRouteAndForeignDirectFile(t *testing.T) {
	p := testProfile()
	p.RouteMode = RouteForeign
	p.CNSetDir = t.TempDir()
	obs := observedPath(testUnderlay(), 12, "192.0.2.1")
	cmd, err := buildRouteRebindCommand(p, obs)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name != "route-rebind" || !hasArgSuffix(cmd.Args, "windows_tun_rebind.ps1") {
		t.Fatalf("route rebind command=%+v", cmd)
	}
	for flag, want := range map[string]string{
		"-Underlay4":                       "198.51.100.10",
		"-ExpectedPhysicalInterfaceIndex": "12",
		"-ExpectedPhysicalNextHop4":        "192.0.2.1",
		"-StatePath":                       p.RouteState,
		"-DirectPrefixFile4":               filepath.Join(p.CNSetDir, ipset.CNIPv4File),
	} {
		if !argPair(cmd.Args, flag, want) {
			t.Fatalf("route rebind args=%v missing %s=%q", cmd.Args, flag, want)
		}
	}
}

func TestBuildRouteRebindCommandRejectsMissingPhysicalMetadata(t *testing.T) {
	if _, err := buildRouteRebindCommand(testProfile(), observedPath(testUnderlay(), 0, "")); err == nil {
		t.Fatal("route rebind accepted missing physical route metadata")
	}
}

func TestExecutorRouteRebindFailureLeavesRuntimeRunning(t *testing.T) {
	r := &recordingRunner{}
	e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err != nil {
		t.Fatal(err)
	}
	cut := len(r.events)
	r.failOnce = "route-rebind"
	if err := e.RebindRoutes(Command{Name: "route-rebind"}); err == nil {
		t.Fatal("expected injected route-rebind failure")
	}
	if !e.Running() || e.CleanupPending() {
		t.Fatalf("rebind failure changed executor state: running=%v cleanup=%v", e.Running(), e.CleanupPending())
	}
	if !reflect.DeepEqual(r.events[cut:], []string{"run:route-rebind"}) {
		t.Fatalf("rebind failure performed destructive runtime work: %v", r.events[cut:])
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
}

func TestUnderlayPathRebindsOnlyAfterAllStaleLanesMigrate(t *testing.T) {
	r := &recordingRunner{}
	oldPath := testUnderlay()
	newPath := changedUnderlay(oldPath)
	d := &observedPathDiscoverer{initial: oldPath, observed: observedPath(oldPath, 7, "192.0.2.1")}
	c := controllerWithObservedPath(r, d)
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 2
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	stopLifecycleMonitorForTest(c)
	d.set(observedPath(newPath, 9, "192.0.2.254"), nil)
	control, requests := gameControlResponder(t, 4)
	setControllerGameControl(c, control)

	state := newUnderlayPathState()
	now := time.Now()
	if !c.runUnderlayPathTick(state, now) {
		t.Fatal("first stale lane did not migrate")
	}
	waitGameControlRequests(t, requests, 2)
	if countEvent(r.events, "run:route-rebind") != 0 {
		t.Fatalf("kernel routes rebound before every lane migrated: %v", r.events)
	}
	if !c.runUnderlayPathTick(state, now.Add(underlayPathLaneStagger)) {
		t.Fatal("second stale lane did not migrate")
	}
	waitGameControlRequests(t, requests, 2)
	if countEvent(r.events, "run:route-rebind") != 0 {
		t.Fatalf("kernel routes rebound inside lane replacement: %v", r.events)
	}
	if !c.runUnderlayPathTick(state, now.Add(2*underlayPathLaneStagger)) {
		t.Fatal("all-lane convergence did not perform route rebind")
	}
	if countEvent(r.events, "run:route-rebind") != 1 {
		t.Fatalf("route rebind count=%d events=%v", countEvent(r.events, "run:route-rebind"), r.events)
	}
}

func TestUnderlayPathRouteOnlyChangeRebindsWithoutLaneRotation(t *testing.T) {
	r := &recordingRunner{}
	base := testUnderlay()
	d := &observedPathDiscoverer{initial: base, observed: observedPath(base, 7, "192.0.2.1")}
	c := controllerWithObservedPath(r, d)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	stopLifecycleMonitorForTest(c)
	state := newUnderlayPathState()
	now := time.Now()
	if !c.runUnderlayPathTick(state, now) {
		t.Fatal("initial physical route observation was not committed")
	}
	cut := len(r.events)
	d.set(observedPath(base, 8, "192.0.2.254"), nil)
	if !c.runUnderlayPathTick(state, now.Add(underlayPathProbeInterval)) {
		t.Fatal("route-only change did not trigger rebind")
	}
	for _, event := range r.events[cut:] {
		if strings.Contains(event, "candidate") {
			t.Fatalf("route-only change rotated a transport lane: %v", r.events[cut:])
		}
	}
	if countEvent(r.events[cut:], "run:route-rebind") != 1 {
		t.Fatalf("route-only change events=%v", r.events[cut:])
	}
}

func TestUnderlayPathRouteRebindFailureIsFailOpenAndRetryable(t *testing.T) {
	r := &recordingRunner{}
	base := testUnderlay()
	d := &observedPathDiscoverer{initial: base, observed: observedPath(base, 7, "192.0.2.1")}
	c := controllerWithObservedPath(r, d)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	stopLifecycleMonitorForTest(c)
	state := newUnderlayPathState()
	r.failOnce = "route-rebind"
	now := time.Now()
	if c.runUnderlayPathTick(state, now) {
		t.Fatal("failed route rebind must not report successful convergence")
	}
	if c.State() != RuntimeConnected || !c.executor.Running() {
		t.Fatalf("route rebind failure tore down runtime: state=%s running=%v", c.State(), c.executor.Running())
	}
	if !state.rebindPending {
		t.Fatal("failed route rebind did not arm bounded retry")
	}
	if !c.runUnderlayPathTick(state, now.Add(underlayPathRetryDelay)) {
		t.Fatal("route rebind retry did not converge")
	}
	if countEvent(r.events, "run:route-rebind") != 2 {
		t.Fatalf("route rebind attempts=%d events=%v", countEvent(r.events, "run:route-rebind"), r.events)
	}
}

func TestWakeRebindsPhysicalRoutesBeforeStartingFirstLane(t *testing.T) {
	r := &recordingRunner{}
	oldPath := testUnderlay()
	newPath := changedUnderlay(oldPath)
	d := &observedPathDiscoverer{initial: oldPath, observed: observedPath(oldPath, 7, "192.0.2.1")}
	c := controllerWithObservedPath(r, d)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	stopLifecycleMonitorForTest(c)
	control, requests := gameControlResponder(t, 2)
	setControllerGameControl(c, control)
	if err := c.Dormant(); err != nil {
		t.Fatal(err)
	}
	waitGameControlRequests(t, requests, 1)
	d.set(observedPath(newPath, 9, "192.0.2.254"), nil)
	cut := len(r.events)
	if err := c.Wake(); err != nil {
		t.Fatal(err)
	}
	waitGameControlRequests(t, requests, 1)
	events := r.events[cut:]
	rebind := eventIndex(events, "run:route-rebind")
	start := eventIndex(events, "start:faketcp-1")
	if rebind < 0 || start < 0 || rebind >= start {
		t.Fatalf("Wake order=%v want route rebind before first lane bootstrap", events)
	}
	c.mu.Lock()
	plan := c.lanePlans[1]
	c.mu.Unlock()
	planUsesUnderlay(t, plan, newPath)
}

func TestWindowsRouteRebindScriptKeepsTransactionalOwnershipGuards(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "scripts", "windows_tun_rebind.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{
		"windows_faketcp_underlay.ps1",
		"-MonitorPhysicalPath",
		"ExpectedPhysicalInterfaceIndex",
		"ExpectedPhysicalNextHop4",
		"$state.UnderlayRoutes = @(Merge-OwnedRoutes $oldUnderlay $createUnderlay)",
		"$state.DirectRoutes = @(Merge-OwnedRoutes $oldDirect $createDirect)",
		"New-NetRoute",
		"Remove-NetRoute",
		"WBD_WINDOWS_TUN_REBIND_PASS",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("Windows route rebind script lost %q guard", want)
		}
	}
	stage := strings.Index(text, "$state.DirectRoutes = @(Merge-OwnedRoutes $oldDirect $createDirect)")
	create := strings.Index(text, "foreach ($route in @($createUnderlay) + @($createDirect))")
	retire := strings.LastIndex(text, "foreach ($route in @($oldUnderlay) + @($oldDirect))")
	if stage < 0 || create < 0 || retire < 0 || !(stage < create && create < retire) {
		t.Fatalf("route rebind transaction order changed: stage=%d create=%d retire=%d", stage, create, retire)
	}
}

func TestObservedPathValidationRejectsPartialRouteIdentity(t *testing.T) {
	for _, obs := range []underlayPathObservation{
		observedPath(testUnderlay(), 7, ""),
		observedPath(testUnderlay(), 0, "192.0.2.1"),
	} {
		if err := obs.Validate(); err == nil {
			t.Fatalf("partial physical route accepted: %+v", obs)
		}
	}
}
