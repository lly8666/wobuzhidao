package rbc

import (
	"errors"
	"math"
	"testing"
)

func TestFixedModes(t *testing.T) {
	cases := []struct {
		mode ProtectionMode
		want MultiplierQ4
	}{
		{ModeNormal, Multiplier10},
		{ModeWeak15, Multiplier15},
		{ModeWeak20, Multiplier20},
	}
	for _, tc := range cases {
		got, err := FixedMultiplier(tc.mode)
		if err != nil || got != tc.want {
			t.Fatalf("mode=%s got=%v err=%v", tc.mode, got, err)
		}
		if got > MaxMultiplier {
			t.Fatalf("mode=%s exceeded hard cap: %v", tc.mode, got)
		}
	}
	if _, err := FixedMultiplier(ModeAuto); err == nil {
		t.Fatal("auto unexpectedly has fixed multiplier")
	}
}

func TestBudgetFixedMultipliersAndKinds(t *testing.T) {
	b := NewBudget()
	if err := b.AdmitSource(10, Multiplier15); err != nil {
		t.Fatal(err)
	}
	if got := b.Available(); got != 5 {
		t.Fatalf("1.5x credit=%d want=5", got)
	}
	if err := b.Spend(SpendReinjection, 2); err != nil {
		t.Fatal(err)
	}
	if err := b.Spend(SpendDuplicate, 3); err != nil {
		t.Fatal(err)
	}
	if err := b.Spend(SpendFEC, 1); !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("overspend=%v", err)
	}
	s := b.Snapshot()
	if s.SourceBytes != 10 || s.EntitledBytes != 5 || s.SpentBytes != 5 || s.ReinjectionBytes != 2 || s.DuplicateBytes != 3 || s.FECBytes != 0 {
		t.Fatalf("snapshot=%+v", s)
	}
}

func TestBudgetQuarterCarryAndModeChangesAreProspective(t *testing.T) {
	b := NewBudget()
	// Four 1-byte source admissions at 1.25x must earn exactly one byte.
	for i := 0; i < 4; i++ {
		if err := b.AdmitSource(1, Multiplier125); err != nil {
			t.Fatal(err)
		}
	}
	if got := b.Available(); got != 1 {
		t.Fatalf("quarter carry credit=%d", got)
	}
	// Ramping to 2x grants full protection only for newly admitted source.
	if err := b.AdmitSource(4, Multiplier20); err != nil {
		t.Fatal(err)
	}
	if got := b.Available(); got != 5 {
		t.Fatalf("prospective auto credit=%d want=5", got)
	}
}

func TestAutoFastUpSevereJumpAndHardCap(t *testing.T) {
	c := NewAutoController()
	if got := c.Multiplier(); got != Multiplier10 {
		t.Fatalf("initial=%v", got)
	}
	bad := QualitySample{Delivered: 100, Late: 3}
	if got := c.Observe(bad); got != Multiplier125 {
		t.Fatalf("first bad=%v", got)
	}
	if got := c.Observe(bad); got != Multiplier15 {
		t.Fatalf("second bad=%v", got)
	}
	if got := c.Observe(QualitySample{Delivered: 100, Stalled: true}); got != Multiplier20 {
		t.Fatalf("severe=%v", got)
	}
	for i := 0; i < 10; i++ {
		if got := c.Observe(QualitySample{Delivered: 1, Stalled: true}); got != Multiplier20 {
			t.Fatalf("hard cap iteration %d got=%v", i, got)
		}
	}
}

func TestAutoSlowDownRequiresConsecutiveCleanWindows(t *testing.T) {
	c := NewAutoController()
	c.Observe(QualitySample{Delivered: 100, Stalled: true})
	if c.Multiplier() != Multiplier20 {
		t.Fatal("failed to enter 2x")
	}
	clean := QualitySample{Delivered: 1000}
	for i := 0; i < int(CleanWindowsToDrop)-1; i++ {
		if got := c.Observe(clean); got != Multiplier20 {
			t.Fatalf("premature drop at %d: %v", i, got)
		}
	}
	// Mild late delivery resets the clean streak without raising protection.
	if got := c.Observe(QualitySample{Delivered: 1000, Late: 6}); got != Multiplier20 {
		t.Fatalf("mild should hold 2x, got=%v", got)
	}
	for i := 0; i < int(CleanWindowsToDrop); i++ {
		got := c.Observe(clean)
		if i < int(CleanWindowsToDrop)-1 && got != Multiplier20 {
			t.Fatalf("drop before full new clean streak: %v", got)
		}
	}
	if got := c.Multiplier(); got != Multiplier15 {
		t.Fatalf("first slow drop=%v", got)
	}
	for step, want := range []MultiplierQ4{Multiplier125, Multiplier10} {
		for i := 0; i < int(CleanWindowsToDrop); i++ {
			c.Observe(clean)
		}
		if got := c.Multiplier(); got != want {
			t.Fatalf("slow step %d got=%v want=%v", step, got, want)
		}
	}
}

func TestAutoGapSignalsAndAntiFlap(t *testing.T) {
	c := NewAutoController()
	// One logical gap is actionable even with no late-ratio denominator.
	if got := c.Observe(QualitySample{GapEvents: 1}); got != Multiplier125 {
		t.Fatalf("gap fast-up=%v", got)
	}
	// Two no-progress gaps are severe and jump to 2x.
	if got := c.Observe(QualitySample{GapEvents: 2}); got != Multiplier20 {
		t.Fatalf("severe gap=%v", got)
	}
	// Alternating clean/mild windows cannot flap downward.
	for i := 0; i < 8; i++ {
		if i%2 == 0 {
			c.Observe(QualitySample{Delivered: 1000})
		} else {
			c.Observe(QualitySample{Delivered: 1000, Late: 6})
		}
	}
	if got := c.Multiplier(); got != Multiplier20 {
		t.Fatalf("anti-flap failed, got=%v", got)
	}
}

func TestBudgetRejectsInvalidAndOverflow(t *testing.T) {
	b := NewBudget()
	if err := b.AdmitSource(1, MultiplierQ4(9)); !errors.Is(err, ErrInvalidMultiplier) {
		t.Fatalf("invalid multiplier=%v", err)
	}
	if err := b.Spend(SpendKind(99), 0); !errors.Is(err, ErrInvalidSpendKind) {
		t.Fatalf("invalid kind=%v", err)
	}
	// Source counter overflow is explicit rather than wrapping.
	if err := b.AdmitSource(math.MaxUint64, Multiplier10); err != nil {
		t.Fatal(err)
	}
	if err := b.AdmitSource(1, Multiplier10); !errors.Is(err, ErrCounterOverflow) {
		t.Fatalf("overflow=%v", err)
	}
}

func TestUnifiedControllerKeepsFixedModesFixed(t *testing.T) {
	for _, tc := range []struct {
		mode ProtectionMode
		want MultiplierQ4
	}{
		{ModeNormal, Multiplier10},
		{ModeWeak15, Multiplier15},
		{ModeWeak20, Multiplier20},
	} {
		c, err := NewController(tc.mode)
		if err != nil {
			t.Fatal(err)
		}
		for i := 0; i < 4; i++ {
			got := c.Observe(QualitySample{Delivered: 1, Stalled: true})
			if got != tc.want {
				t.Fatalf("mode=%s changed on sample: got=%v want=%v", tc.mode, got, tc.want)
			}
		}
	}
	if _, err := NewController(ProtectionMode(99)); err == nil {
		t.Fatal("invalid mode accepted")
	}
}
