package faketcp

import (
	"testing"
	"time"
)

func TestRACKRequiresFreshEvidenceForEveryRepeatedRepair(t *testing.T) {
	t0 := time.Unix(20, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoverySACKRACK)

	p1 := s.Enqueue(make([]byte, 10), t0) // first original is lost
	_ = s.Enqueue(make([]byte, 10), t0)   // 110..120 delivered
	_ = s.Enqueue(make([]byte, 10), t0)   // 120..130 delivered
	_ = s.Enqueue(make([]byte, 10), t0)   // 130..140 delivered

	// Three SACKed originals prove the first hole and trigger the first
	// scoreboard repair. RACK must never bypass this first-loss proof.
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 140}}, t0.Add(10*time.Millisecond)); got != p1 {
		t.Fatalf("first repair=%#v want p1", got)
	}
	if p1.Retries != 1 {
		t.Fatalf("retries after scoreboard repair=%d want 1", p1.Retries)
	}

	// Later packets sent after repair #1 become fresh delivery evidence.
	// Fresh evidence alone must not bypass the conservative reordering window.
	for i := 0; i < 3; i++ {
		s.Enqueue(make([]byte, 10), t0.Add(12*time.Millisecond))
	}
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 170}}, t0.Add(19*time.Millisecond)); got == p1 {
		t.Fatal("fresh evidence bypassed RACK reordering window")
	}
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 170}}, t0.Add(21*time.Millisecond)); got != p1 {
		t.Fatalf("fresh evidence after reordering window did not trigger repair #2: %#v", got)
	}
	if p1.Retries != 2 {
		t.Fatalf("retries after repair #2=%d want 2", p1.Retries)
	}

	// The evidence that justified repair #2 now predates p1.LastSent. Repeating
	// exactly the same SACK must not cause a retransmission storm.
	for i := 0; i < 8; i++ {
		if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 170}}, t0.Add(time.Duration(30+i)*time.Millisecond)); got == p1 {
			t.Fatalf("stale evidence retriggered p1 at repeat %d", i)
		}
	}

	// Prove both a third and fourth fast repair. Every repair needs delivery
	// evidence from data transmitted after the previous p1 repair.
	cases := []struct {
		want   uint32
		sentAt time.Duration
		ackAt  time.Duration
	}{
		{want: 3, sentAt: 41 * time.Millisecond, ackAt: 55 * time.Millisecond},
		{want: 4, sentAt: 60 * time.Millisecond, ackAt: 75 * time.Millisecond},
	}
	for _, tc := range cases {
		for i := 0; i < 3; i++ {
			s.Enqueue(make([]byte, 10), t0.Add(tc.sentAt))
		}
		end := s.NextSeq()
		if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: end}}, t0.Add(tc.ackAt)); got != p1 {
			t.Fatalf("fresh evidence did not trigger repair #%d: %#v", tc.want, got)
		}
		if p1.Retries != tc.want {
			t.Fatalf("retries=%d want %d", p1.Retries, tc.want)
		}
		if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: end}}, t0.Add(tc.ackAt+5*time.Millisecond)); got == p1 {
			t.Fatalf("identical evidence retriggered after repair #%d", tc.want)
		}
	}

	st := s.Stats()
	if st.FastRetransmits != 4 {
		t.Fatalf("fast retransmits=%d want 4; stats=%#v", st.FastRetransmits, st)
	}
	if st.LossMarked != 1 {
		t.Fatalf("loss marks=%d want one unique packet", st.LossMarked)
	}
}

func TestRACKDoesNotInferUnrepairedOriginalByTimeAlone(t *testing.T) {
	t0 := time.Unix(21, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoverySACKRACK)
	p1 := s.Enqueue(make([]byte, 10), t0)
	_ = s.Enqueue(make([]byte, 10), t0.Add(20*time.Millisecond))

	// Only one later packet is SACKed: not enough for the SACK scoreboard's
	// three-packet loss proof. RACK must not bypass that proof for an original
	// transmission just because a newer packet was delivered.
	got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 120}}, t0.Add(50*time.Millisecond))
	if got == p1 {
		t.Fatal("RACK inferred an unrepaired original without scoreboard proof")
	}
	if p1.Retries != 0 {
		t.Fatalf("original retries=%d want 0", p1.Retries)
	}
}
