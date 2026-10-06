package runtimeowner

import (
	"sync/atomic"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

type atomicDuration struct {
	total atomic.Uint64
	max   atomic.Uint64
}

func (d *atomicDuration) observe(elapsed time.Duration) {
	if elapsed < 0 {
		elapsed = 0
	}
	ns := uint64(elapsed)
	d.total.Add(ns)
	for {
		old := d.max.Load()
		if ns <= old || d.max.CompareAndSwap(old, ns) {
			return
		}
	}
}

type transportTiming struct {
	enabled        atomic.Bool
	samples        atomic.Uint64
	lockWait       atomicDuration
	lockHeld       atomicDuration
	ackProcess     atomicDuration
	owner          atomicDuration
	deliver        atomicDuration
	repairEviction atomicDuration
	freshLockWait  atomicDuration
	freshCritical  atomicDuration
	ackFeedback    feedbackDuration
	selectedRepair feedbackDuration
}

// Fixed-size counters, updated only by explicit qualification instrumentation.
// These are wall durations (including locks/driver wait), not CPU durations.
type feedbackDuration struct {
	duration atomicDuration
	samples  atomic.Uint64
	over1MS  atomic.Uint64
	over10MS atomic.Uint64
}

func (d *feedbackDuration) observe(elapsed time.Duration) {
	d.duration.observe(elapsed)
	d.samples.Add(1)
	if elapsed > time.Millisecond {
		d.over1MS.Add(1)
	}
	if elapsed > 10*time.Millisecond {
		d.over10MS.Add(1)
	}
}

// Observe only receive-handler feedback. ACK timer and recovery-tick callbacks
// retain their original behavior and are deliberately outside these counters.
func (t *laneTransport) emitReceiveACK(seg faketcp.Segment, observe bool) error {
	if !observe {
		return t.cfg.Emit(seg)
	}
	started := time.Now()
	err := t.cfg.Emit(seg)
	t.timing.ackFeedback.observe(time.Since(started))
	return err
}

func (t *laneTransport) emitReceiveRepair(repair *selectedRepair, now time.Time, observe bool) error {
	if !observe || repair == nil {
		return t.emitSelectedRepair(repair, now)
	}
	started := time.Now()
	err := t.emitSelectedRepair(repair, now)
	t.timing.selectedRepair.observe(time.Since(started))
	return err
}
func (t *transportTiming) apply(out *TransportStats) {
	if out == nil {
		return
	}
	out.TimingEnabled = t.enabled.Load()
	out.TimingSamples = t.samples.Load()
	out.LockWaitNS = t.lockWait.total.Load()
	out.LockWaitMaxNS = t.lockWait.max.Load()
	out.LockHeldNS = t.lockHeld.total.Load()
	out.LockHeldMaxNS = t.lockHeld.max.Load()
	out.ACKProcessNS = t.ackProcess.total.Load()
	out.ACKProcessMaxNS = t.ackProcess.max.Load()
	out.OwnerNS = t.owner.total.Load()
	out.OwnerMaxNS = t.owner.max.Load()
	out.DeliverNS = t.deliver.total.Load()
	out.DeliverMaxNS = t.deliver.max.Load()
	out.RepairEvictionNS = t.repairEviction.total.Load()
	out.RepairEvictionMaxNS = t.repairEviction.max.Load()
	out.FreshLockWaitNS = t.freshLockWait.total.Load()
	out.FreshLockWaitMaxNS = t.freshLockWait.max.Load()
	out.FreshCriticalNS = t.freshCritical.total.Load()
	out.FreshCriticalMaxNS = t.freshCritical.max.Load()
	out.ACKFeedbackSamples = t.ackFeedback.samples.Load()
	out.ACKFeedbackNS = t.ackFeedback.duration.total.Load()
	out.ACKFeedbackMaxNS = t.ackFeedback.duration.max.Load()
	out.ACKFeedbackOver1MS = t.ackFeedback.over1MS.Load()
	out.ACKFeedbackOver10MS = t.ackFeedback.over10MS.Load()
	out.SelectedRepairSamples = t.selectedRepair.samples.Load()
	out.SelectedRepairNS = t.selectedRepair.duration.total.Load()
	out.SelectedRepairMaxNS = t.selectedRepair.duration.max.Load()
	out.SelectedRepairOver1MS = t.selectedRepair.over1MS.Load()
	out.SelectedRepairOver10MS = t.selectedRepair.over10MS.Load()
}
