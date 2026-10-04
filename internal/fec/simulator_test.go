package fec

import (
	"testing"
	"time"
)

func baseSimConfig(kind ScheduleKind) SimConfig {
	return SimConfig{
		Schedule:      kind,
		Samples:       40,
		PayloadBytes:  1000,
		HeaderBytes:   0,
		OfferedMbps:   8,
		CapacityMbps:  1000,
		OneWay:        10 * time.Millisecond,
		Loss:          0,
		Seed:          1,
		DataShards:    20,
		ParityShards:  10,
		MicroData:     5,
		MicroParity:   3,
		CausalWindow:  20,
	}
}

func TestScheduleSimulationNoFECZeroLoss(t *testing.T) {
	cfg := baseSimConfig(ScheduleOff)
	obs, err := RunScheduleSimulation(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Delivered != cfg.Samples || obs.Recovered != 0 {
		t.Fatalf("unexpected observation: %+v", obs)
	}
	if obs.P99 < cfg.OneWay || obs.P99 > cfg.OneWay+100*time.Microsecond {
		t.Fatalf("p99=%s, want propagation floor plus one serialization", obs.P99)
	}
	if obs.WireRatio != 1 {
		t.Fatalf("wire ratio=%v, want 1", obs.WireRatio)
	}
}

func TestEarlierRepairSchedulersBeatTailForEarlyLoss(t *testing.T) {
	tail := baseSimConfig(ScheduleTail)
	tail.ForceDropSourceIDs = []int{0}
	micro := baseSimConfig(ScheduleMicro)
	micro.ForceDropSourceIDs = []int{0}
	causal := baseSimConfig(ScheduleCausal)
	causal.ForceDropSourceIDs = []int{0}

	tailObs, err := RunScheduleSimulation(tail)
	if err != nil {
		t.Fatal(err)
	}
	microObs, err := RunScheduleSimulation(micro)
	if err != nil {
		t.Fatal(err)
	}
	causalObs, err := RunScheduleSimulation(causal)
	if err != nil {
		t.Fatal(err)
	}

	if tailObs.Recovered < 1 || microObs.Recovered < 1 || causalObs.Recovered < 1 {
		t.Fatalf("missing repair recovery: tail=%+v micro=%+v causal=%+v", tailObs, microObs, causalObs)
	}
	if !(causalObs.Max < microObs.Max && microObs.Max < tailObs.Max) {
		t.Fatalf("want causal < micro < tail for first-source loss; tail=%s micro=%s causal=%s",
			tailObs.Max, microObs.Max, causalObs.Max)
	}
}

func TestCausalNeedsEnoughIndependentEquations(t *testing.T) {
	cfg := baseSimConfig(ScheduleCausal)
	cfg.ForceDropSourceIDs = []int{0, 1}
	obs, err := RunScheduleSimulation(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if obs.Recovered < 2 {
		t.Fatalf("want both forced losses reconstructed, got %+v", obs)
	}
}

func TestBurstLossSimulationDeterministic(t *testing.T) {
	cfg := baseSimConfig(ScheduleTail)
	cfg.Loss = 0.10
	cfg.BurstLength = 4
	cfg.Seed = 260825

	a, err := RunScheduleSimulation(cfg)
	if err != nil {
		t.Fatal(err)
	}
	b, err := RunScheduleSimulation(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("same seed must reproduce identical observation: a=%+v b=%+v", a, b)
	}
}

func TestScheduleSimulationRejectsInvalidConfig(t *testing.T) {
	cfg := baseSimConfig(ScheduleCausal)
	cfg.CausalWindow = 0
	if _, err := RunScheduleSimulation(cfg); err == nil {
		t.Fatal("expected invalid configuration error")
	}
}
