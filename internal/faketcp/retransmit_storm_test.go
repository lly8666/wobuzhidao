package faketcp

import (
	"testing"
	"time"
)

func TestRACKBoundsFastRepairsByFreshEvidence(t *testing.T) {
	t0 := time.Unix(20, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoverySACKRACK)

	p1 := s.Enqueue(make([]byte, 10), t0) // first original stays missing
	_ = s.Enqueue(make([]byte, 10), t0)   // 110..120 delivered
	_ = s.Enqueue(make([]byte, 10), t0)   // 120..130 delivered
	_ = s.Enqueue(make([]byte, 10), t0)   // 130..140 delivered

	// Three SACKed originals prove the cumulative hole and authorize the first
	// scoreboard repair. The repair itself becomes the new freshness fence.
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 140}}, t0.Add(10*time.Millisecond)); got != p1 {
		t.Fatalf("first repair=%#v want p1", got)
	}
	if p1.Retries != 1 {
		t.Fatalf("retries after scoreboard repair=%d want 1", p1.Retries)
	}

	// Stress the same pinned cumulative hole. Every round contributes exactly one
	// genuinely newer delivered transmission, so exactly one new RACK repair is
	// legitimate. Replaying the resulting SACK after that repair contributes no
	// newer delivery evidence and must never self-trigger another repair.
	const rounds = 20
	for round := 0; round < rounds; round++ {
		sentAt := t0.Add(time.Duration(30+round*30) * time.Millisecond)
		s.Enqueue(make([]byte, 10), sentAt)
		end := s.NextSeq()
		repairAt := sentAt.Add(15 * time.Millisecond)
		if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: end}}, repairAt); got != p1 {
			t.Fatalf("round %d fresh evidence repair=%#v want p1", round, got)
		}
		wantRetries := uint32(round + 2)
		if p1.Retries != wantRetries {
			t.Fatalf("round %d retries=%d want %d", round, p1.Retries, wantRetries)
		}

		// Same ACK/SACK, later wall-clock time: no new delivered transmission was
		// sent after p1's just-completed repair, so time alone cannot authorize it.
		if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: end}}, repairAt.Add(15*time.Millisecond)); got != nil {
			t.Fatalf("round %d stale evidence self-triggered repair: %#v", round, got)
		}
		if p1.Retries != wantRetries {
			t.Fatalf("round %d stale evidence changed retries=%d want %d", round, p1.Retries, wantRetries)
		}
	}

	st := s.Stats()
	if st.FastRetransmits != rounds+1 {
		t.Fatalf("fast retransmits=%d want %d; stats=%#v", st.FastRetransmits, rounds+1, st)
	}
	if st.RTOTransmits != 0 {
		t.Fatalf("unexpected RTO retransmits=%d", st.RTOTransmits)
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
