package benchmark

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/rbc"
)

func TestStressProfilesDeterministicAndRequestedLossFamilies(t *testing.T) {
	a, b := StressProfiles(), StressProfiles()
	if !reflect.DeepEqual(a, b) {
		t.Fatal("stress profiles are not deterministic")
	}
	if len(a) != 8 {
		t.Fatalf("profiles=%d", len(a))
	}
	seen10, seen20, seen30 := 0, 0, 0
	for _, p := range a {
		r, err := RunStress(p, StrategyNativeUDP, rbc.Multiplier10)
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case strings.Contains(p.Name, "10pct"):
			seen10++
			if r.DroppedChunks != 7 || r.DeliveryPPM != 890625 {
				t.Fatalf("10pct %s: %+v", p.Name, r)
			}
		case strings.Contains(p.Name, "20pct"):
			seen20++
			if r.DroppedChunks != 13 || r.DeliveryPPM != 796875 {
				t.Fatalf("20pct %s: %+v", p.Name, r)
			}
		case strings.Contains(p.Name, "30pct"):
			seen30++
			if r.DroppedChunks != 20 || r.DeliveryPPM != 687500 {
				t.Fatalf("30pct %s: %+v", p.Name, r)
			}
		}
	}
	if seen10 != 2 || seen20 != 2 || seen30 != 2 {
		t.Fatalf("family counts %d/%d/%d", seen10, seen20, seen30)
	}
}

func TestStressProtectionPaysAlternateLaneDelay(t *testing.T) {
	for _, p := range StressProfiles() {
		if !strings.Contains(p.Name, "150-300ms") {
			continue
		}
		dup, err := RunStress(p, StrategyWBDDuplicate, rbc.Multiplier20)
		if err != nil {
			t.Fatal(err)
		}
		// The old micro-model incorrectly made every proactive duplicate usable at
		// 10 ms. A realistic weak-network protection lane cannot beat its 150 ms
		// minimum propagation class merely because redundancy is enabled.
		if dup.P50 < 150*time.Millisecond {
			t.Fatalf("unrealistic duplicate latency %s: %s", p.Name, dup.P50)
		}
	}
}

func TestStressReliableStrategiesPreserveDelivery(t *testing.T) {
	for _, p := range StressProfiles() {
		tcp, err := RunStress(p, StrategyNativeTCP, rbc.Multiplier10)
		if err != nil {
			t.Fatal(err)
		}
		if tcp.DeliveryPPM != 1_000_000 {
			t.Fatalf("tcp delivery %s: %d", p.Name, tcp.DeliveryPPM)
		}
		for _, s := range []Strategy{StrategyWBDTailDeadline, StrategyWBDDuplicate, StrategyWBDXOR} {
			r, err := RunStress(p, s, rbc.Multiplier20)
			if err != nil {
				t.Fatal(err)
			}
			if r.DeliveryPPM != 1_000_000 {
				t.Fatalf("%s/%s delivery=%d", p.Name, s, r.DeliveryPPM)
			}
		}
	}
}

func TestStressMatrixHasNormalAndSevereRows(t *testing.T) {
	rows, err := StressMatrix()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 80 {
		t.Fatalf("rows=%d", len(rows))
	}
	var normal, weak20, extreme bool
	for _, r := range rows {
		normal = normal || r.Profile == "normal-40-60ms"
		weak20 = weak20 || strings.Contains(r.Profile, "20pct")
		extreme = extreme || strings.Contains(r.Profile, "30pct")
	}
	if !normal || !weak20 || !extreme {
		t.Fatalf("coverage normal=%v weak20=%v extreme=%v", normal, weak20, extreme)
	}
}
