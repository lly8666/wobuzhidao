package runtimeowner

import (
	"math"
	"time"
)

const (
	PartialReliabilityEmergencyLimit = MaxOutstandingRecords - 512
	pressureRateInterval             = 100 * time.Millisecond
	pressureHoleRTTs                 = 1
)

// This is receiver delivery-rate pressure, not a measurement of sender BDP:
// lost records cannot be counted here without changing the wire. Directional
// traffic stays separate; only the association's locally measured RTT is shared.
type receivePressure struct {
	epoch      time.Time
	count      uint64
	rate       float64
	rtt        time.Duration
	holeSince  time.Time
	holeSeq    uint32
	holeActive bool
}

func (p *receivePressure) observe(now time.Time, srtt time.Duration) {
	if srtt > 0 {
		p.rtt = srtt
	}
	if p.epoch.IsZero() {
		p.epoch, p.count = now, 1
		return
	}
	elapsed := now.Sub(p.epoch)
	if elapsed >= pressureRateInterval {
		sample := float64(p.count) / elapsed.Seconds()
		if p.rate == 0 {
			p.rate = sample
		} else {
			p.rate += (sample - p.rate) / 8
		}
		p.epoch, p.count = now, 1
	} else {
		p.count++
	}
}

func (p *receivePressure) softLimit() int {
	// Cold start or a unidirectional association without an RTT witness gets
	// the conservative emergency horizon, never an invented tiny BDP.
	if p.rtt <= 0 || p.rate <= 0 {
		return PartialReliabilityEmergencyLimit
	}
	bdp := p.rate * p.rtt.Seconds()
	soft := bdp + math.Max(128, bdp/10)
	if soft >= PartialReliabilityEmergencyLimit {
		return PartialReliabilityEmergencyLimit
	}
	return int(math.Ceil(soft))
}

func (r *laneTransport) updatePressureHole(now time.Time) {
	p := &r.pressure
	if len(r.received) == 0 {
		p.holeActive = false
		p.holeSince = time.Time{}
		return
	}
	if !p.holeActive || p.holeSeq != r.recvNext {
		p.holeActive, p.holeSeq, p.holeSince = true, r.recvNext, now
	}
}

func (r *laneTransport) pressureAllowsForgiveness(now time.Time) bool {
	n := len(r.received)
	if n >= PartialReliabilityEmergencyLimit {
		return true
	}
	p := &r.pressure
	return n >= p.softLimit() && p.rtt > 0 && p.holeActive &&
		now.Sub(p.holeSince) >= pressureHoleRTTs*p.rtt
}

// Adapted from old/internal/faketcp/adaptive_pressure.go. Delivery is still
// immediate; this releases TCP presentation metadata, never holds payloads.
func (r *laneTransport) observeReceivePressureLocked(now time.Time) {
	r.pressure.observe(now, r.srtt)
	r.updatePressureHole(now)
	if r.pressureAllowsForgiveness(now) && r.forgiveGapLocked(now, true) {
		r.stats.PressureForgiven++
		r.updatePressureHole(now)
	}
}
