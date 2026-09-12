package faketcp

import "testing"

func TestSteadyStateLateFirstArrivalSurvivesForgivenACK(t *testing.T) {
	const (
		start  = uint32(10000)
		record = 100
	)
	r := NewReceiver(start)
	r.EnableSteadyStateDelivery()

	// Lose the oldest record while enough later records arrive to cross the
	// bounded repair-debt horizon and advance cumulative ACK over the hole.
	for i := 1; i <= PartialReliabilityReorderSoftLimit; i++ {
		seq := start + uint32(i*record)
		deliver, _ := r.Accept(seq, record)
		if !deliver {
			t.Fatalf("record %d was not first-arrival delivered", i)
		}
	}
	if !seqLT(start, r.Next()) {
		t.Fatalf("receiver did not forgive original hole: next=%d start=%d", r.Next(), start)
	}

	// The missing record now arrives for the first time. ACK repair debt is gone,
	// but real steady-state data must still reach carrier/DTLS.
	deliver, sack := r.Accept(start, record)
	if !deliver {
		t.Fatal("late first arrival below forgiven ACK was locally dropped")
	}
	if sack {
		t.Fatal("late below-ACK delivery recreated SACK debt")
	}
	st := r.Stats()
	if st.LateBelowACK != 1 {
		t.Fatalf("late below-ACK deliveries=%d want=1", st.LateBelowACK)
	}

	// Exact recent retry is still suppressed locally.
	deliver, _ = r.Accept(start, record)
	if deliver {
		t.Fatal("recent duplicate below ACK was delivered twice")
	}
	if r.Stats().BelowNextDrops == 0 {
		t.Fatal("recent below-ACK duplicate was not accounted")
	}
}

func TestSteadyStateNeverReclassifiesBootstrapSequenceAsDatagram(t *testing.T) {
	const (
		start  = uint32(20000)
		record = 100
	)
	r := NewReceiver(start)
	if deliver, _ := r.Accept(start, record); !deliver {
		t.Fatal("bootstrap record was not delivered")
	}
	floor := r.Next()
	r.EnableSteadyStateDelivery()

	// A late bootstrap retry sits below the mode-transition floor and must remain
	// strict even though steady-state late-first-arrival delivery is now enabled.
	if deliver, _ := r.Accept(start, record); deliver {
		t.Fatal("pre-steady sequence was reclassified as datagram payload")
	}
	if r.steadyFloor != floor || !r.steadyFloorActive {
		t.Fatalf("steady floor=%d active=%t want=%d,true", r.steadyFloor, r.steadyFloorActive, floor)
	}
}

func TestSteadyStateDeliveryHistoryIsBounded(t *testing.T) {
	const record = 100
	r := NewReceiver(30000)
	r.EnableSteadyStateDelivery()

	for i := 0; i < recentDeliveryHistoryLimit+512; i++ {
		seq := r.Next()
		if deliver, _ := r.Accept(seq, record); !deliver {
			t.Fatalf("in-order record %d was not delivered", i)
		}
	}
	if got := len(r.deliveredBySeq); got > recentDeliveryHistoryLimit {
		t.Fatalf("delivery history=%d limit=%d", got, recentDeliveryHistoryLimit)
	}
	if r.steadyFloorActive {
		t.Fatal("bootstrap floor remained permanently active across long steady state")
	}
	if r.deliveryHead >= recentDeliveryHistoryLimit && r.deliveryHead*2 >= len(r.deliveryOrder) {
		t.Fatalf("delivery queue was not compacted: head=%d len=%d", r.deliveryHead, len(r.deliveryOrder))
	}
}
