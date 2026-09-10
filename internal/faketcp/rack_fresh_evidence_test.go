package faketcp

import (
	"testing"
	"time"
)

func TestRACKFreshEvidenceAllowsRepeatedRepairWithoutStaleStorm(t *testing.T) {
	now := time.Unix(100, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoverySACKRACK)
	p1 := s.Enqueue(make([]byte, 10), now)
	p2 := s.Enqueue(make([]byte, 10), now)
	_ = s.Enqueue(make([]byte, 10), now)
	_ = s.Enqueue(make([]byte, 10), now)
	_ = s.Enqueue(make([]byte, 10), now)

	if got := s.AckSelective(100, []SACKBlock{{Start: 120, End: 150}}, now.Add(10*time.Millisecond)); got != p1 || p1.Retries != 1 {
		t.Fatalf("first repair got=%#v retries=%d", got, p1.Retries)
	}
	if got := s.AckSelective(100, []SACKBlock{{Start: 120, End: 150}}, now.Add(20*time.Millisecond)); got != p2 || p2.Retries != 1 {
		t.Fatalf("p2 repair got=%#v retries=%d", got, p2.Retries)
	}
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 150}}, now.Add(30*time.Millisecond)); got != p1 || p1.Retries != 2 {
		t.Fatalf("second p1 repair got=%#v retries=%d", got, p1.Retries)
	}
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 150}}, now.Add(45*time.Millisecond)); got != nil {
		t.Fatalf("stale SACK retriggered repair: %#v", got)
	}

	p6 := s.Enqueue(make([]byte, 10), now.Add(40*time.Millisecond))
	if p6.Seq != 150 {
		t.Fatalf("p6 seq=%d", p6.Seq)
	}
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 160}}, now.Add(55*time.Millisecond)); got != p1 || p1.Retries != 3 {
		t.Fatalf("third p1 repair got=%#v retries=%d", got, p1.Retries)
	}
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 160}}, now.Add(70*time.Millisecond)); got != nil {
		t.Fatalf("same evidence retriggered third repair: %#v", got)
	}

	p7 := s.Enqueue(make([]byte, 10), now.Add(60*time.Millisecond))
	if p7.Seq != 160 {
		t.Fatalf("p7 seq=%d", p7.Seq)
	}
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 170}}, now.Add(75*time.Millisecond)); got != p1 || p1.Retries != 4 {
		t.Fatalf("fourth p1 repair got=%#v retries=%d", got, p1.Retries)
	}

	st := s.Stats()
	if st.RTOTransmits != 0 {
		t.Fatalf("fresh-evidence repairs unexpectedly fell back to RTO: %#v", st)
	}
	if st.FastRetransmits != 5 {
		t.Fatalf("fast retransmits=%d want 5: %#v", st.FastRetransmits, st)
	}
}

func TestRACKLegacyModeDoesNotUseFreshEvidenceRepair(t *testing.T) {
	now := time.Unix(200, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoveryLegacy)
	p1 := s.Enqueue(make([]byte, 10), now)
	_ = s.Enqueue(make([]byte, 10), now)
	_ = s.Enqueue(make([]byte, 10), now)
	_ = s.Enqueue(make([]byte, 10), now)

	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 140}}, now.Add(20*time.Millisecond)); got != nil {
		t.Fatalf("legacy unexpectedly used SACK/RACK repair: %#v", got)
	}
	if p1.Retries != 0 {
		t.Fatalf("legacy retries=%d want 0", p1.Retries)
	}
}
