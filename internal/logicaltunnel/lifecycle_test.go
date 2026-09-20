package logicaltunnel

import (
	"errors"
	"reflect"
	"testing"
)

func laneIDs(refs []LaneRef) []uint8 {
	out := make([]uint8, len(refs))
	for i := range refs {
		out[i] = refs[i].ID
	}
	return out
}

func TestProductLifecycleKeepsOneToFourLogicalLanes(t *testing.T) {
	for _, desired := range []int{1, 2, 3, 4} {
		lc, err := NewLaneLifecycle(desired)
		if err != nil {
			t.Fatalf("desired=%d: %v", desired, err)
		}
		if lc.Desired() != desired {
			t.Fatalf("desired=%d got=%d", desired, lc.Desired())
		}
	}
	for _, desired := range []int{-1, 0, 5} {
		if _, err := NewLaneLifecycle(desired); !errors.Is(err, ErrTransportLanes) {
			t.Fatalf("invalid desired=%d err=%v", desired, err)
		}
	}

	lc, err := NewLaneLifecycle(4)
	if err != nil {
		t.Fatal(err)
	}
	for id := uint8(1); id <= 4; id++ {
		if _, err := lc.AttachInitial(id); err != nil {
			t.Fatalf("attach lane %d: %v", id, err)
		}
	}
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1, 2, 3, 4}) {
		t.Fatalf("active=%v", got)
	}
	if _, err := lc.AttachInitial(5); !errors.Is(err, ErrTransportLanes) {
		t.Fatalf("fifth logical lane err=%v", err)
	}
}

func TestSameIDPromotionAdvancesGenerationWithoutNewLogicalLane(t *testing.T) {
	lc, err := NewLaneLifecycle(1)
	if err != nil {
		t.Fatal(err)
	}
	old, err := lc.AttachInitial(1)
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := lc.PromoteSameIDReplacement(old)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.ID != old.ID || fresh.Generation <= old.Generation {
		t.Fatalf("old=%+v fresh=%+v", old, fresh)
	}
	if len(lc.Snapshot()) != 1 {
		t.Fatalf("logical lanes=%d want=1", len(lc.Snapshot()))
	}
	if _, err := lc.current(old); !errors.Is(err, ErrStaleLaneGeneration) {
		t.Fatalf("old generation not fenced: %v", err)
	}
	if _, err := lc.current(fresh); err != nil {
		t.Fatalf("fresh generation missing: %v", err)
	}
}

func TestDormantClearsTransportMembershipButPreservesWakePolicy(t *testing.T) {
	lc, err := NewLaneLifecycle(3)
	if err != nil {
		t.Fatal(err)
	}
	for id := uint8(1); id <= 3; id++ {
		if _, err := lc.AttachInitial(id); err != nil {
			t.Fatal(err)
		}
	}
	closed := lc.Dormant()
	if got := laneIDs(closed); !reflect.DeepEqual(got, []uint8{1, 2, 3}) {
		t.Fatalf("closed=%v", got)
	}
	if len(lc.ActiveForSend()) != 0 || len(lc.Snapshot()) != 0 {
		t.Fatal("dormant retained active transport membership")
	}
	if lc.Desired() != 3 {
		t.Fatalf("desired wake policy=%d", lc.Desired())
	}
	fresh, err := lc.AttachInitial(1)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Generation <= closed[0].Generation {
		t.Fatalf("wake generation=%d old=%d", fresh.Generation, closed[0].Generation)
	}
}
