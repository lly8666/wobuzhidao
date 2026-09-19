package faketcp

import (
	"testing"
	"time"
)

func TestShadowCreditCannotBankLifetimeFreshTraffic(t *testing.T) {
	s := NewSender(100, time.Second)
	now := time.Unix(1, 0)
	for i := 0; i < 10000; i++ {
		p, err := s.EnqueueSteadyState(make([]byte, 1000), now)
		if err != nil {
			t.Fatal(err)
		}
		s.Ack(p.End, now.Add(time.Millisecond))
	}
	if s.repairCredit != shadowRepairBurstBytes {
		t.Fatal("credit not capped")
	}
	start := s.stats.ShadowRetransmitBytes
	// Previously, the 10 MB loss-free history funded ~2 MB of immediate repair.
	for i := 0; i < 1000; i++ {
		p, err := s.EnqueueSteadyState(make([]byte, 1000), now)
		if err != nil {
			t.Fatal(err)
		}
		s.tryMarkRetry(p, now.Add(time.Second), false)
	}
	if got := s.stats.ShadowRetransmitBytes - start; got > shadowRepairBurstBytes+1000000/5 {
		t.Fatalf("old fresh traffic funded a repair storm: %d", got)
	}
	if s.stats.RepairDeferred == 0 {
		t.Fatal("budget never exhausted")
	}
}

func TestShadowCreditFractionalFreshAndBootstrapExclusion(t *testing.T) {
	s := NewSender(1, time.Second)
	s.repairCredit = 0
	for i := 0; i < 4; i++ {
		s.Enqueue([]byte{1}, time.Unix(1, 0))
	}
	if s.repairCredit != 0 {
		t.Fatal("rounded credit upward")
	}
	s.Enqueue([]byte{1}, time.Unix(1, 0))
	if s.repairCredit != 1 {
		t.Fatal("lost fractional credit")
	}
	payload := make([]byte, 1000)
	var p *Pending
	sendBootstrapPayload(func(b []byte) (uint32, error) {
		p = s.Enqueue(b, time.Unix(1, 0))
		return p.End, nil
	}, payload)
	if !p.Bootstrap || s.repairCredit != 1 {
		t.Fatal("bootstrap earned repair credit")
	}
	if !s.tryMarkRetry(p, time.Unix(2, 0), false) || s.repairCredit != 1 {
		t.Fatal("bootstrap repair was budgeted")
	}
}

func TestContinuousFreshAtFullRepairCapacity(t *testing.T) {
	for _, mode := range []RecoveryMode{RecoveryLegacy, RecoverySACKRACK} {
		s := NewSenderWithRecovery(100, time.Second, mode)
		const count = 20000
		for i := 0; i < count; i++ {
			ps, err := s.EnqueueSteadyStateBatch([][]byte{make([]byte, 1200), make([]byte, 600)}, time.Unix(1, 0))
			if err != nil || len(ps) != 2 {
				t.Fatalf("fresh batch %d: %v", i, err)
			}
			if len(ps[0].Payload) != 1200 || len(ps[1].Payload) != 600 {
				t.Fatal("fresh payload shed")
			}
			if s.Pending() > MaxSteadyStateOutstandingDatagrams {
				t.Fatal("repair bound exceeded")
			}
		}
		st := s.Stats()
		if st.FreshAdmitted != 2*count || st.FreshBlockedByRepair != 0 || st.RepairEvicted == 0 {
			t.Fatalf("fresh isolation failed: %+v", st)
		}
	}
}

func TestSACKMetadataBoundWithStalledCumulativeACK(t *testing.T) {
	s := NewSender(100, time.Second)
	now := time.Unix(1, 0)
	s.EnqueueSteadyState([]byte{1}, now) // permanent cumulative hole
	for i := 0; i < 3*MaxSteadyStateTrackedRecords; i++ {
		p, err := s.EnqueueSteadyState([]byte{1}, now)
		if err != nil {
			t.Fatal(err)
		}
		s.AckSelective(100, []SACKBlock{{Start: p.Seq, End: p.End}}, now.Add(time.Second))
		if len(s.bySeq) > MaxSteadyStateTrackedRecords {
			t.Fatal("unbounded SACK metadata")
		}
	}
	if s.Stats().RepairMetadataEvicted == 0 {
		t.Fatal("metadata bound not exercised")
	}
	if len(s.pending) > 2*MaxSteadyStateTrackedRecords {
		t.Fatal("unbounded index backing slice")
	}
}

func TestBootstrapNeverForgivesGapUnderPressure(t *testing.T) {
	r := NewReceiver(100)
	for i := 1; i <= 2*MaxSteadyStateOutstandingDatagrams; i++ {
		r.Accept(100+uint32(i), 1)
	}
	if r.Next() != 100 || r.Stats().ForgivenGaps != 0 {
		t.Fatal("bootstrap lost reliable gap")
	}
}

func TestProtectedBootstrapDoesNotPinUnboundedSparseIndex(t *testing.T) {
	s := NewSender(100, time.Second)
	var bootstrap *Pending
	sendBootstrapPayload(func(b []byte) (uint32, error) {
		bootstrap = s.Enqueue(b, time.Unix(1, 0))
		return bootstrap.End, nil
	}, []byte{1})
	for i := 0; i < 30000; i++ {
		if _, err := s.EnqueueSteadyState([]byte{2}, time.Unix(1, 0)); err != nil {
			t.Fatal(err)
		}
		if len(s.pending) > 2*MaxSteadyStateTrackedRecords {
			t.Fatal("protected head pinned sparse index")
		}
	}
	if s.Outstanding(bootstrap.Seq) != bootstrap || bootstrap.Payload == nil {
		t.Fatal("bootstrap was evicted")
	}
}
