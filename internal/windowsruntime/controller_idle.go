package windowsruntime

import (
	"errors"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

const (
	minPayloadIdlePollInterval = 10 * time.Millisecond
	maxPayloadIdlePollInterval = 250 * time.Millisecond
	maxIdleTimeoutSeconds      = int64((1<<63 - 1) / 1_000_000_000)
)

func validateIdleTimeoutSeconds(seconds int) error {
	if seconds < 0 {
		return errors.New("idle timeout must be zero (disabled) or a positive number of seconds")
	}
	if int64(seconds) > maxIdleTimeoutSeconds {
		return errors.New("idle timeout is too large")
	}
	return nil
}

func payloadIdlePollInterval(timeout time.Duration) time.Duration {
	interval := timeout / 4
	if interval < minPayloadIdlePollInterval {
		return minPayloadIdlePollInterval
	}
	if interval > maxPayloadIdlePollInterval {
		return maxPayloadIdlePollInterval
	}
	return interval
}

type payloadIdleObservation struct {
	sequence     uint64
	haveSequence bool
	lastPayload  time.Time
}

func newPayloadIdleObservation(now time.Time) payloadIdleObservation {
	return payloadIdleObservation{lastPayload: now}
}

// observe intentionally keys policy to the Game-owned monotonic sequence. The
// child's wall-clock timestamp remains diagnostic only; the controller records
// its own monotonic time when it observes a sequence change.
func (o *payloadIdleObservation) observe(activity gamelane.PayloadActivity, now time.Time) (changed, advanced bool) {
	if !o.haveSequence {
		o.haveSequence = true
		o.sequence = activity.Sequence
		o.lastPayload = now
		return true, false
	}
	if activity.Sequence == o.sequence {
		return false, false
	}
	advanced = activity.Sequence > o.sequence
	o.sequence = activity.Sequence
	o.lastPayload = now
	return true, advanced
}

func (o *payloadIdleObservation) postpone(now time.Time) { o.lastPayload = now }

func (o payloadIdleObservation) expired(now time.Time, timeout time.Duration) bool {
	return timeout > 0 && !now.Before(o.lastPayload.Add(timeout))
}

func (c *Controller) startPayloadIdleMonitor(timeout time.Duration) {
	if timeout <= 0 {
		return
	}
	c.mu.Lock()
	c.stopPayloadIdleMonitorLocked()
	if c.state != RuntimeConnected {
		c.mu.Unlock()
		return
	}
	c.idleGeneration++
	generation := c.idleGeneration
	stop := make(chan struct{})
	c.idleStop = stop
	c.mu.Unlock()
	go c.runPayloadIdleMonitor(generation, stop, timeout)
}

// stopPayloadIdleMonitorLocked must be called with c.mu held.
func (c *Controller) stopPayloadIdleMonitorLocked() {
	if c.idleStop != nil {
		close(c.idleStop)
		c.idleStop = nil
	}
}

func (c *Controller) payloadIdleMonitorCurrent(generation uint64, stop chan struct{}) bool {
	select {
	case <-stop:
		return false
	default:
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.idleGeneration == generation && c.idleStop == stop
}

func (c *Controller) runPayloadIdleMonitor(generation uint64, stop chan struct{}, timeout time.Duration) {
	observation := newPayloadIdleObservation(time.Now())
	ticker := time.NewTicker(payloadIdlePollInterval(timeout))
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		if !c.payloadIdleMonitorCurrent(generation, stop) {
			return
		}

		state := c.State()
		if state == RuntimeDisconnected {
			return
		}
		if state != RuntimeConnected && state != RuntimeDormant {
			continue
		}

		activity, err := c.PayloadActivity()
		now := time.Now()
		if err != nil {
			// Fail open while connected. If Game activity cannot be observed we do
			// not infer idleness and tear down healthy public lanes.
			if state == RuntimeConnected {
				observation.postpone(now)
			}
			continue
		}
		changed, advanced := observation.observe(activity, now)

		if state == RuntimeDormant {
			if advanced && c.payloadIdleMonitorCurrent(generation, stop) {
				if err := c.Wake(); err == nil {
					observation.postpone(time.Now())
				}
			}
			continue
		}
		if changed || !observation.expired(now, timeout) {
			continue
		}

		// Re-read immediately before the lifecycle transition. A payload that
		// races the idle edge refreshes the local monotonic deadline instead of
		// being mistaken for idle transport liveness.
		confirmed, err := c.PayloadActivity()
		confirmNow := time.Now()
		if err != nil {
			observation.postpone(confirmNow)
			continue
		}
		if changed, _ := observation.observe(confirmed, confirmNow); changed {
			continue
		}
		if c.State() != RuntimeConnected || !c.payloadIdleMonitorCurrent(generation, stop) {
			continue
		}
		if err := c.Dormant(); err != nil {
			observation.postpone(time.Now())
			continue
		}

		// Close the remaining query->barrier race: if a real payload advanced
		// after the confirmation but before Game's empty-lane barrier landed, wake
		// immediately. The racing packet itself may be dropped; subsequent payload
		// resumes once Wake publishes the first READY lane.
		if !c.payloadIdleMonitorCurrent(generation, stop) {
			return
		}
		after, err := c.PayloadActivity()
		if err != nil {
			continue
		}
		_, advanced = observation.observe(after, time.Now())
		if advanced && c.State() == RuntimeDormant && c.payloadIdleMonitorCurrent(generation, stop) {
			if err := c.Wake(); err == nil {
				observation.postpone(time.Now())
			}
		}
	}
}
