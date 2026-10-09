package runtimeowner

import (
	"context"
	"errors"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// RunFECDeadlineSchedule owns ONE reusable timer per logical tunnel across
// all active lanes and replacements. No packet creates a timer or goroutine.
// The actual lifecycle alone opts in; Runtime.New remains deterministic for
// tests whose packet timestamps use synthetic clocks.
func (r *Runtime) RunFECDeadlineSchedule(ctx context.Context) {
	if r == nil || ctx == nil {
		return
	}
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	defer timer.Stop()
	for {
		next := r.owner.NextActiveFlushDeadline()
		var clock <-chan time.Time
		if !next.IsZero() {
			delay := time.Until(next)
			if delay < 0 {
				delay = 0
			}
			timer.Reset(delay)
			clock = timer.C
		}
		select {
		case <-ctx.Done():
			return
		case <-r.deadlineStop:
			return
		case <-r.owner.FECDeadlineWake():
			if clock != nil {
				timer.Stop()
			}
			// An earlier size group became ready; re-evaluate the earliest.
		case <-clock:
			if err := r.flushDuePartialParity(time.Now()); err != nil {
				r.mu.Lock()
				if !r.closed {
					r.parityDeadlineErr = errors.Join(r.parityDeadlineErr, err)
				}
				r.mu.Unlock()
			}
		}
	}
}

// flushDuePartialParity does NOT run health/ACK/recovery or platform service
// maintenance: their bounded 100ms tick remains unchanged. Parity sealing and
// emitting happen on the separate owner worker, never the raw receive loop.
// Promotion's outboundMu continues to fence old generations.
func (r *Runtime) flushDuePartialParity(now time.Time) error {
	if r == nil {
		return nil
	}
	r.outboundMu.RLock()
	defer r.outboundMu.RUnlock()
	if due := r.owner.NextActiveFlushDeadline(); due.IsZero() || due.After(now) {
		return nil
	}
	type selected struct {
		ref logicaltunnel.LaneRef
		transport *laneTransport
	}
	var lanes [logicaltunnel.MaxProductPublicTransportLanes]selected
	count := 0
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	for _, ref := range r.active {
		if count >= len(lanes) {
			break
		}
		if transport := r.lanes[ref]; transport != nil {
			lanes[count] = selected{ref, transport}
			count++
		}
	}
	r.mu.Unlock()

	var errs []error
	for _, lane := range lanes[:count] {
		records, err := r.owner.TickLane(lane.ref, now)
		if err != nil {
			if !errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) &&
				!errors.Is(err, datapath.ErrLaneUnavailable) {
				errs = append(errs, err)
			}
			continue
		}
		if len(records) != 0 {
			if err = lane.transport.send(records, now); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}
