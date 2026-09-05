package windowsruntime

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sort"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

const (
	minPayloadIdlePollInterval = 10 * time.Millisecond
	maxPayloadIdlePollInterval = 250 * time.Millisecond
	maxIdleTimeoutSeconds      = int64((1<<63 - 1) / 1_000_000_000)

	minLaneAge        = time.Duration(DefaultLaneRotationMinSeconds) * time.Second
	maxLaneAge        = time.Duration(DefaultLaneRotationMaxSeconds) * time.Second
	minLaneAgeStagger = time.Minute
	laneAgeRetryDelay = time.Minute
)

func validateIdleTimeoutSeconds(seconds int) error {
	if seconds < 0 { return errors.New("idle timeout must be zero (disabled) or a positive number of seconds") }
	if int64(seconds) > maxIdleTimeoutSeconds { return errors.New("idle timeout is too large") }
	return nil
}

func payloadIdlePollInterval(timeout time.Duration) time.Duration {
	interval := timeout / 4
	if interval < minPayloadIdlePollInterval { return minPayloadIdlePollInterval }
	if interval > maxPayloadIdlePollInterval { return maxPayloadIdlePollInterval }
	return interval
}

func lifecycleMonitorPollInterval(timeout time.Duration) time.Duration {
	if timeout <= 0 { return maxPayloadIdlePollInterval }
	return payloadIdlePollInterval(timeout)
}

type payloadIdleObservation struct {
	sequence     uint64
	haveSequence bool
	lastPayload  time.Time
}

func newPayloadIdleObservation(now time.Time) payloadIdleObservation { return payloadIdleObservation{lastPayload: now} }

func (o *payloadIdleObservation) observe(activity gamelane.PayloadActivity, now time.Time) (changed, advanced bool) {
	if !o.haveSequence {
		o.haveSequence = true; o.sequence = activity.Sequence; o.lastPayload = now
		return true, false
	}
	if activity.Sequence == o.sequence { return false, false }
	advanced = activity.Sequence > o.sequence
	o.sequence = activity.Sequence; o.lastPayload = now
	return true, advanced
}
func (o *payloadIdleObservation) postpone(now time.Time) { o.lastPayload = now }
func (o payloadIdleObservation) expired(now time.Time, timeout time.Duration) bool { return timeout > 0 && !now.Before(o.lastPayload.Add(timeout)) }

type laneAgeSampler func(time.Duration) time.Duration

type laneAgeState struct { deadlines map[int]time.Time; slots map[int]int }
func newLaneAgeState() *laneAgeState { return &laneAgeState{deadlines: map[int]time.Time{}, slots: map[int]int{}} }
func (s *laneAgeState) clear() { clear(s.deadlines); clear(s.slots) }

func randomLaneAgeOffset(span time.Duration) time.Duration {
	if span <= 0 { return 0 }
	n, err := rand.Int(rand.Reader, big.NewInt(int64(span)+1))
	if err != nil { return span / 2 }
	return time.Duration(n.Int64())
}

func laneAgeDeadlineSeparated(candidate time.Time, deadlines map[int]time.Time) bool {
	for _, existing := range deadlines {
		delta := candidate.Sub(existing); if delta < 0 { delta = -delta }
		if delta < minLaneAgeStagger { return false }
	}
	return true
}

func chooseLaneAgeDeadline(now time.Time, deadlines map[int]time.Time, sample laneAgeSampler) time.Time {
	return chooseLaneAgeDeadlineWithin(now, deadlines, sample, minLaneAge, maxLaneAge)
}

func chooseLaneAgeDeadlineWithin(now time.Time, deadlines map[int]time.Time, sample laneAgeSampler, minAge, maxAge time.Duration) time.Time {
	if minAge <= 0 || maxAge < minAge { minAge, maxAge = minLaneAge, maxLaneAge }
	span := maxAge - minAge
	for attempt := 0; attempt < 32; attempt++ {
		offset := sample(span)
		if offset < 0 { offset = 0 }
		if offset > span { offset = span }
		candidate := now.Add(minAge + offset)
		if laneAgeDeadlineSeparated(candidate, deadlines) { return candidate }
	}
	for age := minAge; age <= maxAge; age += minLaneAgeStagger {
		candidate := now.Add(age)
		if laneAgeDeadlineSeparated(candidate, deadlines) { return candidate }
		if maxAge-age < minLaneAgeStagger { break }
	}
	return now.Add(maxAge)
}

func normalizedLaneSlot(plan LanePlan) int { if plan.Slot != 0 { return plan.Slot }; return plan.ID }

func (s *laneAgeState) reconcile(plans map[int]LanePlan, now time.Time, sample laneAgeSampler) {
	s.reconcileWithin(plans, now, sample, minLaneAge, maxLaneAge)
}

func (s *laneAgeState) reconcileWithin(plans map[int]LanePlan, now time.Time, sample laneAgeSampler, minAge, maxAge time.Duration) {
	active := make(map[int]bool, len(plans)); ids := make([]int, 0, len(plans))
	for id := range plans { active[id] = true; ids = append(ids, id) }
	sort.Ints(ids)
	for id := range s.deadlines { if !active[id] { delete(s.deadlines, id); delete(s.slots, id) } }
	for _, id := range ids {
		slot := normalizedLaneSlot(plans[id]); deadline, exists := s.deadlines[id]
		if exists && s.slots[id] == slot && !deadline.IsZero() { continue }
		delete(s.deadlines, id)
		s.deadlines[id] = chooseLaneAgeDeadlineWithin(now, s.deadlines, sample, minAge, maxAge)
		s.slots[id] = slot
	}
}

func (s *laneAgeState) nextDue(now time.Time) (int, bool) {
	laneID := 0; var earliest time.Time
	for id, deadline := range s.deadlines {
		if deadline.After(now) { continue }
		if laneID == 0 || deadline.Before(earliest) || (deadline.Equal(earliest) && id < laneID) { laneID = id; earliest = deadline }
	}
	return laneID, laneID != 0
}

func (c *Controller) startPayloadIdleMonitor(timeout time.Duration) {
	c.mu.Lock(); c.stopPayloadIdleMonitorLocked()
	if c.state != RuntimeConnected { c.mu.Unlock(); return }
	c.idleGeneration++; generation := c.idleGeneration; stop := make(chan struct{}); c.idleStop = stop
	c.mu.Unlock(); go c.runPayloadIdleMonitor(generation, stop, timeout)
}

func (c *Controller) stopPayloadIdleMonitorLocked() { if c.idleStop != nil { close(c.idleStop); c.idleStop = nil } }

func (c *Controller) payloadIdleMonitorCurrent(generation uint64, stop chan struct{}) bool {
	select { case <-stop: return false; default: }
	c.mu.Lock(); defer c.mu.Unlock(); return c.idleGeneration == generation && c.idleStop == stop
}

func (c *Controller) laneAgePlans() (RuntimeState, map[int]LanePlan) { c.mu.Lock(); defer c.mu.Unlock(); return c.state, cloneLanePlans(c.lanePlans) }
func (c *Controller) currentLaneRotationBounds() (time.Duration, time.Duration) { c.mu.Lock(); defer c.mu.Unlock(); return laneRotationBounds(c.profile) }

func (c *Controller) runLaneAgeTick(ages *laneAgeState, now time.Time) {
	state, plans := c.laneAgePlans()
	if state == RuntimeDormant { ages.clear(); return }
	if state != RuntimeConnected { return }
	minAge, maxAge := c.currentLaneRotationBounds()
	ages.reconcileWithin(plans, now, randomLaneAgeOffset, minAge, maxAge)
	laneID, ok := ages.nextDue(now); if !ok { return }
	ages.deadlines[laneID] = now.Add(laneAgeRetryDelay)
	_ = c.ReplaceLane(laneID)
}

func (c *Controller) runPayloadIdleMonitor(generation uint64, stop chan struct{}, timeout time.Duration) {
	observation := newPayloadIdleObservation(time.Now()); ages := newLaneAgeState(); exitRetries := newLaneExitRetryState()
	ticker := time.NewTicker(lifecycleMonitorPollInterval(timeout)); defer ticker.Stop()
	for {
		select { case <-stop: return; case <-ticker.C: }
		if !c.payloadIdleMonitorCurrent(generation, stop) { return }
		state := c.State()
		if state == RuntimeDisconnected { return }
		if state != RuntimeConnected && state != RuntimeDormant { continue }
		if state == RuntimeConnected && c.runAuthoritativeLaneExitTick(exitRetries, time.Now()) { continue }
		if timeout <= 0 {
			if state == RuntimeDormant { ages.clear(); exitRetries.clear(); continue }
			c.runLaneAgeTick(ages, time.Now()); continue
		}
		activity, activityErr := c.PayloadActivity(); now := time.Now(); changed, advanced := false, false
		if activityErr == nil { changed, advanced = observation.observe(activity, now) } else if state == RuntimeConnected && timeout > 0 { observation.postpone(now) }
		if state == RuntimeDormant {
			ages.clear(); exitRetries.clear()
			if activityErr == nil && advanced && c.payloadIdleMonitorCurrent(generation, stop) { if err := c.Wake(); err == nil { observation.postpone(time.Now()) } }
			continue
		}
		if timeout > 0 && activityErr == nil && !changed && observation.expired(now, timeout) {
			confirmed, err := c.PayloadActivity(); confirmNow := time.Now()
			if err != nil { observation.postpone(confirmNow) } else if changed, _ := observation.observe(confirmed, confirmNow); !changed && c.State() == RuntimeConnected && c.payloadIdleMonitorCurrent(generation, stop) {
				if err := c.Dormant(); err != nil { observation.postpone(time.Now()) } else {
					ages.clear(); exitRetries.clear()
					if !c.payloadIdleMonitorCurrent(generation, stop) { return }
					after, err := c.PayloadActivity()
					if err == nil {
						_, advanced = observation.observe(after, time.Now())
						if advanced && c.State() == RuntimeDormant && c.payloadIdleMonitorCurrent(generation, stop) { if err := c.Wake(); err == nil { observation.postpone(time.Now()) } }
					}
				}
			}
		}
		if c.State() != RuntimeConnected || !c.payloadIdleMonitorCurrent(generation, stop) { continue }
		c.runLaneAgeTick(ages, time.Now())
	}
}
