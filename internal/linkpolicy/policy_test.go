package linkpolicy

import (
	"math"
	"testing"
)

func TestExpectedDeliveryIIDAndBurst(t *testing.T) {
	iid := ExpectedDelivery(20, 4, 0.05, 1)
	if iid < 0.998 || iid > 0.9995 {
		t.Fatalf("iid delivery=%f", iid)
	}
	burst := ExpectedDelivery(20, 4, 0.05, 4)
	if !(burst < iid) {
		t.Fatalf("burst delivery=%f iid=%f", burst, iid)
	}
	if got := ExpectedDelivery(20, 0, 0.03, 1); math.Abs(got-0.97) > 1e-12 {
		t.Fatalf("off delivery=%f", got)
	}
}

func TestRecommendBalancedAndConservative(t *testing.T) {
	obs := DefaultObservation(100, 0.10, 1)
	balanced, err := Recommend(obs, ModeBalanced)
	if err != nil {
		t.Fatal(err)
	}
	if balanced.FECProfile != "20:8" || balanced.ParityShards != 8 || !balanced.MeetsTarget {
		t.Fatalf("balanced=%+v", balanced)
	}
	conservative, err := Recommend(obs, ModeConservative)
	if err != nil {
		t.Fatal(err)
	}
	if conservative.FECProfile != "20:12" || conservative.ParityShards != 12 {
		t.Fatalf("conservative=%+v", conservative)
	}
	if !(conservative.InnerMbps < balanced.InnerMbps) {
		t.Fatalf("conservative inner=%f balanced=%f", conservative.InnerMbps, balanced.InnerMbps)
	}
	if !(conservative.PredictedDelivery >= balanced.PredictedDelivery) {
		t.Fatalf("conservative delivery=%f balanced=%f", conservative.PredictedDelivery, balanced.PredictedDelivery)
	}
}

func TestConservativeBumpsOffOneProtectionStep(t *testing.T) {
	obs := DefaultObservation(50, 0, 1)
	balanced, _ := Recommend(obs, ModeBalanced)
	conservative, _ := Recommend(obs, ModeConservative)
	if balanced.FECProfile != "off" || conservative.FECProfile != "20:4" {
		t.Fatalf("balanced=%+v conservative=%+v", balanced, conservative)
	}
}

func TestGameTwoLaneRequestOnlyOnLowLossLowBurst(t *testing.T) {
	low := DefaultObservation(100, 0.01, 1)
	r, err := Recommend(low, ModeGame)
	if err != nil {
		t.Fatal(err)
	}
	if r.LaneCount != 2 || !r.GameLaneEligible || !r.ExperimentalLane {
		t.Fatalf("low-loss game=%+v", r)
	}

	highLoss := DefaultObservation(100, 0.05, 1)
	r, _ = Recommend(highLoss, ModeGame)
	if r.LaneCount != 1 || r.GameLaneEligible {
		t.Fatalf("high-loss game=%+v", r)
	}

	bursty := DefaultObservation(100, 0.01, 2)
	r, _ = Recommend(bursty, ModeGame)
	if r.LaneCount != 1 || r.GameLaneEligible {
		t.Fatalf("bursty game=%+v", r)
	}
}

func TestCarrierExpansionReducesInnerRateWithoutChangingCapacity(t *testing.T) {
	base := DefaultObservation(100, 0.10, 1)
	a, _ := Recommend(base, ModeBalanced)
	base.CarrierExpansion = 1.25
	b, _ := Recommend(base, ModeBalanced)
	if math.Abs(b.InnerMbps-a.InnerMbps/1.25) > 1e-9 {
		t.Fatalf("base=%f expanded=%f", a.InnerMbps, b.InnerMbps)
	}
	if a.FECProfile != b.FECProfile {
		t.Fatalf("carrier expansion changed FEC: %s -> %s", a.FECProfile, b.FECProfile)
	}
}

func TestExtremeBurstCanReportTargetUnmet(t *testing.T) {
	obs := DefaultObservation(100, 0.30, 4)
	r, err := Recommend(obs, ModeBalanced)
	if err != nil {
		t.Fatal(err)
	}
	if r.FECProfile != "20:20" || r.MeetsTarget {
		t.Fatalf("extreme burst=%+v", r)
	}
}

func TestRejectsInvalidObservation(t *testing.T) {
	obs := DefaultObservation(0, 0.01, 1)
	if _, err := Recommend(obs, ModeBalanced); err == nil {
		t.Fatal("expected capacity validation error")
	}
	obs = DefaultObservation(10, 0.01, 1)
	obs.CarrierExpansion = 0.9
	if _, err := Recommend(obs, ModeBalanced); err == nil {
		t.Fatal("expected carrier expansion validation error")
	}
}
