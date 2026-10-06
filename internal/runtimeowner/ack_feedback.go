package runtimeowner

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// A wake token and one latest-state bit replace ACK packet history. All state
// below is guarded by the transport mutex; a worker belongs to this exact ref.
type ackFeedbackWorker struct {
	notify  chan struct{}
	stop    chan struct{}
	done    chan struct{}
	stopped bool
	failed  error
}

func (t *laneTransport) emitACKFeedback(seg faketcp.Segment) error {
	if !t.cfg.AsyncACKFeedback {
		return t.cfg.Emit(seg)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	if t.ackWorker != nil && t.ackWorker.failed != nil {
		return t.ackWorker.failed
	}
	if t.ackWorker == nil {
		t.ackWorker = &ackFeedbackWorker{notify: make(chan struct{}, 1), stop: make(chan struct{}), done: make(chan struct{})}
		go t.runACKFeedback(t.ackWorker)
	}
	t.stats.ACKWorkerQueued++
	if t.ackWorkerPending {
		t.stats.ACKWorkerCoalesced++
	}
	t.ackWorkerPending = true
	select {
	case t.ackWorker.notify <- struct{}{}:
	default:
	}
	return nil
}

func (t *laneTransport) runACKFeedback(w *ackFeedbackWorker) {
	defer close(w.done)
	for {
		select {
		case <-w.stop:
			return
		case <-w.notify:
		}
		t.mu.Lock()
		if t.closed || w.stopped {
			t.mu.Unlock()
			return
		}
		if !t.ackWorkerPending {
			t.mu.Unlock()
			continue
		}
		t.ackWorkerPending = false
		t.ackWorkerRunning = true
		// Read the current sequence and SACK scoreboard at selection, never
		// replay a cached ACK after later receive progress or a replacement.
		seg := t.outboundSegment(t.sendNext, t.recvNext, nil)
		t.stats.ACKWorkerAttempts++
		t.mu.Unlock()
		observe := t.cfg.ObserveFeedbackTiming && t.timing.enabled.Load()
		started := time.Time{}
		if observe {
			started = time.Now()
		}
		err := t.cfg.Emit(seg)
		if observe {
			t.timing.ackWorkerEmit.observe(time.Since(started))
		}
		t.mu.Lock()
		t.ackWorkerRunning = false
		if err != nil {
			t.stats.ACKWorkerFailures++
			w.failed = err
			t.ackWorkerPending = false
			// Closed generations cannot publish a failure into a new lane.
			// Active errors use the same fatal tick exit as existing ACK timers.
			if !t.closed && t.ackAsyncError == nil {
				t.ackAsyncError = err
			}
			t.mu.Unlock()
			return
		}
		t.stats.ACKWorkerSent++
		t.mu.Unlock()
	}
}

func (t *laneTransport) stopACKFeedbackLocked() {
	t.ackWorkerPending = false
	if w := t.ackWorker; w != nil && !w.stopped {
		w.stopped = true
		close(w.stop)
	}
	// Do not wait here: native Close owns the IO shutdown, and waiting while
	// holding transport/owner locks would deadlock a returning emit callback.
}
