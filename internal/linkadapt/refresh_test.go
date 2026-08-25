package linkadapt

import (
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestRefreshWaitsForDueLowLoadAndCommitsOnlyAfterRotation(t *testing.T) {
	policy := DefaultRefreshPolicy(200e6)
	policy.MinSamples = 1000
	s, err := NewRefreshState(policy, ProfileOff)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Unix(1000, 0)
	before := Snapshot{At: start, Stats: faketcp.SenderStats{Enqueued: 100, EnqueuedBytes: 120000, LossMarked: 1}}
	after := Snapshot{At: start.Add(20*time.Second), Stats: faketcp.SenderStats{Enqueued: 1100, EnqueuedBytes: 1320000, LossMarked: 101, LossMarkedBytes: 120000}}
	rec, err := s.Evaluate(before, after)
	if err != nil {
		t.Fatal(err)
	}
	if rec.Profile != Profile20x12 {
		t.Fatalf("profile=%s estimate=%.4f upper=%.4f", rec.Profile.Name, rec.Estimate, rec.WilsonHigh95)
	}
	if !s.RotationNeeded() || s.Current != ProfileOff {
		t.Fatalf("recommendation must not mutate current before new association: %#v", s)
	}
	s.CommitRotation()
	if s.RotationNeeded() || s.Current != Profile20x12 {
		t.Fatalf("rotation commit failed: %#v", s)
	}
	if s.Due(after.At.Add(59 * time.Minute)) {
		t.Fatal("refresh became due before one-hour interval")
	}
	if !s.Due(after.At.Add(time.Hour)) {
		t.Fatal("refresh not due at interval")
	}
}

func TestRefreshRejectsBusyWindowWithoutChangingRecommendation(t *testing.T) {
	policy := DefaultRefreshPolicy(200e6)
	policy.MinSamples = 1000
	s, _ := NewRefreshState(policy, Profile20x8)
	start := time.Unix(2000, 0)
	// 80 MB in 20 seconds = 32 Mbps, above 5% of a 200 Mbps path.
	before := Snapshot{At: start, Stats: faketcp.SenderStats{}}
	after := Snapshot{At: start.Add(20*time.Second), Stats: faketcp.SenderStats{Enqueued: 1000, EnqueuedBytes: 80_000_000, LossMarked: 200}}
	if _, err := s.Evaluate(before, after); !errors.Is(err, ErrSampleTooBusy) {
		t.Fatalf("err=%v", err)
	}
	if s.Recommended != Profile20x8 || s.RotationNeeded() {
		t.Fatalf("busy sample changed profile: %#v", s)
	}
	if !s.LastGood.IsZero() {
		t.Fatalf("busy sample recorded as last good: %v", s.LastGood)
	}
}

func TestRefreshNeedsMinimumWindowAndSamples(t *testing.T) {
	policy := DefaultRefreshPolicy(100e6)
	policy.MinSamples = 1024
	s, _ := NewRefreshState(policy, ProfileOff)
	start := time.Unix(3000, 0)
	if _, err := s.Evaluate(Snapshot{At:start}, Snapshot{At:start.Add(10*time.Second)}); !errors.Is(err, ErrBadWindow) {
		t.Fatalf("short window err=%v", err)
	}

	// New state because a failed attempt still consumes this scheduled attempt;
	// the next timer slot can try again rather than spinning repeatedly.
	s, _ = NewRefreshState(policy, ProfileOff)
	before := Snapshot{At:start}
	after := Snapshot{At:start.Add(20*time.Second), Stats:faketcp.SenderStats{Enqueued:100, EnqueuedBytes:12000}}
	if _, err := s.Evaluate(before, after); !errors.Is(err, ErrInsufficientSamples) {
		t.Fatalf("sample err=%v", err)
	}
	if got := ProbeDeficit(Sample{Segments:100}, policy.MinSamples); got != 924 {
		t.Fatalf("probe deficit=%d", got)
	}
}
