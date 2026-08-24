package benchmark

import (
	"fmt"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/rbc"
)

func TestStandardMatrixAndTailDeadline(t *testing.T) {
	strategies := []struct {
		s Strategy
		m rbc.MultiplierQ4
	}{
		{StrategyNativeTCP, rbc.Multiplier10},
		{StrategyNativeUDP, rbc.Multiplier10},
		{StrategyWBDReinjection, rbc.Multiplier15},
		{StrategyWBDTailDeadline, rbc.Multiplier15},
		{StrategyWBDDuplicate, rbc.Multiplier15},
		{StrategyWBDXOR, rbc.Multiplier15},
	}
	got := map[string]Result{}
	for _, p := range StandardProfiles() {
		for _, sm := range strategies {
			r, err := Run(p, sm.s, sm.m)
			if err != nil {
				t.Fatalf("%s/%s: %v", p.Name, sm.s, err)
			}
			if r.ProtectionBytes > r.EntitledBytes {
				t.Fatalf("overspend: %+v", r)
			}
			got[fmt.Sprintf("%s/%s", p.Name, sm.s)] = r
			t.Logf("%-17s %-18s p50=%-5s p95=%-5s p99=%-5s completion=%-5s protection=%d/%d steps=%v", p.Name, sm.s, r.P50, r.P95, r.P99, r.Completion, r.ProtectionBytes, r.EntitledBytes, r.ChunkUsableStep)
		}
	}

	tcp := got["single-stall/native-tcp"]
	udp := got["single-stall/native-udp"]
	reinj := got["single-stall/wbd-reinjection"]
	if !(udp.P50 < tcp.P50 && reinj.P95 < tcp.P95) {
		t.Fatalf("expected bypass/recovery advantage: tcp=%+v udp=%+v reinj=%+v", tcp, udp, reinj)
	}

	gapOnly := got["final-chunk-stall/wbd-reinjection"]
	deadline := got["final-chunk-stall/wbd-tail-deadline"]
	if gapOnly.Completion != 8*10*time.Millisecond || deadline.Completion >= gapOnly.Completion {
		t.Fatalf("tail recovery mismatch gap=%s deadline=%s", gapOnly.Completion, deadline.Completion)
	}
}

func TestProfileValidationAndQuantiles(t *testing.T) {
	p := StandardProfiles()[0]
	p.Version = 99
	if _, err := Run(p, StrategyNativeTCP, rbc.Multiplier10); err == nil {
		t.Fatal("bad version accepted")
	}
	p = StandardProfiles()[0]
	if _, err := Run(p, StrategyNativeTCP, rbc.Multiplier15); err == nil {
		t.Fatal("native TCP accepted redundancy multiplier")
	}
}

func TestTwoXDuplicateCeilingAndXORUsesSpareReinjection(t *testing.T) {
	var same Profile
	for _, p := range StandardProfiles() {
		if p.Name == "burst-same-xor" {
			same = p
			break
		}
	}
	if same.Name == "" {
		t.Fatal("missing same-pair profile")
	}
	dup, err := Run(same, StrategyWBDDuplicate, rbc.Multiplier20)
	if err != nil {
		t.Fatal(err)
	}
	if dup.Completion != same.Step || dup.ProtectionBytes != dup.EntitledBytes {
		t.Fatalf("2x duplicate ceiling mismatch: %+v", dup)
	}
	x, err := Run(same, StrategyWBDXOR, rbc.Multiplier20)
	if err != nil {
		t.Fatal(err)
	}
	if x.Completion != 2*same.Step || x.FECBytes == 0 || x.ReinjectionBytes == 0 {
		t.Fatalf("2x XOR should spend spare budget on reinjection: %+v", x)
	}
}
