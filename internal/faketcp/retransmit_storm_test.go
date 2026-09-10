package faketcp

import (
	"testing"
	"time"
)

func TestRACKRepeatedRepairsRequireFreshEvidence(t *testing.T) {
	t0 := time.Unix(20, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoverySACKRACK)

	p1 := s.Enqueue(make([]byte, 10), t0)
	_ = s.Enqueue(make([]byte, 10), t0)
	_ = s.Enqueue(make([]byte, 10), t0)
	_ = s.Enqueue(make([]byte, 10), t0)

	// Three delivered originals prove the first hole and trigger the scoreboard repair.
	if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: 140}}, t0.Add(10*time.Millisecond)); got != p1 {
		t.Fatalf("first repair=%#v want p1", got)
	}
	if p1.Retries != 1 {
		t.Fatalf("retries after scoreboard repair=%d want 1", p1.Retries)
	}

	// Each round creates genuinely newer delivered data. That can prove the previous
	// repair lost and permit exactly one more fast repair. Replaying the same ACK/SACK
	// immediately afterwards must never trigger another repair.
	for round := 0; round < 8; round++ {
		sentAt := t0.Add(time.Duration(20+round*30) * time.Millisecond)
		for i := 0; i < 3; i++ {
			s.Enqueue(make([]byte, 10), sentAt)
		}
		end := s.NextSeq()
		ackAt := sentAt.Add(15 * time.Millisecond)
		if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: end}}, ackAt); got != p1 {
			t.Fatalf("round %d fresh evidence repair=%#v want p1", round, got)
		}
		wantRetries := uint32(round + 2)
		if p1.Retries != wantRetries {
			t.Fatalf("round %d retries=%d want %d", round, p1.Retries, wantRetries)
		}
		if got := s.AckSelective(100, []SACKBlock{{Start: 110, End: end}}, ackAt.Add(12*time.Millisecond)); got != nil {
			t.Fatalf("round %d stale evidence retriggered repair: %#v", round, got)
		}
	}

	stt := s.Stats()
	if stt.FastRetransmits != 9 {
		t.Fatalf("fast retransmits=%d want 9; stats=%#v", stt.FastRetransmits, stt)
	}
	if stt.RTOTransmits != 0 {
		t.Fatalf("fresh-evidence path unexpectedly used RTO: %#v", stt)
	}
	if stt.LossMarked != 1 {
		t.Fatalf("loss marks=%d want one unique packet", stt.LossMarked)
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
