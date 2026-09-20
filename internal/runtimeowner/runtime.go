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
	DefaultRepairRTO       = time.Second
	DefaultRepairHorizon   = 3 * time.Second
)

var (
	ErrRuntimeClosed     = errors.New("runtimeowner: runtime is closed")
	ErrTransportConfig   = errors.New("runtimeowner: invalid transport config")
	ErrTransportMissing  = errors.New("runtimeowner: lane transport is missing")
	ErrTransportFlow     = errors.New("runtimeowner: segment does not match lane flow")
	ErrACKRange          = errors.New("runtimeowner: ACK exceeds steady send sequence")
	ErrPayloadConflict   = errors.New("runtimeowner: same sequence has conflicting payload")
	ErrOutstandingBounds = errors.New("runtimeowner: outstanding record bound reached")
)

type PacketSink func(packets [][]byte, now time.Time) error

type TransportConfig struct {
	LocalIP  [4]byte
	PeerIP   [4]byte
	LocalPort uint16
	PeerPort  uint16

	SendNext    uint32
	ReceiveNext uint32

	InitialRTO    time.Duration
	RepairHorizon time.Duration
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
	return nil
}

type pendingRecord struct {
	seq       uint32
	end       uint32
	payload   []byte
	firstSent time.Time
	lastSent  time.Time
	retries   uint32
}

type receiveSpan struct {
	end   uint32
	first time.Time
}

type deliveredMark struct {
	end  uint32
	hash [32]byte
}

type TransportStats struct {
	FreshSent         uint64
	Retransmitted     uint64
	Acked             uint64
	Abandoned         uint64
	Received          uint64
	Duplicates        uint64
	LateFirstArrival  uint64
	ForgivenGaps      uint64
	RecordErrors      uint64
	PathErrors        uint64
	PeakOutstanding   int
	Outstanding       int
	OutOfOrder        int
	Closed            bool
}

type laneTransport struct {
	mu sync.Mutex

	owner   *datapath.TunnelOwner
	ref     logicaltunnel.LaneRef
	deliver PacketSink
	cfg     TransportConfig

	sendNext uint32
	recvNext uint32

	pending      map[uint32]*pendingRecord
	pendingOrder []uint32
	received     map[uint32]receiveSpan

	delivered      map[uint32]deliveredMark
	deliveredOrder []uint32
	deliveredHead  int

	stats  TransportStats
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
		sendNext: cfg.SendNext, recvNext: cfg.ReceiveNext,
		pending: make(map[uint32]*pendingRecord, MaxOutstandingRecords),
		received: make(map[uint32]receiveSpan, MaxOutstandingRecords),
		delivered: make(map[uint32]deliveredMark, MaxOutstandingRecords),
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
	return faketcp.Segment{
		SrcIP: t.cfg.LocalIP, DstIP: t.cfg.PeerIP,
		SrcPort: t.cfg.LocalPort, DstPort: t.cfg.PeerPort,
		Seq: seq, Ack: ack, Flags: flags, Window: 65535,
		Payload: append([]byte(nil), payload...),
	}
}

func (t *laneTransport) send(records []datapath.WireRecord, now time.Time) error {
	for _, record := range records {
		if len(record.Wire) == 0 {
			continue
		}
		t.mu.Lock()
		if t.closed {
			t.mu.Unlock()
			return ErrRuntimeClosed
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
			seq: seq, end: end, payload: append([]byte(nil), record.Wire...),
			firstSent: now, lastSent: now,
		}
		t.pending[seq] = p
		t.pendingOrder = append(t.pendingOrder, seq)
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
				delete(t.pending, seq)
				t.stats.Abandoned++
			}
			t.mu.Unlock()
			return err
		}
	}
	return nil
}

func (t *laneTransport) handleSegment(seg faketcp.Segment, now time.Time) error {
	if !t.matches(seg) {
		return ErrTransportFlow
	}

	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return ErrRuntimeClosed
	}
	if seg.Flags&faketcp.FlagACK != 0 {
		if seqLT(t.sendNext, seg.Ack) {
			t.mu.Unlock()
			return ErrACKRange
		}
		t.retireACKLocked(seg.Ack)
	}
	if len(seg.Payload) == 0 {
		t.mu.Unlock()
		return nil
	}

	deliver, err := t.acceptPayloadLocked(seg.Seq, seg.Payload, now)
	ack := t.recvNext
	ackSeg := t.outboundSegment(t.sendNext, ack, nil)
	t.mu.Unlock()
	if err != nil {
		return err
	}

	if deliver {
		var result datapath.InboundResult
		stats := t.owner.Stats()
		if stats.DesiredLanes == 1 {
			result, err = t.owner.InboundPayload(t.ref, seg.Payload, now)
		} else {
			result, err = t.owner.GameInboundPayload(t.ref, seg.Payload, now)
		}
		if err != nil {
			return err
		}
		t.mu.Lock()
		t.stats.RecordErrors += uint64(len(result.RecordErrors))
		t.stats.PathErrors += uint64(len(result.PathErrors))
		t.mu.Unlock()
		if len(result.Datagrams) != 0 && t.deliver != nil {
			if err := t.deliver(result.Datagrams, now); err != nil {
				return err
			}
		}
	}
	return t.cfg.Emit(ackSeg)
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
		}
		if len(t.received) > MaxOutstandingRecords {
			t.forgiveGapLocked(now, true)
		}
	}
	t.rememberDeliveredLocked(seq, deliveredMark{end: end, hash: hash})
	t.stats.Received++
	return true, nil
}

func (t *laneTransport) rememberDeliveredLocked(seq uint32, mark deliveredMark) {
	t.delivered[seq] = mark
	t.deliveredOrder = append(t.deliveredOrder, seq)
	for len(t.delivered) > MaxOutstandingRecords && t.deliveredHead < len(t.deliveredOrder) {
		old := t.deliveredOrder[t.deliveredHead]
		t.deliveredHead++
		delete(t.delivered, old)
	}
	if t.deliveredHead >= MaxOutstandingRecords && t.deliveredHead*2 >= len(t.deliveredOrder) {
		t.deliveredOrder = append([]uint32(nil), t.deliveredOrder[t.deliveredHead:]...)
		t.deliveredHead = 0
	}
}

func (t *laneTransport) advanceReceiveLocked() {
	for {
		span, ok := t.received[t.recvNext]
		if !ok {
			return
		}
		delete(t.received, t.recvNext)
		t.recvNext = span.end
	}
}

func (t *laneTransport) forgiveGapLocked(now time.Time, force bool) bool {
	var (
		bestSeq uint32
		best receiveSpan
		found bool
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
	t.advanceReceiveLocked()
	t.stats.ForgivenGaps++
	return true
}

func (t *laneTransport) retireACKLocked(ack uint32) {
	for _, seq := range t.pendingOrder {
		p := t.pending[seq]
		if p == nil {
			continue
		}
		if !seqLE(p.end, ack) {
			continue
		}
		delete(t.pending, seq)
		t.stats.Acked++
	}
	t.compactPendingOrderLocked()
}

func (t *laneTransport) abandonOldestLocked() bool {
	for _, seq := range t.pendingOrder {
		if _, ok := t.pending[seq]; !ok {
			continue
		}
		delete(t.pending, seq)
		t.stats.Abandoned++
		t.compactPendingOrderLocked()
		return true
	}
	return false
}

func (t *laneTransport) compactPendingOrderLocked() {
	n := 0
	for _, seq := range t.pendingOrder {
		if _, ok := t.pending[seq]; ok {
			t.pendingOrder[n] = seq
			n++
		}
	}
	t.pendingOrder = t.pendingOrder[:n]
}

func (t *laneTransport) tick(now time.Time) error {
	var emit *faketcp.Segment
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}

	for _, seq := range t.pendingOrder {
		p := t.pending[seq]
		if p == nil {
			continue
		}
		if now.Sub(p.firstSent) >= t.cfg.RepairHorizon {
			delete(t.pending, seq)
			t.stats.Abandoned++
			continue
		}
		if now.Sub(p.lastSent) >= t.cfg.InitialRTO {
			p.lastSent = now
			p.retries++
			t.stats.Retransmitted++
			seg := t.outboundSegment(p.seq, t.recvNext, p.payload)
			emit = &seg
			break
		}
	}
	t.compactPendingOrderLocked()
	for t.forgiveGapLocked(now, false) {
		seg := t.outboundSegment(t.sendNext, t.recvNext, nil)
		emit = &seg
		break
	}
	t.mu.Unlock()

	if emit != nil {
		return t.cfg.Emit(*emit)
	}
	return nil
}

func (t *laneTransport) statsSnapshot() TransportStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.stats
	out.Outstanding = len(t.pending)
	out.OutOfOrder = len(t.received)
	out.Closed = t.closed
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
		lanes: make(map[logicaltunnel.LaneRef]*laneTransport),
		active: make(map[uint8]logicaltunnel.LaneRef),
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

func (r *Runtime) Dormant() ([]logicaltunnel.LaneRef, error) {
	if r == nil {
		return nil, ErrRuntimeClosed
	}
	refs, err := r.owner.Dormant()
	if err != nil {
		return nil, err
	}
	r.mu.Lock()
	lanes := make([]*laneTransport, 0, len(r.lanes))
	for _, transport := range r.lanes {
		lanes = append(lanes, transport)
	}
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
	cfg := TransportConfig{
		LocalIP: flow.ServerIP, PeerIP: flow.ClientIP,
		LocalPort: flow.ServerPort, PeerPort: flow.ClientPort,
		SendNext: assoc.SenderNext(), ReceiveNext: session.Boundary,
		InitialRTO: DefaultRepairRTO, RepairHorizon: DefaultRepairHorizon,
		Emit: emit,
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
	result, err := assoc.HandleSegment(seg, now)
	if err != nil {
		return false, err
	}
	if result.Disposition == faketcp.RouteRecord && result.Record != nil {
		if err := r.HandleSegment(ref, seg, now); err != nil {
			return false, err
		}
		return true, nil
	}
	if result.AckNeeded {
		return false, r.emitForRef(ref, assoc.ACKSegment(result.Ack))
	}
	if seg.Flags&faketcp.FlagACK != 0 {
		return false, r.handleACKOnly(ref, seg)
	}
	return false, nil
}

func (r *Runtime) handleACKOnly(ref logicaltunnel.LaneRef, seg faketcp.Segment) error {
	r.mu.Lock()
	transport := r.lanes[ref]
	r.mu.Unlock()
	if transport == nil {
		return ErrTransportMissing
	}
	transport.mu.Lock()
	defer transport.mu.Unlock()
	if !transport.matches(seg) {
		return ErrTransportFlow
	}
	if seqLT(transport.sendNext, seg.Ack) {
		return ErrACKRange
	}
	transport.retireACKLocked(seg.Ack)
	return nil
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
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	lanes := make([]*laneTransport, 0, len(r.lanes))
	for _, transport := range r.lanes {
		lanes = append(lanes, transport)
	}
	r.mu.Unlock()

	var errs []error
	for _, transport := range lanes {
		if err := transport.tick(now); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (r *Runtime) TransportStats(ref logicaltunnel.LaneRef) (TransportStats, bool) {
	if r == nil {
		return TransportStats{}, false
	}
	r.mu.Lock()
	transport := r.lanes[ref]
	r.mu.Unlock()
	if transport == nil {
		return TransportStats{}, false
	}
	return transport.statsSnapshot(), true
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
	for _, transport := range lanes {
		transport.close()
	}
	r.owner.Close()
}

func seqLT(a, b uint32) bool { return int32(a-b) < 0 }
func seqLE(a, b uint32) bool { return a == b || seqLT(a, b) }
