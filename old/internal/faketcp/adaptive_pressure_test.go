package faketcp

import (
	"testing"
	"time"
)

func TestAdaptivePressurePaced20Mbps600ms(t *testing.T) {
	r := NewReceiver(100)
	r.EnableSteadyStateDelivery()
	now := time.Unix(1, 0)
	rtt := 600 * time.Millisecond
	// 5000 coded carrier records/s, representing ~20 Mbit/s inner at 1000 B
	// sources with 20:20. This is a no-loss arrival rate, not a loss estimator.
	for i := 0; i < 2000; i++ {
		r.AcceptAt(r.Next(), 1000, now, rtt)
		now = now.Add(200 * time.Microsecond)
	}
	if soft := r.Stats().PressureSoftLimit; soft < 3299 || soft > 3301 {
		t.Fatalf("expected about 3300 records, got %d", soft)
	}
	hole := r.Next()
	for i := 1; i < 3300; i++ {
		r.AcceptAt(hole+uint32(i*1000), 1000, now, rtt)
		now = now.Add(200 * time.Microsecond)
	}
	if r.Next() != hole {
		t.Fatal("normal 600ms flight was prematurely forgiven")
	}
	for i := 3300; i <= PartialReliabilityEmergencyLimit; i++ {
		r.AcceptAt(hole+uint32(i*1000), 1000, now, rtt)
		now = now.Add(200 * time.Microsecond)
	}
	if r.Stats().ForgivenGaps != 1 {
		t.Fatalf("horizon did not converge: %+v", r.Stats())
	}
	if pressureHoleRTTs == 1 && r.Stats().SoftForgivenGaps != 1 {
		t.Fatal("one-RTT profile missed soft pressure")
	}
	if deliver, _ := r.AcceptAt(hole, 1000, now, rtt); !deliver {
		t.Fatal("late first arrival lost")
	}
	if deliver, _ := r.AcceptAt(hole, 1000, now, rtt); deliver {
		t.Fatal("late repair duplicated")
	}
}

func TestAdaptivePressureBurstWaitsForHoleAgeAndResetsNextHole(t *testing.T) {
	r := NewReceiver(100)
	r.EnableSteadyStateDelivery()
	now := time.Unix(1, 0)
	rtt := 600 * time.Millisecond
	for i := 0; i < 100; i++ {
		r.AcceptAt(r.Next(), 10, now, rtt)
		now = now.Add(10 * time.Millisecond)
	}
	hole := r.Next()
	// Keep two holes so one retirement must not lend its age to the next.
	for i := 1; i <= 300; i++ {
		r.AcceptAt(hole+uint32(i*20), 10, now, rtt)
	}
	if r.Next() != hole {
		t.Fatal("burst bypassed hole age")
	}
	wait := time.Duration(pressureHoleRTTs) * rtt
	r.AcceptAt(hole+6020, 10, now.Add(wait-time.Nanosecond), rtt)
	if r.Next() != hole {
		t.Fatal("hole retired before one configured wait")
	}
	r.AcceptAt(hole+6040, 10, now.Add(wait), rtt)
	if r.Stats().SoftForgivenGaps != 1 {
		t.Fatalf("expected exactly one aged hole: %+v", r.Stats())
	}
	if r.pressure.holeSince != now.Add(wait) {
		t.Fatal("new hole inherited old age")
	}
}

func TestAdaptivePressureEmergencyWithoutRTTAndAcrossSequenceWrap(t *testing.T) {
	r := NewReceiver(^uint32(0) - 100)
	r.EnableSteadyStateDelivery()
	now := time.Unix(1, 0)
	hole := r.Next()
	for i := 1; i <= PartialReliabilityEmergencyLimit+500; i++ {
		if deliver, _ := r.AcceptAt(hole+uint32(i*100), 100, now, 0); !deliver {
			t.Fatal("fresh dropped at emergency")
		}
		if len(r.outOfOrder) >= PartialReliabilityEmergencyLimit {
			t.Fatal("emergency debt not reduced")
		}
	}
	if r.Stats().EmergencyForgivenGaps != 1 || r.Stats().SoftForgivenGaps != 0 {
		t.Fatalf("bad cold-start pressure: %+v", r.Stats())
	}
}

func TestAdaptivePressureIgnoresRecentDuplicatesAndRecoversAfterIdle(t *testing.T) {
	r := NewReceiver(100)
	r.EnableSteadyStateDelivery()
	now := time.Unix(1, 0)
	for i := 0; i < 2000; i++ {
		r.AcceptAt(r.Next(), 10, now, 600*time.Millisecond)
		now = now.Add(200 * time.Microsecond)
	}
	before := r.pressure.count
	for i := 0; i < 10000; i++ {
		r.AcceptAt(r.Next()-10, 10, now, 600*time.Millisecond)
	}
	if r.pressure.count != before {
		t.Fatal("duplicates inflated rate")
	}
	now = now.Add(10 * time.Second)
	for i := 0; i < 1000; i++ {
		r.AcceptAt(r.Next(), 10, now, 600*time.Millisecond)
		now = now.Add(10 * time.Millisecond)
	}
	if r.Stats().PressureSoftLimit > 200 {
		t.Fatalf("rate stayed pinned after slowdown: %+v", r.Stats())
	}
	if r.Stats().ForgivenGaps != 0 {
		t.Fatal("in-order low-rate traffic abandoned repair")
	}
}
