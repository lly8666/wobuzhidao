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

func TestProductLifecycleOwnsExactlyOneActivePublicTransport(t *testing.T) {
	lc, err := NewLaneLifecycle(1)
	if err != nil { t.Fatal(err) }
	a, err := lc.AttachInitial(1)
	if err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1}) { t.Fatalf("active=%v", got) }
	if _, err := lc.AttachInitial(2); !errors.Is(err, ErrTransportLanes) { t.Fatalf("second initial public transport err=%v", err) }
	if _, err := lc.BeginReplacement(a, 2); !errors.Is(err, ErrTransportLanes) { t.Fatalf("overlapping replacement err=%v", err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1}) { t.Fatalf("overlap attempt mutated active=%v", got) }
}

func TestProductLifecycleRejectsNonSingleDesiredCardinality(t *testing.T) {
	for _, desired := range []int{0, 2, 3, 4, 5} {
		if _, err := NewLaneLifecycle(desired); !errors.Is(err, ErrTransportLanes) { t.Fatalf("desired=%d err=%v", desired, err) }
	}
}

func TestDormantClosesOnlyProductTransportAndPreservesSingleWakePolicy(t *testing.T) {
	lc, err := NewLaneLifecycle(1)
	if err != nil { t.Fatal(err) }
	ref, err := lc.AttachInitial(1)
	if err != nil { t.Fatal(err) }
	closed := lc.Dormant()
	if got := laneIDs(closed); !reflect.DeepEqual(got, []uint8{1}) { t.Fatalf("closed=%v", got) }
	if closed[0] != ref { t.Fatalf("closed ref=%+v want=%+v", closed[0], ref) }
	if len(lc.ActiveForSend()) != 0 || len(lc.Snapshot()) != 0 { t.Fatal("dormant retained active transport state") }
	if lc.Desired() != 1 { t.Fatalf("desired wake policy=%d", lc.Desired()) }
}

func TestSameIDGenerationFenceRemainsValidForSingleTransport(t *testing.T) {
	lc, err := NewLaneLifecycle(1)
	if err != nil { t.Fatal(err) }
	old, err := lc.AttachInitial(1)
	if err != nil { t.Fatal(err) }
	fresh, err := lc.PromoteSameIDReplacement(old)
	if err != nil { t.Fatal(err) }
	if fresh.ID != old.ID || fresh.Generation <= old.Generation { t.Fatalf("old=%+v fresh=%+v", old, fresh) }
	if len(lc.Snapshot()) != 1 { t.Fatalf("transport count=%d want=1", len(lc.Snapshot())) }
	if _, err := lc.current(old); !errors.Is(err, ErrStaleLaneGeneration) { t.Fatalf("old generation not fenced: %v", err) }
	if _, err := lc.current(fresh); err != nil { t.Fatalf("fresh generation missing: %v", err) }
}
