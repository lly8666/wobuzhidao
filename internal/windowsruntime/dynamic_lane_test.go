package windowsruntime

import (
	"reflect"
	"strings"
	"testing"
)

func dynamicTestLane(id, slot int, suffix string) LanePlan {
	if suffix != "" && !strings.HasPrefix(suffix, "-") {
		suffix = "-" + suffix
	}
	return LanePlan{
		ID:   id,
		Slot: slot,
		FakeTCP: Command{Name: "faketcp-" + itoa(id) + suffix},
		DTLS:    Command{Name: "dtls-" + itoa(id) + suffix},
		Link:    Command{Name: "link-" + itoa(id) + suffix},
	}
}

func TestValidateDynamicLanePlanAllowsLogicalIDsAndPrivateCandidateSlot(t *testing.T) {
	for id := 1; id <= 4; id++ {
		if err := validateDynamicLanePlan(dynamicTestLane(id, id, "")); err != nil {
			t.Fatalf("normal lane %d rejected: %v", id, err)
		}
		if err := validateDynamicLanePlan(dynamicTestLane(id, makeBeforeBreakCandidateSlot, "candidate-s5")); err != nil {
			t.Fatalf("candidate lane %d rejected: %v", id, err)
		}
	}
	if err := validateDynamicLanePlan(dynamicTestLane(5, 5, "")); err == nil {
		t.Fatal("logical lane 5 must be rejected")
	}
	if err := validateDynamicLanePlan(dynamicTestLane(1, 2, "candidate-s2")); err == nil {
		t.Fatal("logical lane 1 must not steal normal transport slot 2")
	}
	if err := validateDynamicLanePlan(dynamicTestLane(1, 6, "candidate-s6")); err == nil {
		t.Fatal("transport slot 6 must be rejected")
	}
}

func TestExecutorDynamicLaneFailureKeepsSharedRuntimeAndOldLane(t *testing.T) {
	r := &recordingRunner{}
	e := NewExecutor(r)
	pre := map[int]Process{1: &recordingProcess{runner: r, name: "faketcp-1"}}
	if err := e.StartMultiLane(testMultiExecutorPlan(1), pre); err != nil {
		t.Fatal(err)
	}
	baseline := append([]string(nil), r.events...)
	r.failReady = "dtls-2"
	lane2 := dynamicTestLane(2, 2, "")
	if err := e.StartDynamicLane(lane2, &recordingProcess{runner: r, name: "faketcp-2"}); err == nil {
		t.Fatal("expected lane-2 readiness failure")
	}
	if got := e.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("surviving lanes=%v", got)
	}
	newEvents := r.events[len(baseline):]
	want := []string{"ready:faketcp-2", "start:dtls-2", "ready:dtls-2", "stop:dtls-2", "stop:faketcp-2"}
	if !reflect.DeepEqual(newEvents, want) {
		t.Fatalf("lane rollback=%v want=%v", newEvents, want)
	}
	for _, ev := range newEvents {
		if ev == "stop:game" || ev == "stop:tun" || ev == "run:route-cleanup" {
			t.Fatalf("lane failure disturbed shared runtime: %v", newEvents)
		}
	}
}

func TestExecutorIndependentDynamicLaneRunsAlongsideExistingLane(t *testing.T) {
	r := &recordingRunner{}
	e := NewExecutor(r)
	pre := map[int]Process{1: &recordingProcess{runner: r, name: "faketcp-1"}}
	if err := e.StartMultiLane(testMultiExecutorPlan(1), pre); err != nil {
		t.Fatal(err)
	}
	lane2 := dynamicTestLane(2, 2, "")
	if err := e.StartDynamicLane(lane2, &recordingProcess{runner: r, name: "faketcp-2"}); err != nil {
		t.Fatal(err)
	}
	if got := e.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatalf("lanes after dynamic start=%v", got)
	}
	cut := len(r.events)
	if err := e.StopDynamicLane(1); err != nil {
		t.Fatal(err)
	}
	if got := e.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{2}) {
		t.Fatalf("lanes after retiring lane 1=%v", got)
	}
	want := []string{"stop:link-1", "stop:dtls-1", "stop:faketcp-1"}
	if !reflect.DeepEqual(r.events[cut:], want) {
		t.Fatalf("retire events=%v want=%v", r.events[cut:], want)
	}
}

func TestExecutorCandidateSlotFiveCoexistsAndStopsWithoutTouchingOldLane(t *testing.T) {
	r := &recordingRunner{}
	e := NewExecutor(r)
	pre := map[int]Process{1: &recordingProcess{runner: r, name: "faketcp-1"}}
	if err := e.StartMultiLane(testMultiExecutorPlan(1), pre); err != nil {
		t.Fatal(err)
	}
	baseline := len(r.events)
	candidate := dynamicTestLane(1, makeBeforeBreakCandidateSlot, "candidate-s5")
	if err := e.StartDynamicLane(candidate, &recordingProcess{runner: r, name: candidate.FakeTCP.Name}); err != nil {
		t.Fatal(err)
	}
	if got := e.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("candidate must share logical lane identity, got=%v", got)
	}
	startEvents := r.events[baseline:]
	wantStart := []string{
		"ready:faketcp-1-candidate-s5",
		"start:dtls-1-candidate-s5", "ready:dtls-1-candidate-s5",
		"start:link-1-candidate-s5", "ready:link-1-candidate-s5",
	}
	if !reflect.DeepEqual(startEvents, wantStart) {
		t.Fatalf("candidate start events=%v want=%v", startEvents, wantStart)
	}

	cut := len(r.events)
	if err := e.StopDynamicLanePlan(candidate); err != nil {
		t.Fatal(err)
	}
	if got := e.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("old lane must survive candidate stop, got=%v", got)
	}
	wantStop := []string{"stop:link-1-candidate-s5", "stop:dtls-1-candidate-s5", "stop:faketcp-1-candidate-s5"}
	if !reflect.DeepEqual(r.events[cut:], wantStop) {
		t.Fatalf("candidate stop events=%v want=%v", r.events[cut:], wantStop)
	}
	for _, ev := range r.events[cut:] {
		if ev == "stop:link-1" || ev == "stop:dtls-1" || ev == "stop:faketcp-1" || ev == "stop:game" || ev == "stop:tun" {
			t.Fatalf("candidate stop disturbed old/shared runtime: %v", r.events[cut:])
		}
	}
}

func TestExecutorDynamicLaneRejectsProcessNameCollisionWithoutMutation(t *testing.T) {
	r := &recordingRunner{}
	e := NewExecutor(r)
	pre := map[int]Process{1: &recordingProcess{runner: r, name: "faketcp-1"}}
	if err := e.StartMultiLane(testMultiExecutorPlan(1), pre); err != nil {
		t.Fatal(err)
	}
	baseline := append([]string(nil), r.events...)
	collision := LanePlan{
		ID:      1,
		Slot:    makeBeforeBreakCandidateSlot,
		FakeTCP: Command{Name: "faketcp-1-candidate-s5"},
		DTLS:    Command{Name: "dtls-1"},
		Link:    Command{Name: "link-1-candidate-s5"},
	}
	if err := e.StartDynamicLane(collision, &recordingProcess{runner: r, name: collision.FakeTCP.Name}); err == nil {
		t.Fatal("expected process-name collision")
	}
	if !reflect.DeepEqual(r.events, baseline) {
		t.Fatalf("collision mutated runtime: before=%v after=%v", baseline, r.events)
	}
	if got := e.DynamicLaneIDs(); !reflect.DeepEqual(got, []int{1}) {
		t.Fatalf("collision changed active lanes=%v", got)
	}
}
