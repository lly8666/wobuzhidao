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

func TestActualFECGeometryControlsCeiling(t *testing.T) {
	cfg := DefaultConfig()
	cfg.LinkSpeedMode = linkpolicy.LinkSpeedManual
	cfg.ManualLinkSpeedMbps = 100
	cfg.RequestedLanes = 2
	cfg.MaxLanes = 2
	m := Measurement{AutoLinkSpeedMbps: 100, Loss: 0.10, MeanBurst: 1, CarrierExpansion: 1}
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

func TestAutoSpeedUsesObservedServiceCapacity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.RequestedLanes = 2
	cfg.MaxLanes = 2
	m := Measurement{AutoLinkSpeedMbps: 123, Loss: 0.02, MeanBurst: 1, CarrierExpansion: 1.1}
	p, err := BuildPlan(cfg, m)
	if err != nil { t.Fatal(err) }
	if p.LinkSpeedMode != linkpolicy.LinkSpeedAuto || p.EffectiveLinkMbps != 123 {
		t.Fatalf("plan=%+v", p)
	}
}

func TestRejectsUnsupportedLiveFEC(t *testing.T) {
	cfg := DefaultConfig()
	cfg.FEC = FECProfile("20:8")
	_, err := BuildPlan(cfg, Measurement{AutoLinkSpeedMbps: 100, Loss: 0.01, MeanBurst: 1, CarrierExpansion: 1})
	if err == nil { t.Fatal("expected error") }
}
