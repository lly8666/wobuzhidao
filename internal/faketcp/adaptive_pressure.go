package faketcp

import (
	"math"
	"time"
)

const (
	PartialReliabilityEmergencyLimit = MaxSteadyStateOutstandingDatagrams - 512
	pressureRateInterval             = 100 * time.Millisecond
	pressureHoleRTTs                 = 1
)

// This is receiver delivery-rate pressure, not a measurement of sender BDP:
// lost records cannot be counted here without changing the wire. Directional
// traffic stays separate; only the association's locally measured RTT is shared.
type repairPressure struct {
	epoch      time.Time
	count      uint64
	rate       float64
	rtt        time.Duration
	holeSince  time.Time
	holeSeq    uint32
	holeActive bool
}

func (p *repairPressure) observe(now time.Time, srtt time.Duration) {
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

func (p *repairPressure) softLimit() int {
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

// requiredHoleAge keeps a full RTT of repair opportunity when pressure first
// reaches the adaptive soft horizon, then continuously shortens that grace as
// out-of-order debt approaches the emergency horizon. The emergency limit is
// therefore a safety fuse instead of the normal steady-state controller.
func (p *repairPressure) requiredHoleAge(n int) time.Duration {
	if p.rtt <= 0 {
		return 0
	}
	soft := p.softLimit()
	if soft >= PartialReliabilityEmergencyLimit || n <= soft {
		return time.Duration(pressureHoleRTTs) * p.rtt
	}
	if n >= PartialReliabilityEmergencyLimit {
		return 0
	}
	span := PartialReliabilityEmergencyLimit - soft
	remaining := PartialReliabilityEmergencyLimit - n
	fraction := float64(remaining) / float64(span)
	return time.Duration(float64(time.Duration(pressureHoleRTTs)*p.rtt) * fraction)
}

func (r *Receiver) updatePressureHole(now time.Time) {
	p := &r.pressure
	if len(r.outOfOrder) == 0 {
		p.holeActive = false
		p.holeSince = time.Time{}
		return
	}
	if !p.holeActive || p.holeSeq != r.next {
		p.holeActive, p.holeSeq, p.holeSince = true, r.next, now
	}
}

func (r *Receiver) pressureAllowsForgiveness(now time.Time) bool {
	n := len(r.outOfOrder)
	if n >= PartialReliabilityEmergencyLimit {
		return true
	}
	p := &r.pressure
	soft := p.softLimit()
	if n < soft || p.rtt <= 0 || !p.holeActive {
		return false
	}
	return now.Sub(p.holeSince) >= p.requiredHoleAge(n)
}
