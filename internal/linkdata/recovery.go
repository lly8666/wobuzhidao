package linkdata

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

// candidateFECRecoveryHorizon is the first bounded-recovery candidate from
// PRE_RELEASE.md. It is intentionally a fixed experiment point inside the
// documented 1-3 second exploration range, not a claimed final production
// default. A later change can derive this from smoothed RTT without changing
// the absolute-deadline semantics below.
const candidateFECRecoveryHorizon = 2 * time.Second

type FECRecoveryStats struct {
	HorizonMillis         int64  `json:"horizon_millis"`
	PendingDeadlines      int    `json:"pending_deadlines"`
	ExpireEvents          uint64 `json:"expire_events"`
	ExpiredIncomplete     uint64 `json:"expired_incomplete"`
	ExpiredMissingSources uint64 `json:"expired_missing_sources"`
}

type fecRecoveryDeadline struct {
	blockID  uint32
	deadline time.Time
}

type fecRecoveryTracker struct {
	horizon time.Duration
	seen    map[uint32]time.Time
	queue   []fecRecoveryDeadline
	head    int

	expireEvents          uint64
	expiredIncomplete     uint64
	expiredMissingSources uint64
}

func newFECRecoveryTracker(horizon time.Duration) *fecRecoveryTracker {
	return &fecRecoveryTracker{horizon: horizon, seen: make(map[uint32]time.Time)}
}

func (r *fecRecoveryTracker) observeHeavy(id uint32, now time.Time) {
	if r == nil {
		return
	}
	if _, ok := r.seen[id]; ok {
		return
	}
	deadline := now.Add(r.horizon)
	r.seen[id] = deadline
	r.queue = append(r.queue, fecRecoveryDeadline{blockID: id, deadline: deadline})
}

func (r *fecRecoveryTracker) forget(id uint32) {
	if r == nil {
		return
	}
	delete(r.seen, id)
}

func (r *fecRecoveryTracker) expire(dec *fec.BlockDecoder, now time.Time) {
	if r == nil || dec == nil {
		return
	}
	for r.head < len(r.queue) {
		e := r.queue[r.head]
		if e.deadline.After(now) {
			break
		}
		r.head++
		deadline, ok := r.seen[e.blockID]
		if !ok || !deadline.Equal(e.deadline) {
			continue
		}
		delete(r.seen, e.blockID)
		result := dec.RetireRecoveryExpired(e.blockID)
		if !result.Retired {
			continue
		}
		r.expireEvents++
		if result.MissingSources > 0 {
			r.expiredIncomplete++
			r.expiredMissingSources += uint64(result.MissingSources)
		}
	}
	if r.head >= 1024 && r.head*2 >= len(r.queue) {
		copy(r.queue, r.queue[r.head:])
		r.queue = r.queue[:len(r.queue)-r.head]
		r.head = 0
	}
}

func (r *fecRecoveryTracker) stats() FECRecoveryStats {
	if r == nil {
		return FECRecoveryStats{}
	}
	return FECRecoveryStats{
		HorizonMillis:         r.horizon.Milliseconds(),
		PendingDeadlines:      len(r.seen),
		ExpireEvents:          r.expireEvents,
		ExpiredIncomplete:     r.expiredIncomplete,
		ExpiredMissingSources: r.expiredMissingSources,
	}
}

func (p *Path) expireFECRecovery(now time.Time) {
	if p == nil || p.recovery == nil || p.dec == nil {
		return
	}
	p.recovery.expire(p.dec, now)
}

func (p *Path) FECRecoveryStats() FECRecoveryStats {
	if p == nil {
		return FECRecoveryStats{}
	}
	return p.recovery.stats()
}
