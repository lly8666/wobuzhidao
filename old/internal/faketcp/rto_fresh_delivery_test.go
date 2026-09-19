package faketcp

import (
	"testing"
	"time"
)

func TestLegacyFreshDeliveryEvidenceBoundsBackedOffRTO(t *testing.T) {
	t0 := time.Unix(30, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoveryLegacy)

	p1 := s.Enqueue(make([]byte, 10), t0)
	_ = s.Enqueue(make([]byte, 10), t0)

	// The oldest hole times out once, so the loss episode backs off to 2s.
	if got := s.RetransmitDue(t0.Add(time.Second)); got != p1 {
		t.Fatalf("first RTO repair=%#v want p1", got)
	}
	if got := s.RTO(); got != 2*time.Second {
		t.Fatalf("backed-off RTO=%v want 2s", got)
	}

	// A datagram transmitted after that repair is then selectively delivered.
	// This proves the path is still making forward progress even though the
	// cumulative ACK remains pinned at p1. Legacy must not turn this into an
	// immediate SACK/RACK fast repair.
	fresh := s.Enqueue(make([]byte, 10), t0.Add(1100*time.Millisecond))
	if got := s.AckSelective(100, []SACKBlock{{Start: fresh.Seq, End: fresh.End}}, t0.Add(1200*time.Millisecond)); got != nil {
		t.Fatalf("legacy unexpectedly fast-repaired from SACK evidence: %#v", got)
	}

	// Fresh post-retry delivery evidence must keep the next RTO repair on the
	// estimator/base cadence instead of waiting for the old exponential backoff.
	if got := s.RetransmitDue(t0.Add(2100 * time.Millisecond)); got != p1 {
		t.Fatalf("fresh-evidence RTO repair=%#v want p1", got)
	}

	// The evidence used above is now stale because p1's LastSent advanced. Time
	// alone (or replay of the same already-SACKed range) must not authorize yet
	// another base-cadence repair.
	if got := s.AckSelective(100, []SACKBlock{{Start: fresh.Seq, End: fresh.End}}, t0.Add(2200*time.Millisecond)); got != nil {
		t.Fatalf("stale SACK unexpectedly triggered repair: %#v", got)
	}
	if got := s.RetransmitDue(t0.Add(3200 * time.Millisecond)); got != nil {
		t.Fatalf("stale evidence shortened backed-off RTO: %#v", got)
	}

	// One genuinely newer delivered transmission refreshes the proof and permits
	// one more base-cadence RTO repair, still without changing recovery mode.
	newer := s.Enqueue(make([]byte, 10), t0.Add(3300*time.Millisecond))
	if got := s.AckSelective(100, []SACKBlock{{Start: newer.Seq, End: newer.End}}, t0.Add(3400*time.Millisecond)); got != nil {
		t.Fatalf("legacy unexpectedly fast-repaired from newer SACK: %#v", got)
	}
	if got := s.RetransmitDue(t0.Add(4200 * time.Millisecond)); got != p1 {
		t.Fatalf("second fresh-evidence RTO repair=%#v want p1", got)
	}
}
