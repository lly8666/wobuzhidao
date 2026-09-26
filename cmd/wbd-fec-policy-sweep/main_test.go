package main

import "testing"

func TestParseProfile(t *testing.T) {
	p, err := parseProfile("10:10")
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "10:10" || p.K != 10 || p.R != 10 {
		t.Fatalf("unexpected profile: %+v", p)
	}
	if _, err := parseProfile("10:0"); err == nil {
		t.Fatal("expected invalid zero-parity profile")
	}
}

func TestBetterCandidatePrefersInnerRateThenLatency(t *testing.T) {
	base := aggregateRow{OfferedInnerMbps: 10, P99MaxMS: 30, WireRatioMax: 1.5, K: 20, R: 10}
	faster := base
	faster.OfferedInnerMbps = 11
	faster.P99MaxMS = 100
	if !betterCandidate(faster, base) {
		t.Fatal("higher qualified inner rate must win")
	}
	lowerLatency := base
	lowerLatency.P99MaxMS = 20
	if !betterCandidate(lowerLatency, base) {
		t.Fatal("lower p99 must break equal-rate ties")
	}
	lowerWire := base
	lowerWire.WireRatioMax = 1.4
	if !betterCandidate(lowerWire, base) {
		t.Fatal("lower wire ratio must break equal-rate/equal-p99 ties")
	}
}

func TestSelectRecommendationsRejectsIneligibleRows(t *testing.T) {
	rows := []aggregateRow{
		{CapacityMbps: 20, LossPct: 5, RTTMS: 20, BurstLength: 1, Profile: "off", K: 20, R: 0, OfferedInnerMbps: 12, Eligible: false},
		{CapacityMbps: 20, LossPct: 5, RTTMS: 20, BurstLength: 1, Profile: "20:8", K: 20, R: 8, OfferedInnerMbps: 8, Eligible: true, DeliveryMin: 1, P99MaxMS: 15, WireRatioMax: 1.5, PhysicalUtilMax: 0.6},
		{CapacityMbps: 20, LossPct: 5, RTTMS: 20, BurstLength: 1, Profile: "20:20", K: 20, R: 20, OfferedInnerMbps: 10, Eligible: true, DeliveryMin: 1, P99MaxMS: 18, WireRatioMax: 2.1, PhysicalUtilMax: 0.9},
	}
	recs := selectRecommendations(rows)
	if len(recs) != 1 {
		t.Fatalf("recommendations=%d want=1", len(recs))
	}
	if recs[0].Profile != "20:20" || recs[0].InnerMbps != 10 {
		t.Fatalf("unexpected recommendation: %+v", recs[0])
	}
}

func TestCompareBlocksPairsEqualOperatingPoint(t *testing.T) {
	rows := []aggregateRow{
		{CapacityMbps: 50, LossPct: 10, RTTMS: 100, BurstLength: 4, Profile: "20:20", OfferedInnerMbps: 20, DeliveryMin: .99, P99MaxMS: 80, WireRatioMax: 2.1},
		{CapacityMbps: 50, LossPct: 10, RTTMS: 100, BurstLength: 4, Profile: "10:10", OfferedInnerMbps: 20, DeliveryMin: .995, P99MaxMS: 70, WireRatioMax: 2.1},
	}
	cmp := compareBlocks(rows)
	if len(cmp) != 1 {
		t.Fatalf("comparisons=%d want=1", len(cmp))
	}
	if cmp[0].P99DeltaMS != -10 {
		t.Fatalf("p99 delta=%v want=-10", cmp[0].P99DeltaMS)
	}
	if cmp[0].DeliveryDeltaPP < 0.49 || cmp[0].DeliveryDeltaPP > 0.51 {
		t.Fatalf("delivery delta pp=%v want about 0.5", cmp[0].DeliveryDeltaPP)
	}
}
