package rbc

import (
	"errors"
	"fmt"
	"math/bits"
)

// ProtectionMode is a user-facing policy. Auto changes only the internal
// multiplier; it does not change the WBD wire format.
type ProtectionMode uint8

const (
	ModeNormal ProtectionMode = iota
	ModeWeak15
	ModeWeak20
	ModeAuto
)

func (m ProtectionMode) String() string {
	switch m {
	case ModeNormal:
		return "normal"
	case ModeWeak15:
		return "weak-1.5x"
	case ModeWeak20:
		return "weak-2x"
	case ModeAuto:
		return "auto"
	default:
		return "unknown"
	}
}

// MultiplierQ4 represents total intentional WBD bytes in quarter units:
// 4=1.0x, 5=1.25x, 6=1.5x, 8=2.0x. The hard protocol-policy ceiling is 2.0x.
type MultiplierQ4 uint8

const (
	Multiplier10  MultiplierQ4 = 4
	Multiplier125 MultiplierQ4 = 5
	Multiplier15  MultiplierQ4 = 6
	Multiplier20  MultiplierQ4 = 8
	MaxMultiplier              = Multiplier20
)

func (m MultiplierQ4) Valid() bool {
	switch m {
	case Multiplier10, Multiplier125, Multiplier15, Multiplier20:
		return true
	default:
		return false
	}
}

func (m MultiplierQ4) String() string {
	switch m {
	case Multiplier10:
		return "1.0x"
	case Multiplier125:
		return "1.25x"
	case Multiplier15:
		return "1.5x"
	case Multiplier20:
		return "2.0x"
	default:
		return fmt.Sprintf("q4(%d)", m)
	}
}

func FixedMultiplier(mode ProtectionMode) (MultiplierQ4, error) {
	switch mode {
	case ModeNormal:
		return Multiplier10, nil
	case ModeWeak15:
		return Multiplier15, nil
	case ModeWeak20:
		return Multiplier20, nil
	case ModeAuto:
		return 0, errors.New("auto mode has no fixed multiplier")
	default:
		return 0, errors.New("invalid protection mode")
	}
}

// Controller presents one uniform mode API to the caller. Fixed modes ignore
// quality samples; Auto owns the hysteresis state. This keeps mode selection
// separate from later FEC/duplicate/reinjection allocation.
type Controller struct {
	mode ProtectionMode
	auto *AutoController
}

func NewController(mode ProtectionMode) (*Controller, error) {
	switch mode {
	case ModeNormal, ModeWeak15, ModeWeak20:
		return &Controller{mode: mode}, nil
	case ModeAuto:
		return &Controller{mode: mode, auto: NewAutoController()}, nil
	default:
		return nil, errors.New("invalid protection mode")
	}
}

func (c *Controller) Mode() ProtectionMode {
	if c == nil {
		return ModeNormal
	}
	return c.mode
}

func (c *Controller) Multiplier() MultiplierQ4 {
	if c == nil {
		return Multiplier10
	}
	if c.mode == ModeAuto {
		return c.auto.Multiplier()
	}
	m, err := FixedMultiplier(c.mode)
	if err != nil {
		return Multiplier10
	}
	return m
}

func (c *Controller) Observe(s QualitySample) MultiplierQ4 {
	if c == nil {
		return Multiplier10
	}
	if c.mode != ModeAuto {
		return c.Multiplier()
	}
	return c.auto.Observe(s)
}

// QualitySample contains only logical-delivery observations for one sampling
// window. It deliberately does not expose TCP packet loss, TCP_INFO, or wall
// clock state to the controller.
type QualitySample struct {
	Delivered uint64 // logical items observed in the window
	Late      uint64 // items that missed their soft delivery target
	GapEvents uint32 // receiver-observed logical gaps
	Stalled   bool   // logical delivery made no useful progress for the window
}

type severity uint8

const (
	severityClean severity = iota
	severityMild
	severityBad
	severitySevere
)

func classify(s QualitySample) severity {
	if s.Stalled {
		return severitySevere
	}
	// Missing denominator with gap evidence is still actionable.
	if s.Delivered == 0 {
		if s.GapEvents >= 2 {
			return severitySevere
		}
		if s.GapEvents == 1 {
			return severityBad
		}
		return severityClean
	}
	// Compare ratios without floating point. Severe >=10%, bad >=2%, mild >=0.5%.
	if s.Late >= ceilFrac(s.Delivered, 1, 10) || s.GapEvents >= 3 {
		return severitySevere
	}
	if s.Late >= ceilFrac(s.Delivered, 1, 50) || s.GapEvents > 0 {
		return severityBad
	}
	if s.Late >= ceilFrac(s.Delivered, 1, 200) {
		return severityMild
	}
	return severityClean
}

func ceilFrac(v, num, den uint64) uint64 {
	if v == 0 || num == 0 || den == 0 {
		return 0
	}
	// Callers currently use num=1. Keep this integer-only so Auto decisions are
	// deterministic across architectures and never depend on float rounding.
	base := v / den
	if v%den != 0 {
		base++
	}
	return base * num
}

// AutoController implements deterministic fast-up / slow-down hysteresis.
// Bad windows raise one level immediately; severe windows jump directly to
// 2.0x. Protection falls by only one level after CleanWindowsToDrop consecutive
// clean windows. Any non-clean observation resets the downshift streak.
type AutoController struct {
	level       MultiplierQ4
	cleanStreak uint8
}

const CleanWindowsToDrop uint8 = 3

func NewAutoController() *AutoController { return &AutoController{level: Multiplier10} }

func (c *AutoController) Multiplier() MultiplierQ4 {
	if c == nil || !c.level.Valid() {
		return Multiplier10
	}
	return c.level
}

func (c *AutoController) Observe(s QualitySample) MultiplierQ4 {
	if c == nil {
		return Multiplier10
	}
	if !c.level.Valid() {
		c.level = Multiplier10
	}
	switch classify(s) {
	case severitySevere:
		c.level = Multiplier20
		c.cleanStreak = 0
	case severityBad:
		c.level = nextUp(c.level)
		c.cleanStreak = 0
	case severityMild:
		// Mild degradation is not enough to spend more bandwidth, but it is
		// enough to prevent a downshift.
		c.cleanStreak = 0
	case severityClean:
		if c.level == Multiplier10 {
			c.cleanStreak = 0
			break
		}
		c.cleanStreak++
		if c.cleanStreak >= CleanWindowsToDrop {
			c.level = nextDown(c.level)
			c.cleanStreak = 0
		}
	}
	if c.level > MaxMultiplier {
		c.level = MaxMultiplier
	}
	return c.level
}

func nextUp(v MultiplierQ4) MultiplierQ4 {
	switch v {
	case Multiplier10:
		return Multiplier125
	case Multiplier125:
		return Multiplier15
	case Multiplier15, Multiplier20:
		return Multiplier20
	default:
		return Multiplier10
	}
}

func nextDown(v MultiplierQ4) MultiplierQ4 {
	switch v {
	case Multiplier20:
		return Multiplier15
	case Multiplier15:
		return Multiplier125
	case Multiplier125, Multiplier10:
		return Multiplier10
	default:
		return Multiplier10
	}
}

type SpendKind uint8

const (
	SpendDuplicate SpendKind = iota + 1
	SpendReinjection
	SpendFEC
)

var (
	ErrInvalidMultiplier = errors.New("invalid WBD redundancy multiplier")
	ErrBudgetExceeded    = errors.New("WBD redundancy budget exceeded")
	ErrCounterOverflow   = errors.New("WBD redundancy counter overflow")
	ErrInvalidSpendKind  = errors.New("invalid WBD redundancy spend kind")
)

// Budget is an intentional-overhead ledger. Source bytes earn protection
// credit at the multiplier that was active when those source bytes were
// admitted. This avoids retroactively granting a full 2x budget to all earlier
// traffic when Auto ramps up.
type Budget struct {
	source       uint64
	entitled     uint64
	spent        uint64
	quarterCarry uint8 // fractional protection credit in quarter-byte units
	byKind       map[SpendKind]uint64
}

type BudgetSnapshot struct {
	SourceBytes      uint64
	EntitledBytes    uint64
	SpentBytes       uint64
	AvailableBytes   uint64
	DuplicateBytes   uint64
	ReinjectionBytes uint64
	FECBytes         uint64
}

func NewBudget() *Budget { return &Budget{byKind: make(map[SpendKind]uint64)} }

// AdmitSource records original logical payload and earns future protection
// credit. Multiplier 1.5x means each two source bytes earn one protection byte.
func (b *Budget) AdmitSource(sourceBytes uint64, multiplier MultiplierQ4) error {
	if b == nil {
		return ErrCounterOverflow
	}
	if !multiplier.Valid() || multiplier > MaxMultiplier {
		return ErrInvalidMultiplier
	}
	if ^uint64(0)-b.source < sourceBytes {
		return ErrCounterOverflow
	}
	extraQ := uint64(multiplier - Multiplier10) // 0,1,2,4 quarter units per source byte
	whole := sourceBytes / 4
	rem := sourceBytes % 4
	hi, lo := bits.Mul64(whole, extraQ)
	if hi != 0 {
		return ErrCounterOverflow
	}
	frac := rem*extraQ + uint64(b.quarterCarry)
	add := lo + frac/4
	if add < lo || ^uint64(0)-b.entitled < add {
		return ErrCounterOverflow
	}
	b.source += sourceBytes
	b.entitled += add
	b.quarterCarry = uint8(frac % 4)
	return nil
}

func (b *Budget) Spend(kind SpendKind, bytes uint64) error {
	if b == nil {
		return ErrBudgetExceeded
	}
	if kind != SpendDuplicate && kind != SpendReinjection && kind != SpendFEC {
		return ErrInvalidSpendKind
	}
	if bytes > b.Available() {
		return ErrBudgetExceeded
	}
	if ^uint64(0)-b.spent < bytes || ^uint64(0)-b.byKind[kind] < bytes {
		return ErrCounterOverflow
	}
	b.spent += bytes
	b.byKind[kind] += bytes
	return nil
}

func (b *Budget) Available() uint64 {
	if b == nil || b.spent >= b.entitled {
		return 0
	}
	return b.entitled - b.spent
}

func (b *Budget) Snapshot() BudgetSnapshot {
	if b == nil {
		return BudgetSnapshot{}
	}
	return BudgetSnapshot{
		SourceBytes:      b.source,
		EntitledBytes:    b.entitled,
		SpentBytes:       b.spent,
		AvailableBytes:   b.Available(),
		DuplicateBytes:   b.byKind[SpendDuplicate],
		ReinjectionBytes: b.byKind[SpendReinjection],
		FECBytes:         b.byKind[SpendFEC],
	}
}
