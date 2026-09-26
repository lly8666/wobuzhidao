package linkadapt

import (
	"errors"
	"time"
)

var (
	ErrRefreshNotDue = errors.New("linkadapt: refresh not due")
	ErrSampleTooBusy = errors.New("linkadapt: measurement window not low load")
)

type RefreshPolicy struct {
	Interval        time.Duration
	Window          time.Duration
	MaxLoadFraction float64
	MinSamples      uint64
	CapacityBps     float64
}

func DefaultRefreshPolicy(capacityBps float64) RefreshPolicy {
	return RefreshPolicy{
		Interval: time.Hour, Window: 20 * time.Second,
		MaxLoadFraction: 0.05, MinSamples: 1024, CapacityBps: capacityBps,
	}
}

func (p RefreshPolicy) Validate() error {
	if p.Interval < 30*time.Minute || p.Window <= 0 || p.Window > time.Minute ||
		p.MaxLoadFraction <= 0 || p.MaxLoadFraction >= 0.25 || p.MinSamples == 0 || p.CapacityBps <= 0 {
		return ErrBadBudget
	}
	return nil
}

type RefreshState struct {
	Policy      RefreshPolicy
	LastAttempt time.Time
	LastGood    time.Time
	Current     Profile
	Recommended Profile
	LastSample  Sample
	LastResult  Recommendation
}

func NewRefreshState(policy RefreshPolicy, current Profile) (*RefreshState, error) {
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	return &RefreshState{Policy: policy, Current: current, Recommended: current}, nil
}

func (s *RefreshState) Due(now time.Time) bool {
	if s == nil || now.IsZero() {
		return false
	}
	return s.LastAttempt.IsZero() || !now.Before(s.LastAttempt.Add(s.Policy.Interval))
}

// Evaluate consumes an already completed local sender-counter window. It does
// no network I/O. The caller is responsible for waiting Policy.Window between
// the before/after snapshots and may optionally fill an organic-sample deficit
// with diagnostic datagrams before calling Evaluate.
func (s *RefreshState) Evaluate(before, after Snapshot) (Recommendation, error) {
	if s == nil || !s.Due(after.At) {
		return Recommendation{}, ErrRefreshNotDue
	}
	s.LastAttempt = after.At
	sample, err := Between(before, after)
	if err != nil {
		return Recommendation{}, err
	}
	if sample.Duration < s.Policy.Window {
		return Recommendation{}, ErrBadWindow
	}
	if !sample.LowLoad(s.Policy.CapacityBps, s.Policy.MaxLoadFraction) {
		return Recommendation{}, ErrSampleTooBusy
	}
	rec, err := RecommendFixed(sample, s.Policy.MinSamples)
	if err != nil {
		return Recommendation{}, err
	}
	s.LastGood = after.At
	s.LastSample = sample
	s.LastResult = rec
	s.Recommended = rec.Profile
	return rec, nil
}

func (s *RefreshState) RotationNeeded() bool {
	if s == nil {
		return false
	}
	return s.Recommended != s.Current
}

// CommitRotation is called only after a fresh association using Recommended
// reaches Established and packet ownership has switched. A failed new
// association therefore never mutates the current profile.
func (s *RefreshState) CommitRotation() {
	if s != nil {
		s.Current = s.Recommended
	}
}
