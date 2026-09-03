package windowsruntime

import (
	"reflect"
	"strings"
	"testing"
)

func laneGenerations(c *Controller) map[int]uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := map[int]uint64{}
	for _, snapshot := range c.lifecycle.Snapshot() {
		out[int(snapshot.Ref.ID)] = snapshot.Ref.Generation
	}
	return out
}

func TestControllerRotateActiveLanesSequentiallyKeepsLogicalSet(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 3
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	control, requests := gameControlResponder(t, 6)
	setControllerGameControl(c, control)
	before := laneGenerations(c)

	if err := c.RotateActiveLanes(); err != nil {
		t.Fatal(err)
	}
	for lane := 1; lane <= 3; lane++ {
		overlap := <-requests
		promotion := <-requests
		if len(overlap.Lanes) != 4 {
			t.Fatalf("lane %d overlap target count=%d want=4: %v", lane, len(overlap.Lanes), overlap.Lanes)
		}
		if len(promotion.Lanes) != 3 {
			t.Fatalf("lane %d promotion target count=%d want=3: %v", lane, len(promotion.Lanes), promotion.Lanes)
		}
		if len(targetsForID(overlap.Lanes, uint8(lane))) != 2 {
			t.Fatalf("lane %d was not the only overlapping logical ID: %v", lane, overlap.Lanes)
		}
	}

	after := laneGenerations(c)
	for lane := 1; lane <= 3; lane++ {
		if after[lane] != before[lane]+1 {
			t.Fatalf("lane %d generation before=%d after=%d", lane, before[lane], after[lane])
		}
	}
	if got := c.executor.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{1, 2, 3}) {
		t.Fatalf("manual reconnect changed logical lane set=%v", got)
	}
	c.mu.Lock()
	plans := cloneLanePlans(c.lanePlans)
	state := c.state
	c.mu.Unlock()
	if state != RuntimeConnected {
		t.Fatalf("manual reconnect state=%s", state)
	}
	seenSlots := map[int]bool{}
	for id, plan := range plans {
		if plan.ID != id || plan.Slot < 1 || plan.Slot > 5 {
			t.Fatalf("lane %d plan=%+v", id, plan)
		}
		if seenSlots[plan.Slot] {
			t.Fatalf("manual reconnect reused authoritative physical slot %d: %+v", plan.Slot, plans)
		}
		seenSlots[plan.Slot] = true
	}
}

func TestControllerRotateActiveLanesCandidateFailurePreservesHealthyA(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 1
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	control, _ := gameControlResponder(t, 1)
	setControllerGameControl(c, control)
	c.mu.Lock()
	oldPlan := c.lanePlans[1]
	oldRef := c.lifecycle.Snapshot()[0].Ref
	c.mu.Unlock()
	r.failReady = "dtls-1-candidate-s5"

	err := c.RotateActiveLanes()
	if err == nil || !strings.Contains(err.Error(), "manual reconnect lane 1") {
		t.Fatalf("manual reconnect error=%v", err)
	}
	c.mu.Lock()
	gotPlan := c.lanePlans[1]
	gotRef := c.lifecycle.Snapshot()[0].Ref
	state := c.state
	c.mu.Unlock()
	if gotPlan.Slot != oldPlan.Slot || gotRef != oldRef || state != RuntimeConnected {
		t.Fatalf("candidate failure changed authority plan=%+v ref=%+v state=%s", gotPlan, gotRef, state)
	}
	if got := c.executor.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("candidate failure changed active lanes=%v", got)
	}
}

func TestControllerRotateActiveLanesRejectsDormant(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 1
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	control, _ := gameControlResponder(t, 1)
	setControllerGameControl(c, control)
	if err := c.Dormant(); err != nil {
		t.Fatal(err)
	}
	if err := c.RotateActiveLanes(); err == nil || !strings.Contains(err.Error(), "while dormant") {
		t.Fatalf("dormant manual reconnect error=%v", err)
	}
}
