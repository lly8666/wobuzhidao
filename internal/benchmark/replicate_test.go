package benchmark

import (
	"context"
	"testing"
	"time"
)

func TestReplicatedUpperBoundRuns2xAnd3x(t *testing.T) {
	for _, copies := range []int{2, 3} {
		p := RealFaultProfile{
			LaneCount: copies, Seed: 991, Samples: 12, PayloadBytes: 64,
			MinOneWay: time.Millisecond, MaxOneWay: time.Millisecond,
			ImpairBasisPoints: 1500, ExtraHold: 10 * time.Millisecond,
			SoftDeadline: 8 * time.Millisecond, Window: 4, BurstLength: 1,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		got, err := RunRealFaultWBDReplicated(ctx, p, copies)
		cancel()
		if err != nil {
			t.Fatalf("copies=%d: %v", copies, err)
		}
		if got.Completed != p.Samples || got.DeliveryRatio != 1 {
			t.Fatalf("copies=%d observation=%+v", copies, got)
		}
		if got.IntentionalBytes != got.SourceBytes*uint64(copies) {
			t.Fatalf("copies=%d bytes source=%d intentional=%d", copies, got.SourceBytes, got.IntentionalBytes)
		}
	}
}
