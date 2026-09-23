package datapath

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

var (
	ErrTunnelOwnerClosed          = errors.New("datapath: tunnel owner is closed")
	ErrTunnelDormant              = errors.New("datapath: logical tunnel is dormant")
	ErrFlowLimit                  = errors.New("datapath: business flow registry is full")
	ErrFlowClosed                 = errors.New("datapath: business flow is closed")
	ErrNormalLaneMode             = errors.New("datapath: normal flow requires exactly one logical lane")
	ErrLaneUnavailable            = errors.New("datapath: logical lane is unavailable")
	ErrTunnelMismatch             = errors.New("datapath: lane belongs to a different logical tunnel")
	ErrReplacementPending         = errors.New("datapath: replacement candidate already exists")
	ErrReplacementNotFound        = errors.New("datapath: replacement candidate not found")
	ErrPhysicalIncarnationLimit   = errors.New("datapath: physical lane incarnation limit reached")
	ErrTransitionIncarnationLimit = errors.New("datapath: retiring/transition lane incarnation limit reached")
)

type FlowID uint64

type TunnelLaneSnapshot struct {
	Ref          logicaltunnel.LaneRef
	Role         Role
	ParityShards int
}

type TunnelOwnerStats struct {
	DesiredLanes       int
	BusinessFlows      int
	ActiveLogicalLanes int
	Candidates         int
	Retiring           int
	PhysicalLanes      int
	GenerationDiscards uint64
	SourceDiscards     uint64
	GameLogicalOutbound      uint64
	GameLogicalOutboundBytes uint64
	GameLaneCopies           uint64
	GameLaneCopyBytes        uint64
	GameDelivered            uint64
	GameDuplicates      uint64
	GameStale           uint64
	GameLaneMismatches  uint64
	Padding             TunnelPaddingStats
	Dormant             bool
	Closed             bool
}

type tunnelLaneBinding struct {
	ref  logicaltunnel.LaneRef
	lane *Lane
}

type replacementCandidate struct {
	old  logicaltunnel.LaneRef
	lane *Lane
}

type TunnelOwner struct {
	mu sync.Mutex

	lifecycle *logicaltunnel.LaneLifecycle
	desired   int
	maxFlows  int
	nextFlow  FlowID
	flows     map[FlowID]struct{}

	active     map[uint8]tunnelLaneBinding
	candidates map[uint8]replacementCandidate
	retiring   map[logicaltunnel.LaneRef]*Lane

	lease    logicaltunnel.Lease
	hasLease bool

	role       Role
	tunnelID   []byte
	identified bool

	generationDiscards uint64
	sourceDiscards     uint64
	game               *gameTunnelState
	padding            tunnelPaddingState
	closed             bool
}

func NewTunnelOwner(desiredLanes, maxFlows int) (*TunnelOwner, error) {
	lc, err := logicaltunnel.NewLaneLifecycle(desiredLanes)
	if err != nil {
		return nil, err
	}
	if maxFlows <= 0 {
		return nil, ErrFlowLimit
	}
	return &TunnelOwner{
		lifecycle:  lc,
		desired:    desiredLanes,
		maxFlows:   maxFlows,
		nextFlow:   1,
		flows:      make(map[FlowID]struct{}),
		active:     make(map[uint8]tunnelLaneBinding, desiredLanes),
		candidates: make(map[uint8]replacementCandidate, desiredLanes),
		retiring:   make(map[logicaltunnel.LaneRef]*Lane, logicaltunnel.MaxRetiringPublicTransportIncarnations),
	}, nil
}

func (o *TunnelOwner) AttachInitial(laneID uint8, lane *Lane) (TunnelLaneSnapshot, error) {
	if lane == nil || !logicaltunnel.ValidProductLaneID(laneID) {
		return TunnelLaneSnapshot{}, logicaltunnel.ErrLaneState
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return TunnelLaneSnapshot{}, ErrTunnelOwnerClosed
	}
	if o.desired == 1 && laneID != 1 {
		return TunnelLaneSnapshot{}, ErrNormalLaneMode
	}
	if len(o.active) >= o.desired {
		return TunnelLaneSnapshot{}, logicaltunnel.ErrTransportLanes
	}
	if o.physicalLocked() >= logicaltunnel.MaxConcurrentPublicTransportIncarnations {
		return TunnelLaneSnapshot{}, ErrPhysicalIncarnationLimit
	}
	if err := o.checkLaneIdentityLocked(lane); err != nil {
		return TunnelLaneSnapshot{}, err
	}
	ref, err := o.lifecycle.AttachInitial(laneID)
	if err != nil {
		return TunnelLaneSnapshot{}, err
	}
	o.active[laneID] = tunnelLaneBinding{ref: ref, lane: lane}
	return snapshotFor(ref, lane), nil
}

func (o *TunnelOwner) OpenFlow() (*BusinessFlow, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return nil, ErrTunnelOwnerClosed
	}
	if len(o.flows) >= o.maxFlows || o.nextFlow == 0 {
		return nil, ErrFlowLimit
	}
	id := o.nextFlow
	o.nextFlow++
	o.flows[id] = struct{}{}
	return &BusinessFlow{owner: o, id: id}, nil
}

type BusinessFlow struct {
	owner *TunnelOwner
	id    FlowID
}

func (f *BusinessFlow) ID() FlowID {
	if f == nil {
		return 0
	}
	return f.id
}

func (f *BusinessFlow) Outbound(packet []byte, now time.Time) ([]WireRecord, error) {
	if f == nil || f.owner == nil || f.id == 0 {
		return nil, ErrFlowClosed
	}
	binding, err := f.owner.normalBinding(f.id)
	if err != nil {
		return nil, err
	}
	if binding.lane.Config().Role == RoleClient {
		if err := f.owner.validateLeasedIPv4Source(packet); err != nil {
			return nil, err
		}
	}
	selector := f.owner.paddingSelectorForPayload(packet, now)
	var records []WireRecord
	if selector == nil {
		records, err = binding.lane.Outbound(packet, now)
	} else {
		records, err = binding.lane.outboundWithPaddingSelector(packet, now, selector)
	}
	if err != nil {
		return nil, err
	}
	return f.owner.FenceOutbound(binding.ref, records)
}

// NormalOutbound is the owner-level Normal-mode send boundary used by a
// platform packet router such as the Linux shared TUN. It does not create a
// BusinessFlow and therefore does not turn inner application flows into
// transport-lane membership. RoleClient leased owners still enforce the same
// source==lease fence before touching Lane state.
func (o *TunnelOwner) NormalOutbound(packet []byte, now time.Time) ([]WireRecord, error) {
	if o == nil {
		return nil, ErrTunnelOwnerClosed
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil, ErrTunnelOwnerClosed
	}
	if o.desired != 1 {
		o.mu.Unlock()
		return nil, ErrNormalLaneMode
	}
	binding, ok := o.active[1]
	if !ok {
		dormant := len(o.active) == 0
		o.mu.Unlock()
		if dormant {
			return nil, ErrTunnelDormant
		}
		return nil, ErrLaneUnavailable
	}
	o.mu.Unlock()

	if binding.lane.Config().Role == RoleClient {
		if err := o.validateLeasedIPv4Source(packet); err != nil {
			return nil, err
		}
	}
	selector := o.paddingSelectorForPayload(packet, now)
	var (
		records []WireRecord
		err     error
	)
	if selector == nil {
		records, err = binding.lane.Outbound(packet, now)
	} else {
		records, err = binding.lane.outboundWithPaddingSelector(packet, now, selector)
	}
	if err != nil {
		return nil, err
	}
	return o.FenceOutbound(binding.ref, records)
}

func (f *BusinessFlow) Close() error {
	if f == nil || f.owner == nil || f.id == 0 {
		return ErrFlowClosed
	}
	return f.owner.closeFlow(f.id)
}

func (o *TunnelOwner) closeFlow(id FlowID) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return ErrTunnelOwnerClosed
	}
	if _, ok := o.flows[id]; !ok {
		return ErrFlowClosed
	}
	delete(o.flows, id)
	return nil
}

func (o *TunnelOwner) normalBinding(flowID FlowID) (tunnelLaneBinding, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return tunnelLaneBinding{}, ErrTunnelOwnerClosed
	}
	if _, ok := o.flows[flowID]; !ok {
		return tunnelLaneBinding{}, ErrFlowClosed
	}
	if o.desired != 1 {
		return tunnelLaneBinding{}, ErrNormalLaneMode
	}
	binding, ok := o.active[1]
	if !ok {
		if len(o.active) == 0 {
			return tunnelLaneBinding{}, ErrTunnelDormant
		}
		return tunnelLaneBinding{}, ErrLaneUnavailable
	}
	return binding, nil
}

func (o *TunnelOwner) BeginSameIDReplacement(old logicaltunnel.LaneRef, candidate *Lane) error {
	if candidate == nil || !logicaltunnel.ValidProductLaneID(old.ID) || old.Generation == 0 {
		return logicaltunnel.ErrLaneState
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return ErrTunnelOwnerClosed
	}
	current, ok := o.active[old.ID]
	if !ok {
		return ErrLaneUnavailable
	}
	if current.ref != old {
		return staleGeneration(old, current.ref)
	}
	if _, ok := o.candidates[old.ID]; ok {
		return ErrReplacementPending
	}
	if o.physicalLocked() >= logicaltunnel.MaxConcurrentPublicTransportIncarnations {
		return ErrPhysicalIncarnationLimit
	}
	if len(o.candidates)+len(o.retiring) >= logicaltunnel.MaxRetiringPublicTransportIncarnations {
		return ErrTransitionIncarnationLimit
	}
	if err := o.checkLaneIdentityLocked(candidate); err != nil {
		return err
	}
	o.candidates[old.ID] = replacementCandidate{old: old, lane: candidate}
	return nil
}

func (o *TunnelOwner) FailSameIDReplacement(old logicaltunnel.LaneRef) error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return ErrTunnelOwnerClosed
	}
	candidate, ok := o.candidates[old.ID]
	if !ok || candidate.old != old {
		o.mu.Unlock()
		return ErrReplacementNotFound
	}
	delete(o.candidates, old.ID)
	o.mu.Unlock()
	candidate.lane.Close()
	return nil
}

func (o *TunnelOwner) PromoteSameIDReplacement(old logicaltunnel.LaneRef) (TunnelLaneSnapshot, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return TunnelLaneSnapshot{}, ErrTunnelOwnerClosed
	}
	current, ok := o.active[old.ID]
	if !ok {
		return TunnelLaneSnapshot{}, ErrLaneUnavailable
	}
	if current.ref != old {
		return TunnelLaneSnapshot{}, staleGeneration(old, current.ref)
	}
	candidate, ok := o.candidates[old.ID]
	if !ok || candidate.old != old {
		return TunnelLaneSnapshot{}, ErrReplacementNotFound
	}
	fresh, err := o.lifecycle.PromoteSameIDReplacement(old)
	if err != nil {
		return TunnelLaneSnapshot{}, err
	}
	o.active[old.ID] = tunnelLaneBinding{ref: fresh, lane: candidate.lane}
	delete(o.candidates, old.ID)
	o.retiring[old] = current.lane
	return snapshotFor(fresh, candidate.lane), nil
}

func (o *TunnelOwner) RetireIncarnation(ref logicaltunnel.LaneRef) error {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return ErrTunnelOwnerClosed
	}
	lane := o.retiring[ref]
	if lane == nil {
		o.mu.Unlock()
		return ErrLaneUnavailable
	}
	delete(o.retiring, ref)
	o.mu.Unlock()
	lane.Close()
	return nil
}

func (o *TunnelOwner) FenceOutbound(ref logicaltunnel.LaneRef, records []WireRecord) ([]WireRecord, error) {
	if err := o.ValidateGeneration(ref); err != nil {
		return nil, err
	}
	return records, nil
}

// SetLaneTimingDiagnostics applies observation only to the currently
// authoritative generation. Late generations cannot enable work on a new lane.
func (o *TunnelOwner) SetLaneTimingDiagnostics(ref logicaltunnel.LaneRef, enabled bool) error {
	if o == nil {
		return ErrTunnelOwnerClosed
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return ErrTunnelOwnerClosed
	}
	binding, ok := o.active[ref.ID]
	if !ok {
		o.mu.Unlock()
		return ErrLaneUnavailable
	}
	if binding.ref != ref {
		current := binding.ref
		o.mu.Unlock()
		return staleGeneration(ref, current)
	}
	lane := binding.lane
	o.mu.Unlock()
	lane.SetTimingDiagnostics(enabled)
	return nil
}

// InboundPayload is the owner boundary for one authoritative Lane incarnation.
// It generation-fences late transport work before exposing decoded business
// packets. On the server side, a leased owner additionally drops every inner
// datagram whose IPv4 source is not the Logical Tunnel lease.
func (o *TunnelOwner) InboundPayload(ref logicaltunnel.LaneRef, payload []byte, now time.Time) (InboundResult, error) {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return InboundResult{}, ErrTunnelOwnerClosed
	}
	binding, ok := o.active[ref.ID]
	if !ok {
		o.mu.Unlock()
		return InboundResult{}, fmt.Errorf("%w: lane=%d got=%d current=none", logicaltunnel.ErrStaleLaneGeneration, ref.ID, ref.Generation)
	}
	if binding.ref != ref {
		current := binding.ref
		o.mu.Unlock()
		return InboundResult{}, staleGeneration(ref, current)
	}
	role := o.role
	lease := o.lease.Clone()
	hasLease := o.hasLease
	startupEnabled := o.padding.policy.TLSStartupOnly
	o.mu.Unlock()

	result, err := binding.lane.InboundPayload(payload, now)
	if err != nil {
		return InboundResult{}, err
	}
	if err := o.ValidateGeneration(ref); err != nil {
		return InboundResult{}, err
	}
	if role != RoleServer || !hasLease {
		if startupEnabled {
			o.observeStartupInbound(result.Datagrams, now)
		}
		return result, nil
	}
	leased, err := lease.Config.LeaseIPv4()
	if err != nil {
		return InboundResult{}, err
	}
	kept := result.Datagrams[:0]
	for _, packet := range result.Datagrams {
		if err := logicaltunnel.ValidateIPv4Source(packet, leased); err != nil {
			result.PathErrors = append(result.PathErrors, err)
			o.noteSourceDiscard()
			continue
		}
		kept = append(kept, packet)
	}
	result.Datagrams = kept
	if startupEnabled {
		o.observeStartupInbound(result.Datagrams, now)
	}
	return result, nil
}

func (o *TunnelOwner) validateLeasedIPv4Source(packet []byte) error {
	o.mu.Lock()
	if !o.hasLease {
		o.mu.Unlock()
		return nil
	}
	lease := o.lease.Clone()
	o.mu.Unlock()

	leased, err := lease.Config.LeaseIPv4()
	if err != nil {
		return err
	}
	if err := logicaltunnel.ValidateIPv4Source(packet, leased); err != nil {
		o.noteSourceDiscard()
		return err
	}
	return nil
}

func (o *TunnelOwner) noteSourceDiscard() {
	o.mu.Lock()
	o.sourceDiscards++
	o.mu.Unlock()
}

func (o *TunnelOwner) ValidateGeneration(ref logicaltunnel.LaneRef) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed {
		return ErrTunnelOwnerClosed
	}
	current, ok := o.active[ref.ID]
	if ok && current.ref == ref {
		return nil
	}
	o.generationDiscards++
	if ok {
		return staleGeneration(ref, current.ref)
	}
	return fmt.Errorf("%w: lane=%d got=%d current=none", logicaltunnel.ErrStaleLaneGeneration, ref.ID, ref.Generation)
}

func (o *TunnelOwner) Dormant() ([]logicaltunnel.LaneRef, error) {
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil, ErrTunnelOwnerClosed
	}
	refs := o.lifecycle.Dormant()
	lanes := o.collectTransportLocked()
	clear(o.active)
	clear(o.candidates)
	clear(o.retiring)
	o.mu.Unlock()
	closeLaneSet(lanes)
	return refs, nil
}

func (o *TunnelOwner) Close() {
	if o == nil {
		return
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return
	}
	o.lifecycle.Dormant()
	lanes := o.collectTransportLocked()
	clear(o.active)
	clear(o.candidates)
	clear(o.retiring)
	clear(o.flows)
	o.game = nil
	// Release observation state while retaining lifetime diagnostic counters.
	o.padding.startup.flows = nil
	o.padding.startup.active.Init()
	o.padding.startup.retained.Init()
	o.closed = true
	o.mu.Unlock()
	closeLaneSet(lanes)
}

// TickLane advances timer-owned lane state for one authoritative incarnation.
// It flushes due partial FEC parity without adding useful-payload padding credit,
// then expires receive-side FEC/LINK state. Returned records remain generation-
// fenced so a concurrent replacement cannot emit newly formed parity on a stale
// incarnation.
func (o *TunnelOwner) TickLane(ref logicaltunnel.LaneRef, now time.Time) ([]WireRecord, error) {
	if o == nil {
		return nil, ErrTunnelOwnerClosed
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return nil, ErrTunnelOwnerClosed
	}
	binding, ok := o.active[ref.ID]
	if !ok {
		o.mu.Unlock()
		return nil, ErrLaneUnavailable
	}
	if binding.ref != ref {
		current := binding.ref
		o.mu.Unlock()
		return nil, staleGeneration(ref, current)
	}
	if o.padding.policy.TLSStartupOnly {
		o.padding.startup.expire(now)
	}
	paddingEnabled := o.padding.policy.Enabled && !o.padding.policy.TLSStartupOnly
	o.mu.Unlock()

	var (
		records []WireRecord
		flushErr error
	)
	if paddingEnabled {
		records, flushErr = binding.lane.flushDue(now, o.allocateRecordPadding)
	} else {
		records, flushErr = binding.lane.FlushDue(now)
	}
	expireErr := binding.lane.Expire(now)
	if flushErr != nil {
		return nil, errors.Join(flushErr, expireErr)
	}
	records, fenceErr := o.FenceOutbound(ref, records)
	return records, errors.Join(expireErr, fenceErr)
}

// LaneStats returns the current authoritative lane snapshot for exact
// measurement/diagnostic use. It does not expose the Lane pointer or mutate
// owner/lifecycle state.
func (o *TunnelOwner) LaneStats(ref logicaltunnel.LaneRef) (LaneStats, bool) {
	if o == nil {
		return LaneStats{}, false
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return LaneStats{}, false
	}
	binding, ok := o.active[ref.ID]
	if !ok || binding.ref != ref {
		o.mu.Unlock()
		return LaneStats{}, false
	}
	lane := binding.lane
	o.mu.Unlock()
	stats := lane.Stats()
	if err := o.ValidateGeneration(ref); err != nil {
		return LaneStats{}, false
	}
	return stats, true
}

func (o *TunnelOwner) ActiveLanes() []TunnelLaneSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := make([]TunnelLaneSnapshot, 0, len(o.active))
	for _, binding := range o.active {
		out = append(out, snapshotFor(binding.ref, binding.lane))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref.ID < out[j].Ref.ID })
	return out
}

func (o *TunnelOwner) Stats() TunnelOwnerStats {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := TunnelOwnerStats{
		DesiredLanes:       o.desired,
		BusinessFlows:      len(o.flows),
		ActiveLogicalLanes: len(o.active),
		Candidates:         len(o.candidates),
		Retiring:           len(o.retiring),
		PhysicalLanes:      o.physicalLocked(),
		GenerationDiscards: o.generationDiscards,
		SourceDiscards:     o.sourceDiscards,
		Dormant:            len(o.active) == 0,
		Closed:             o.closed,
	}
	if o.game != nil {
		out.GameLogicalOutbound = o.game.logicalOutbound
		out.GameLogicalOutboundBytes = o.game.logicalOutboundBytes
		out.GameLaneCopies = o.game.laneCopies
		out.GameLaneCopyBytes = o.game.laneCopyBytes
		out.GameDelivered = o.game.delivered
		out.GameDuplicates = o.game.duplicates
		out.GameStale = o.game.stale
		out.GameLaneMismatches = o.game.laneMismatches
	}
	out.Padding = o.paddingStatsLocked()
	return out
}

func (o *TunnelOwner) physicalLocked() int {
	return len(o.active) + len(o.candidates) + len(o.retiring)
}

func (o *TunnelOwner) checkLaneIdentityLocked(lane *Lane) error {
	cfg := lane.Config()
	if o.hasLease && !bytes.Equal(cfg.TunnelID, o.lease.Config.TunnelID.Bytes()) {
		return ErrTunnelMismatch
	}
	if !o.identified {
		o.role = cfg.Role
		o.tunnelID = append([]byte(nil), cfg.TunnelID...)
		o.identified = true
		return nil
	}
	if cfg.Role != o.role || !bytes.Equal(cfg.TunnelID, o.tunnelID) {
		return ErrTunnelMismatch
	}
	return nil
}

func (o *TunnelOwner) collectTransportLocked() map[*Lane]struct{} {
	out := make(map[*Lane]struct{}, o.physicalLocked())
	for _, binding := range o.active {
		out[binding.lane] = struct{}{}
	}
	for _, candidate := range o.candidates {
		out[candidate.lane] = struct{}{}
	}
	for _, lane := range o.retiring {
		out[lane] = struct{}{}
	}
	return out
}

func closeLaneSet(lanes map[*Lane]struct{}) {
	for lane := range lanes {
		lane.Close()
	}
}

func snapshotFor(ref logicaltunnel.LaneRef, lane *Lane) TunnelLaneSnapshot {
	cfg := lane.Config()
	return TunnelLaneSnapshot{Ref: ref, Role: cfg.Role, ParityShards: cfg.ParityShards}
}

func staleGeneration(got, current logicaltunnel.LaneRef) error {
	return fmt.Errorf("%w: lane=%d got=%d current=%d", logicaltunnel.ErrStaleLaneGeneration, got.ID, got.Generation, current.Generation)
}
