package runtimeowner

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

const (
	MaxOutstandingRecords = 4096
	DefaultRepairRTO      = time.Second
	DefaultRepairHorizon  = 3 * time.Second
)

var (
	ErrRuntimeClosed        = errors.New("runtimeowner: runtime is closed")
	ErrTransportConfig      = errors.New("runtimeowner: invalid transport config")
	ErrTransportMissing     = errors.New("runtimeowner: lane transport is missing")
	ErrTransportFlow        = errors.New("runtimeowner: segment does not match lane flow")
	ErrACKRange             = errors.New("runtimeowner: ACK exceeds steady send sequence")
	ErrPayloadConflict      = errors.New("runtimeowner: same sequence has conflicting payload")
	ErrOutstandingBounds    = errors.New("runtimeowner: outstanding record bound reached")
	ErrTransportWriteClosed = errors.New("runtimeowner: steady write side is closed")
	ErrTransportPeerReset   = errors.New("runtimeowner: peer reset steady transport")
)

type PacketSink func(packets [][]byte, now time.Time) error

type TransportConfig struct {
	LocalIP   [4]byte
	PeerIP    [4]byte
	LocalPort uint16
	PeerPort  uint16

	SendNext    uint32
	ReceiveNext uint32

	AdvertisedWindow    uint16
	AdvertisedWindowSet bool
	WindowScale         uint8
	WindowScaleSet      bool

	InitialRTO    time.Duration
	RepairHorizon time.Duration
	SACKPermitted bool
	Emit          faketcp.SegmentEmitter
}

func (c *TransportConfig) normalize() error {
	if c.LocalIP == ([4]byte{}) || c.PeerIP == ([4]byte{}) ||
		c.LocalPort == 0 || c.PeerPort == 0 || c.Emit == nil {
		return ErrTransportConfig
	}
	if c.InitialRTO <= 0 {
		c.InitialRTO = DefaultRepairRTO
	}
	if c.RepairHorizon <= 0 {
		c.RepairHorizon = DefaultRepairHorizon
	}
	if c.RepairHorizon < c.InitialRTO {
		return ErrTransportConfig
	}
	if !c.AdvertisedWindowSet {
		c.AdvertisedWindow = 65535
	}
	if c.WindowScaleSet {
		if c.WindowScale > faketcp.MaxWindowScale {
			return ErrTransportConfig
		}
	} else {
		c.WindowScale = 0
	}
	return nil
}

type pendingRecord struct {
	control         bool
	seq             uint32
	end             uint32
	flags           uint8
	payload         []byte
	firstSent       time.Time
	lastSent        time.Time
	retries         uint32
	wasRetried      bool
	rttSampled      bool
	sacked          bool
	retired         bool
	repairInFlight  bool
	repairNotBefore time.Time
	repairPrev      *pendingRecord
	repairNext      *pendingRecord
	repairLinked    bool
}

type receiveSpan struct {
	end   uint32
	first time.Time
	fin   bool
}

type deliveredMark struct {
	end  uint32
	hash [32]byte
}

type TransportStats struct {
	AuthenticatedRecords  uint64
	HealthSent            uint64
	HealthReceived        uint64
	LastAuthenticated     time.Time
	PressureForgiven      uint64
	FreshSent             uint64
	RepairSelected        uint64
	RepairAttempts        uint64
	RepairSucceeded       uint64
	RepairFailures        uint64
	Retransmitted         uint64
	Acked                 uint64
	SACKed                uint64
	SACKRetired           uint64
	Abandoned             uint64
	RepairEvicted         uint64
	RepairMetadataEvicted uint64
	FastRepairs           uint64
	RTORepairs            uint64
	RepairDeferred        uint64
	RepairBudgetSpent     uint64
	RepairCreditBytes     uint64
	Received              uint64
	Duplicates            uint64
	LateFirstArrival      uint64
	ForgivenGaps          uint64
	RecordErrors          uint64
	PathErrors            uint64
	FINAttempts           uint64
	FINTransmits          uint64
	FINAcked              uint64
	RSTAttempts           uint64
	RSTSent               uint64
	PeakOutstanding       int
	Outstanding           int
	OutstandingBytes      uint64
	OldestOutstandingAge  time.Duration
	RepairQueue           int
	SACKedOutstanding     int
	OutOfOrder            int
	OutOfOrderBytes       uint64
	OldestOutOfOrderAge   time.Duration
	WriteClosed           bool
	LocalFINAcked         bool
	PeerFIN               bool
	PeerRST               bool
	SRTT                  time.Duration
	RTO                   time.Duration
	AdvertisedWindow      uint16
	WindowScale           uint8
	WindowScaleSet        bool
	Closed                bool

	TimingEnabled bool   `json:"timing_enabled,omitempty"`
	TimingSamples uint64 `json:"timing_samples,omitempty"`
	LockWaitNS    uint64 `json:"lock_wait_ns,omitempty"`
	LockWaitMaxNS uint64 `json:"lock_wait_max_ns,omitempty"`
	LockHeldNS    uint64 `json:"lock_held_ns,omitempty"`
	LockHeldMaxNS uint64 `json:"lock_held_max_ns,omitempty"`
	OwnerNS       uint64 `json:"owner_ns,omitempty"`
	OwnerMaxNS    uint64 `json:"owner_max_ns,omitempty"`
	DeliverNS     uint64 `json:"deliver_ns,omitempty"`
	DeliverMaxNS  uint64 `json:"deliver_max_ns,omitempty"`
}

type laneTransport struct {
	health   healthState
	pressure receivePressure
	mu       sync.Mutex

	owner   *datapath.TunnelOwner
	ref     logicaltunnel.LaneRef
	deliver PacketSink
	cfg     TransportConfig

	sendNext  uint32
	lastAck   uint32
	recvStart uint32
	recvNext  uint32

	srtt              time.Duration
	rttvar            time.Duration
	baseRTO           time.Duration
	rto               time.Duration
	timeoutEpisode    bool
	timeoutEpisodeEnd uint32
	rackLatestTx      time.Time
	repairCredit      uint64
	repairRemainder   uint64

	localFINQueued bool
	localFINAcked  bool
	peerFIN        bool
	peerFINEnd     uint32
	peerRST        bool

	pending      map[uint32]*pendingRecord
	pendingOrder []uint32
	pendingHead  int

	repairHead  *pendingRecord
	repairTail  *pendingRecord
	repairScan  *pendingRecord
	repairCount int

	sackedOutstanding int
	sackSeen          [steadySenderSACKHistory]faketcp.SACKBlock
	sackSeenN         int

	received  map[uint32]receiveSpan
	recvSACK  [faketcp.MaxSACKBlocks]faketcp.SACKBlock
	recvSACKN int

	delivered      map[uint32]deliveredMark
	deliveredOrder []uint32
	deliveredHead  int

	stats  TransportStats
	timing transportTiming
	closed bool
}

func newLaneTransport(owner *datapath.TunnelOwner, ref logicaltunnel.LaneRef, deliver PacketSink, cfg TransportConfig) (*laneTransport, error) {
	if owner == nil || ref.ID == 0 || ref.Generation == 0 {
		return nil, ErrTransportConfig
	}
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	return &laneTransport{
		owner: owner, ref: ref, deliver: deliver, cfg: cfg,
		sendNext: cfg.SendNext, lastAck: cfg.SendNext,
		recvStart: cfg.ReceiveNext, recvNext: cfg.ReceiveNext,
		baseRTO: cfg.InitialRTO, rto: cfg.InitialRTO,
		repairCredit: steadyRepairBurstBytes,
		pending:      make(map[uint32]*pendingRecord, MaxOutstandingRecords),
		received:     make(map[uint32]receiveSpan, MaxOutstandingRecords),
		delivered:    make(map[uint32]deliveredMark, MaxOutstandingRecords),
	}, nil
}

func (t *laneTransport) matches(seg faketcp.Segment) bool {
	return seg.SrcIP == t.cfg.PeerIP && seg.DstIP == t.cfg.LocalIP &&
		seg.SrcPort == t.cfg.PeerPort && seg.DstPort == t.cfg.LocalPort
}

func (t *laneTransport) outboundSegment(seq, ack uint32, payload []byte) faketcp.Segment {
	flags := uint8(faketcp.FlagACK)
	if len(payload) != 0 {
		flags |= faketcp.FlagPSH
	}
	return t.outboundSegmentFlags(seq, ack, flags, payload)
}

func (t *laneTransport) outboundSegmentFlags(seq, ack uint32, flags uint8, payload []byte) faketcp.Segment {
	seg := faketcp.Segment{
		SrcIP: t.cfg.LocalIP, DstIP: t.cfg.PeerIP,
		SrcPort: t.cfg.LocalPort, DstPort: t.cfg.PeerPort,
		Seq: seq, Ack: ack, Flags: flags, Window: t.cfg.AdvertisedWindow,
		Payload: append([]byte(nil), payload...),
	}
	// Keep SACK on ACK-only/control packets. Data records are already sized to
	// the negotiated MTU and must not silently grow when recovery options appear.
	if t.cfg.SACKPermitted && flags&faketcp.FlagACK != 0 && len(payload) == 0 {
		blocks, n := t.sackBlocksLocked()
		seg.SACKN = n
		copy(seg.SACK[:], blocks[:n])
	}
	return seg
}

func (t *laneTransport) send(records []datapath.WireRecord, now time.Time) error {
	for _, record := range records {
		if len(record.Wire) == 0 {
			continue
		}
		t.mu.Lock()
		if t.closed {
			peerRST := t.peerRST
			t.mu.Unlock()
			if peerRST {
				return ErrTransportPeerReset
			}
			return ErrRuntimeClosed
		}
		if t.localFINQueued {
			t.mu.Unlock()
			return ErrTransportWriteClosed
		}
		if len(t.pending) >= MaxOutstandingRecords {
			t.abandonOldestLocked()
		}
		if len(t.pending) >= MaxOutstandingRecords {
			t.mu.Unlock()
			return ErrOutstandingBounds
		}
		seq := t.sendNext
		end := seq + uint32(len(record.Wire))
		p := &pendingRecord{
			control: record.Control, seq: seq, end: end, flags: faketcp.FlagACK | faketcp.FlagPSH,
			payload:   append([]byte(nil), record.Wire...),
			firstSent: now, lastSent: now,
		}
		t.pending[seq] = p
		t.pendingOrder = append(t.pendingOrder, seq)
		if !record.Control {
			t.linkRepairLocked(p)
		}
		t.sendNext = end
		if n := len(t.pending); n > t.stats.PeakOutstanding {
			t.stats.PeakOutstanding = n
		}
		t.stats.FreshSent++
		ack := t.recvNext
		seg := t.outboundSegment(seq, ack, p.payload)
		t.mu.Unlock()

		if err := t.cfg.Emit(seg); err != nil {
			t.mu.Lock()
			if current := t.pending[seq]; current == p {
				t.removePendingLocked(p)
				t.stats.Abandoned++
			}
			t.mu.Unlock()
			return err
		}
		t.mu.Lock()
		if !record.Control {
			t.refillRepairCreditLocked(uint64(len(p.payload)))
		}
		t.mu.Unlock()
	}
	return nil
}

func (t *laneTransport) handleSegment(seg faketcp.Segment, now time.Time) error {
	if !t.matches(seg) {
		return ErrTransportFlow
	}

	observeTiming := t.timing.enabled.Load()
	var lockWaitStarted time.Time
	if observeTiming {
		lockWaitStarted = time.Now()
	}
	var repair *selectedRepair
	t.mu.Lock()
	var lockHeldStarted time.Time
	if observeTiming {
		t.timing.samples.Add(1)
		t.timing.lockWait.observe(time.Since(lockWaitStarted))
		lockHeldStarted = time.Now()
	}
	if t.closed {
		peerRST := t.peerRST
		t.mu.Unlock()
		if peerRST {
			return ErrTransportPeerReset
		}
		return ErrRuntimeClosed
	}
	if seg.Flags&faketcp.FlagACK != 0 {
		if seqLT(t.sendNext, seg.Ack) {
			t.mu.Unlock()
			return ErrACKRange
		}
		t.retireACKLocked(seg.Ack, now)
		if t.cfg.SACKPermitted && seg.SACKN > 0 {
			n := seg.SACKN
			if n > faketcp.MaxSACKBlocks {
				n = faketcp.MaxSACKBlocks
			}
			t.applySACKLocked(seg.SACK[:n], now)
		}
		repair = t.selectFastRepairLocked(now)
	}

	if (len(seg.Payload) != 0 || seg.Flags&(faketcp.FlagFIN|faketcp.FlagRST) != 0) &&
		seqLT(seg.Seq, t.recvStart) {
		ackSeg := t.outboundSegment(t.sendNext, t.recvNext, nil)
		t.mu.Unlock()
		if err := t.cfg.Emit(ackSeg); err != nil {
			return err
		}
		return t.emitSelectedRepair(repair, now)
	}

	if seg.Flags&faketcp.FlagRST != 0 {
		if seg.Seq != t.recvNext {
			ackSeg := t.outboundSegment(t.sendNext, t.recvNext, nil)
			t.mu.Unlock()
			if err := t.cfg.Emit(ackSeg); err != nil {
				return err
			}
			return t.emitSelectedRepair(repair, now)
		}
		t.peerRST = true
		t.closed = true
		clear(t.pending)
		clear(t.received)
		t.pendingOrder = nil
		t.clearSteadyIndexesLocked()
		t.mu.Unlock()
		return nil
	}

	hasPayload := len(seg.Payload) != 0
	hasFIN := seg.Flags&faketcp.FlagFIN != 0
	if !hasPayload && !hasFIN {
		t.mu.Unlock()
		return t.emitSelectedRepair(repair, now)
	}

	deliver := false
	var err error
	if hasPayload {
		deliver, err = t.acceptPayloadLocked(seg.Seq, seg.Payload, now)
		if err != nil {
			t.mu.Unlock()
			return err
		}
	}
	if hasFIN {
		finSeq := seg.Seq + uint32(len(seg.Payload))
		if err := t.acceptFINLocked(finSeq, now); err != nil {
			t.mu.Unlock()
			return err
		}
	}
	ackSeg := t.outboundSegment(t.sendNext, t.recvNext, nil)
	if observeTiming {
		t.timing.lockHeld.observe(time.Since(lockHeldStarted))
	}
	t.mu.Unlock()

	if deliver {
		var result datapath.InboundResult
		stats := t.owner.Stats()
		ownerStarted := time.Time{}
		if observeTiming {
			ownerStarted = time.Now()
		}
		if stats.DesiredLanes == 1 {
			result, err = t.owner.InboundPayload(t.ref, seg.Payload, now)
		} else {
			result, err = t.owner.GameInboundPayload(t.ref, seg.Payload, now)
		}
		if observeTiming {
			t.timing.owner.observe(time.Since(ownerStarted))
		}
		if err != nil {
			return err
		}
		t.mu.Lock()
		t.observeHealthLocked(result, now)
		t.stats.RecordErrors += uint64(len(result.RecordErrors))
		t.stats.PathErrors += uint64(len(result.PathErrors))
		t.mu.Unlock()
		if len(result.Datagrams) != 0 && t.deliver != nil {
			deliverStarted := time.Time{}
			if observeTiming {
				deliverStarted = time.Now()
			}
			err := t.deliver(result.Datagrams, now)
			if observeTiming {
				t.timing.deliver.observe(time.Since(deliverStarted))
			}
			if err != nil {
				return err
			}
		}
	}
	if err := t.cfg.Emit(ackSeg); err != nil {
		return err
	}
	return t.emitSelectedRepair(repair, now)
}

func (t *laneTransport) acceptPayloadLocked(seq uint32, payload []byte, now time.Time) (bool, error) {
	end := seq + uint32(len(payload))
	hash := sha256.Sum256(payload)
	if mark, ok := t.delivered[seq]; ok {
		if mark.end != end || mark.hash != hash {
			return false, ErrPayloadConflict
		}
		t.stats.Duplicates++
		return false, nil
	}

	if seqLT(seq, t.recvNext) {
		t.stats.LateFirstArrival++
	} else if seq == t.recvNext {
		t.recvNext = end
		t.advanceReceiveLocked()
	} else {
		if span, ok := t.received[seq]; ok {
			if span.end != end {
				return false, ErrPayloadConflict
			}
		} else {
			t.received[seq] = receiveSpan{end: end, first: now}
			t.noteRecvSACKLocked(seq, end)
		}
		if len(t.received) > MaxOutstandingRecords {
			t.forgiveGapLocked(now, true)
		}
	}
	t.observeReceivePressureLocked(now)
	t.rememberDeliveredLocked(seq, deliveredMark{end: end, hash: hash})
	t.stats.Received++
	return true, nil
}

func (t *laneTransport) acceptFINLocked(seq uint32, now time.Time) error {
	end := seq + 1
	if t.peerFIN {
		if end == t.peerFINEnd || seqLT(end, t.peerFINEnd) {
			return nil
		}
		return ErrPayloadConflict
	}
	if seqLT(seq, t.recvNext) {
		return nil
	}
	if seq == t.recvNext {
		t.recvNext = end
		t.markPeerFINLocked(end)
		return nil
	}
	if span, ok := t.received[seq]; ok {
		if span.end != end || !span.fin {
			return ErrPayloadConflict
		}
		return nil
	}
	t.received[seq] = receiveSpan{end: end, first: now, fin: true}
	if len(t.received) > MaxOutstandingRecords {
		t.forgiveGapLocked(now, true)
	}
	return nil
}

func (t *laneTransport) markPeerFINLocked(end uint32) {
	if t.peerFIN {
		return
	}
	t.peerFIN = true
	t.peerFINEnd = end
}

func (t *laneTransport) rememberDeliveredLocked(seq uint32, mark deliveredMark) {
	t.delivered[seq] = mark
	t.deliveredOrder = append(t.deliveredOrder, seq)
	for len(t.delivered) > MaxOutstandingRecords && t.deliveredHead < len(t.deliveredOrder) {
		old := t.deliveredOrder[t.deliveredHead]
		t.deliveredHead++
		delete(t.delivered, old)
	}
	if t.deliveredHead >= steadyIndexCompactThreshold {
		copy(t.deliveredOrder, t.deliveredOrder[t.deliveredHead:])
		t.deliveredOrder = t.deliveredOrder[:len(t.deliveredOrder)-t.deliveredHead]
		t.deliveredHead = 0
	}
}

func (t *laneTransport) advanceReceiveLocked() {
	defer t.pruneRecvSACKLocked()
	for {
		span, ok := t.received[t.recvNext]
		if !ok {
			return
		}
		delete(t.received, t.recvNext)
		t.recvNext = span.end
		if span.fin {
			t.markPeerFINLocked(span.end)
			return
		}
	}
}

func (t *laneTransport) forgiveGapLocked(now time.Time, force bool) bool {
	var (
		bestSeq   uint32
		best      receiveSpan
		found     bool
		bestDelta uint32
	)
	for seq, span := range t.received {
		delta := seq - t.recvNext
		if delta == 0 || delta >= 1<<31 {
			continue
		}
		if !force && now.Sub(span.first) < t.cfg.RepairHorizon {
			continue
		}
		if !found || delta < bestDelta {
			bestSeq, best, bestDelta, found = seq, span, delta, true
		}
	}
	if !found {
		return false
	}
	delete(t.received, bestSeq)
	t.recvNext = best.end
	if best.fin {
		t.markPeerFINLocked(best.end)
		t.pruneRecvSACKLocked()
	} else {
		t.advanceReceiveLocked()
	}
	t.stats.ForgivenGaps++
	return true
}

func (t *laneTransport) retireACKLocked(ack uint32, now time.Time) {
	t.retireSelectiveACKLocked(ack, now)
}

func (t *laneTransport) abandonOldestLocked() bool {
	return t.evictRepairForFreshLocked()
}

func (t *laneTransport) tick(now time.Time) error {
	return t.tickRecovery(now)
}

func (t *laneTransport) closeWrite(now time.Time) error {
	t.mu.Lock()
	if t.closed {
		peerRST := t.peerRST
		t.mu.Unlock()
		if peerRST {
			return ErrTransportPeerReset
		}
		return ErrRuntimeClosed
	}
	if t.localFINQueued {
		t.mu.Unlock()
		return nil
	}
	if len(t.pending) >= MaxOutstandingRecords {
		t.evictRepairForFreshLocked()
	}
	if len(t.pending) >= MaxOutstandingRecords {
		t.mu.Unlock()
		return ErrOutstandingBounds
	}
	seq := t.sendNext
	p := &pendingRecord{
		seq: seq, end: seq + 1, flags: faketcp.FlagACK | faketcp.FlagFIN,
		firstSent: now,
	}
	t.pending[seq] = p
	t.pendingOrder = append(t.pendingOrder, seq)
	t.linkRepairLocked(p)
	t.sendNext = p.end
	t.localFINQueued = true
	t.stats.FINAttempts++
	if n := len(t.pending); n > t.stats.PeakOutstanding {
		t.stats.PeakOutstanding = n
	}
	seg := t.outboundSegmentFlags(seq, t.recvNext, p.flags, nil)
	t.mu.Unlock()

	err := t.cfg.Emit(seg)
	t.mu.Lock()
	if err == nil {
		t.stats.FINTransmits++
		if current := t.pending[seq]; current == p {
			current.lastSent = now
		}
	}
	t.mu.Unlock()
	return err
}

func (t *laneTransport) reset(now time.Time) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.stats.RSTAttempts++
	seg := t.outboundSegmentFlags(t.sendNext, t.recvNext, faketcp.FlagACK|faketcp.FlagRST, nil)
	t.mu.Unlock()

	err := t.cfg.Emit(seg)
	t.mu.Lock()
	if err == nil {
		t.stats.RSTSent++
	}
	t.closed = true
	clear(t.pending)
	clear(t.received)
	clear(t.delivered)
	t.pendingOrder = nil
	t.deliveredOrder = nil
	t.deliveredHead = 0
	t.clearSteadyIndexesLocked()
	t.mu.Unlock()
	return err
}

func (t *laneTransport) statsSnapshot() TransportStats {
	return t.statsSnapshotAt(time.Now())
}

func (t *laneTransport) statsSnapshotAt(now time.Time) TransportStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.stats
	out.Outstanding = len(t.pending)
	out.RepairQueue = t.repairCount
	out.SACKedOutstanding = t.sackedOutstanding
	var oldestPending time.Time
	for _, p := range t.pending {
		if p == nil {
			continue
		}
		if p.payload != nil {
			out.OutstandingBytes += uint64(len(p.payload))
		}
		if !p.firstSent.IsZero() && (oldestPending.IsZero() || p.firstSent.Before(oldestPending)) {
			oldestPending = p.firstSent
		}
	}
	if !oldestPending.IsZero() && !now.Before(oldestPending) {
		out.OldestOutstandingAge = now.Sub(oldestPending)
	}
	out.OutOfOrder = len(t.received)
	var oldestOOO time.Time
	for start, span := range t.received {
		if seqLT(start, span.end) {
			out.OutOfOrderBytes += uint64(span.end - start)
		}
		if !span.first.IsZero() && (oldestOOO.IsZero() || span.first.Before(oldestOOO)) {
			oldestOOO = span.first
		}
	}
	if !oldestOOO.IsZero() && !now.Before(oldestOOO) {
		out.OldestOutOfOrderAge = now.Sub(oldestOOO)
	}
	out.RepairCreditBytes = t.repairCredit
	out.SRTT = t.srtt
	out.RTO = t.rto
	out.AdvertisedWindow = t.cfg.AdvertisedWindow
	out.WindowScale = t.cfg.WindowScale
	out.WindowScaleSet = t.cfg.WindowScaleSet
	out.WriteClosed = t.localFINQueued
	out.LocalFINAcked = t.localFINAcked
	out.PeerFIN = t.peerFIN
	out.PeerRST = t.peerRST
	out.Closed = t.closed
	t.timing.apply(&out)
	return out
}

func (t *laneTransport) close() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	clear(t.pending)
	clear(t.received)
	clear(t.delivered)
	t.pendingOrder = nil
	t.deliveredOrder = nil
	t.deliveredHead = 0
	t.clearSteadyIndexesLocked()
	t.mu.Unlock()
}

type candidateTransport struct {
	old logicaltunnel.LaneRef
	cfg TransportConfig
}

type Runtime struct {
	mu sync.Mutex

	owner      *datapath.TunnelOwner
	deliver    PacketSink
	lanes      map[logicaltunnel.LaneRef]*laneTransport
	active     map[uint8]logicaltunnel.LaneRef
	candidates map[uint8]candidateTransport
	closed     bool
}

func New(owner *datapath.TunnelOwner, deliver PacketSink) (*Runtime, error) {
	if owner == nil {
		return nil, ErrTransportConfig
	}
	return &Runtime{
		owner: owner, deliver: deliver,
		lanes:      make(map[logicaltunnel.LaneRef]*laneTransport),
		active:     make(map[uint8]logicaltunnel.LaneRef),
		candidates: make(map[uint8]candidateTransport),
	}, nil
}

func (r *Runtime) AttachInitial(laneID uint8, lane *datapath.Lane, cfg TransportConfig) (datapath.TunnelLaneSnapshot, error) {
	if r == nil || lane == nil {
		return datapath.TunnelLaneSnapshot{}, ErrTransportConfig
	}
	if err := cfg.normalize(); err != nil {
		return datapath.TunnelLaneSnapshot{}, err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return datapath.TunnelLaneSnapshot{}, ErrRuntimeClosed
	}
	r.mu.Unlock()

	snapshot, err := r.owner.AttachInitial(laneID, lane)
	if err != nil {
		return datapath.TunnelLaneSnapshot{}, err
	}
	transport, err := newLaneTransport(r.owner, snapshot.Ref, r.deliver, cfg)
	if err != nil {
		_, _ = r.owner.Dormant()
		return datapath.TunnelLaneSnapshot{}, err
	}

	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		transport.close()
		_, _ = r.owner.Dormant()
		return datapath.TunnelLaneSnapshot{}, ErrRuntimeClosed
	}
	r.lanes[snapshot.Ref] = transport
	r.active[laneID] = snapshot.Ref
	r.mu.Unlock()
	return snapshot, nil
}

func (r *Runtime) BeginSameIDReplacement(old logicaltunnel.LaneRef, lane *datapath.Lane, cfg TransportConfig) error {
	if r == nil || lane == nil {
		return ErrTransportConfig
	}
	if err := cfg.normalize(); err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return ErrRuntimeClosed
	}
	if current, ok := r.active[old.ID]; !ok || current != old {
		r.mu.Unlock()
		return fmt.Errorf("%w: lane=%d generation=%d", ErrTransportMissing, old.ID, old.Generation)
	}
	if _, exists := r.candidates[old.ID]; exists {
		r.mu.Unlock()
		return datapath.ErrReplacementPending
	}
	r.mu.Unlock()

	if err := r.owner.BeginSameIDReplacement(old, lane); err != nil {
		return err
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		_ = r.owner.FailSameIDReplacement(old)
		return ErrRuntimeClosed
	}
	r.candidates[old.ID] = candidateTransport{old: old, cfg: cfg}
	r.mu.Unlock()
	return nil
}

func (r *Runtime) FailSameIDReplacement(old logicaltunnel.LaneRef) error {
	if r == nil {
		return ErrRuntimeClosed
	}
	r.mu.Lock()
	candidate, ok := r.candidates[old.ID]
	if !ok || candidate.old != old {
		r.mu.Unlock()
		return datapath.ErrReplacementNotFound
	}
	delete(r.candidates, old.ID)
	r.mu.Unlock()
	return r.owner.FailSameIDReplacement(old)
}

func (r *Runtime) PromoteSameIDReplacement(old logicaltunnel.LaneRef) (datapath.TunnelLaneSnapshot, error) {
	if r == nil {
		return datapath.TunnelLaneSnapshot{}, ErrRuntimeClosed
	}
	r.mu.Lock()
	candidate, ok := r.candidates[old.ID]
	if !ok || candidate.old != old {
		r.mu.Unlock()
		return datapath.TunnelLaneSnapshot{}, datapath.ErrReplacementNotFound
	}
	if r.closed {
		r.mu.Unlock()
		return datapath.TunnelLaneSnapshot{}, ErrRuntimeClosed
	}
	r.mu.Unlock()

	fresh, err := r.owner.PromoteSameIDReplacement(old)
	if err != nil {
		return datapath.TunnelLaneSnapshot{}, err
	}
	transport, err := newLaneTransport(r.owner, fresh.Ref, r.deliver, candidate.cfg)
	if err != nil {
		return datapath.TunnelLaneSnapshot{}, err
	}

	r.mu.Lock()
	delete(r.candidates, old.ID)
	r.lanes[fresh.Ref] = transport
	r.active[old.ID] = fresh.Ref
	r.mu.Unlock()
	return fresh, nil
}

func (r *Runtime) RetireIncarnation(ref logicaltunnel.LaneRef) error {
	if r == nil {
		return ErrRuntimeClosed
	}
	if err := r.owner.RetireIncarnation(ref); err != nil {
		return err
	}
	r.mu.Lock()
	transport := r.lanes[ref]
	delete(r.lanes, ref)
	r.mu.Unlock()
	if transport != nil {
		transport.close()
	}
	return nil
}

func (r *Runtime) CloseWrite(ref logicaltunnel.LaneRef, now time.Time) error {
	if r == nil {
		return ErrRuntimeClosed
	}
	r.mu.Lock()
	transport := r.lanes[ref]
	r.mu.Unlock()
	if transport == nil {
		return ErrTransportMissing
	}
	return transport.closeWrite(now)
}

func (r *Runtime) Reset(ref logicaltunnel.LaneRef, now time.Time) error {
	if r == nil {
		return ErrRuntimeClosed
	}
	r.mu.Lock()
	transport := r.lanes[ref]
	r.mu.Unlock()
	if transport == nil {
		return ErrTransportMissing
	}
	return transport.reset(now)
}

func (r *Runtime) steadyOwnsSegment(ref logicaltunnel.LaneRef, seg faketcp.Segment) bool {
	r.mu.Lock()
	transport := r.lanes[ref]
	r.mu.Unlock()
	if transport == nil {
		return false
	}
	if len(seg.Payload) == 0 && seg.Flags&(faketcp.FlagFIN|faketcp.FlagRST) == 0 {
		return true
	}
	transport.mu.Lock()
	start := transport.recvStart
	transport.mu.Unlock()
	return !seqLT(seg.Seq, start)
}

func (r *Runtime) Dormant() ([]logicaltunnel.LaneRef, error) {
	if r == nil {
		return nil, ErrRuntimeClosed
	}
	r.mu.Lock()
	lanes := make([]*laneTransport, 0, len(r.lanes))
	for _, transport := range r.lanes {
		lanes = append(lanes, transport)
	}
	r.mu.Unlock()
	now := time.Now()
	for _, transport := range lanes {
		_ = transport.closeWrite(now)
	}

	refs, err := r.owner.Dormant()
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	clear(r.lanes)
	clear(r.active)
	clear(r.candidates)
	r.mu.Unlock()
	for _, transport := range lanes {
		transport.close()
	}
	return refs, nil
}

func (r *Runtime) AttachServerAdmission(laneID uint8, session *realityfront.ServerAdmissionSession, assoc *faketcp.ServerAssociation, params datapath.ServerLaneParams, emit faketcp.SegmentEmitter, now time.Time) (datapath.TunnelLaneSnapshot, error) {
	if r == nil || session == nil || assoc == nil || emit == nil {
		return datapath.TunnelLaneSnapshot{}, ErrTransportConfig
	}
	lease, ok := r.owner.Lease()
	if !ok {
		return datapath.TunnelLaneSnapshot{}, datapath.ErrAdmissionHandoff
	}
	laneCfg, err := datapath.ServerLaneConfigFromLeasedAdmission(session, assoc, lease, params)
	if err != nil {
		return datapath.TunnelLaneSnapshot{}, err
	}
	lane, err := datapath.NewLane(laneCfg)
	if err != nil {
		return datapath.TunnelLaneSnapshot{}, err
	}
	flow := assoc.Flow()
	peer := assoc.PeerTCPProfile()
	window, scale, scaleSet := assoc.SteadyWindowProfile()
	cfg := TransportConfig{
		LocalIP: flow.ServerIP, PeerIP: flow.ClientIP,
		LocalPort: flow.ServerPort, PeerPort: flow.ClientPort,
		SendNext: assoc.SenderNext(), ReceiveNext: session.Boundary,
		AdvertisedWindow: window, AdvertisedWindowSet: true,
		WindowScale: scale, WindowScaleSet: scaleSet,
		InitialRTO: DefaultRepairRTO, RepairHorizon: DefaultRepairHorizon,
		SACKPermitted: peer.SACKPermitted,
		Emit:          emit,
	}
	snapshot, err := r.AttachInitial(laneID, lane, cfg)
	if err != nil {
		lane.Close()
		return datapath.TunnelLaneSnapshot{}, err
	}
	for _, early := range session.EarlyRecords {
		seg := faketcp.Segment{
			SrcIP: flow.ClientIP, DstIP: flow.ServerIP,
			SrcPort: flow.ClientPort, DstPort: flow.ServerPort,
			Seq: early.Seq, Ack: cfg.SendNext,
			Flags: faketcp.FlagACK | faketcp.FlagPSH, Window: 65535,
			Payload: append([]byte(nil), early.Payload...),
		}
		if err := r.HandleSegment(snapshot.Ref, seg, now); err != nil {
			return datapath.TunnelLaneSnapshot{}, err
		}
	}
	return snapshot, nil
}

func (r *Runtime) AttachClientAdmission(laneID uint8, session *realityfront.ClientAdmissionSession, params datapath.ClientLaneParams, cfg TransportConfig) (datapath.TunnelLaneSnapshot, error) {
	if r == nil || session == nil {
		return datapath.TunnelLaneSnapshot{}, ErrTransportConfig
	}
	laneCfg, err := datapath.ClientLaneConfigFromAdmission(session, params)
	if err != nil {
		return datapath.TunnelLaneSnapshot{}, err
	}
	lane, err := datapath.NewLane(laneCfg)
	if err != nil {
		return datapath.TunnelLaneSnapshot{}, err
	}
	snapshot, err := r.AttachInitial(laneID, lane, cfg)
	if err != nil {
		lane.Close()
		return datapath.TunnelLaneSnapshot{}, err
	}
	return snapshot, nil
}

func (r *Runtime) SendNormal(records []datapath.WireRecord, now time.Time) error {
	r.mu.Lock()
	ref, ok := r.active[1]
	transport := r.lanes[ref]
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return ErrRuntimeClosed
	}
	if !ok || transport == nil {
		return ErrTransportMissing
	}
	return transport.send(records, now)
}

func (r *Runtime) SendGame(out datapath.GameOutboundResult, now time.Time) error {
	if r == nil {
		return ErrRuntimeClosed
	}
	var errs []error
	for _, lane := range out.Lanes {
		r.mu.Lock()
		transport := r.lanes[lane.Ref]
		closed := r.closed
		r.mu.Unlock()
		if closed {
			return ErrRuntimeClosed
		}
		if transport == nil {
			errs = append(errs, fmt.Errorf("%w: lane=%d generation=%d", ErrTransportMissing, lane.Ref.ID, lane.Ref.Generation))
			continue
		}
		if err := transport.send(lane.Records, now); err != nil {
			errs = append(errs, err)
		}
	}
	for _, failure := range out.Failures {
		if failure.Err != nil {
			errs = append(errs, failure.Err)
		}
	}
	return errors.Join(errs...)
}

func (r *Runtime) PlatformWireSink(out platformflow.Outbound) error {
	now := time.Now()
	if out.IsGame {
		return r.SendGame(out.Game, now)
	}
	return r.SendNormal(out.Normal, now)
}

// SetTimingDiagnostics toggles bounded aggregate timing for one active lane and
// its record/FEC/LINK decode path. It changes observation only, never queueing.
func (r *Runtime) SetTimingDiagnostics(ref logicaltunnel.LaneRef, enabled bool) error {
	if r == nil {
		return ErrRuntimeClosed
	}
	r.mu.Lock()
	transport := r.lanes[ref]
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return ErrRuntimeClosed
	}
	if transport == nil {
		return ErrTransportMissing
	}
	transport.timing.enabled.Store(enabled)
	return r.owner.SetLaneTimingDiagnostics(ref, enabled)
}

func (r *Runtime) HandleSegment(ref logicaltunnel.LaneRef, seg faketcp.Segment, now time.Time) error {
	if r == nil {
		return ErrRuntimeClosed
	}
	r.mu.Lock()
	transport := r.lanes[ref]
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return ErrRuntimeClosed
	}
	if transport == nil {
		return ErrTransportMissing
	}
	return transport.handleSegment(seg, now)
}

func (r *Runtime) authenticatedRecordCount(ref logicaltunnel.LaneRef) (uint64, bool) {
	r.mu.Lock()
	transport := r.lanes[ref]
	r.mu.Unlock()
	if transport == nil {
		return 0, false
	}
	transport.mu.Lock()
	count := transport.stats.AuthenticatedRecords
	transport.mu.Unlock()
	return count, true
}

func (r *Runtime) HandleServerSegment(ref logicaltunnel.LaneRef, assoc *faketcp.ServerAssociation, seg faketcp.Segment, now time.Time) error {
	_, err := r.HandleServerSegmentQualified(ref, assoc, seg, now)
	return err
}

// HandleServerSegmentQualified reports whether this segment carried the first
// detached steady-state record path. Callers use the signal only to gate server
// business egress until the client has proved it owns the post-admission
// sequence space; record validation and delivery remain owned by Runtime/Lane.
func (r *Runtime) HandleServerSegmentQualified(ref logicaltunnel.LaneRef, assoc *faketcp.ServerAssociation, seg faketcp.Segment, now time.Time) (bool, error) {
	if assoc == nil {
		return false, ErrTransportConfig
	}
	if state, ok := assoc.TransitionState(); ok && state == faketcp.TransitionDetached &&
		r.steadyOwnsSegment(ref, seg) {
		before, _ := r.authenticatedRecordCount(ref)
		if err := r.HandleSegment(ref, seg, now); err != nil {
			return false, err
		}
		after, _ := r.authenticatedRecordCount(ref)
		return after.AuthenticatedRecords > before.AuthenticatedRecords, nil
	}

	result, err := assoc.HandleSegment(seg, now)
	if err != nil {
		return false, err
	}
	if result.Disposition == faketcp.RouteRecord && result.Record != nil {
		before, _ := r.authenticatedRecordCount(ref)
		if err := r.HandleSegment(ref, seg, now); err != nil {
			return false, err
		}
		after, _ := r.authenticatedRecordCount(ref)
		return after.AuthenticatedRecords > before.AuthenticatedRecords, nil
	}
	if result.AckNeeded {
		return false, r.emitForRef(ref, assoc.ACKSegment(result.Ack))
	}
	if seg.Flags&faketcp.FlagACK != 0 {
		return false, r.handleACKOnly(ref, seg, now)
	}
	return false, nil
}

func (r *Runtime) handleACKOnly(ref logicaltunnel.LaneRef, seg faketcp.Segment, now time.Time) error {
	r.mu.Lock()
	transport := r.lanes[ref]
	r.mu.Unlock()
	if transport == nil {
		return ErrTransportMissing
	}
	transport.mu.Lock()
	if !transport.matches(seg) {
		transport.mu.Unlock()
		return ErrTransportFlow
	}
	if seqLT(transport.sendNext, seg.Ack) {
		transport.mu.Unlock()
		return ErrACKRange
	}
	transport.retireACKLocked(seg.Ack, now)
	if transport.cfg.SACKPermitted && seg.SACKN > 0 {
		n := seg.SACKN
		if n > faketcp.MaxSACKBlocks {
			n = faketcp.MaxSACKBlocks
		}
		transport.applySACKLocked(seg.SACK[:n], now)
	}
	repair := transport.selectFastRepairLocked(now)
	transport.mu.Unlock()
	return transport.emitSelectedRepair(repair, now)
}

func (r *Runtime) emitForRef(ref logicaltunnel.LaneRef, seg faketcp.Segment) error {
	r.mu.Lock()
	transport := r.lanes[ref]
	r.mu.Unlock()
	if transport == nil {
		return ErrTransportMissing
	}
	return transport.cfg.Emit(seg)
}

func (r *Runtime) Tick(now time.Time) error {
	if r == nil {
		return nil
	}
	type tickLane struct {
		ref       logicaltunnel.LaneRef
		transport *laneTransport
		active    bool
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	active := make(map[logicaltunnel.LaneRef]struct{}, len(r.active))
	for _, ref := range r.active {
		active[ref] = struct{}{}
	}
	lanes := make([]tickLane, 0, len(r.lanes))
	for ref, transport := range r.lanes {
		_, isActive := active[ref]
		lanes = append(lanes, tickLane{ref: ref, transport: transport, active: isActive})
	}
	r.mu.Unlock()

	var errs []error
	for _, lane := range lanes {
		// Only the authoritative incarnation may form new timer-driven FEC
		// parity. Retiring transports keep their bounded repair tick but cannot
		// create fresh steady records after generation replacement.
		if lane.active {
			if err := lane.transport.tickHealth(now); err != nil {
				errs = append(errs, err)
			}
			records, err := r.owner.TickLane(lane.ref, now)
			if err != nil {
				if !errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) && !errors.Is(err, datapath.ErrLaneUnavailable) {
					errs = append(errs, err)
				}
			} else if len(records) != 0 {
				if err := lane.transport.send(records, now); err != nil {
					errs = append(errs, err)
				}
			}
		}
		if err := lane.transport.tick(now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Runtime) TransportStats(ref logicaltunnel.LaneRef) (TransportStats, bool) {
	return r.TransportStatsAt(ref, time.Now())
}

func (r *Runtime) TransportStatsAt(ref logicaltunnel.LaneRef, now time.Time) (TransportStats, bool) {
	if r == nil {
		return TransportStats{}, false
	}
	r.mu.Lock()
	transport := r.lanes[ref]
	r.mu.Unlock()
	if transport == nil {
		return TransportStats{}, false
	}
	return transport.statsSnapshotAt(now), true
}

func (r *Runtime) Close() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return
	}
	r.closed = true
	lanes := make([]*laneTransport, 0, len(r.lanes))
	for _, transport := range r.lanes {
		lanes = append(lanes, transport)
	}
	clear(r.lanes)
	clear(r.active)
	clear(r.candidates)
	r.mu.Unlock()
	now := time.Now()
	for _, transport := range lanes {
		_ = transport.closeWrite(now)
		transport.close()
	}
	r.owner.Close()
}

func seqLT(a, b uint32) bool { return int32(a-b) < 0 }
func seqLE(a, b uint32) bool { return a == b || seqLT(a, b) }
