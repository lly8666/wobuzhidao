package windowsruntime

import (
	"encoding/json"
	"net"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func payloadIdleControlResponder(t *testing.T) (string, func(), <-chan gamelane.LaneSetCommand) {
	t.Helper()
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	var sequence atomic.Uint64
	var lastUnixNano atomic.Int64
	sets := make(chan gamelane.LaneSetCommand, 16)
	go func() {
		buf := make([]byte, 4096)
		for {
			n, peer, err := conn.ReadFromUDP(buf)
			if err != nil {
				return
			}
			op, err := gamelane.ParseLaneControlOp(buf[:n])
			if err != nil {
				continue
			}
			switch op {
			case gamelane.LaneControlActivity:
				seq := sequence.Load()
				last := int64(0)
				if seq > 0 {
					last = lastUnixNano.Load()
				}
				wire, _ := json.Marshal(gamelane.LaneActivityReply{OK: true, Activity: gamelane.PayloadActivity{Sequence: seq, LastPayloadActivityUnixNano: last}})
				_, _ = conn.WriteToUDP(wire, peer)
			case gamelane.LaneControlSet:
				cmd, err := gamelane.ParseLaneSetCommand(buf[:n])
				if err != nil {
					continue
				}
				sets <- cmd
				wire, _ := json.Marshal(gamelane.LaneControlReply{OK: true, Active: uniqueGameLaneIDsFromTargets(cmd.Lanes)})
				_, _ = conn.WriteToUDP(wire, peer)
			}
		}
	}()

	markPayload := func() {
		lastUnixNano.Store(time.Now().UnixNano())
		sequence.Add(1)
	}
	return conn.LocalAddr().String(), markPayload, sets
}

func waitControllerState(t *testing.T, c *Controller, want RuntimeState, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.State() == want {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("runtime state=%s want=%s", c.State(), want)
}

func TestPayloadIdleMonitorDormantsWithoutPayloadAndWakesOnSequenceAdvance(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	control, markPayload, sets := payloadIdleControlResponder(t)
	setControllerGameControl(c, control)
	c.startPayloadIdleMonitor(50 * time.Millisecond)

	waitControllerState(t, c, RuntimeDormant, 2*time.Second)
	select {
	case cmd := <-sets:
		if len(cmd.Lanes) != 0 {
			t.Fatalf("idle DORMANT Game barrier lanes=%v", cmd.Lanes)
		}
	case <-time.After(time.Second):
		t.Fatal("idle DORMANT did not publish empty Game target set")
	}

	// Activity/control queries above did not refresh idle. Only a real payload
	// sequence advance is allowed to wake the existing Logical Tunnel.
	markPayload()
	waitControllerState(t, c, RuntimeConnected, 2*time.Second)
	select {
	case cmd := <-sets:
		if len(cmd.Lanes) == 0 || cmd.Lanes[0].ID != 1 {
			t.Fatalf("wake first READY Game lanes=%v", cmd.Lanes)
		}
	case <-time.After(time.Second):
		t.Fatal("payload wake did not publish first READY lane")
	}

	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestConnectStartsLifecycleMonitorEvenWhenPayloadIdleDisabled(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds int
	}{
		{name: "idle-disabled-age-still-enabled", seconds: 0},
		{name: "idle-enabled", seconds: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &recordingRunner{}
			c := testController(r)
			p := testProfile()
			p.TunnelIPv4 = ""
			p.IdleTimeoutSeconds = tc.seconds
			if err := c.Connect(p); err != nil {
				t.Fatal(err)
			}
			c.mu.Lock()
			running := c.idleStop != nil
			c.mu.Unlock()
			if !running {
				t.Fatal("lifecycle monitor is not running")
			}
			if tc.seconds == 0 && c.State() != RuntimeConnected {
				t.Fatalf("idle_timeout=0 changed state=%s", c.State())
			}
			if err := c.Disconnect(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLaneAgeDeadlinesAreThirtyToSixtyMinutesAndStaggered(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ages := newLaneAgeState()
	plans := map[int]LanePlan{}
	for id := 1; id <= 4; id++ {
		plans[id] = LanePlan{ID: id, Slot: id}
	}
	zero := func(time.Duration) time.Duration { return 0 }
	ages.reconcile(plans, now, zero)
	if len(ages.deadlines) != 4 {
		t.Fatalf("deadline count=%d", len(ages.deadlines))
	}

	ordered := make([]time.Time, 0, 4)
	for id := 1; id <= 4; id++ {
		deadline := ages.deadlines[id]
		age := deadline.Sub(now)
		if age < minLaneAge || age > maxLaneAge {
			t.Fatalf("lane %d age=%s outside [%s,%s]", id, age, minLaneAge, maxLaneAge)
		}
		ordered = append(ordered, deadline)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Before(ordered[j]) })
	for i := 1; i < len(ordered); i++ {
		if gap := ordered[i].Sub(ordered[i-1]); gap < minLaneAgeStagger {
			t.Fatalf("age deadlines not staggered: gap=%s", gap)
		}
	}
}

func TestLaneAgeIncarnationSlotChangeGetsFreshDeadline(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ages := newLaneAgeState()
	zero := func(time.Duration) time.Duration { return 0 }
	plans := map[int]LanePlan{1: {ID: 1, Slot: 1}}
	ages.reconcile(plans, now, zero)
	before := ages.deadlines[1]

	plans[1] = LanePlan{ID: 1, Slot: makeBeforeBreakCandidateSlot}
	later := now.Add(time.Second)
	ages.reconcile(plans, later, zero)
	after := ages.deadlines[1]
	if ages.slots[1] != makeBeforeBreakCandidateSlot {
		t.Fatalf("tracked slot=%d", ages.slots[1])
	}
	if !after.After(before) {
		t.Fatalf("replacement did not refresh deadline before=%s after=%s", before, after)
	}
	if age := after.Sub(later); age < minLaneAge || age > maxLaneAge {
		t.Fatalf("replacement age=%s outside [%s,%s]", age, minLaneAge, maxLaneAge)
	}
}

func TestLaneAgeDueSelectsOnlyEarliestLane(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ages := newLaneAgeState()
	ages.deadlines[1] = now.Add(-time.Second)
	ages.deadlines[2] = now.Add(-2 * time.Second)
	ages.deadlines[3] = now.Add(time.Second)
	laneID, ok := ages.nextDue(now)
	if !ok || laneID != 2 {
		t.Fatalf("next due lane=%d ok=%v", laneID, ok)
	}
}

func TestLaneAgeStateClearsRemovedLanesAndRebuilds(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	ages := newLaneAgeState()
	zero := func(time.Duration) time.Duration { return 0 }
	ages.reconcile(map[int]LanePlan{1: {ID: 1, Slot: 1}, 2: {ID: 2, Slot: 2}}, now, zero)
	ages.reconcile(map[int]LanePlan{}, now.Add(time.Second), zero)
	if len(ages.deadlines) != 0 || len(ages.slots) != 0 {
		t.Fatalf("removed lanes retained deadlines=%v slots=%v", ages.deadlines, ages.slots)
	}
	ages.reconcile(map[int]LanePlan{1: {ID: 1, Slot: 1}}, now.Add(2*time.Second), zero)
	if len(ages.deadlines) != 1 || ages.slots[1] != 1 {
		t.Fatalf("rebuilt age state deadlines=%v slots=%v", ages.deadlines, ages.slots)
	}
}

func TestProfileRejectsNegativeIdleTimeout(t *testing.T) {
	p := testProfile()
	p.IdleTimeoutSeconds = -1
	if err := p.Validate(); err == nil {
		t.Fatal("negative idle timeout unexpectedly accepted")
	}
}

func TestPayloadIdleObservationUsesSequenceAsAuthority(t *testing.T) {
	base := time.Unix(1700000000, 0)
	o := newPayloadIdleObservation(base)
	if changed, advanced := o.observe(gamelane.PayloadActivity{Sequence: 7, LastPayloadActivityUnixNano: 1}, base.Add(time.Second)); !changed || advanced {
		t.Fatalf("initial observation changed=%v advanced=%v", changed, advanced)
	}
	if changed, _ := o.observe(gamelane.PayloadActivity{Sequence: 7, LastPayloadActivityUnixNano: 999999}, base.Add(2*time.Second)); changed {
		t.Fatal("child timestamp changed idle policy without a payload sequence advance")
	}
	if changed, advanced := o.observe(gamelane.PayloadActivity{Sequence: 8, LastPayloadActivityUnixNano: 2}, base.Add(3*time.Second)); !changed || !advanced {
		t.Fatalf("payload sequence advance changed=%v advanced=%v", changed, advanced)
	}
}
