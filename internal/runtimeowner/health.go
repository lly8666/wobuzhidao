package runtimeowner

import (
	"errors"
	"github.com/lly8666/wobuzhidao/internal/acceptancefault"
	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"time"
)

type healthState struct {
	waitForPeer               bool
	interval                  time.Duration
	idleSource                func(time.Time) time.Duration
	started, nextSend, hintAt time.Time
	hintPN                    uint64
	haveHint                  bool
	idle                      time.Duration
}

func (t *laneTransport) observeHealthLocked(result datapath.InboundResult, now time.Time) {
	if result.Authenticated > 0 {
		t.stats.AuthenticatedRecords += result.Authenticated
		t.stats.LastAuthenticated = now
	}
	for _, h := range result.Health {
		if t.health.haveHint && h.PN <= t.health.hintPN {
			continue
		}
		t.health.haveHint = true
		t.health.hintPN = h.PN
		t.health.hintAt = now
		t.health.idle = h.IdleFor
		t.stats.HealthReceived++
	}
}

func (t *laneTransport) tickHealth(now time.Time) error { return t.sendHealth(now, false) }

func (t *laneTransport) sendHealth(now time.Time, force bool) (err error) {
	defer func() {
		if errors.Is(err, datapath.ErrLaneUnavailable) || errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) || errors.Is(err, ErrRuntimeClosed) || errors.Is(err, ErrTransportWriteClosed) || errors.Is(err, ErrTransportPeerReset) {
			err = nil
		}
	}()
	t.mu.Lock()
	interval := t.health.interval
	source := t.health.idleSource
	if interval <= 0 {
		t.mu.Unlock()
		return nil
	}
	if t.closed || t.localFINQueued || (t.health.waitForPeer && t.stats.AuthenticatedRecords == 0) {
		t.mu.Unlock()
		return nil
	}
	if t.health.started.IsZero() {
		t.health.started = now
	}
	if !force && now.Before(t.health.nextSend) {
		t.mu.Unlock()
		return nil
	}
	t.health.nextSend = now.Add(interval)
	t.mu.Unlock()
	idle := time.Duration(0)
	if source != nil {
		idle = source(now)
	}
	// The acceptance-only build tag can consume a scheduled health send before
	// sealing/FEC. Production builds compile this to a constant false no-op.
	if acceptancefault.Consume("health", t.ref.ID) {
		return nil
	}
	record, err := t.owner.HealthRecord(t.ref, idle)
	if err != nil {
		return err
	}
	if err = t.owner.ValidateGeneration(t.ref); err != nil {
		return err
	}
	if err = t.send([]datapath.WireRecord{record}, now); err != nil {
		return err
	}
	t.mu.Lock()
	t.stats.HealthSent++
	t.mu.Unlock()
	return nil
}

// PeerIdle requires a recent authenticated observation from every active lane.
// Missing keepalives are UNKNOWN, never evidence that the application is idle.
func (r *Runtime) PeerIdle(now time.Time, idle time.Duration) bool {
	r.mu.Lock()
	lanes := make([]*laneTransport, 0, len(r.active))
	for _, ref := range r.active {
		if t := r.lanes[ref]; t != nil {
			lanes = append(lanes, t)
		}
	}
	r.mu.Unlock()
	if len(lanes) == 0 {
		return false
	}
	for _, t := range lanes {
		t.mu.Lock()
		h := t.health
		ok := t.health.interval > 0 && h.haveHint && !now.Before(h.hintAt) && now.Sub(h.hintAt) <= 2*t.health.interval && h.idle >= idle
		t.mu.Unlock()
		if !ok {
			return false
		}
	}
	return true
}

// Unhealthy is diagnostic suspicion, not a terminal error and not DORMANT.
// Pure TCP ACKs cannot keep an encrypted lane alive.
func (r *Runtime) Unhealthy(ref logicaltunnel.LaneRef, now time.Time, timeout time.Duration) bool {
	if timeout <= 0 {
		return false
	}
	r.mu.Lock()
	t := r.lanes[ref]
	r.mu.Unlock()
	if t == nil {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return true
	}
	last := t.stats.LastAuthenticated
	if last.IsZero() {
		last = t.health.started
	}
	return !last.IsZero() && !now.Before(last) && now.Sub(last) >= timeout
}

func (r *Runtime) ConfigureHealth(ref logicaltunnel.LaneRef, interval time.Duration, source func(time.Time) time.Duration, waitForPeer ...bool) error {
	r.mu.Lock()
	t := r.lanes[ref]
	r.mu.Unlock()
	if t == nil {
		return ErrTransportMissing
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if len(waitForPeer) > 0 {
		t.health.waitForPeer = waitForPeer[0]
	}
	t.health.interval = interval
	t.health.idleSource = source
	return nil
}

// AdvertiseIdle sends one final authenticated idle hint before releasing lanes.
// It is bounded by the serialized idle transition, not a response/amplification loop.
func (r *Runtime) AdvertiseIdle(now time.Time) {
	r.mu.Lock()
	lanes := make([]*laneTransport, 0, len(r.active))
	for _, ref := range r.active {
		if t := r.lanes[ref]; t != nil {
			lanes = append(lanes, t)
		}
	}
	r.mu.Unlock()
	for _, t := range lanes {
		_ = t.sendHealth(now, true)
	}
}
