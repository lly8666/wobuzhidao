package benchmark

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/rbc"
)

func TestBuildRealFaultScheduleDeterministicLowImpairment(t *testing.T) {
	p := DefaultLowImpairmentProfile()
	a, err := BuildRealFaultSchedule(p)
	if err != nil {
		t.Fatal(err)
	}
	b, err := BuildRealFaultSchedule(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(a.Impaired) != p.Samples || len(b.Impaired) != p.Samples {
		t.Fatalf("bad schedule length: %d/%d", len(a.Impaired), len(b.Impaired))
	}
	want := (p.Samples*int(p.ImpairBasisPoints) + 5000) / 10000
	if want < 1 {
		want = 1
	}
	if got := countImpaired(a.Impaired); got != want {
		t.Fatalf("impaired=%d want=%d", got, want)
	}
	for i := range a.Impaired {
		if a.Impaired[i] != b.Impaired[i] || a.Forward[i] != b.Forward[i] || a.Reverse[i] != b.Reverse[i] {
			t.Fatalf("schedule differs at sample %d", i)
		}
		if a.Impaired[i] && i%2 != 0 {
			t.Fatalf("impairment landed on lane-2 source index %d", i)
		}
	}
}

func TestRealTwoLaneZeroImpairment(t *testing.T) {
	p := DefaultLowImpairmentProfile()
	p.Samples = 16
	p.ImpairBasisPoints = 0
	p.ExtraHold = 0
	p.Window = 1
	sched, err := BuildRealFaultSchedule(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	for _, mode := range []rbc.ProtectionMode{rbc.ModeNormal, rbc.ModeAuto} {
		obs, err := RunRealFaultWBD(ctx, p, sched, mode)
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if obs.Completed != p.Samples || obs.CompletionRatio != 1 || obs.DeliveryRatio != 1 {
			t.Fatalf("%s completion=%d/%d completion_ratio=%.4f delivery_ratio=%.4f", mode, obs.Completed, p.Samples, obs.CompletionRatio, obs.DeliveryRatio)
		}
		if obs.GapEvents != 0 || obs.ReinjectionBytes != 0 || obs.IntentionalBytes != obs.SourceBytes {
			t.Fatalf("%s unexpected recovery on clean path: gaps=%d reinject=%d intentional=%d source=%d", mode, obs.GapEvents, obs.ReinjectionBytes, obs.IntentionalBytes, obs.SourceBytes)
		}
		if mode == rbc.ModeAuto && obs.FinalMultiplier != rbc.Multiplier10 {
			t.Fatalf("Auto left 1.0x on clean two-lane path: %s", obs.FinalMultiplier)
		}
		t.Logf("%s target=%v mean=%v p50=%v p95=%v p99=%v completion=%.3f intentional=%d", obs.Name, obs.TargetMeanRTT, obs.Mean, obs.P50, obs.P95, obs.P99, obs.CompletionRatio, obs.IntentionalBytes)
	}
}

func TestRealTwoLaneLowImpairment(t *testing.T) {
	p := DefaultLowImpairmentProfile()
	sched, err := BuildRealFaultSchedule(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	all, err := RunRealFaultMatrix(ctx, p, sched)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("observations=%d", len(all))
	}
	for _, obs := range all {
		if obs.Completed != p.Samples || obs.CompletionRatio != 1 || obs.DeliveryRatio != 1 {
			t.Fatalf("%s completion=%d/%d completion_ratio=%.4f delivery_ratio=%.4f", obs.Name, obs.Completed, p.Samples, obs.CompletionRatio, obs.DeliveryRatio)
		}
		if math.Abs(obs.ImpairmentRatio-0.01) > 0.0001 {
			t.Fatalf("%s impairment ratio=%.4f", obs.Name, obs.ImpairmentRatio)
		}
		if obs.IntentionalBytes > obs.SourceBytes*2 {
			t.Fatalf("%s intentional=%d source=%d exceeds 2x", obs.Name, obs.IntentionalBytes, obs.SourceBytes)
		}
		t.Logf("%s target=%v mean=%v p50=%v p95=%v p99=%v completion=%.3f late=%.3f impairment=%.3f source=%d intentional=%d reinject=%d gaps=%d final=%s", obs.Name, obs.TargetMeanRTT, obs.Mean, obs.P50, obs.P95, obs.P99, obs.CompletionRatio, obs.LateRatio, obs.ImpairmentRatio, obs.SourceBytes, obs.IntentionalBytes, obs.ReinjectionBytes, obs.GapEvents, obs.FinalMultiplier)
	}
	for _, obs := range all[2:] {
		if obs.GapEvents == 0 || obs.ReinjectionBytes == 0 {
			t.Fatalf("%s did not exercise logical gap/reinjection: gaps=%d reinject=%d", obs.Name, obs.GapEvents, obs.ReinjectionBytes)
		}
		if obs.IntentionalBytes <= obs.SourceBytes {
			t.Fatalf("%s did not account intentional recovery bytes", obs.Name)
		}
	}
}

func TestRealTwoLaneModerateImpairment(t *testing.T) {
	p := DefaultModerateImpairmentProfile()
	sched, err := BuildRealFaultSchedule(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	all, err := RunRealFaultMatrix(ctx, p, sched)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 4 {
		t.Fatalf("observations=%d", len(all))
	}
	for _, obs := range all {
		if obs.Completed != p.Samples || obs.CompletionRatio != 1 || obs.DeliveryRatio != 1 {
			t.Fatalf("%s completion=%d/%d completion_ratio=%.4f delivery_ratio=%.4f", obs.Name, obs.Completed, p.Samples, obs.CompletionRatio, obs.DeliveryRatio)
		}
		if math.Abs(obs.ImpairmentRatio-0.02) > 0.0001 {
			t.Fatalf("%s impairment ratio=%.4f", obs.Name, obs.ImpairmentRatio)
		}
		if obs.IntentionalBytes > obs.SourceBytes*2 {
			t.Fatalf("%s intentional=%d source=%d exceeds 2x", obs.Name, obs.IntentionalBytes, obs.SourceBytes)
		}
		t.Logf("moderate %s target=%v mean=%v p50=%v p95=%v p99=%v completion=%.3f late=%.3f impairment=%.3f source=%d intentional=%d reinject=%d gaps=%d final=%s", obs.Name, obs.TargetMeanRTT, obs.Mean, obs.P50, obs.P95, obs.P99, obs.CompletionRatio, obs.LateRatio, obs.ImpairmentRatio, obs.SourceBytes, obs.IntentionalBytes, obs.ReinjectionBytes, obs.GapEvents, obs.FinalMultiplier)
	}
	for _, obs := range all[2:] {
		if obs.GapEvents == 0 || obs.ReinjectionBytes == 0 {
			t.Fatalf("%s did not exercise logical gap/reinjection: gaps=%d reinject=%d", obs.Name, obs.GapEvents, obs.ReinjectionBytes)
		}
	}
}
