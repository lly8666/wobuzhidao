package windowsruntime

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

type scriptedPathDiscoverer struct {
	mu        sync.Mutex
	initial   Underlay
	path      Underlay
	pathErr   error
	pathCalls int
}

func (d *scriptedPathDiscoverer) Discover(Profile) (Underlay, error) { return d.initial, nil }
func (d *scriptedPathDiscoverer) DiscoverPath(Profile) (Underlay, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pathCalls++
	if d.pathErr != nil {
		return Underlay{}, d.pathErr
	}
	return d.path, nil
}
func (d *scriptedPathDiscoverer) setPath(path Underlay, err error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.path = path
	d.pathErr = err
}
func (d *scriptedPathDiscoverer) resetCalls() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.pathCalls = 0
}
func (d *scriptedPathDiscoverer) calls() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.pathCalls
}

func controllerWithPathDiscoverer(r *recordingRunner, d *scriptedPathDiscoverer) *Controller {
	return NewController(r, d, &recordingTicketStore{runner: r, ticket: strings.Repeat("ab", 32), tunnelConfig: testTunnelConfigJSON})
}

func stopLifecycleMonitorForTest(c *Controller) {
	c.mu.Lock()
	c.stopPayloadIdleMonitorLocked()
	c.mu.Unlock()
}

func changedUnderlay(base Underlay) Underlay {
	out := base
	out.SourceIP = "192.0.2.99"
	return out
}

func planUsesUnderlay(t *testing.T, plan LanePlan, want Underlay) {
	t.Helper()
	got, err := lanePlanUnderlay(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !sameUnderlayPath(got, want) {
		t.Fatalf("plan underlay=%+v want=%+v", got, want)
	}
}

func countEvent(events []string, want string) int {
	n := 0
	for _, event := range events {
		if event == want {
			n++
		}
	}
	return n
}

func waitGameControlRequests(t *testing.T, requests <-chan gamelane.LaneSetCommand, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		select {
		case <-requests:
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for Game control request %d/%d", i+1, count)
		}
	}
}

func TestSameUnderlayPathCoversAuthoritativeIdentityFields(t *testing.T) {
	base := testUnderlay()
	cases := []struct {
		name   string
		mutate func(*Underlay)
	}{
		{"source-ip", func(u *Underlay) { u.SourceIP = "192.0.2.99" }},
		{"packet-device", func(u *Underlay) { u.PacketDevice = `\Device\NPF_{11111111-2222-3333-4444-555555555555}` }},
		{"source-mac", func(u *Underlay) { u.SourceMAC = "02:00:00:00:00:99" }},
		{"next-hop-mac", func(u *Underlay) { u.NextHopMAC = "02:00:00:00:01:99" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := base
			tc.mutate(&changed)
			if sameUnderlayPath(base, changed) {
				t.Fatalf("%s change was ignored", tc.name)
			}
		})
	}
	caseOnly := base
	caseOnly.PacketDevice = strings.ToLower(caseOnly.PacketDevice)
	caseOnly.SourceMAC = strings.ToUpper(caseOnly.SourceMAC)
	caseOnly.NextHopMAC = strings.ToUpper(caseOnly.NextHopMAC)
	if !sameUnderlayPath(base, caseOnly) {
		t.Fatal("case-only Windows device/MAC change must not rotate a lane")
	}
}

func TestUnderlayPathDiscoveryArgsExplicitlyUsePhysicalMonitorMode(t *testing.T) {
	args, err := underlayPathDiscoveryArgs(testProfile())
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-MonitorPhysicalPath", "-StatePath", testProfile().RouteState} {
		if !strings.Contains(joined, want) {
			t.Fatalf("path discovery args=%q missing %q", joined, want)
		}
	}
}

func TestUnderlayPathTickMigratesTwoLanesOneAtATime(t *testing.T) {
	r := &recordingRunner{}
	oldPath := testUnderlay()
	newPath := changedUnderlay(oldPath)
	d := &scriptedPathDiscoverer{initial: oldPath, path: oldPath}
	c := controllerWithPathDiscoverer(r, d)
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 2
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	stopLifecycleMonitorForTest(c)
	d.setPath(newPath, nil)
	control, requests := gameControlResponder(t, 4)
	setControllerGameControl(c, control)

	pathState := newUnderlayPathState()
	now := time.Now()
	if !c.runUnderlayPathTick(pathState, now) {
		t.Fatal("first path change did not own lifecycle tick")
	}
	waitGameControlRequests(t, requests, 2)
	c.mu.Lock()
	firstPlans := cloneLanePlans(c.lanePlans)
	firstBase := c.baseUnderlay
	c.mu.Unlock()
	planUsesUnderlay(t, firstPlans[1], newPath)
	planUsesUnderlay(t, firstPlans[2], oldPath)
	if !sameUnderlayPath(firstBase, newPath) {
		t.Fatalf("committed base underlay=%+v want=%+v", firstBase, newPath)
	}
	if got := c.executor.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("logical lanes after first path replacement=%v", got)
	}

	if !c.runUnderlayPathTick(pathState, now.Add(underlayPathLaneStagger)) {
		t.Fatal("second stale lane did not migrate")
	}
	waitGameControlRequests(t, requests, 2)
	c.mu.Lock()
	secondPlans := cloneLanePlans(c.lanePlans)
	c.mu.Unlock()
	planUsesUnderlay(t, secondPlans[1], newPath)
	planUsesUnderlay(t, secondPlans[2], newPath)
	if got := c.executor.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("private replacement slot leaked as logical lane: %v", got)
	}
}

func TestUnderlayPathCandidateFailurePreservesOldLaneAndBaseline(t *testing.T) {
	r := &recordingRunner{}
	oldPath := testUnderlay()
	newPath := changedUnderlay(oldPath)
	d := &scriptedPathDiscoverer{initial: oldPath, path: oldPath}
	c := controllerWithPathDiscoverer(r, d)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	stopLifecycleMonitorForTest(c)
	d.setPath(newPath, nil)
	r.failReady = "dtls-1-candidate-s5"
	c.mu.Lock()
	oldPlan := c.lanePlans[1]
	oldRef := c.lifecycle.Snapshot()[0].Ref
	c.mu.Unlock()

	pathState := newUnderlayPathState()
	now := time.Now()
	if !c.runUnderlayPathTick(pathState, now) {
		t.Fatal("failed path candidate must still own lifecycle tick")
	}
	c.mu.Lock()
	gotPlan := c.lanePlans[1]
	gotRef := c.lifecycle.Snapshot()[0].Ref
	gotBase := c.baseUnderlay
	c.mu.Unlock()
	if !reflect.DeepEqual(gotPlan, oldPlan) || gotRef != oldRef {
		t.Fatalf("candidate failure changed authoritative lane: plan=%+v ref=%+v", gotPlan, gotRef)
	}
	if !sameUnderlayPath(gotBase, oldPath) {
		t.Fatalf("candidate failure committed unqualified path: %+v", gotBase)
	}
	attempts := countEvent(r.events, "start:faketcp-1-candidate-s5")
	if !c.runUnderlayPathTick(pathState, now.Add(underlayPathRetryDelay/2)) {
		t.Fatal("pending path retry must suppress stale-path lifecycle work")
	}
	if got := countEvent(r.events, "start:faketcp-1-candidate-s5"); got != attempts {
		t.Fatalf("path candidate retried before bounded deadline: before=%d after=%d", attempts, got)
	}
}

func TestUnderlayPathDiscoveryFailureIsFailOpen(t *testing.T) {
	r := &recordingRunner{}
	oldPath := testUnderlay()
	d := &scriptedPathDiscoverer{initial: oldPath, path: oldPath}
	c := controllerWithPathDiscoverer(r, d)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	stopLifecycleMonitorForTest(c)
	d.setPath(Underlay{}, errors.New("route query failed"))
	cut := len(r.events)
	if c.runUnderlayPathTick(newUnderlayPathState(), time.Now()) {
		t.Fatal("discovery failure must not claim replacement work")
	}
	for _, event := range r.events[cut:] {
		if strings.Contains(event, "candidate") {
			t.Fatalf("discovery failure started candidate: %v", r.events[cut:])
		}
	}
	c.mu.Lock()
	base := c.baseUnderlay
	c.mu.Unlock()
	if !sameUnderlayPath(base, oldPath) {
		t.Fatalf("discovery failure changed baseline: %+v", base)
	}
}

func TestUnderlayReplacementRejectsStaleGeneration(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	stopLifecycleMonitorForTest(c)
	control, requests := gameControlResponder(t, 2)
	setControllerGameControl(c, control)
	c.mu.Lock()
	stale := c.lifecycle.Snapshot()[0].Ref
	c.mu.Unlock()
	if err := c.ReplaceLane(1); err != nil {
		t.Fatal(err)
	}
	waitGameControlRequests(t, requests, 2)
	cut := len(r.events)
	if err := c.replaceLaneOnUnderlay(1, stale, changedUnderlay(testUnderlay())); !errors.Is(err, errStaleLaneReplacement) {
		t.Fatalf("stale path trigger err=%v", err)
	}
	if len(r.events) != cut {
		t.Fatalf("stale generation started transport work: %v", r.events[cut:])
	}
}

func TestDormantDoesNotReplaceAndWakeRediscoversUnderlay(t *testing.T) {
	r := &recordingRunner{}
	oldPath := testUnderlay()
	newPath := changedUnderlay(oldPath)
	d := &scriptedPathDiscoverer{initial: oldPath, path: oldPath}
	c := controllerWithPathDiscoverer(r, d)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	stopLifecycleMonitorForTest(c)
	d.resetCalls()
	d.setPath(newPath, nil)
	control, requests := gameControlResponder(t, 2)
	setControllerGameControl(c, control)
	if err := c.Dormant(); err != nil {
		t.Fatal(err)
	}
	waitGameControlRequests(t, requests, 1)
	pathState := newUnderlayPathState()
	if c.runUnderlayPathTick(pathState, time.Now()) {
		t.Fatal("DORMANT path tick attempted replacement")
	}
	if d.calls() != 0 {
		t.Fatalf("DORMANT path discovery calls=%d want=0", d.calls())
	}
	if err := c.Wake(); err != nil {
		t.Fatal(err)
	}
	waitGameControlRequests(t, requests, 1)
	if d.calls() != 1 {
		t.Fatalf("Wake path discovery calls=%d want=1", d.calls())
	}
	c.mu.Lock()
	plan := c.lanePlans[1]
	base := c.baseUnderlay
	c.mu.Unlock()
	planUsesUnderlay(t, plan, newPath)
	if !sameUnderlayPath(base, newPath) {
		t.Fatalf("Wake baseline=%+v want=%+v", base, newPath)
	}
}
