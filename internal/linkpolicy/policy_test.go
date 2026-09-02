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

func TestManualLinkSpeedIsForcedAuthority(t *testing.T) {
	obs := DefaultObservation(37, 0.01, 1)
	auto, err := Recommend(obs, ModeBalanced)
	if err != nil { t.Fatal(err) }
	obs.LinkSpeedMode = LinkSpeedManual
	obs.ManualLinkSpeedMbps = 120
	manual, err := Recommend(obs, ModeBalanced)
	if err != nil { t.Fatal(err) }
	if manual.EffectiveCapacityMbps != 120 || manual.LinkSpeedMode != LinkSpeedManual {
		t.Fatalf("manual=%+v", manual)
	}
	if math.Abs(manual.InnerMbps/auto.InnerMbps-120.0/37.0) > 1e-9 {
		t.Fatalf("auto=%f manual=%f", auto.InnerMbps, manual.InnerMbps)
	}
}

func TestManualLinkSpeedWorksWithoutAutoSample(t *testing.T) {
	obs := DefaultObservation(0, 0.02, 1)
	obs.LinkSpeedMode = LinkSpeedManual
	obs.ManualLinkSpeedMbps = 88
	r, err := Recommend(obs, ModeGame)
	if err != nil {
		t.Fatal(err)
	}
	if r.EffectiveCapacityMbps != 88 || r.LinkSpeedMode != LinkSpeedManual {
		t.Fatalf("manual no-auto=%+v", r)
	}
}

func TestGameManualFourLaneAndInnerCeilingPaysForCopies(t *testing.T) {
	obs := DefaultObservation(100, 0.01, 1)
	obs.GameRequestedLanes = 4
	obs.GameMaxLanes = 4
	game, err := Recommend(obs, ModeGame)
	if err != nil { t.Fatal(err) }
	if game.Mode != ModeGame || game.LaneCount != 4 || game.AutoLaneAdded {
		t.Fatalf("game=%+v", game)
	}

	singleObs := obs
	singleObs.GameRequestedLanes = 1
	singleObs.GameMaxLanes = 4
	single, err := Recommend(singleObs, ModeGame)
	if err != nil { t.Fatal(err) }
	if math.Abs(game.InnerMbps-single.InnerMbps/4) > 1e-9 {
		t.Fatalf("single=%f four=%f", single.InnerMbps, game.InnerMbps)
	}
	if math.Abs(game.WireExpansion-game.PerLaneWireExpansion*4) > 1e-9 {
		t.Fatalf("expansion=%+v", game)
	}
}

func TestGameAutoAddOnlyRaisesLaneFloorUpToFour(t *testing.T) {
	obs := DefaultObservation(100, 0.10, 1)
	obs.GameRequestedLanes = 1
	obs.GameAutoAddLanes = true
	obs.GameMaxLanes = 4
	game, err := Recommend(obs, ModeGame)
	if err != nil { t.Fatal(err) }
	if game.Mode != ModeGame || game.LaneCount != 4 || !game.AutoLaneAdded {
		t.Fatalf("auto game=%+v", game)
	}
	if game.FECProfile != "20:8" {
		t.Fatalf("game FEC unexpectedly replaced: %+v", game)
	}

	obs.GameRequestedLanes = 3
	obs.Loss = 0
	game, err = Recommend(obs, ModeGame)
	if err != nil { t.Fatal(err) }
	if game.LaneCount != 3 || game.AutoLaneAdded {
		t.Fatalf("auto lane must not downshift requested floor: %+v", game)
	}
}

func TestGameHighLossNeverChangesOutOfGameMode(t *testing.T) {
	obs := DefaultObservation(100, 0.30, 4)
	obs.GameRequestedLanes = 2
	obs.GameAutoAddLanes = true
	obs.GameMaxLanes = 4
	game, err := Recommend(obs, ModeGame)
	if err != nil { t.Fatal(err) }
	if game.Mode != ModeGame || game.LaneCount < 2 || game.LaneCount > 4 {
		t.Fatalf("game degraded=%+v", game)
	}
	if game.FECProfile != "20:20" || game.MeetsTarget {
		t.Fatalf("extreme game=%+v", game)
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
		t.Fatal("expected auto capacity validation error")
	}
	obs = DefaultObservation(10, 0.01, 1)
	obs.CarrierExpansion = 0.9
	if _, err := Recommend(obs, ModeBalanced); err == nil {
		t.Fatal("expected carrier expansion validation error")
	}
	obs = DefaultObservation(0, 0.01, 1)
	obs.LinkSpeedMode = LinkSpeedManual
	obs.ManualLinkSpeedMbps = 0
	if _, err := Recommend(obs, ModeBalanced); err == nil {
		t.Fatal("expected manual speed validation error")
	}
	obs = DefaultObservation(-1, 0.01, 1)
	obs.LinkSpeedMode = LinkSpeedManual
	obs.ManualLinkSpeedMbps = 10
	if _, err := Recommend(obs, ModeBalanced); err == nil {
		t.Fatal("expected invalid diagnostic auto sample error")
	}
	obs = DefaultObservation(10, 0.01, 1)
	obs.GameRequestedLanes = 5
	if _, err := Recommend(obs, ModeGame); err == nil {
		t.Fatal("expected four-lane cap validation error")
	}
}
