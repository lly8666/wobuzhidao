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

func TestNormalMakeBeforeBreakAtoABtoB(t *testing.T) {
	lc, err := NewLaneLifecycle(1)
	if err != nil { t.Fatal(err) }
	a, err := lc.AttachInitial(1)
	if err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1}) { t.Fatalf("A active=%v", got) }

	b, err := lc.BeginReplacement(a, 2)
	if err != nil { t.Fatal(err) }
	// Candidate is not used for logical sends before full FakeTCP+Reality+DTLS+LINK health.
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1}) { t.Fatalf("candidate leaked before health: %v", got) }
	if err := lc.CandidateHealthy(b); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1, 2}) { t.Fatalf("A+B active=%v", got) }
	if err := lc.BeginDrain(a, b); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{2}) { t.Fatalf("new sends did not move to B: %v", got) }
	if err := lc.Retire(a); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{2}) { t.Fatalf("B active=%v", got) }
}

func TestCandidateFailureLeavesOldLaneUntouched(t *testing.T) {
	lc, _ := NewLaneLifecycle(1)
	a, _ := lc.AttachInitial(1)
	b, err := lc.BeginReplacement(a, 2)
	if err != nil { t.Fatal(err) }
	if err := lc.CandidateFailed(b); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1}) { t.Fatalf("old lane changed after candidate failure: %v", got) }
	if snaps := lc.Snapshot(); len(snaps) != 1 || snaps[0].Ref != a || snaps[0].Phase != LaneActive { t.Fatalf("snapshot=%+v", snaps) }
}

func TestGameReplacementABtoABCtoBC(t *testing.T) {
	lc, err := NewLaneLifecycle(2)
	if err != nil { t.Fatal(err) }
	a, _ := lc.AttachInitial(1)
	_, _ = lc.AttachInitial(2)
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1, 2}) { t.Fatalf("A+B=%v", got) }

	c, err := lc.BeginReplacement(a, 3)
	if err != nil { t.Fatal(err) }
	if err := lc.CandidateHealthy(c); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{1, 2, 3}) { t.Fatalf("A+B+C=%v", got) }
	if err := lc.BeginDrain(a, c); err != nil { t.Fatal(err) }
	if err := lc.Retire(a); err != nil { t.Fatal(err) }
	if got := laneIDs(lc.ActiveForSend()); !reflect.DeepEqual(got, []uint8{2, 3}) { t.Fatalf("B+C=%v", got) }
}

func TestLifecycleRejectsFifthLaneAndFencesStaleGeneration(t *testing.T) {
	lc, _ := NewLaneLifecycle(4)
	refs := make([]LaneRef, 0, 4)
	for id := uint8(1); id <= 4; id++ {
		ref, err := lc.AttachInitial(id)
		if err != nil { t.Fatal(err) }
		refs = append(refs, ref)
	}
	if _, err := lc.BeginReplacement(refs[0], 5); !errors.Is(err, ErrTransportLanes) { t.Fatalf("fifth lane err=%v", err) }

	// Reuse an ID after a completed replacement and prove the old generation can
	// no longer retire or mutate the new incarnation.
	lc2, _ := NewLaneLifecycle(1)
	a, _ := lc2.AttachInitial(1)
	b, _ := lc2.BeginReplacement(a, 2)
	_ = lc2.CandidateHealthy(b)
	_ = lc2.BeginDrain(a, b)
	_ = lc2.Retire(a)
	a2, err := lc2.BeginReplacement(b, 1)
	if err != nil { t.Fatal(err) }
	if a2.Generation == a.Generation { t.Fatalf("lane generation reused: old=%+v new=%+v", a, a2) }
	if err := lc2.CandidateHealthy(a); !errors.Is(err, ErrStaleLaneGeneration) && !errors.Is(err, ErrLaneNotFound) {
		t.Fatalf("stale generation unexpectedly accepted: %v", err)
	}
}

func TestDormantClosesAllLanesButPreservesDesiredWakePolicy(t *testing.T) {
	lc, _ := NewLaneLifecycle(3)
	_, _ = lc.AttachInitial(1)
	_, _ = lc.AttachInitial(2)
	_, _ = lc.AttachInitial(3)
	closed := lc.Dormant()
	if got := laneIDs(closed); !reflect.DeepEqual(got, []uint8{1, 2, 3}) { t.Fatalf("closed=%v", got) }
	if len(lc.ActiveForSend()) != 0 || len(lc.Snapshot()) != 0 { t.Fatal("dormant retained active transport state") }
	if lc.Desired() != 3 { t.Fatalf("desired wake policy=%d", lc.Desired()) }
}
