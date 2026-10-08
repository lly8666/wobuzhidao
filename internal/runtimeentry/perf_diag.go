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
	Tick              DurationDiagnostic `json:"tick"`
	TickRetransmit    DurationDiagnostic `json:"tick_retransmit"`
	TickSweep         DurationDiagnostic `json:"tick_sweep"`
	TickRuntime       DurationDiagnostic `json:"tick_runtime"`
	TickService       DurationDiagnostic `json:"tick_service"`
	TickServiceUDP    DurationDiagnostic `json:"tick_service_udp"`
	TickServiceTCP    DurationDiagnostic `json:"tick_service_tcp"`
	TickTCPFlowSnapshot DurationDiagnostic `json:"tick_tcp_flow_snapshot"`
	TickTCPScan      DurationDiagnostic `json:"tick_tcp_scan"`
	TickTCPEmit      DurationDiagnostic `json:"tick_tcp_emit"`
	TickTCPAbort     DurationDiagnostic `json:"tick_tcp_abort"`
	TickTCPFlows     uint64 `json:"tick_tcp_flows"`
	TickTCPDueFrames uint64 `json:"tick_tcp_due_frames"`
	TickTCPAborts    uint64 `json:"tick_tcp_aborts"`
	TickTCPAsyncScheduled uint64 `json:"tick_tcp_async_scheduled"`
	TickTCPAsyncCoalesced uint64 `json:"tick_tcp_async_coalesced"`
	TickBookkeeping   DurationDiagnostic `json:"tick_bookkeeping"`
	ReplacementChecks uint64             `json:"replacement_checks"`
	Downstream        DurationDiagnostic `json:"downstream"`
	ReadyCapacity     int                `json:"ready_capacity"`
	ReadyCurrent      int64              `json:"ready_current"`
	ReadyPeak         uint64             `json:"ready_peak"`
	ReadyBytes        int64              `json:"ready_bytes"`
	ReadyBytesPeak    uint64             `json:"ready_bytes_peak"`
	OverflowDrops     uint64             `json:"overflow_drops"`
	OverflowBytes     uint64             `json:"overflow_bytes"`
	OverflowAge       DurationDiagnostic `json:"overflow_age"`
	DownstreamBatches uint64             `json:"downstream_batches"`
	DownstreamPackets uint64             `json:"downstream_packets"`
	GameIngressShards []GameIngressShardDiagnostic `json:"game_ingress_shards,omitempty"`
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
	tick durationAccumulator
	tickRetransmit durationAccumulator
	tickSweep durationAccumulator
	tickRuntime durationAccumulator
	tickService durationAccumulator
	tickServiceUDP durationAccumulator
	tickServiceTCP durationAccumulator
	tickTCPFlowSnapshot durationAccumulator
	tickTCPScan durationAccumulator
	tickTCPEmit durationAccumulator
	tickTCPAbort durationAccumulator
	tickTCPFlows atomic.Uint64
	tickTCPDueFrames atomic.Uint64
	tickTCPAborts atomic.Uint64
	tickTCPAsyncScheduled atomic.Uint64
	tickTCPAsyncCoalesced atomic.Uint64
	tickBookkeeping durationAccumulator
	replacementChecks atomic.Uint64
	downstream durationAccumulator
	readyCurrent atomic.Int64
	readyPeak atomic.Uint64
	readyBytes atomic.Int64
	readyBytesPeak atomic.Uint64
	overflowDrops atomic.Uint64
	overflowBytes atomic.Uint64
	overflowAge durationAccumulator
	downstreamBatches atomic.Uint64
	downstreamPackets atomic.Uint64
	gameIngressDesired atomic.Uint32
	gameIngressShards [4]gameIngressShardTiming
}
func (p *serverPipelineTiming) observeRead(readAt, previous time.Time) { p.reads.Add(1); if !previous.IsZero(){p.readGap.observe(readAt.Sub(previous))} }
func (p *serverPipelineTiming) ready(bytes int) {
	c:=p.readyCurrent.Add(1); if c>0 { atomicMax(&p.readyPeak,uint64(c)) }
	b:=p.readyBytes.Add(int64(bytes)); if b>0 { atomicMax(&p.readyBytesPeak,uint64(b)) }
}
func (p *serverPipelineTiming) readyDone(bytes int, age time.Duration){p.readyCurrent.Add(-1);p.readyBytes.Add(-int64(bytes));p.queueAge.observe(age)}
func (p *serverPipelineTiming) readyCancel(bytes int){p.readyCurrent.Add(-1);p.readyBytes.Add(-int64(bytes))}
func (p *serverPipelineTiming) overflowDrop(bytes int, age time.Duration){p.readyCurrent.Add(-1);p.readyBytes.Add(-int64(bytes));p.overflowDrops.Add(1);if bytes>0{p.overflowBytes.Add(uint64(bytes))};p.overflowAge.observe(age)}
func (p *serverPipelineTiming) overflowReject(bytes int){p.readyCurrent.Add(-1);p.readyBytes.Add(-int64(bytes));p.overflowDrops.Add(1);if bytes>0{p.overflowBytes.Add(uint64(bytes))}}
func (p *serverPipelineTiming) snapshot(enabled bool) ServerPipelineDiagnostic {
	return ServerPipelineDiagnostic{Enabled:enabled,Reads:p.reads.Load(),ReadGap:p.readGap.snapshot(),HandoffBlock:p.handoffBlock.snapshot(),QueueAge:p.queueAge.snapshot(),Handler:p.handler.snapshot(),Tick:p.tick.snapshot(),TickRetransmit:p.tickRetransmit.snapshot(),TickSweep:p.tickSweep.snapshot(),TickRuntime:p.tickRuntime.snapshot(),TickService:p.tickService.snapshot(),TickServiceUDP:p.tickServiceUDP.snapshot(),TickServiceTCP:p.tickServiceTCP.snapshot(),TickTCPFlowSnapshot:p.tickTCPFlowSnapshot.snapshot(),TickTCPScan:p.tickTCPScan.snapshot(),TickTCPEmit:p.tickTCPEmit.snapshot(),TickTCPAbort:p.tickTCPAbort.snapshot(),TickTCPFlows:p.tickTCPFlows.Load(),TickTCPDueFrames:p.tickTCPDueFrames.Load(),TickTCPAborts:p.tickTCPAborts.Load(),TickTCPAsyncScheduled:p.tickTCPAsyncScheduled.Load(),TickTCPAsyncCoalesced:p.tickTCPAsyncCoalesced.Load(),TickBookkeeping:p.tickBookkeeping.snapshot(),ReplacementChecks:p.replacementChecks.Load(),Downstream:p.downstream.snapshot(),ReadyCapacity:serverReadQueueDepth,ReadyCurrent:p.readyCurrent.Load(),ReadyPeak:p.readyPeak.Load(),ReadyBytes:p.readyBytes.Load(),ReadyBytesPeak:p.readyBytesPeak.Load(),OverflowDrops:p.overflowDrops.Load(),OverflowBytes:p.overflowBytes.Load(),OverflowAge:p.overflowAge.snapshot(),DownstreamBatches:p.downstreamBatches.Load(),DownstreamPackets:p.downstreamPackets.Load(),GameIngressShards:p.gameIngressSnapshot()}
}


type SegmentMuxRouteDiagnostic struct {
	LocalPort    uint16             `json:"local_port"`
	PeerPort     uint16             `json:"peer_port"`
	Capacity     int                `json:"capacity"`
	Current      int64              `json:"current"`
	Peak         uint64             `json:"peak"`
	Bytes        int64              `json:"bytes"`
	BytesPeak    uint64             `json:"bytes_peak"`
	FullWaits     uint64             `json:"full_waits"`
	OverflowDrops uint64             `json:"overflow_drops"`
	OverflowBytes uint64             `json:"overflow_bytes"`
	OverflowAge   DurationDiagnostic `json:"overflow_age"`
	HandoffBlock  DurationDiagnostic `json:"handoff_block"`
	QueueAge      DurationDiagnostic `json:"queue_age"`
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
	overflowDrops atomic.Uint64
	overflowBytes atomic.Uint64
	overflowAge durationAccumulator
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

func (q *segmentMuxQueueTiming) overflowDrop(bytes int, age time.Duration) {
	q.current.Add(-1)
	q.bytes.Add(-int64(bytes))
	q.overflowDrops.Add(1)
	if bytes > 0 { q.overflowBytes.Add(uint64(bytes)) }
	q.overflowAge.observe(age)
}

func (q *segmentMuxQueueTiming) overflowReject(bytes int) {
	q.current.Add(-1)
	q.bytes.Add(-int64(bytes))
	q.overflowDrops.Add(1)
	if bytes > 0 { q.overflowBytes.Add(uint64(bytes)) }
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
		OverflowDrops: q.overflowDrops.Load(),
		OverflowBytes: q.overflowBytes.Load(),
		OverflowAge: q.overflowAge.snapshot(),
		HandoffBlock: q.handoffBlock.snapshot(),
		QueueAge: q.queueAge.snapshot(),
	}
}
