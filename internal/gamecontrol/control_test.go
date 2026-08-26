package gamecontrol

import (
	"math"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/linkpolicy"
)

func TestManualSpeedForcesCeilingAndFourLaneCost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LinkSpeedMode = linkpolicy.LinkSpeedManual
	cfg.ManualLinkSpeedMbps = 100
	cfg.RequestedLanes = 4
	cfg.MaxLanes = 4
	m := Measurement{AutoLinkSpeedMbps: 37, Loss: 0.01, MeanBurst: 1, CarrierExpansion: 1}
	p, err := BuildPlan(cfg, m)
	if err != nil { t.Fatal(err) }
	if p.EffectiveLinkMbps != 100 || p.ActiveLanes != 4 || p.AutoLaneAdded {
		t.Fatalf("plan=%+v", p)
	}
	wantExpansion := (1256.0 / 1200.0) * 4
	if math.Abs(p.TotalWireExpansion-wantExpansion) > 1e-12 {
		t.Fatalf("expansion=%f want=%f", p.TotalWireExpansion, wantExpansion)
	}
	wantInner := 100 * 0.92 / wantExpansion
	if math.Abs(p.InnerCeilingMbps-wantInner) > 1e-12 {
		t.Fatalf("inner=%f want=%f", p.InnerCeilingMbps, wantInner)
	}
}

func TestManualSpeedWorksWithNoAutoEstimate(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LinkSpeedMode = linkpolicy.LinkSpeedManual
	cfg.ManualLinkSpeedMbps = 75
	cfg.RequestedLanes = 2
	cfg.MaxLanes = 2
	p, err := BuildPlan(cfg, Measurement{AutoLinkSpeedMbps: 0, Loss: 0.01, MeanBurst: 1, CarrierExpansion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if p.EffectiveLinkMbps != 75 || p.AutoLinkSpeedMbps != 0 {
		t.Fatalf("manual no-auto=%+v", p)
	}
}

func TestFixedLaneSettingOneThroughFour(t *testing.T) {
	for lanes := 1; lanes <= 4; lanes++ {
		lanes := lanes
		t.Run(string(rune('0'+lanes)), func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.RequestedLanes = lanes
			cfg.MaxLanes = 4
			cfg.AutoAddLanes = false
			p, err := BuildPlan(cfg, Measurement{AutoLinkSpeedMbps: 100, Loss: 0.30, MeanBurst: 4, CarrierExpansion: 1})
			if err != nil { t.Fatal(err) }
			if p.ActiveLanes != lanes || p.AutoLaneAdded {
				t.Fatalf("lanes=%d plan=%+v", lanes, p)
			}
		})
	}
}

func TestAutoAddRespectsRequestedFloorAndMaxLaneCap(t *testing.T) {
	for maxLanes := 1; maxLanes <= 4; maxLanes++ {
		cfg := DefaultConfig()
		cfg.RequestedLanes = 1
		cfg.AutoAddLanes = true
		cfg.MaxLanes = maxLanes
		p, err := BuildPlan(cfg, Measurement{AutoLinkSpeedMbps: 100, Loss: 0.20, MeanBurst: 2, CarrierExpansion: 1})
		if err != nil { t.Fatal(err) }
		if p.ActiveLanes < 1 || p.ActiveLanes > maxLanes {
			t.Fatalf("max=%d plan=%+v", maxLanes, p)
		}
		if maxLanes == 1 && (p.ActiveLanes != 1 || p.AutoLaneAdded) {
			t.Fatalf("max one=%+v", p)
		}
	}

	cfg := DefaultConfig()
	cfg.RequestedLanes = 3
	cfg.AutoAddLanes = true
	cfg.MaxLanes = 4
	p, err := BuildPlan(cfg, Measurement{AutoLinkSpeedMbps: 100, Loss: 0, MeanBurst: 1, CarrierExpansion: 1})
	if err != nil { t.Fatal(err) }
	if p.ActiveLanes != 3 || p.AutoLaneAdded {
		t.Fatalf("requested floor changed=%+v", p)
	}
}

func TestActualFECGeometryControlsCeiling(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LinkSpeedMode = linkpolicy.LinkSpeedManual
	cfg.ManualLinkSpeedMbps = 100
	cfg.RequestedLanes = 2
	cfg.MaxLanes = 2
	m := Measurement{AutoLinkSpeedMbps: 0, Loss: 0.10, MeanBurst: 1, CarrierExpansion: 1}
	off, err := BuildPlan(cfg, m)
	if err != nil { t.Fatal(err) }
	cfg.FEC = FEC20x20
	fec, err := BuildPlan(cfg, m)
	if err != nil { t.Fatal(err) }
	if fec.FEC != FEC20x20 || math.Abs(fec.InnerCeilingMbps-off.InnerCeilingMbps/2) > 1e-12 {
		t.Fatalf("off=%+v fec=%+v", off, fec)
	}
}

func TestAutoAddNeverLeavesGameOrDropsRequestedFloor(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequestedLanes = 1
	cfg.AutoAddLanes = true
	cfg.MaxLanes = 4
	m := Measurement{AutoLinkSpeedMbps: 80, Loss: 0.10, MeanBurst: 1, CarrierExpansion: 1}
	p, err := BuildPlan(cfg, m)
	if err != nil { t.Fatal(err) }
	if p.ActiveLanes != 4 || !p.AutoLaneAdded {
		t.Fatalf("auto=%+v", p)
	}
	cfg.RequestedLanes = 3
	m.Loss = 0
	p, err = BuildPlan(cfg, m)
	if err != nil { t.Fatal(err) }
	if p.ActiveLanes != 3 || p.AutoLaneAdded {
		t.Fatalf("floor=%+v", p)
	}
}

func TestAutoSpeedUsesObservedServiceCapacityAndIgnoresManualValue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequestedLanes = 2
	cfg.MaxLanes = 2
	cfg.ManualLinkSpeedMbps = 999
	m := Measurement{AutoLinkSpeedMbps: 123, Loss: 0.02, MeanBurst: 1, CarrierExpansion: 1.1}
	p, err := BuildPlan(cfg, m)
	if err != nil { t.Fatal(err) }
	if p.LinkSpeedMode != linkpolicy.LinkSpeedAuto || p.EffectiveLinkMbps != 123 {
		t.Fatalf("plan=%+v", p)
	}
}

func TestManualSpeedIgnoresConflictingAutoValue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LinkSpeedMode = linkpolicy.LinkSpeedManual
	cfg.ManualLinkSpeedMbps = 60
	cfg.RequestedLanes = 1
	cfg.MaxLanes = 1
	p, err := BuildPlan(cfg, Measurement{AutoLinkSpeedMbps: 400, Loss: 0, MeanBurst: 1, CarrierExpansion: 1})
	if err != nil { t.Fatal(err) }
	if p.EffectiveLinkMbps != 60 {
		t.Fatalf("manual authority lost=%+v", p)
	}
}

func TestCarrierExpansionAndMaxWireUtilAffectCeilingOnly(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LinkSpeedMode = linkpolicy.LinkSpeedManual
	cfg.ManualLinkSpeedMbps = 100
	cfg.RequestedLanes = 1
	cfg.MaxLanes = 1
	base, err := BuildPlan(cfg, Measurement{AutoLinkSpeedMbps: 0, Loss: 0.01, MeanBurst: 1, CarrierExpansion: 1})
	if err != nil { t.Fatal(err) }
	expanded, err := BuildPlan(cfg, Measurement{AutoLinkSpeedMbps: 0, Loss: 0.01, MeanBurst: 1, CarrierExpansion: 1.25})
	if err != nil { t.Fatal(err) }
	if math.Abs(expanded.InnerCeilingMbps-base.InnerCeilingMbps/1.25) > 1e-12 {
		t.Fatalf("base=%+v expanded=%+v", base, expanded)
	}
	cfg.MaxWireUtil = 0.46
	half, err := BuildPlan(cfg, Measurement{AutoLinkSpeedMbps: 0, Loss: 0.01, MeanBurst: 1, CarrierExpansion: 1})
	if err != nil { t.Fatal(err) }
	if math.Abs(half.InnerCeilingMbps-base.InnerCeilingMbps/2) > 1e-12 {
		t.Fatalf("base=%+v half=%+v", base, half)
	}
}

func TestRejectsInvalidSettings(t *testing.T) {
	validM := Measurement{AutoLinkSpeedMbps: 100, Loss: 0.01, MeanBurst: 1, CarrierExpansion: 1}
	cases := []struct{
		name string
		mut  func(*Config, *Measurement)
	}{
		{"auto-zero", func(c *Config, m *Measurement) { m.AutoLinkSpeedMbps = 0 }},
		{"manual-zero", func(c *Config, m *Measurement) { c.LinkSpeedMode = linkpolicy.LinkSpeedManual; c.ManualLinkSpeedMbps = 0; m.AutoLinkSpeedMbps = 0 }},
		{"lanes-zero", func(c *Config, m *Measurement) { c.RequestedLanes = 0 }},
		{"lanes-five", func(c *Config, m *Measurement) { c.RequestedLanes = 5 }},
		{"max-below-floor", func(c *Config, m *Measurement) { c.RequestedLanes = 3; c.MaxLanes = 2 }},
		{"max-five", func(c *Config, m *Measurement) { c.MaxLanes = 5 }},
		{"bad-fec", func(c *Config, m *Measurement) { c.FEC = FECProfile("20:8") }},
		{"bad-race-target", func(c *Config, m *Measurement) { c.RaceTarget = 1 }},
		{"bad-carrier", func(c *Config, m *Measurement) { m.CarrierExpansion = .9 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			m := validM
			tc.mut(&cfg, &m)
			if _, err := BuildPlan(cfg, m); err == nil {
				t.Fatalf("expected error cfg=%+v measurement=%+v", cfg, m)
			}
		})
	}
}
