package faketcp

import (
	"testing"
	"time"
)

func TestLegacyRTOSweepRepairsMultipleExpiredPacketsWithOneBackoff(t *testing.T) {
	t0 := time.Unix(40, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoveryLegacy)
	pkts := make([]*Pending, 8)
	for i := range pkts {
		pkts[i] = s.Enqueue(make([]byte, 10), t0)
	}

	// One expired timer epoch must not let the cumulative-ACK boundary monopolize
	// recovery. The caller may pace the sweep in bounded bursts, but every packet
	// that was already expired when the epoch began must be eligible without
	// multiplying the connection-wide exponential backoff once per packet.
	got := s.RetransmitDueBatch(t0.Add(time.Second), 3)
	if len(got) != 3 || got[0] != pkts[0] || got[1] != pkts[1] || got[2] != pkts[2] {
		t.Fatalf("first sweep batch=%#v want p0,p1,p2", got)
	}
	if gotRTO := s.RTO(); gotRTO != 2*time.Second {
		t.Fatalf("first timeout epoch RTO=%v want 2s", gotRTO)
	}

	got = s.RetransmitDueBatch(t0.Add(time.Second+2*time.Millisecond), 3)
	if len(got) != 3 || got[0] != pkts[3] || got[1] != pkts[4] || got[2] != pkts[5] {
		t.Fatalf("second sweep batch=%#v want p3,p4,p5", got)
	}
	if gotRTO := s.RTO(); gotRTO != 2*time.Second {
		t.Fatalf("same timeout epoch multiplied backoff: RTO=%v want 2s", gotRTO)
	}

	got = s.RetransmitDueBatch(t0.Add(time.Second+4*time.Millisecond), 3)
	if len(got) != 2 || got[0] != pkts[6] || got[1] != pkts[7] {
		t.Fatalf("final sweep batch=%#v want p6,p7", got)
	}
	if st := s.Stats(); st.RTOTransmits != 8 || st.LossMarked != 8 {
		t.Fatalf("sweep accounting=%#v want eight first-time RTO repairs", st)
	}
	if gotRTO := s.RTO(); gotRTO != 2*time.Second {
		t.Fatalf("completed timeout epoch RTO=%v want 2s", gotRTO)
	}
}

func TestLegacyRTOSweepSkipsSACKedPacketsAndRespectsBurstBound(t *testing.T) {
	t0 := time.Unix(41, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoveryLegacy)
	pkts := make([]*Pending, 7)
	for i := range pkts {
		pkts[i] = s.Enqueue(make([]byte, 10), t0)
	}

	// p1..p3 are proven delivered. They remain retained until cumulative ACK by
	// contract, but a timer sweep must never retransmit them merely to drain the
	// bounded outstanding window.
	if repair := s.AckSelective(100, []SACKBlock{{Start: pkts[1].Seq, End: pkts[3].End}}, t0.Add(100*time.Millisecond)); repair != nil {
		t.Fatalf("legacy SACK unexpectedly returned immediate repair: %#v", repair)
	}

	got := s.RetransmitDueBatch(t0.Add(time.Second), 2)
	if len(got) != 2 || got[0] != pkts[0] || got[1] != pkts[4] {
		t.Fatalf("bounded sweep batch=%#v want unsacked p0,p4", got)
	}
	if pkts[1].Retries != 0 || pkts[2].Retries != 0 || pkts[3].Retries != 0 {
		t.Fatalf("SACKed packets retransmitted: retries=%d,%d,%d", pkts[1].Retries, pkts[2].Retries, pkts[3].Retries)
	}
	if len(got) > 2 {
		t.Fatalf("burst exceeded caller bound: %d", len(got))
	}

	got = s.RetransmitDueBatch(t0.Add(time.Second+2*time.Millisecond), 2)
	if len(got) != 2 || got[0] != pkts[5] || got[1] != pkts[6] {
		t.Fatalf("continued bounded sweep=%#v want p5,p6", got)
	}
	if got = s.RetransmitDueBatch(t0.Add(1500*time.Millisecond), 2); len(got) != 0 {
		t.Fatalf("completed epoch repeated before backed-off deadline: %#v", got)
	}
}

func TestLegacyRTOSweepBacksOffOnceAgainOnNextExpiredEpoch(t *testing.T) {
	t0 := time.Unix(42, 0)
	s := NewSenderWithRecovery(100, time.Second, RecoveryLegacy)
	p1 := s.Enqueue(make([]byte, 10), t0)
	p2 := s.Enqueue(make([]byte, 10), t0)

	got := s.RetransmitDueBatch(t0.Add(time.Second), 8)
	if len(got) != 2 || got[0] != p1 || got[1] != p2 {
		t.Fatalf("first epoch=%#v want p1,p2", got)
	}
	if s.RTO() != 2*time.Second {
		t.Fatalf("RTO after first epoch=%v want 2s", s.RTO())
	}

	// Both retransmissions were sent at the same epoch time. If neither repair is
	// acknowledged, the next sweep is due after the backed-off two-second wait,
	// and that new epoch doubles the RTO exactly once again.
	got = s.RetransmitDueBatch(t0.Add(3*time.Second), 8)
	if len(got) != 2 || got[0] != p1 || got[1] != p2 {
		t.Fatalf("second epoch=%#v want p1,p2", got)
	}
	if s.RTO() != 4*time.Second {
		t.Fatalf("RTO after second epoch=%v want 4s", s.RTO())
	}
}
