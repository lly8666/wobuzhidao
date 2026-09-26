package protection

import (
	"fmt"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/rbc"
)

func chunks(n, size int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		out[i] = make([]byte, size)
		for j := range out[i] {
			out[i][j] = byte((i+1)*17 + j%251)
		}
	}
	return out
}

func TestSameBudgetAdmissionMatrix(t *testing.T) {
	base := chunks(8, 1024)
	scenarios := []Scenario{
		{Name: "single-even", Chunks: base, StallUntil: map[int]int{0: 9}},
		{Name: "single-odd", Chunks: base, StallUntil: map[int]int{1: 9}},
		{Name: "burst-same-xor-pair", Chunks: base, StallUntil: map[int]int{0: 9, 1: 9}},
		{Name: "burst-cross-xor-pair", Chunks: base, StallUntil: map[int]int{1: 9, 2: 9}},
	}
	strategies := []Strategy{StrategyReinjection, StrategyDuplicate, StrategyXORRepair}
	multipliers := []rbc.MultiplierQ4{rbc.Multiplier15, rbc.Multiplier20}

	got := make(map[string]Result)
	for _, m := range multipliers {
		for _, sc := range scenarios {
			for _, strategy := range strategies {
				res, err := Run(sc, strategy, m)
				if err != nil {
					t.Fatalf("%s/%s/%s: %v", m, sc.Name, strategy, err)
				}
				if res.ProtectionBytes > res.EntitledBytes {
					t.Fatalf("overspend %s/%s/%s: %+v", m, sc.Name, strategy, res)
				}
				key := fmt.Sprintf("%s/%s/%s", m, sc.Name, strategy)
				got[key] = res
				t.Logf("%-4s %-22s %-12s completion=%d protection=%d/%d dup=%d reinj=%d fec=%d xorRecovered=%d", m, sc.Name, strategy, res.CompletionStep, res.ProtectionBytes, res.EntitledBytes, res.DuplicateBytes, res.ReinjectionBytes, res.FECBytes, res.XORRecovered)
			}
		}
	}

	// At 1.5x, one XOR symbol protects either member of every 2-chunk pair.
	// It therefore beats reactive reinjection by one deterministic step for
	// either single-stall position and for a burst split across two XOR groups.
	for _, name := range []string{"single-even", "single-odd", "burst-cross-xor-pair"} {
		x := got[fmt.Sprintf("1.5x/%s/xor-repair", name)]
		r := got[fmt.Sprintf("1.5x/%s/reinjection", name)]
		if x.CompletionStep >= r.CompletionStep {
			t.Fatalf("expected repeatable 1.5x XOR advantage for %s: xor=%d reinj=%d", name, x.CompletionStep, r.CompletionStep)
		}
	}

	// The minimal pairwise XOR code is not universal: two stalled chunks from
	// the same pair cannot be recovered from one parity symbol. Reactive
	// reinjection wins this case and keeps FEC experimental rather than making it
	// a production wire-format commitment in M9.
	x := got["1.5x/burst-same-xor-pair/xor-repair"]
	r := got["1.5x/burst-same-xor-pair/reinjection"]
	if x.CompletionStep <= r.CompletionStep {
		t.Fatalf("expected pairwise XOR failure case: xor=%d reinj=%d", x.CompletionStep, r.CompletionStep)
	}

	// At 2.0x full proactive duplication is the latency ceiling in this simple
	// model, while XOR can retain spare budget for reinjection of same-pair
	// bursts. This is a reason to keep RBC allocation policy separate from the
	// codec choice.
	for _, name := range []string{"single-even", "single-odd", "burst-same-xor-pair", "burst-cross-xor-pair"} {
		d := got[fmt.Sprintf("2.0x/%s/duplicate", name)]
		if d.CompletionStep != 1 {
			t.Fatalf("2x duplicate should complete at step 1 for %s, got %d", name, d.CompletionStep)
		}
	}
}

func TestXORRecoveryReturnsExactBytes(t *testing.T) {
	base := chunks(4, 257)
	parity := xor(base[0], base[1])
	recovered := xor(parity, base[0])
	if string(recovered) != string(base[1]) {
		t.Fatal("xor reconstruction changed logical bytes")
	}
	res, err := Run(Scenario{Name: "exact", Chunks: base, StallUntil: map[int]int{1: 7}}, StrategyXORRepair, rbc.Multiplier15)
	if err != nil {
		t.Fatal(err)
	}
	if res.XORRecovered != 1 || res.CompletionStep != 1 {
		t.Fatalf("unexpected recovery result: %+v", res)
	}
}

func TestWeak15SingleStallSweepIsNotCherryPicked(t *testing.T) {
	base := chunks(8, 1024)
	dupFast := 0
	dupSlow := 0
	reinjFast := 0
	reinjTailBlind := 0
	for stalled := range base {
		sc := Scenario{Name: fmt.Sprintf("single-%d", stalled), Chunks: base, StallUntil: map[int]int{stalled: 9}}
		x, err := Run(sc, StrategyXORRepair, rbc.Multiplier15)
		if err != nil {
			t.Fatal(err)
		}
		r, err := Run(sc, StrategyReinjection, rbc.Multiplier15)
		if err != nil {
			t.Fatal(err)
		}
		d, err := Run(sc, StrategyDuplicate, rbc.Multiplier15)
		if err != nil {
			t.Fatal(err)
		}
		if x.CompletionStep != 1 {
			t.Fatalf("stall=%d xor=%d", stalled, x.CompletionStep)
		}
		// GAP-driven recovery needs a later known offset. A stalled final chunk
		// has no successor to reveal its hole, so the current M6 baseline waits
		// for the original TCP copy. This is an intentional admission finding,
		// not papered over with a timer that M6 does not yet implement.
		if stalled == len(base)-1 {
			if r.CompletionStep != 9 {
				t.Fatalf("tail stall reinjection=%d want=9", r.CompletionStep)
			}
			reinjTailBlind++
		} else {
			if r.CompletionStep != 2 {
				t.Fatalf("stall=%d reinjection=%d want=2", stalled, r.CompletionStep)
			}
			reinjFast++
		}
		switch d.CompletionStep {
		case 1:
			dupFast++
		case 9:
			dupSlow++
		default:
			t.Fatalf("stall=%d duplicate completion=%d", stalled, d.CompletionStep)
		}
	}
	if dupFast != 4 || dupSlow != 4 {
		t.Fatalf("1.5x duplicate coverage fast=%d slow=%d", dupFast, dupSlow)
	}
	if reinjFast != 7 || reinjTailBlind != 1 {
		t.Fatalf("reinjection gap visibility fast=%d tailBlind=%d", reinjFast, reinjTailBlind)
	}
}

func TestInvalidScenarioAndBudgetBounds(t *testing.T) {
	if _, err := Run(Scenario{Chunks: [][]byte{{1}, {1, 2}}}, StrategyXORRepair, rbc.Multiplier15); err == nil {
		t.Fatal("mismatched chunks accepted")
	}
	if _, err := Run(Scenario{Chunks: chunks(2, 1)}, StrategyXORRepair, rbc.Multiplier125); err == nil {
		t.Fatal("M9 unexpectedly accepted sub-1.5x experiment")
	}
}
