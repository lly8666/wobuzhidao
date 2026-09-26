package linkestimate

import (
	"math"
	"testing"
	"time"
)

func TestProbeTrainKeepsLossSeparateFromServiceRate(t *testing.T) {
	base := time.Unix(100, 0)
	packets := make([]ProbePacket, 0, 64)
	for i := 0; i < 64; i++ {
		// 1200 bytes every 48us is a 200-Mbit sender. Receiver serialization is
		// 96us, i.e. 100 Mbit/s. Drop 20% of sequence numbers; surviving
		// consecutive pairs must still estimate ~100 Mbit/s rather than 80.
		if i%5 == 0 {
			continue
		}
		packets = append(packets, ProbePacket{
			Seq:        1000 + uint64(i),
			Bytes:      1200,
			SentAt:     base.Add(time.Duration(i) * 48 * time.Microsecond),
			ReceivedAt: base.Add(time.Second + time.Duration(i)*96*time.Microsecond),
		})
	}
	e, err := EstimateProbeTrain(1000, 64, packets)
	if err != nil {
		t.Fatal(err)
	}
	if e.Loss < 0.19 || e.Loss > 0.22 {
		t.Fatalf("loss=%f", e.Loss)
	}
	if math.Abs(e.ServiceMbps-100) > 0.01 {
		t.Fatalf("service=%f", e.ServiceMbps)
	}
	if e.PairSamples < 20 || e.Confidence < 0.9 {
		t.Fatalf("pairs=%d confidence=%f", e.PairSamples, e.Confidence)
	}
}

func TestProbeTrainMeasuresMissingRunBurst(t *testing.T) {
	base := time.Unix(200, 0)
	packets := make([]ProbePacket, 0, 20)
	missing := map[int]bool{3: true, 4: true, 5: true, 12: true}
	for i := 0; i < 20; i++ {
		if missing[i] {
			continue
		}
		packets = append(packets, ProbePacket{
			Seq:        uint64(i),
			Bytes:      1000,
			SentAt:     base.Add(time.Duration(i) * 100 * time.Microsecond),
			ReceivedAt: base.Add(time.Second + time.Duration(i)*100*time.Microsecond),
		})
	}
	e, err := EstimateProbeTrain(0, 20, packets)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(e.MeanBurst-2) > 1e-12 {
		t.Fatalf("mean burst=%f", e.MeanBurst)
	}
}

func TestDeliveryRateCapsAckCompressionBySendRate(t *testing.T) {
	got := DeliveryRateMbps(1_000_000, 100*time.Millisecond, 50*time.Millisecond)
	if math.Abs(got-80) > 1e-9 {
		t.Fatalf("delivery rate=%f", got)
	}
	got = DeliveryRateMbps(1_000_000, 50*time.Millisecond, 100*time.Millisecond)
	if math.Abs(got-80) > 1e-9 {
		t.Fatalf("ack-limited delivery rate=%f", got)
	}
}

func TestRateWindowPrefersNonAppLimitedAndMarksFallback(t *testing.T) {
	base := time.Unix(300, 0)
	w := NewRateWindow(10 * time.Second)
	w.Add(RatePoint{At: base, Mbps: 30, AppLimited: true})
	w.Add(RatePoint{At: base.Add(time.Second), Mbps: 80, AppLimited: false})
	w.Add(RatePoint{At: base.Add(2 * time.Second), Mbps: 60, AppLimited: false})
	rate, n, lower := w.Estimate(base.Add(3 * time.Second))
	if rate != 80 || n != 2 || lower {
		t.Fatalf("rate=%f n=%d lower=%t", rate, n, lower)
	}

	w2 := NewRateWindow(10 * time.Second)
	w2.Add(RatePoint{At: base, Mbps: 25, AppLimited: true})
	rate, n, lower = w2.Estimate(base.Add(time.Second))
	if rate != 25 || n != 1 || !lower {
		t.Fatalf("fallback rate=%f n=%d lower=%t", rate, n, lower)
	}
}

func TestRateWindowExpiresOldSamples(t *testing.T) {
	base := time.Unix(400, 0)
	w := NewRateWindow(5 * time.Second)
	w.Add(RatePoint{At: base, Mbps: 100})
	w.Add(RatePoint{At: base.Add(6 * time.Second), Mbps: 40})
	rate, n, lower := w.Estimate(base.Add(6 * time.Second))
	if rate != 40 || n != 1 || lower {
		t.Fatalf("rate=%f n=%d lower=%t", rate, n, lower)
	}
}

func TestChirpRates(t *testing.T) {
	r, err := ChirpRates(2, 200, 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(r) != 9 || r[0] != 2 || r[len(r)-1] != 200 {
		t.Fatalf("rates=%v", r)
	}
	for i := 1; i < len(r); i++ {
		if !(r[i] > r[i-1]) {
			t.Fatalf("not increasing: %v", r)
		}
	}
}
