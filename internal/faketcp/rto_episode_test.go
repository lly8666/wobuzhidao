package faketcp

import (
	"testing"
	"time"
)

func TestSenderRTOBackoffRepeatsWithinSameLossEpisode(t *testing.T) {
	t0 := time.Unix(30, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoveryLegacy)
	p := s.Enqueue(make([]byte, 10), t0)

	if got := s.RetransmitDue(t0.Add(time.Second)); got != p {
		t.Fatalf("first timeout=%#v want p", got)
	}
	if got := s.RTO(); got != 2*time.Second {
		t.Fatalf("RTO after first timeout=%v want 2s", got)
	}
	if got := s.RetransmitDue(t0.Add(3 * time.Second)); got != p {
		t.Fatalf("second timeout=%#v want p", got)
	}
	if got := s.RTO(); got != 4*time.Second {
		t.Fatalf("RTO after second timeout=%v want 4s", got)
	}
	if got := s.RetransmitDue(t0.Add(7 * time.Second)); got != p {
		t.Fatalf("third timeout=%#v want p", got)
	}
	if got := s.RTO(); got != 8*time.Second {
		t.Fatalf("RTO after third timeout=%v want 8s", got)
	}
}

func TestSenderRTOBackoffResetsForNewOldestAfterKarnSafeForwardProgress(t *testing.T) {
	t0 := time.Unix(31, 0)
	s := NewSenderWithRecovery(100, 5*time.Second, RecoveryLegacy)

	// Establish a real estimator-derived base RTO before entering loss recovery.
	probe := s.Enqueue(make([]byte, 10), t0)
	s.Ack(probe.End, t0.Add(100*time.Millisecond))
	if got := s.RTO(); got != time.Second {
		t.Fatalf("estimated base RTO=%v want 1s", got)
	}
	baseSRTT, baseRTTVar := s.srtt, s.rttvar

	sentAt := t0.Add(200 * time.Millisecond)
	p1 := s.Enqueue(make([]byte, 10), sentAt)
	p2 := s.Enqueue(make([]byte, 10), sentAt)
	if got := s.RetransmitDue(sentAt.Add(time.Second)); got != p1 {
		t.Fatalf("first p1 timeout=%#v want p1", got)
	}
	if got := s.RetransmitDue(sentAt.Add(3 * time.Second)); got != p1 {
		t.Fatalf("second p1 timeout=%#v want p1", got)
	}
	if got := s.RTO(); got != 4*time.Second {
		t.Fatalf("p1 backed-off RTO=%v want 4s", got)
	}

	// ACKing a retransmitted p1 is ambiguous for RTT estimation (Karn), but it
	// conclusively ends p1's timeout episode and makes p2 the new oldest loss.
	ackAt := sentAt.Add(3100 * time.Millisecond)
	s.Ack(p1.End, ackAt)
	if s.srtt != baseSRTT || s.rttvar != baseRTTVar {
		t.Fatalf("ambiguous p1 ACK changed estimator: srtt=%v/%v rttvar=%v/%v", s.srtt, baseSRTT, s.rttvar, baseRTTVar)
	}
	if got := s.RTO(); got != time.Second {
		t.Fatalf("new oldest inherited prior loss episode RTO=%v want base 1s", got)
	}
	if got := s.oldest(); got != p2 {
		t.Fatalf("oldest after p1 ACK=%#v want p2", got)
	}

	// p2 was sent more than one base RTO ago, so the new episode is already due.
	// It must not wait for p1's stale 4-second backoff.
	if got := s.RetransmitDue(ackAt); got != p2 {
		t.Fatalf("new oldest timeout=%#v want p2 immediately at base RTO", got)
	}
	if got := s.RTO(); got != 2*time.Second {
		t.Fatalf("p2 first timeout RTO=%v want 2s", got)
	}
}

func TestSenderRTOBackoffDoesNotResetUntilTimedOutOldestIsCovered(t *testing.T) {
	t0 := time.Unix(32, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoverySACKRACK)
	p1 := s.Enqueue(make([]byte, 10), t0)
	_ = s.Enqueue(make([]byte, 10), t0)

	// One later SACK is not enough to infer loss, but it gives a stable old SACK
	// advertisement to replay while the cumulative hole remains unresolved.
	oldSACK := []SACKBlock{{Start: 110, End: 120}}
	if got := s.AckSelective(100, oldSACK, t0.Add(100*time.Millisecond)); got != nil {
		t.Fatalf("single SACK unexpectedly repaired p1: %#v", got)
	}
	if got := s.RetransmitDue(t0.Add(time.Second)); got != p1 {
		t.Fatalf("p1 timeout=%#v want p1", got)
	}
	if got := s.RTO(); got != 2*time.Second {
		t.Fatalf("RTO after p1 timeout=%v want 2s", got)
	}

	// Partial cumulative progress does not cover the timed-out p1, and duplicate
	// ACK plus replayed stale SACK evidence does not end its timeout episode.
	if got := s.AckSelective(105, oldSACK, t0.Add(1100*time.Millisecond)); got != nil {
		t.Fatalf("partial ACK/stale SACK returned repair: %#v", got)
	}
	if got := s.RTO(); got != 2*time.Second {
		t.Fatalf("partial ACK reset active loss episode RTO=%v", got)
	}
	if got := s.AckSelective(105, oldSACK, t0.Add(1200*time.Millisecond)); got != nil {
		t.Fatalf("duplicate ACK/stale SACK returned repair: %#v", got)
	}
	if got := s.RTO(); got != 2*time.Second {
		t.Fatalf("duplicate ACK/stale SACK reset active loss episode RTO=%v", got)
	}

	// Only cumulative ACK coverage of p1.End ends the episode.
	s.Ack(p1.End, t0.Add(1300*time.Millisecond))
	if got := s.RTO(); got != time.Second {
		t.Fatalf("covering cumulative ACK left stale episode RTO=%v want 1s", got)
	}
}
