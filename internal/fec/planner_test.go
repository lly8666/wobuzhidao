package fec

import (
	"math"
	"testing"
)

func TestMinParityForRepresentativeRandomLossTargets(t *testing.T) {
	tests := []struct {
		loss   float64
		target float64
		wantR  int
	}{
		{0.01, 1e-5, 4},
		{0.05, 1e-5, 8},
		{0.10, 1e-5, 12},
		{0.15, 1e-5, 16},
		{0.20, 1e-5, 20},
		{0.01, 1e-3, 3},
		{0.05, 1e-3, 6},
		{0.10, 1e-3, 9},
		{0.15, 1e-3, 12},
		{0.20, 1e-3, 15},
	}
	for _, tc := range tests {
		r, p, err := MinParityForTarget(20, 20, tc.loss, tc.target)
		if err != nil {
			t.Fatal(err)
		}
		if r != tc.wantR {
			t.Fatalf("loss=%.2f target=%g parity=%d want=%d p=%g", tc.loss, tc.target, r, tc.wantR, p)
		}
		if p > tc.target {
			t.Fatalf("loss=%.2f parity=%d p=%g exceeds target=%g", tc.loss, r, p, tc.target)
		}
		if r > 0 {
			prev, err := BlockFailureProbability(20, r-1, tc.loss)
			if err != nil {
				t.Fatal(err)
			}
			if prev <= tc.target {
				t.Fatalf("loss=%.2f parity=%d not minimal: previous p=%g target=%g", tc.loss, r, prev, tc.target)
			}
		}
	}
}

func TestBlockFailureProbabilityKnown20x20AtTwentyPercent(t *testing.T) {
	got, err := BlockFailureProbability(20, 20, 0.20)
	if err != nil {
		t.Fatal(err)
	}
	const want = 5.027270342857917e-6
	if math.Abs(got-want) > 1e-15 {
		t.Fatalf("got %.17g want %.17g", got, want)
	}
}

func TestRepairDebtLowerBound(t *testing.T) {
	tests := []struct {
		loss float64
		want float64
	}{
		{0, 0},
		{0.05, 0.05 / 0.95},
		{0.10, 0.10 / 0.90},
		{0.20, 0.25},
	}
	for _, tc := range tests {
		got, err := RepairDebtLowerBound(tc.loss)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-tc.want) > 1e-12 {
			t.Fatalf("loss=%.2f got=%g want=%g", tc.loss, got, tc.want)
		}
	}
}

func TestApproxMeanNextRepairWait(t *testing.T) {
	got, err := ApproxMeanNextRepairWait(0.001, 0.5)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(got-0.001) > 1e-12 {
		t.Fatalf("got=%g want=0.001", got)
	}
}

func TestPlannerRejectsInvalidInputs(t *testing.T) {
	if _, err := BlockFailureProbability(0, 1, 0.1); err == nil {
		t.Fatal("zero data accepted")
	}
	if _, _, err := MinParityForTarget(20, 20, 0.1, 1); err == nil {
		t.Fatal("invalid target accepted")
	}
	if _, err := RepairDebtLowerBound(1); err == nil {
		t.Fatal("loss=1 accepted")
	}
	if _, err := ApproxMeanNextRepairWait(0.001, 0); err == nil {
		t.Fatal("zero repair ratio accepted")
	}
}
