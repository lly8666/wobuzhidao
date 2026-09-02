package linkadapt

import (
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

var (
	ErrBadWindow           = errors.New("linkadapt: invalid measurement window")
	ErrCounterReset        = errors.New("linkadapt: sender counters moved backwards")
	ErrInsufficientSamples = errors.New("linkadapt: insufficient loss samples")
	ErrBadBudget           = errors.New("linkadapt: invalid rate budget")
)

// Snapshot is intentionally tiny: periodic adaptation reuses counters that the
// FakeTCP sender already maintains for TCP-like ACK/SACK/retransmission state.
// Merely taking a snapshot sends no network traffic.
type Snapshot struct {
	At    time.Time
	Stats faketcp.SenderStats
}

// Sample describes one completed low-load observation window. LossMarked is a
// unique-segment signal: one original carrier segment is counted at most once
// even when TCP-like recovery retransmits it several times.
type Sample struct {
	Duration          time.Duration
	Segments          uint64
	OriginalBytes     uint64
	LossMarked        uint64
	LossMarkedBytes   uint64
	RetransmitAttempts uint64
	RetransmitBytes   uint64
}

func Between(before, after Snapshot) (Sample, error) {
	if before.At.IsZero() || !after.At.After(before.At) {
		return Sample{}, ErrBadWindow
	}
	b, a := before.Stats, after.Stats
	if a.Enqueued < b.Enqueued || a.EnqueuedBytes < b.EnqueuedBytes ||
		a.LossMarked < b.LossMarked || a.LossMarkedBytes < b.LossMarkedBytes ||
		a.FastRetransmits < b.FastRetransmits || a.RTOTransmits < b.RTOTransmits ||
		a.RetransmitBytes < b.RetransmitBytes {
		return Sample{}, ErrCounterReset
	}
	return Sample{
		Duration:           after.At.Sub(before.At),
		Segments:           a.Enqueued - b.Enqueued,
		OriginalBytes:      a.EnqueuedBytes - b.EnqueuedBytes,
		LossMarked:         a.LossMarked - b.LossMarked,
		LossMarkedBytes:    a.LossMarkedBytes - b.LossMarkedBytes,
		RetransmitAttempts: (a.FastRetransmits - b.FastRetransmits) + (a.RTOTransmits - b.RTOTransmits),
		RetransmitBytes:    a.RetransmitBytes - b.RetransmitBytes,
	}, nil
}

func (s Sample) LossRate() float64 {
	if s.Segments == 0 {
		return 0
	}
	return float64(s.LossMarked) / float64(s.Segments)
}

func (s Sample) OriginalBitrate() float64 {
	if s.Duration <= 0 {
		return 0
	}
	return float64(s.OriginalBytes*8) / s.Duration.Seconds()
}

// ARQFactor is the actual shadow-retransmission byte multiplier seen in the
// window. It is not used as the packet-loss estimate because repeated retries
// of one hole would bias that estimate upward.
func (s Sample) ARQFactor() float64 {
	if s.OriginalBytes == 0 {
		return 1
	}
	return 1 + float64(s.RetransmitBytes)/float64(s.OriginalBytes)
}

// Wilson95 returns a two-sided 95% Wilson score interval for the unique
// first-retransmission proportion. The upper bound is deliberately used for
// fixed-profile selection so borderline measurements choose the safer preset.
func (s Sample) Wilson95() (low, high float64) {
	if s.Segments == 0 {
		return 0, 1
	}
	n := float64(s.Segments)
	p := float64(s.LossMarked) / n
	z := 1.959963984540054
	z2 := z * z
	den := 1 + z2/n
	center := (p + z2/(2*n)) / den
	half := z * math.Sqrt((p*(1-p)+z2/(4*n))/n) / den
	low, high = center-half, center+half
	if low < 0 {
		low = 0
	}
	if high > 1 {
		high = 1
	}
	return low, high
}

// LowLoad reports whether the observation traffic stayed below a configured
// fraction of the physical-path capacity. Adaptation should wait rather than
// sample a queue-saturated period, because that would measure self-inflicted
// congestion loss instead of the path's low-load condition.
func (s Sample) LowLoad(capacityBps, maxFraction float64) bool {
	if capacityBps <= 0 || maxFraction <= 0 || maxFraction >= 1 || s.Duration <= 0 {
		return false
	}
	return s.OriginalBitrate() <= capacityBps*maxFraction
}

type Profile struct {
	Name string
	K    int
	R    int
}

var (
	ProfileOff  = Profile{Name: "off"}
	Profile20x4 = Profile{Name: "20:4", K: 20, R: 4}
	Profile20x8 = Profile{Name: "20:8", K: 20, R: 8}
	Profile20x12 = Profile{Name: "20:12", K: 20, R: 12}
	Profile20x16 = Profile{Name: "20:16", K: 20, R: 16}
	Profile20x20 = Profile{Name: "20:20", K: 20, R: 20}
)

func (p Profile) FECFactor() float64 {
	if p.K == 0 && p.R == 0 {
		return 1
	}
	if p.K <= 0 || p.R < 0 {
		return math.Inf(1)
	}
	return float64(p.K+p.R) / float64(p.K)
}

type Recommendation struct {
	Profile       Profile
	Estimate      float64
	WilsonLow95   float64
	WilsonHigh95  float64
	Samples       uint64
	Above20Pct    bool
}

// RecommendFixed is intentionally a coarse table, not a continuously adaptive
// controller. The breakpoints follow the existing K=20 iid planning work:
// roughly R=4/8/12/16/20 around 1/5/10/15/20% loss for a strong block-failure
// target. The 95% upper bound, rather than point estimate, selects the preset.
func RecommendFixed(s Sample, minSamples uint64) (Recommendation, error) {
	if minSamples == 0 {
		minSamples = 512
	}
	if s.Segments < minSamples {
		return Recommendation{}, fmt.Errorf("%w: have %d want %d", ErrInsufficientSamples, s.Segments, minSamples)
	}
	lo, hi := s.Wilson95()
	r := Recommendation{Estimate: s.LossRate(), WilsonLow95: lo, WilsonHigh95: hi, Samples: s.Segments}
	switch {
	case hi <= 0.005:
		r.Profile = ProfileOff
	case hi <= 0.02:
		r.Profile = Profile20x4
	case hi <= 0.05:
		r.Profile = Profile20x8
	case hi <= 0.10:
		r.Profile = Profile20x12
	case hi <= 0.15:
		r.Profile = Profile20x16
	default:
		r.Profile = Profile20x20
		r.Above20Pct = hi > 0.20
	}
	return r, nil
}

// ProbeDeficit returns how many additional statistically useful carrier
// segments would be needed to reach minSamples. The caller may first consume
// organic traffic; only the remainder, if any, needs a low-rate diagnostic
// datagram. This keeps the normal steady-state overhead exactly zero.
func ProbeDeficit(s Sample, minSamples uint64) uint64 {
	if s.Segments >= minSamples {
		return 0
	}
	return minSamples - s.Segments
}

// ProbeBitrate estimates the payload+outer bytes per second required to fill a
// sample deficit inside the remaining measurement window. It is an accounting
// helper, not a requirement to send probes.
func ProbeBitrate(packets uint64, approximateWireBytes int, window time.Duration) float64 {
	if packets == 0 || approximateWireBytes <= 0 || window <= 0 {
		return 0
	}
	return float64(packets*uint64(approximateWireBytes)*8) / window.Seconds()
}

type RateBudget struct {
	CapacityBps            float64
	TargetUtilization      float64
	ACKReserveFraction     float64
	PayloadBytes           int
	BaseOuterOverheadBytes int
	FECHeaderBytes         int
}

// MaxInnerBitrate computes a performance-first but queue-safe inner payload cap.
// The path is kept below TargetUtilization after accounting for fixed FEC,
// datagram/header expansion and shadow TCP retransmission. The retransmission
// multiplier is max(measured ARQ bytes, 1/(1-p95upper)) so a quiet measurement
// cannot accidentally promise more capacity than its loss confidence permits.
func MaxInnerBitrate(profile Profile, sample Sample, lossUpper95 float64, b RateBudget) (float64, error) {
	if b.CapacityBps <= 0 || b.TargetUtilization <= 0 || b.TargetUtilization >= 1 ||
		b.ACKReserveFraction < 0 || b.ACKReserveFraction >= 0.25 || b.PayloadBytes <= 0 ||
		b.BaseOuterOverheadBytes < 0 || b.FECHeaderBytes < 0 || lossUpper95 < 0 || lossUpper95 >= 1 {
		return 0, ErrBadBudget
	}
	fecFactor := profile.FECFactor()
	if math.IsInf(fecFactor, 1) {
		return 0, ErrBadBudget
	}
	header := b.BaseOuterOverheadBytes
	if profile.K != 0 {
		header += b.FECHeaderBytes
	}
	packetFactor := float64(b.PayloadBytes+header) / float64(b.PayloadBytes)
	arqFactor := sample.ARQFactor()
	confidenceARQ := 1 / (1 - lossUpper95)
	if confidenceARQ > arqFactor {
		arqFactor = confidenceARQ
	}
	usable := b.CapacityBps * b.TargetUtilization * (1 - b.ACKReserveFraction)
	return usable / (fecFactor * packetFactor * arqFactor), nil
}
