package faketcp

import (
	"testing"
	"time"
)

func TestSACKRACKSteadyStateDoesNotForgiveUnresolvedGap(t *testing.T) {
	r := NewReceiver(100)
	r.EnableSteadyStateDelivery()
	r.SetSteadyStateRecoveryMode(RecoverySACKRACK)

	now := time.Unix(1, 0)
	rtt := 10 * time.Millisecond
	// Seed a measured delivery rate so the legacy soft-pressure rule would be
	// eligible after the hole ages by one RTT.
	for i := 0; i < 200; i++ {
		if deliver, _ := r.AcceptAt(r.Next(), 10, now, rtt); !deliver {
			t.Fatal("in-order seed unexpectedly dropped")
		}
		now = now.Add(time.Millisecond)
	}

	hole := r.Next()
	limit := r.pressure.softLimit()
	if limit >= PartialReliabilityEmergencyLimit {
		t.Fatalf("test did not establish a finite soft pressure limit: %d", limit)
	}
	for i := 1; i <= limit+32; i++ {
		if deliver, _ := r.AcceptAt(hole+uint32(i*10), 10, now, rtt); !deliver {
			t.Fatalf("out-of-order first arrival %d was not delivered", i)
		}
		now = now.Add(time.Millisecond)
	}
	// Age the same unresolved hole beyond the normal forgiveness threshold and
	// add another first arrival. Strong recovery must still keep cumulative ACK
	// behind the missing datagram.
	now = now.Add(2 * rtt)
	if deliver, _ := r.AcceptAt(hole+uint32((limit+33)*10), 10, now, rtt); !deliver {
		t.Fatal("aged out-of-order first arrival was not delivered")
	}
	if r.Next() != hole {
		t.Fatalf("SACK/RACK cumulative ACK forgave missing datagram: next=%d hole=%d", r.Next(), hole)
	}
	if st := r.Stats(); st.ForgivenGaps != 0 || st.ForgivenBytes != 0 {
		t.Fatalf("SACK/RACK recorded gap forgiveness: %+v", st)
	}

	if deliver, _ := r.AcceptAt(hole, 10, now.Add(time.Millisecond), rtt); !deliver {
		t.Fatal("repaired hole was not delivered")
	}
	if r.Next() <= hole {
		t.Fatalf("cumulative ACK did not advance after real repair: next=%d hole=%d", r.Next(), hole)
	}
}

func TestLegacySteadyStateStillForgivesPressureGap(t *testing.T) {
	r := NewReceiver(100)
	r.EnableSteadyStateDelivery()
	r.SetSteadyStateRecoveryMode(RecoveryLegacy)
	if r.pressure.forgivenessDisabled {
		t.Fatal("legacy mode unexpectedly disabled pressure forgiveness")
	}
}
