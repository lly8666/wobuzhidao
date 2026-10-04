package gamelane

import (
	"errors"
	"math"
	"sync"
	"time"
)

type InnerPacer struct {
	mu   sync.Mutex
	bps  float64
	next time.Time
}

func NewInnerPacer(mbps float64) (*InnerPacer, error) {
	if mbps < 0 || math.IsNaN(mbps) || math.IsInf(mbps, 0) {
		return nil, errors.New("gamelane: inner rate must be finite and non-negative")
	}
	return &InnerPacer{bps: mbps * 1e6}, nil
}

func (p *InnerPacer) Mbps() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.bps / 1e6
}

func (p *InnerPacer) SetMbps(mbps float64, now time.Time) error {
	if mbps < 0 || math.IsNaN(mbps) || math.IsInf(mbps, 0) {
		return errors.New("gamelane: inner rate must be finite and non-negative")
	}
	p.mu.Lock()
	p.bps = mbps * 1e6
	if now.IsZero() || p.next.Before(now) {
		p.next = now
	}
	p.mu.Unlock()
	return nil
}

func (p *InnerPacer) Reserve(bytes int, now time.Time) time.Duration {
	if bytes <= 0 || now.IsZero() {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bps <= 0 {
		p.next = now
		return 0
	}
	start := p.next
	if start.IsZero() || start.Before(now) {
		start = now
	}
	wait := start.Sub(now)
	if wait < 0 {
		wait = 0
	}
	seconds := float64(bytes*8) / p.bps
	serialization := time.Duration(math.Ceil(seconds * float64(time.Second)))
	if serialization < time.Nanosecond {
		serialization = time.Nanosecond
	}
	p.next = start.Add(serialization)
	return wait
}
