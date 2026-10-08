package platformflow

import (
	"sync"
	"time"
)

// boundedTickWork runs at most one maintenance pass. It never queues future
// passes: when the worker is busy the next periodic tick may retry using
// its newer time. This is not a business-packet queue or a timer.
type boundedTickWork struct {
	mu sync.Mutex
	busy bool
	closed bool
	wg sync.WaitGroup
}

func (w *boundedTickWork) tryRun(now time.Time, work func(time.Time)) bool {
	w.mu.Lock()
	if w.closed || w.busy {
		w.mu.Unlock()
		return false
	}
	w.busy = true
	w.wg.Add(1)
	w.mu.Unlock()
	go func() {
		defer w.wg.Done()
		defer func() {
			w.mu.Lock()
			w.busy = false
			w.mu.Unlock()
		}()
		work(now)
	}()
	return true
}

// close waits for the one in-flight pass before the owning tunnel/flows may
// be retired. Closing also forbids new submissions before Wait is invoked.
func (w *boundedTickWork) close() {
	w.mu.Lock()
	w.closed = true
	w.mu.Unlock()
	w.wg.Wait()
}
