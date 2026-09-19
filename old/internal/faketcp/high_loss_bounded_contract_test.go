package faketcp

import (
	"testing"
	"time"
)

func TestLongHighLossKeepsRepairBudgetAndStateBounded(t *testing.T) {
	for _, mode := range []RecoveryMode{RecoveryLegacy, RecoverySACKRACK} {
		t.Run(recoveryModeName(mode), func(t *testing.T) {
			s := NewSenderWithRecovery(100, time.Second, mode)
			now := time.Unix(60, 0)
			const records = 20000
			for i := 0; i < records; i++ {
				p, err := s.EnqueueSteadyState(make([]byte, 1200), now)
				if err != nil {
					t.Fatalf("fresh record %d blocked: %v", i, err)
				}
				// Model a persistently lossy path by asking repair policy to retransmit
				// every fresh record without supplying ACK/SACK retirement. This drives
				// both the finite credit budget and the fixed repair horizon hard.
				_ = s.tryMarkRetry(p, now.Add(time.Second), false)

				if got := s.Pending(); got > MaxSteadyStateOutstandingDatagrams {
					t.Fatalf("active repair=%d limit=%d", got, MaxSteadyStateOutstandingDatagrams)
				}
				if got := len(s.bySeq); got > MaxSteadyStateTrackedRecords {
					t.Fatalf("tracked metadata=%d limit=%d", got, MaxSteadyStateTrackedRecords)
				}
				if got := len(s.pending); got > 2*MaxSteadyStateTrackedRecords {
					t.Fatalf("sparse pending index=%d limit=%d", got, 2*MaxSteadyStateTrackedRecords)
				}
				if s.repairCredit > shadowRepairBurstBytes {
					t.Fatalf("repair credit=%d burst_limit=%d", s.repairCredit, shadowRepairBurstBytes)
				}
				now = now.Add(time.Millisecond)
			}

			st := s.Stats()
			if st.FreshAdmitted != records || st.FreshBlockedByRepair != 0 {
				t.Fatalf("fresh traffic was blocked by repair debt: %+v", st)
			}
			if st.RepairEvicted == 0 {
				t.Fatal("fixed repair horizon was never exercised")
			}
			if st.RepairDeferred == 0 || st.RepairDeferredBytes == 0 {
				t.Fatal("finite retransmission budget was never exhausted/deferred")
			}
			if st.RepairBudgetSpent == 0 || st.ShadowRetransmitBytes == 0 {
				t.Fatal("repair path was not exercised before budget pressure")
			}
		})
	}
}
