package windowsruntime

import (
	"strings"
	"testing"
	"time"
)

func TestLaneRotationBoundsZeroDisablesAutomaticRotation(t *testing.T) {
	p := Profile{}
	minAge, maxAge := laneRotationBounds(p)
	if minAge != 0 || maxAge != 0 {
		t.Fatalf("disabled bounds=%s..%s want=0..0", minAge, maxAge)
	}
	if automaticLaneRotationEnabled(p) {
		t.Fatal("0/0 unexpectedly enables automatic lane rotation")
	}
	if err := validateLaneRotationProfile(p); err != nil {
		t.Fatalf("0/0 must be valid disabled configuration: %v", err)
	}
}

func TestLaneRotationBoundsConfigurableAndFixed(t *testing.T) {
	p := Profile{LaneRotationMinSeconds: 75, LaneRotationMaxSeconds: 125}
	minAge, maxAge := laneRotationBounds(p)
	if minAge != 75*time.Second || maxAge != 125*time.Second {
		t.Fatalf("bounds=%s..%s", minAge, maxAge)
	}
	if !automaticLaneRotationEnabled(p) {
		t.Fatal("positive bounds unexpectedly disable automatic lane rotation")
	}
	fixed := Profile{LaneRotationMinSeconds: 90, LaneRotationMaxSeconds: 90}
	minAge, maxAge = laneRotationBounds(fixed)
	if minAge != 90*time.Second || maxAge != 90*time.Second {
		t.Fatalf("fixed=%s..%s", minAge, maxAge)
	}
}

func TestLaneRotationValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		p    Profile
		want string
	}{
		{"zero-min-only", Profile{LaneRotationMinSeconds: 0, LaneRotationMaxSeconds: 60}, "both be zero"},
		{"zero-max-only", Profile{LaneRotationMinSeconds: 60, LaneRotationMaxSeconds: 0}, "both be zero"},
		{"too-short", Profile{LaneRotationMinSeconds: 9, LaneRotationMaxSeconds: 60}, "at least 10 seconds"},
		{"reversed", Profile{LaneRotationMinSeconds: 120, LaneRotationMaxSeconds: 60}, "maximum must be greater"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateLaneRotationProfile(tc.p); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want substring %q", err, tc.want)
			}
		})
	}
}

func TestLaneAgeTickDisabledClearsScheduledDeadlines(t *testing.T) {
	now := time.Unix(1000, 0)
	ages := newLaneAgeState()
	ages.deadlines[1] = now.Add(-time.Second)
	ages.slots[1] = 1
	c := &Controller{
		state: RuntimeConnected,
		profile: Profile{LaneRotationMinSeconds: 0, LaneRotationMaxSeconds: 0},
		lanePlans: map[int]LanePlan{1: {ID: 1, Slot: 1}},
	}
	c.runLaneAgeTick(ages, now)
	if len(ages.deadlines) != 0 || len(ages.slots) != 0 {
		t.Fatalf("disabled automatic rotation retained age state: deadlines=%v slots=%v", ages.deadlines, ages.slots)
	}
}

func TestChooseLaneAgeDeadlineWithinConfiguredBounds(t *testing.T) {
	now := time.Unix(123, 0)
	got := chooseLaneAgeDeadlineWithin(now, nil, func(span time.Duration) time.Duration {
		if span != 50*time.Second {
			t.Fatalf("span=%s", span)
		}
		return 17 * time.Second
	}, 70*time.Second, 120*time.Second)
	want := now.Add(87 * time.Second)
	if !got.Equal(want) {
		t.Fatalf("got=%s want=%s", got, want)
	}
}

func TestLaneAgeReconcileResamplesReplacementInsideBounds(t *testing.T) {
	now := time.Unix(456, 0)
	ages := newLaneAgeState()
	plans := map[int]LanePlan{1: {ID: 1, Slot: 1}}
	ages.reconcileWithin(plans, now, func(time.Duration) time.Duration { return 5 * time.Second }, 60*time.Second, 120*time.Second)
	first := ages.deadlines[1]
	if first.Before(now.Add(60*time.Second)) || first.After(now.Add(120*time.Second)) {
		t.Fatalf("first deadline=%s", first)
	}
	plans[1] = LanePlan{ID: 1, Slot: 5}
	nextNow := now.Add(10 * time.Second)
	ages.reconcileWithin(plans, nextNow, func(time.Duration) time.Duration { return 20 * time.Second }, 60*time.Second, 120*time.Second)
	second := ages.deadlines[1]
	if !second.Equal(nextNow.Add(80 * time.Second)) {
		t.Fatalf("replacement deadline=%s", second)
	}
	if second.Equal(first) {
		t.Fatal("replacement did not resample")
	}
}
