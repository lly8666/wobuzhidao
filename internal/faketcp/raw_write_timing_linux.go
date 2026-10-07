//go:build linux

package faketcp

import (
	"sync/atomic"
	"time"
)

// RawWriteDurationDiagnostic contains wall durations in nanoseconds. These
// include scheduler delay, not just CPU time. Storage is fixed-size regardless
// of the number of packets sent.
type RawWriteDurationDiagnostic struct {
	Samples  uint64 `json:"samples"`
	TotalNS  uint64 `json:"total_ns"`
	MaxNS    uint64 `json:"max_ns"`
	Over10MS uint64 `json:"over_10ms"`
}

type RawWriteTimingDiagnostic struct {
	Enabled  bool                       `json:"enabled"`
	LockWait RawWriteDurationDiagnostic `json:"lock_wait"`
	LockHold RawWriteDurationDiagnostic `json:"lock_hold"`
	// Marshal includes construction of the ready batch's syscall messages.
	Marshal RawWriteDurationDiagnostic `json:"marshal"`
	// Syscall observes each actual invocation, including EINTR/ENOSYS attempts,
	// independently of the interval spent waiting to acquire the send lock.
	Syscall RawWriteDurationDiagnostic `json:"syscall"`
}

type rawWriteDuration struct {
	samples  atomic.Uint64
	totalNS  atomic.Uint64
	maxNS    atomic.Uint64
	over10MS atomic.Uint64
}

func (d *rawWriteDuration) observe(elapsed time.Duration) {
	if elapsed < 0 {
		elapsed = 0
	}
	ns := uint64(elapsed)
	d.samples.Add(1)
	d.totalNS.Add(ns)
	for {
		old := d.maxNS.Load()
		if ns <= old || d.maxNS.CompareAndSwap(old, ns) {
			break
		}
	}
	if elapsed > 10*time.Millisecond {
		d.over10MS.Add(1)
	}
}

func (d *rawWriteDuration) snapshot() RawWriteDurationDiagnostic {
	return RawWriteDurationDiagnostic{
		Samples: d.samples.Load(), TotalNS: d.totalNS.Load(),
		MaxNS: d.maxNS.Load(), Over10MS: d.over10MS.Load(),
	}
}

type rawWriteTiming struct {
	enabled  atomic.Bool
	lockWait rawWriteDuration
	lockHold rawWriteDuration
	marshal  rawWriteDuration
	syscall  rawWriteDuration
}

// SetWriteTimingDiagnostics is qualification-only and defaults off. Toggling
// it does not reset cumulative counters or alter socket flags, deadlines,
// ownership, batching or transmission behavior. Each call snapshots the switch
// once so a concurrent toggle cannot produce unmatched lock wait/hold samples.
func (e *RawIPv4Endpoint) SetWriteTimingDiagnostics(enabled bool) {
	e.writeTiming.enabled.Store(enabled)
}

func (t *rawWriteTiming) snapshot() RawWriteTimingDiagnostic {
	return RawWriteTimingDiagnostic{
		Enabled: t.enabled.Load(), LockWait: t.lockWait.snapshot(),
		LockHold: t.lockHold.snapshot(), Marshal: t.marshal.snapshot(),
		Syscall: t.syscall.snapshot(),
	}
}
