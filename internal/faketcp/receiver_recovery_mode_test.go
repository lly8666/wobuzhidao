package faketcp

import (
	"testing"
	"time"
)

func TestSteadyStateTransientGapRepairsBeforeForgiveness(t *testing.T) {
	const (
		start  = uint32(1000)
		record = 10
	)
	r := NewReceiver(start)
	r.EnableSteadyStateDelivery()
	now := time.Unix(20, 0)
	rtt := 100 * time.Millisecond

	// Two later first arrivals are delivered immediately, but neither pressure
	// nor hole age is sufficient to abandon the missing first record.
	for i := 1; i <= 2; i++ {
		deliver, sack := r.AcceptAt(start+uint32(i*record), record, now.Add(time.Duration(i)*time.Millisecond), rtt)
		if !deliver || !sack {
			t.Fatalf("out-of-order first arrival %d deliver=%t sack=%t", i, deliver, sack)
		}
	}
	if got := r.Next(); got != start {
		t.Fatalf("cumulative ACK crossed transient gap: got=%d want=%d", got, start)
	}
	if st := r.Stats(); st.ForgivenGaps != 0 {
		t.Fatalf("transient gap was forgiven early: %+v", st)
	}

	// Once the missing record arrives, ordinary contiguous SACK consumption
	// advances ACK across all three records.
	deliver, sack := r.AcceptAt(start, record, now.Add(10*time.Millisecond), rtt)
	if !deliver || sack {
		t.Fatalf("repair arrival deliver=%t sack=%t", deliver, sack)
	}
	if got, want := r.Next(), start+3*record; got != want {
		t.Fatalf("ACK after repair=%d want=%d", got, want)
	}
}

func TestSteadyStateBoundedRecoveryPolicyIsIndependentOfSenderMode(t *testing.T) {
	for _, mode := range []RecoveryMode{RecoveryLegacy, RecoverySACKRACK} {
		t.Run(recoveryModeName(mode), func(t *testing.T) {
			const (
				start  = uint32(10000)
				record = 10
			)
			// More than 8192 steady-state records with the first record permanently
			// lost. Sender recovery choice may differ, but receiver forgiveness and
			// all bounded-state contracts must be identical.
			total := MaxSteadyStateTrackedRecords + 257
			now := time.Unix(30, 0)
			s := NewSenderWithRecovery(start, time.Second, mode)
			r := NewReceiver(start)
			r.EnableSteadyStateDelivery()

			for i := 0; i < total; i++ {
				p, err := s.EnqueueSteadyState(make([]byte, record), now)
				if err != nil {
					t.Fatalf("enqueue record %d: %v", i, err)
				}
				if i != 0 { // permanently lose the earliest record
					deliver, sackNeeded := r.AcceptAt(p.Seq, record, now, 0)
					if !deliver {
						t.Fatalf("fresh record %d was not delivered immediately", i)
					}
					var blocks [4]SACKBlock
					n := 0
					if sackNeeded {
						n = r.SACKBlocks(&blocks)
					}
					_ = s.AckSelective(r.Next(), blocks[:n], now)
				}

				if got := s.Pending(); got > MaxSteadyStateOutstandingDatagrams {
					t.Fatalf("active repair state=%d limit=%d", got, MaxSteadyStateOutstandingDatagrams)
				}
				if got := len(s.bySeq); got > MaxSteadyStateTrackedRecords {
					t.Fatalf("sender metadata=%d limit=%d", got, MaxSteadyStateTrackedRecords)
				}
				if got := len(s.pending); got > 2*MaxSteadyStateTrackedRecords {
					t.Fatalf("sender sparse index=%d limit=%d", got, 2*MaxSteadyStateTrackedRecords)
				}
				if got := len(r.outOfOrder); got >= PartialReliabilityEmergencyLimit {
					t.Fatalf("receiver reorder debt=%d emergency_limit=%d", got, PartialReliabilityEmergencyLimit)
				}
				if got := len(r.deliveredBySeq); got > recentDeliveryHistoryLimit {
					t.Fatalf("receiver delivery history=%d limit=%d", got, recentDeliveryHistoryLimit)
				}
				liveCoverage := len(r.coverageQueue) - r.coverageAt
				if liveCoverage > len(r.sacksByStart)*4+256 {
					t.Fatalf("receiver coverage metadata=%d sacks=%d", liveCoverage, len(r.sacksByStart))
				}
				now = now.Add(time.Millisecond)
			}

			st := r.Stats()
			if st.ForgivenGaps == 0 || st.EmergencyForgivenGaps == 0 {
				t.Fatalf("permanent gap never left the bounded repair horizon: %+v", st)
			}
			if got, want := r.Next(), s.NextSeq(); got != want {
				t.Fatalf("fresh traffic stopped after gap retirement: receiver=%d sender=%d", got, want)
			}
			sst := s.Stats()
			if sst.FreshBlockedByRepair != 0 {
				t.Fatalf("old repair debt blocked fresh traffic: %+v", sst)
			}
		})
	}
}

func recoveryModeName(mode RecoveryMode) string {
	if mode == RecoveryLegacy {
		return "legacy"
	}
	return "sack-rack"
}
