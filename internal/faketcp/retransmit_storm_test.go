package faketcp

import (
	"testing"
	"time"
)

func TestRACKBoundsFastRepairsPerPacket(t *testing.T) {
	t0 := time.Unix(20, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoverySACKRACK)

	p1 := s.Enqueue(make([]byte, 10), t0) // first original is lost
	_ = s.Enqueue(make([]byte, 10), t0)   // 110..120 delivered
	_ = s.Enqueue(make([]byte, 10), t0)   // 120..130 delivered
	_ = s.Enqueue(make([]byte, 10), t0)   // 130..140 delivered

	// Three SACKed originals prove the first hole and trigger exactly one
	// scoreboard repair.
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 140}}, t0.Add(10*time.Millisecond)); got != p1 {
		t.Fatalf("first repair=%#v want p1", got)
	}
	if p1.Retries != 1 {
		t.Fatalf("retries after scoreboard repair=%d want 1", p1.Retries)
	}

	// Later data sent after the first repair is delivered. This is sufficient
	// RACK evidence that the first repair itself may have been lost, so one
	// second fast repair is allowed.
	for i := 0; i < 3; i++ {
		s.Enqueue(make([]byte, 10), t0.Add(20*time.Millisecond))
	}
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 170}}, t0.Add(35*time.Millisecond)); got != p1 {
		t.Fatalf("lost-repair RACK retry=%#v want p1", got)
	}
	if p1.Retries != 2 {
		t.Fatalf("retries after RACK repair=%d want 2", p1.Retries)
	}

	// Keep presenting newer delivered data while the cumulative ACK remains
	// stuck. Before the storm fix every such ACK could retransmit p1 again.
	// The bounded policy must not generate a third fast repair; further recovery
	// is left to the backed-off RTO path.
	for round := 0; round < 20; round++ {
		for i := 0; i < 3; i++ {
			s.Enqueue(make([]byte, 10), t0.Add(time.Duration(40+round*20)*time.Millisecond))
		}
		end := s.NextSeq()
		if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: end}}, t0.Add(time.Duration(55+round*20)*time.Millisecond)); got == p1 {
			t.Fatalf("round %d generated third-or-later fast repair for same hole", round)
		}
	}

	st := s.Stats()
	if st.FastRetransmits != 2 {
		t.Fatalf("fast retransmits=%d want 2; stats=%#v", st.FastRetransmits, st)
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
