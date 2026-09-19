package session

import (
	"sync"
	"time"
)

// Clock is the only time dependency used by receive semantics. Production can
// use SystemClock while deterministic tests and future network simulations use
// ManualClock.
type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now() }

// ManualClock is a deterministic, goroutine-safe virtual clock.
type ManualClock struct {
	mu  sync.Mutex
	now time.Time
}

func NewManualClock(start time.Time) *ManualClock {
	return &ManualClock{now: start}
}

func (c *ManualClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *ManualClock) Advance(d time.Duration) {
	if d < 0 {
		panic("session.ManualClock: negative advance")
	}
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}
