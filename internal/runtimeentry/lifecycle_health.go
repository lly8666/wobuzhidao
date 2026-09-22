package runtimeentry

import (
	"context"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"time"
)

const (
	DefaultKeepaliveInterval = 15 * time.Second
	DefaultDeadAfter         = 90 * time.Second
	DefaultReconnectMin      = time.Second
	DefaultReconnectMax      = 30 * time.Second
)

type LifecycleStats struct {
	RecoveryAttempts  uint64
	RecoverySucceeded uint64
	RecoveryFailed    uint64
	RetryableErrors   uint64
	LastError         string
	NextRetry         time.Time
}

func normalizeClientHealth(c *TunnelClientConfig) error {
	if c.KeepaliveInterval == 0 {
		c.KeepaliveInterval = DefaultKeepaliveInterval
	}
	if c.DeadAfter == 0 {
		c.DeadAfter = DefaultDeadAfter
	}
	if c.ReconnectMin == 0 {
		c.ReconnectMin = DefaultReconnectMin
	}
	if c.ReconnectMax == 0 {
		c.ReconnectMax = DefaultReconnectMax
	}
	if c.KeepaliveInterval < time.Second || c.KeepaliveInterval > time.Hour || c.DeadAfter < c.KeepaliveInterval*3 || c.DeadAfter > 24*time.Hour || c.ReconnectMin < time.Second || c.ReconnectMax < c.ReconnectMin || c.ReconnectMax > 10*time.Minute {
		return ErrLifecycleLaneState
	}
	return nil
}

func idleDuration(now, last time.Time) time.Duration {
	if last.IsZero() || now.Before(last) {
		return 0
	}
	return now.Sub(last)
}

func (c *TunnelClient) noteBusiness(now time.Time) {
	c.mu.Lock()
	if now.After(c.lastPayload) {
		c.lastPayload = now
	}
	c.mu.Unlock()
}

func (c *TunnelClient) businessIdle(now time.Time) time.Duration {
	c.mu.Lock()
	last := c.lastPayload
	c.mu.Unlock()
	return idleDuration(now, last)
}

func (c *TunnelClient) LifecycleStats() LifecycleStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := c.lifecycleStats
	out.NextRetry = c.retryAt
	return out
}

func (c *TunnelClient) noteLifecycleError(err error) {
	if err == nil {
		return
	}
	message := err.Error()
	if len(message) > 512 {
		message = message[:512]
	}
	c.mu.Lock()
	c.lifecycleStats.RetryableErrors++
	c.lifecycleStats.LastError = message
	c.mu.Unlock()
}

func (c *TunnelClient) laneFailure(lane *clientLifecycleLane, err error) {
	c.mu.Lock()
	lane.failed = true
	c.mu.Unlock()
	c.noteLifecycleError(err)
}

// One transition per tunnel. Failure keeps the old incarnation authoritative;
// retries use fresh TLS admission and a capped jittered backoff, not a tight loop.
func (c *TunnelClient) maybeScheduleLifecycle(now time.Time) {
	c.mu.Lock()
	if c.closed || c.actionPending || c.dormant {
		c.mu.Unlock()
		return
	}
	last := c.lastPayload
	idle := c.cfg.DormantAfter > 0 && idleDuration(now, last) >= c.cfg.DormantAfter
	c.mu.Unlock()
	if idle && c.rt.PeerIdle(now, c.cfg.DormantAfter) {
		c.mu.Lock()
		if c.closed || c.actionPending || c.dormant || !c.lastPayload.Equal(last) {
			c.mu.Unlock()
			return
		}
		c.actionPending = true
		c.mu.Unlock()
		go func() {
			err := c.dormantIfIdle(last)
			c.mu.Lock()
			c.actionPending = false
			c.mu.Unlock()
			c.noteLifecycleError(err)
		}()
		return
	}
	c.mu.Lock()
	if c.closed || c.actionPending || c.dormant || len(c.retiring) != 0 || now.Before(c.retryAt) {
		c.mu.Unlock()
		return
	}
	var selected *clientLifecycleLane
	// Prefer unhealthy lanes, deterministic selection avoids starving a lane.
	for _, lane := range c.lanes {
		if lane.failed || c.rt.Unhealthy(lane.ref, now, c.cfg.DeadAfter) {
			if selected == nil || lane.id < selected.id {
				selected = lane
			}
		}
	}
	if selected == nil && c.cfg.RotateMin > 0 && !c.nextRotation.IsZero() && !now.Before(c.nextRotation) {
		for _, lane := range c.lanes {
			if selected == nil || lane.ref.Generation < selected.ref.Generation {
				selected = lane
			}
		}
	}
	if selected == nil {
		c.mu.Unlock()
		return
	}
	ref := selected.ref
	c.actionPending = true
	c.lifecycleStats.RecoveryAttempts++
	c.mu.Unlock()
	go c.replaceLane(ref)
}

func (c *TunnelClient) replaceLane(ref logicaltunnel.LaneRef) {
	timeout := c.cfg.Admission.TLS.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.runCtx, timeout)
	defer cancel()
	c.opMu.Lock()
	c.mu.Lock()
	lane := c.lanes[ref.ID]
	valid := !c.closed && !c.dormant && lane != nil && lane.ref == ref && len(c.retiring) == 0
	c.mu.Unlock()
	var err error
	if valid {
		_, err = c.connectLaneLocked(ctx, ref.ID, ref)
	}
	c.opMu.Unlock()
	now := time.Now()
	c.mu.Lock()
	c.actionPending = false
	if !valid {
		c.mu.Unlock()
		return
	}
	if err == nil {
		c.lifecycleStats.RecoverySucceeded++
		c.retryAt = time.Time{}
		c.retryDelay = 0
		c.scheduleNextRotationLocked(now)
	} else {
		c.lifecycleStats.RecoveryFailed++
		c.scheduleRetryLocked(now)
	}
	c.mu.Unlock()
	c.noteLifecycleError(err)
}

func (c *TunnelClient) scheduleRetryLocked(now time.Time) {
	if c.retryDelay == 0 {
		c.retryDelay = c.cfg.ReconnectMin
	} else {
		c.retryDelay *= 2
	}
	if c.retryDelay > c.cfg.ReconnectMax {
		c.retryDelay = c.cfg.ReconnectMax
	}
	// Equal jitter remains >= minimum; no retry storm on total blackhole.
	lower := c.retryDelay / 2
	if lower < c.cfg.ReconnectMin {
		lower = c.cfg.ReconnectMin
	}
	delay, e := randomDuration(lower, c.retryDelay)
	if e != nil {
		delay = c.retryDelay
	}
	c.retryAt = now.Add(delay)
}
