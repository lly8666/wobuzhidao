package faketcp

import "testing"

func TestEffectiveCarrierMTUHonorsConfiguredCeiling(t *testing.T) {
	got, err := EffectiveCarrierMTU(1500, 1300)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1300 {
		t.Fatalf("effective carrier MTU=%d want 1300", got)
	}
	budget, err := CarrierPayloadBudget(got)
	if err != nil {
		t.Fatal(err)
	}
	if budget != 1260 {
		t.Fatalf("payload budget=%d want 1260", budget)
	}
}

func TestEffectiveCarrierMTUNeverExceedsLocalInterface(t *testing.T) {
	got, err := EffectiveCarrierMTU(1280, 1500)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1280 {
		t.Fatalf("effective carrier MTU=%d want local 1280", got)
	}
}

func TestEffectiveCarrierMTUZeroPreservesLegacyInterfaceBehavior(t *testing.T) {
	got, err := EffectiveCarrierMTU(1500, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != 1500 {
		t.Fatalf("effective carrier MTU=%d want 1500", got)
	}
}

func TestEffectiveCarrierMTURejectsNegativeCeiling(t *testing.T) {
	if _, err := EffectiveCarrierMTU(1500, -1); err == nil {
		t.Fatal("negative configured connection MTU unexpectedly accepted")
	}
}
