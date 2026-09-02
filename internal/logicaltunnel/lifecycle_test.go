package logicaltunnel

import (
	"errors"
	"reflect"
	"testing"
)

func laneIDs(refs []LaneRef) []uint8 {
	out := make([]uint8, len(refs))
	for i := range refs { out[i] = refs[i].ID }
	return out
}

func TestProductLifecycleSupportsOneToFourActivePublicTransports(t *testing.T) {
	lc, err := NewLaneLifecycle(4)
	if err != nil { t.Fatal(err) }
	for id := uint8(1); id <= 4; id++ {
		if _, err := lc.AttachInitial(id); err != nil { t.Fatalf("attach lane %d: %v", id, err) }
	}
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1, 2, 3, 4}) { t.Fatalf("active=%v", got) }
	if _, err := lc.AttachInitial(5); !errors.Is(err, ErrTransportLanes) { t.Fatalf("fifth public transport err=%v", err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1, 2, 3, 4}) { t.Fatalf("fifth-lane rejection mutated active=%v", got) }
}

func TestProductLifecycleAcceptsAuthorizedDesiredCardinality(t *testing.T) {
	for _, desired := range []int{1, 2, 3, 4} {
		lc, err := NewLaneLifecycle(desired)
		if err != nil { t.Fatalf("desired=%d rejected: %v", desired, err) }
		if lc.Desired() != desired { t.Fatalf("desired=%d got=%d", desired, lc.Desired()) }
	}
	for _, desired := range []int{-1, 0, 5} {
		if _, err := NewLaneLifecycle(desired); !errors.Is(err, ErrTransportLanes) { t.Fatalf("invalid desired=%d err=%v", desired, err) }
	}
}

func TestDormantClosesAllProductTransportsAndPreservesWakePolicy(t *testing.T) {
	lc, err := NewLaneLifecycle(3)
	if err != nil { t.Fatal(err) }
	for id := uint8(1); id <= 3; id++ {
		if _, err := lc.AttachInitial(id); err != nil { t.Fatal(err) }
	}
	closed := lc.Dormant()
	if got := laneIDs(closed); !reflect.DeepEqual(got, []uint8{1, 2, 3}) { t.Fatalf("closed=%v", got) }
	if len(lc.ActiveForSend()) != 0 || len(lc.Snapshot()) != 0 { t.Fatal("dormant retained active transport state") }
	if lc.Desired() != 3 { t.Fatalf("desired wake policy=%d", lc.Desired()) }
}

func TestMakeBeforeBreakCandidatePreservesHealthyOldLane(t *testing.T) {
	lc, err := NewLaneLifecycle(2)
	if err != nil { t.Fatal(err) }
	old, err := lc.AttachInitial(1)
	if err != nil { t.Fatal(err) }
	candidate, err := lc.BeginReplacement(old, 2)
	if err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1}) { t.Fatalf("candidate became send-active before health gate: %v", got) }
	if err := lc.CandidateFailed(candidate); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1}) { t.Fatalf("candidate failure disturbed old lane: %v", got) }

	candidate, err = lc.BeginReplacement(old, 2)
	if err != nil { t.Fatal(err) }
	if err := lc.CandidateHealthy(candidate); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1, 2}) { t.Fatalf("healthy overlap=%v", got) }
	if err := lc.BeginDrain(old, candidate); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{2}) { t.Fatalf("draining old remained send-active: %v", got) }
	if err := lc.Retire(old); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{2}) { t.Fatalf("replacement active=%v", got) }
}

func TestSameIDGenerationFenceRemainsValidPerLane(t *testing.T) {
	lc, err := NewLaneLifecycle(2)
	if err != nil { t.Fatal(err) }
	old, err := lc.AttachInitial(1)
	if err != nil { t.Fatal(err) }
	if _, err := lc.AttachInitial(2); err != nil { t.Fatal(err) }
	fresh, err := lc.PromoteSameIDReplacement(old)
	if err != nil { t.Fatal(err) }
	if fresh.ID != old.ID || fresh.Generation <= old.Generation { t.Fatalf("old=%+v fresh=%+v", old, fresh) }
	if len(lc.Snapshot()) != 2 { t.Fatalf("transport count=%d want=2", len(lc.Snapshot())) }
	if _, err := lc.current(old); !errors.Is(err, ErrStaleLaneGeneration) { t.Fatalf("old generation not fenced: %v", err) }
	if _, err := lc.current(fresh); err != nil { t.Fatalf("fresh generation missing: %v", err) }
}
