package runtimeowner

import (
	"sync/atomic"
	"time"
)

type atomicDuration struct {
	total atomic.Uint64
	max   atomic.Uint64
}
func (d *atomicDuration) observe(elapsed time.Duration) {
	if elapsed < 0 { elapsed = 0 }
	ns:=uint64(elapsed); d.total.Add(ns)
	for { old:=d.max.Load(); if ns<=old || d.max.CompareAndSwap(old,ns) { return } }
}
type transportTiming struct {
	enabled atomic.Bool
	samples atomic.Uint64
	lockWait atomicDuration
	lockHeld atomicDuration
	owner atomicDuration
	deliver atomicDuration
}
func (t *transportTiming) apply(out *TransportStats) {
	if out==nil{return}
	out.TimingEnabled=t.enabled.Load()
	out.TimingSamples=t.samples.Load()
	out.LockWaitNS=t.lockWait.total.Load(); out.LockWaitMaxNS=t.lockWait.max.Load()
	out.LockHeldNS=t.lockHeld.total.Load(); out.LockHeldMaxNS=t.lockHeld.max.Load()
	out.OwnerNS=t.owner.total.Load(); out.OwnerMaxNS=t.owner.max.Load()
	out.DeliverNS=t.deliver.total.Load(); out.DeliverMaxNS=t.deliver.max.Load()
}
