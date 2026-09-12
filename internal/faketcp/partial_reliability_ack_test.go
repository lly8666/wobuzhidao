package faketcp

import (
	"bytes"
	"testing"
	"time"
)

func TestPartialReliabilityBelowPressureKeepsNormalSACK(t *testing.T) {
	const start = uint32(1000)
	r := NewReceiver(start)

	for i := 1; i < PartialReliabilityReorderSoftLimit; i++ {
		seq := start + uint32(i*100)
		deliver, sack := r.Accept(seq, 100)
		if !deliver || !sack {
			t.Fatalf("accept %d deliver=%t sack=%t", i, deliver, sack)
		}
	}
	if got := r.Next(); got != start {
		t.Fatalf("cumulative ACK advanced below pressure: got=%d want=%d", got, start)
	}
	st := r.Stats()
	if st.ForgivenGaps != 0 || st.ForgivenBytes != 0 {
		t.Fatalf("unexpected forgiveness below pressure: %+v", st)
	}
}

func TestPartialReliabilityPressureForgivesOldestGapAndBoundsReceiverState(t *testing.T) {
	const (
		start   = uint32(5000)
		record  = 100
		missing = uint32(record)
	)
	r := NewReceiver(start)

	var lastEnd uint32
	for i := 1; i <= PartialReliabilityReorderSoftLimit; i++ {
		seq := start + uint32(i*record)
		deliver, _ := r.Accept(seq, record)
		if !deliver {
			t.Fatalf("record %d was not first-arrival delivered", i)
		}
		lastEnd = seq + record
	}

	if got := r.Next(); got != lastEnd {
		t.Fatalf("pressure ACK=%d want=%d", got, lastEnd)
	}
	if len(r.outOfOrder) != 0 || len(r.sacksByStart) != 0 {
		t.Fatalf("pressure retirement left receiver debt: out_of_order=%d sacks=%d", len(r.outOfOrder), len(r.sacksByStart))
	}
	if len(r.coverageQueue) != 0 || r.coverageAt != 0 {
		t.Fatalf("coverage queue retained stale pressure state: len=%d at=%d", len(r.coverageQueue), r.coverageAt)
	}
	st := r.Stats()
	if st.ForgivenGaps != 1 || st.ForgivenBytes != uint64(missing) {
		t.Fatalf("forgiveness stats=%+v", st)
	}
	if st.PeakBufferedOO != PartialReliabilityReorderSoftLimit {
		t.Fatalf("peak buffered=%d want=%d", st.PeakBufferedOO, PartialReliabilityReorderSoftLimit)
	}

	// A repair already in flight after the receiver crossed the horizon is a
	// harmless duplicate. It must not be delivered upward or recreate SACK debt.
	deliver, sack := r.Accept(start, record)
	if deliver || sack {
		t.Fatalf("late abandoned repair deliver=%t sack=%t", deliver, sack)
	}
	if r.Stats().Duplicates == 0 {
		t.Fatal("late abandoned repair was not classified as duplicate")
	}
}

func TestPartialReliabilityCumulativeAckStopsFurtherRepairAndReleasesPayload(t *testing.T) {
	const (
		start  = uint32(9000)
		record = 100
	)
	now := time.Unix(1700000200, 0)
	s := NewSenderWithRecovery(start, time.Second, RecoverySACKRACK)
	r := NewReceiver(start)

	pending := make([]*Pending, 0, PartialReliabilityReorderSoftLimit+1)
	for i := 0; i <= PartialReliabilityReorderSoftLimit; i++ {
		pending = append(pending, s.Enqueue(bytes.Repeat([]byte{byte(i)}, record), now))
	}

	// Simulate the oldest record being lost while all later first arrivals reach
	// the receiver. The horizon advances ACK across that loss without any custom
	// control packet.
	for i := 1; i < len(pending); i++ {
		r.Accept(pending[i].Seq, len(pending[i].Payload))
	}
	ack := r.Next()
	if ack != pending[len(pending)-1].End {
		t.Fatalf("receiver pressure ACK=%d want=%d", ack, pending[len(pending)-1].End)
	}

	if repair := s.Ack(ack, now.Add(600*time.Millisecond)); repair != nil {
		t.Fatalf("cumulative pressure ACK unexpectedly scheduled repair: %#v", repair)
	}
	if got := s.Pending(); got != 0 {
		t.Fatalf("sender retained repair payload after pressure ACK: pending=%d", got)
	}
	for i, p := range pending {
		if s.Outstanding(p.Seq) != nil {
			t.Fatalf("record %d still has sender tombstone after cumulative pressure ACK", i)
		}
		if p.Payload != nil {
			t.Fatalf("record %d still owns payload after cumulative pressure ACK", i)
		}
	}
	if p := s.RetransmitDue(now.Add(2 * time.Second)); p != nil {
		t.Fatalf("abandoned range generated a later RTO repair: %#v", p)
	}
}

func TestPartialReliabilityHorizonUsesFullBoundedRepairWindow(t *testing.T) {
	if got, want := PartialReliabilityReorderSoftLimit, MaxSteadyStateOutstandingDatagrams; got != want {
		t.Fatalf("receiver horizon=%d want=%d", got, want)
	}
	// 10 Mbit/s of 1000-byte source packets with FEC20:20 is about 2500
	// carrier records/s; at 600ms RTT that is ~1500 records in flight. The old
	// 1536 threshold left ~2.4%% margin. 4096 leaves substantial room while
	// remaining independently bounded.
	if PartialReliabilityReorderSoftLimit <= 2*1500 {
		t.Fatalf("receiver horizon=%d leaves insufficient 10M/600ms BDP margin", PartialReliabilityReorderSoftLimit)
	}
}
