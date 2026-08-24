package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/rbc"
)

func TestNormalNetworkRealRTTGate(t *testing.T) {
	p := DefaultNormalRTTProfile()
	p.Samples = 16
	sched, err := BuildRTTSchedule(p)
	if err != nil {
		t.Fatal(err)
	}
	target := scheduleMean(sched)
	if target < 48*time.Millisecond || target > 52*time.Millisecond {
		t.Fatalf("bad seeded target mean RTT: %v", target)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	all, err := RunNormalRTTMatrix(ctx, p, sched)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("got %d observations", len(all))
	}
	tcp, udp, normal, auto := all[0], all[1], all[2], all[3]

	// Run all modes concurrently against the same schedule. Host-wide VM
	// descheduling then becomes common-mode noise instead of penalizing whichever
	// transport happened to run later. Gate systematic protocol overhead using
	// median per-sample excess and report raw arithmetic mean/outliers separately.
	for _, got := range []RTTObservation{tcp, udp, normal, auto} {
		if got.P50 < 47*time.Millisecond || got.P50 > 60*time.Millisecond {
			t.Fatalf("%s p50=%v not consistent with 40-60ms RTT schedule", got.Name, got.P50)
		}
		if got.InlierSamples < p.Samples/2 {
			t.Fatalf("%s only %d/%d inlier samples", got.Name, got.InlierSamples, p.Samples)
		}
		t.Logf("%s target=%v mean=%v inlier_mean=%v p50=%v p95=%v p99=%v median_excess=%v host_outliers=%d", got.Name, target, got.Mean, got.InlierMean, got.P50, got.P95, got.P99, got.MedianExcess, got.HostOutliers)
	}
	base := tcp.MedianExcess
	if normal.MedianExcess > base+2*time.Millisecond {
		t.Fatalf("WBD normal adds systematic latency: tcp=%v wbd=%v", base, normal.MedianExcess)
	}
	if auto.MedianExcess > base+2*time.Millisecond {
		t.Fatalf("WBD auto adds systematic latency: tcp=%v auto=%v", base, auto.MedianExcess)
	}
	if auto.FinalMultiplier != rbc.Multiplier10 {
		t.Fatalf("Auto should remain 1.0x on clean network, got %s", auto.FinalMultiplier)
	}
}
