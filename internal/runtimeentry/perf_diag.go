package runtimeentry

import (
	"sync/atomic"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

type DurationDiagnostic struct {
	Samples  uint64 `json:"samples"`
	TotalNS  uint64 `json:"total_ns"`
	MaxNS    uint64 `json:"max_ns"`
	Over50US uint64 `json:"over_50us"`
	Over1MS  uint64 `json:"over_1ms"`
	Over10MS uint64 `json:"over_10ms"`
}

type ServerPipelineDiagnostic struct {
	Enabled           bool               `json:"enabled"`
	Reads             uint64             `json:"reads"`
	ReadGap           DurationDiagnostic `json:"read_gap"`
	HandoffBlock      DurationDiagnostic `json:"handoff_block"`
	QueueAge          DurationDiagnostic `json:"queue_age"`
	Handler           DurationDiagnostic `json:"handler"`
	Downstream        DurationDiagnostic `json:"downstream"`
	ReadyCurrent      int64              `json:"ready_current"`
	ReadyPeak         uint64             `json:"ready_peak"`
	ReadyBytes        int64              `json:"ready_bytes"`
	ReadyBytesPeak    uint64             `json:"ready_bytes_peak"`
	DownstreamBatches uint64             `json:"downstream_batches"`
	DownstreamPackets uint64             `json:"downstream_packets"`
}

type durationAccumulator struct {
	samples atomic.Uint64
	totalNS atomic.Uint64
	maxNS atomic.Uint64
	over50US atomic.Uint64
	over1MS atomic.Uint64
	over10MS atomic.Uint64
}
func (a *durationAccumulator) observe(d time.Duration) {
	if d < 0 { d = 0 }
	ns := uint64(d)
	a.samples.Add(1); a.totalNS.Add(ns); atomicMax(&a.maxNS, ns)
	if d >= 50*time.Microsecond { a.over50US.Add(1) }
	if d >= time.Millisecond { a.over1MS.Add(1) }
	if d >= 10*time.Millisecond { a.over10MS.Add(1) }
}
func (a *durationAccumulator) snapshot() DurationDiagnostic {
	return DurationDiagnostic{Samples:a.samples.Load(), TotalNS:a.totalNS.Load(), MaxNS:a.maxNS.Load(), Over50US:a.over50US.Load(), Over1MS:a.over1MS.Load(), Over10MS:a.over10MS.Load()}
}
func atomicMax(dst *atomic.Uint64, value uint64) {
	for { old:=dst.Load(); if value<=old || dst.CompareAndSwap(old,value) { return } }
}
type serverPipelineTiming struct {
	reads atomic.Uint64
	readGap durationAccumulator
	handoffBlock durationAccumulator
	queueAge durationAccumulator
	handler durationAccumulator
	downstream durationAccumulator
	readyCurrent atomic.Int64
	readyPeak atomic.Uint64
	readyBytes atomic.Int64
	readyBytesPeak atomic.Uint64
	downstreamBatches atomic.Uint64
	downstreamPackets atomic.Uint64
}
func (p *serverPipelineTiming) observeRead(readAt, previous time.Time) { p.reads.Add(1); if !previous.IsZero(){p.readGap.observe(readAt.Sub(previous))} }
func (p *serverPipelineTiming) ready(bytes int) {
	c:=p.readyCurrent.Add(1); if c>0 { atomicMax(&p.readyPeak,uint64(c)) }
	b:=p.readyBytes.Add(int64(bytes)); if b>0 { atomicMax(&p.readyBytesPeak,uint64(b)) }
}
func (p *serverPipelineTiming) readyDone(bytes int, age time.Duration){p.readyCurrent.Add(-1);p.readyBytes.Add(-int64(bytes));p.queueAge.observe(age)}
func (p *serverPipelineTiming) readyCancel(bytes int){p.readyCurrent.Add(-1);p.readyBytes.Add(-int64(bytes))}
func (p *serverPipelineTiming) snapshot(enabled bool) ServerPipelineDiagnostic {
	return ServerPipelineDiagnostic{Enabled:enabled,Reads:p.reads.Load(),ReadGap:p.readGap.snapshot(),HandoffBlock:p.handoffBlock.snapshot(),QueueAge:p.queueAge.snapshot(),Handler:p.handler.snapshot(),Downstream:p.downstream.snapshot(),ReadyCurrent:p.readyCurrent.Load(),ReadyPeak:p.readyPeak.Load(),ReadyBytes:p.readyBytes.Load(),ReadyBytesPeak:p.readyBytesPeak.Load(),DownstreamBatches:p.downstreamBatches.Load(),DownstreamPackets:p.downstreamPackets.Load()}
}


type SegmentMuxRouteDiagnostic struct {
	LocalPort    uint16             `json:"local_port"`
	PeerPort     uint16             `json:"peer_port"`
	Capacity     int                `json:"capacity"`
	Current      int64              `json:"current"`
	Peak         uint64             `json:"peak"`
	Bytes        int64              `json:"bytes"`
	BytesPeak    uint64             `json:"bytes_peak"`
	FullWaits    uint64             `json:"full_waits"`
	HandoffBlock DurationDiagnostic `json:"handoff_block"`
	QueueAge     DurationDiagnostic `json:"queue_age"`
}

type SegmentMuxDiagnostic struct {
	Enabled bool                        `json:"enabled"`
	Routes  []SegmentMuxRouteDiagnostic `json:"routes"`
}

type segmentMuxQueueTiming struct {
	current      atomic.Int64
	peak         atomic.Uint64
	bytes        atomic.Int64
	bytesPeak    atomic.Uint64
	fullWaits    atomic.Uint64
	handoffBlock durationAccumulator
	queueAge     durationAccumulator
}

func (q *segmentMuxQueueTiming) ready(bytes int) {
	current := q.current.Add(1)
	if current > 0 {
		atomicMax(&q.peak, uint64(current))
	}
	currentBytes := q.bytes.Add(int64(bytes))
	if currentBytes > 0 {
		atomicMax(&q.bytesPeak, uint64(currentBytes))
	}
}

func (q *segmentMuxQueueTiming) dequeue(bytes int, age time.Duration) {
	q.current.Add(-1)
	q.bytes.Add(-int64(bytes))
	q.queueAge.observe(age)
}

func (q *segmentMuxQueueTiming) cancel(bytes int) {
	q.current.Add(-1)
	q.bytes.Add(-int64(bytes))
}

func (q *segmentMuxQueueTiming) snapshot(flow faketcp.ClientFlow, capacity int) SegmentMuxRouteDiagnostic {
	return SegmentMuxRouteDiagnostic{
		LocalPort: flow.LocalPort,
		PeerPort: flow.PeerPort,
		Capacity: capacity,
		Current: q.current.Load(),
		Peak: q.peak.Load(),
		Bytes: q.bytes.Load(),
		BytesPeak: q.bytesPeak.Load(),
		FullWaits: q.fullWaits.Load(),
		HandoffBlock: q.handoffBlock.snapshot(),
		QueueAge: q.queueAge.snapshot(),
	}
}
