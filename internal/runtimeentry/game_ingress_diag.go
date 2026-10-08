package runtimeentry

import (
	"sync/atomic"
	"time"
)

// Per-shard diagnostics are opt-in: neither routing nor queue admission checks
// them in a normal (profile-off) run. These counters are scheduling evidence,
// not permission to bypass authenticated Game PacketID deduplication.
type GameIngressShardDiagnostic struct {
	Index         int                `json:"index"`
	Capacity      int                `json:"capacity"`
	Enqueued      uint64             `json:"enqueued"`
	Handled       uint64             `json:"handled"`
	OldestDropped uint64             `json:"oldest_dropped"`
	Rejected      uint64             `json:"rejected"`
	QueuePeak     uint64             `json:"queue_peak"`
	QueueAge      DurationDiagnostic `json:"queue_age"`
	Handler       DurationDiagnostic `json:"handler"`
}

type gameIngressShardTiming struct {
	enqueued      atomic.Uint64
	handled       atomic.Uint64
	oldestDropped atomic.Uint64
	rejected      atomic.Uint64
	queuePeak     atomic.Uint64
	queueAge      durationAccumulator
	handler       durationAccumulator
}

func (p *serverPipelineTiming) gameIngressEnqueue(index, queueDepth int) {
	if index < 0 || index >= len(p.gameIngressShards) {
		return
	}
	q := &p.gameIngressShards[index]
	q.enqueued.Add(1)
	if queueDepth > 0 {
		atomicMax(&q.queuePeak, uint64(queueDepth))
	}
}
func (p *serverPipelineTiming) gameIngressDrop(index int) {
	if index >= 0 && index < len(p.gameIngressShards) {
		p.gameIngressShards[index].oldestDropped.Add(1)
	}
}
func (p *serverPipelineTiming) gameIngressReject(index int) {
	if index >= 0 && index < len(p.gameIngressShards) {
		p.gameIngressShards[index].rejected.Add(1)
	}
}
func (p *serverPipelineTiming) gameIngressHandled(index int, queuedFor, handleFor time.Duration) {
	if index < 0 || index >= len(p.gameIngressShards) {
		return
	}
	q := &p.gameIngressShards[index]
	q.handled.Add(1)
	q.queueAge.observe(queuedFor)
	q.handler.observe(handleFor)
}
func (p *serverPipelineTiming) gameIngressSnapshot() []GameIngressShardDiagnostic {
	desired := int(p.gameIngressDesired.Load())
	if desired < 2 || desired > len(p.gameIngressShards) {
		return nil
	}
	capacity := (serverReadQueueDepth - gameIngressControlDepth) / desired
	out := make([]GameIngressShardDiagnostic, desired)
	for i := 0; i < desired; i++ {
		q := &p.gameIngressShards[i]
		out[i] = GameIngressShardDiagnostic{
			Index:i,Capacity:capacity,
			Enqueued:q.enqueued.Load(),Handled:q.handled.Load(),
			OldestDropped:q.oldestDropped.Load(),Rejected:q.rejected.Load(),
			QueuePeak:q.queuePeak.Load(),QueueAge:q.queueAge.snapshot(),Handler:q.handler.snapshot(),
		}
	}
	return out
}
