package logicaltunnel

import (
	"errors"
	"fmt"
	"sort"
)

var (
	ErrLaneNotFound        = errors.New("logicaltunnel: transport lane not found")
	ErrLaneExists          = errors.New("logicaltunnel: transport lane already exists")
	ErrLaneState           = errors.New("logicaltunnel: invalid transport lane lifecycle state")
	ErrStaleLaneGeneration = errors.New("logicaltunnel: stale transport lane generation")
)

type LanePhase string

const (
	LaneCandidate LanePhase = "candidate"
	LaneActive    LanePhase = "active"
	LaneDraining  LanePhase = "draining"
)

// LaneRef fences every product transport lane incarnation. Reusing a lane ID
// after retirement always receives a newer Generation so a stale child/process
// cannot reclaim ownership after replacement or reconnect.
type LaneRef struct {
	ID         uint8
	Generation uint64
}

type LaneSnapshot struct {
	Ref      LaneRef
	Phase    LanePhase
	Replaces *LaneRef
}

type laneRecord struct {
	ref      LaneRef
	phase    LanePhase
	replaces *LaneRef
}

// LaneLifecycle is the Logical Tunnel control-plane state for product transport
// membership. It deliberately owns no FakeTCP/DTLS/LINK/FEC wire state. Each
// LaneRef represents one complete independent ADR-0011 transport lane.
//
// Normal policy uses desired=1. Game/weak-network policy uses desired=2..4.
// Dormant state is represented by zero attached lanes while desired remains the
// wake target. Planned replacement is make-before-break: create a candidate,
// prove it healthy, overlap it with the old lane, drain the old lane, retire it.
type LaneLifecycle struct {
	desired        int
	max            int
	nextGeneration uint64
	lanes          map[uint8]*laneRecord
}

func NewLaneLifecycle(desired int) (*LaneLifecycle, error) {
	if err := ValidateProductTransportLaneCount(desired); err != nil {
		return nil, err
	}
	return &LaneLifecycle{
		desired:        desired,
		max:            MaxProductPublicTransportLanes,
		nextGeneration: 1,
		lanes:          make(map[uint8]*laneRecord, MaxProductPublicTransportLanes),
	}, nil
}

func (l *LaneLifecycle) Desired() int {
	if l == nil { return 0 }
	return l.desired
}

func (l *LaneLifecycle) AttachInitial(id uint8) (LaneRef, error) {
	if l == nil { return LaneRef{}, ErrLaneState }
	if id == 0 { return LaneRef{}, ErrLaneState }
	if _, exists := l.lanes[id]; exists { return LaneRef{}, ErrLaneExists }
	if len(l.lanes) >= l.max { return LaneRef{}, ErrTransportLanes }
	ref := l.newRef(id)
	l.lanes[id] = &laneRecord{ref: ref, phase: LaneActive}
	return ref, nil
}

// BeginReplacement adds candidateID without disturbing old. The candidate may
// coexist with all healthy lanes while it performs FakeTCP + same-flow Reality
// bootstrap + DTLS + LINK health qualification. Candidate failure therefore
// never requires the old lane to be stopped.
func (l *LaneLifecycle) BeginReplacement(old LaneRef, candidateID uint8) (LaneRef, error) {
	if l == nil { return LaneRef{}, ErrLaneState }
	oldRec, err := l.current(old)
	if err != nil { return LaneRef{}, err }
	if oldRec.phase != LaneActive { return LaneRef{}, ErrLaneState }
	if candidateID == 0 || candidateID == old.ID { return LaneRef{}, ErrLaneState }
	if _, exists := l.lanes[candidateID]; exists { return LaneRef{}, ErrLaneExists }
	if len(l.lanes) >= l.max { return LaneRef{}, ErrTransportLanes }
	ref := l.newRef(candidateID)
	oldCopy := old
	l.lanes[candidateID] = &laneRecord{ref: ref, phase: LaneCandidate, replaces: &oldCopy}
	return ref, nil
}

// CandidateHealthy is the bootstrap/health barrier. Only after this transition
// may logical PacketIDs race over old + candidate for bounded overlap.
func (l *LaneLifecycle) CandidateHealthy(candidate LaneRef) error {
	rec, err := l.current(candidate)
	if err != nil { return err }
	if rec.phase != LaneCandidate || rec.replaces == nil { return ErrLaneState }
	if _, err := l.current(*rec.replaces); err != nil { return err }
	rec.phase = LaneActive
	return nil
}

// CandidateFailed removes only the unproven candidate. The healthy lane it was
// intended to replace remains active and untouched.
func (l *LaneLifecycle) CandidateFailed(candidate LaneRef) error {
	rec, err := l.current(candidate)
	if err != nil { return err }
	if rec.phase != LaneCandidate { return ErrLaneState }
	delete(l.lanes, candidate.ID)
	return nil
}

// BeginDrain stops new logical sends to old only after the replacement is
// proven active. Existing in-flight data may drain for a bounded interval.
func (l *LaneLifecycle) BeginDrain(old, replacement LaneRef) error {
	oldRec, err := l.current(old)
	if err != nil { return err }
	replacementRec, err := l.current(replacement)
	if err != nil { return err }
	if oldRec.phase != LaneActive || replacementRec.phase != LaneActive || replacementRec.replaces == nil || *replacementRec.replaces != old {
		return ErrLaneState
	}
	oldRec.phase = LaneDraining
	return nil
}

func (l *LaneLifecycle) Retire(ref LaneRef) error {
	rec, err := l.current(ref)
	if err != nil { return err }
	if rec.phase != LaneDraining { return ErrLaneState }
	delete(l.lanes, ref.ID)
	return nil
}

// Dormant closes all Transport Lanes but intentionally leaves desired policy in
// place; the Logical Tunnel identity/lease is owned elsewhere and survives.
func (l *LaneLifecycle) Dormant() []LaneRef {
	if l == nil { return nil }
	out := make([]LaneRef, 0, len(l.lanes))
	for _, rec := range l.lanes { out = append(out, rec.ref) }
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	clear(l.lanes)
	return out
}

func (l *LaneLifecycle) Snapshot() []LaneSnapshot {
	if l == nil { return nil }
	out := make([]LaneSnapshot, 0, len(l.lanes))
	for _, rec := range l.lanes {
		var replaces *LaneRef
		if rec.replaces != nil { copyRef := *rec.replaces; replaces = &copyRef }
		out = append(out, LaneSnapshot{Ref: rec.ref, Phase: rec.phase, Replaces: replaces})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Ref.ID < out[j].Ref.ID })
	return out
}

func (l *LaneLifecycle) ActiveForSend() []LaneRef {
	if l == nil { return nil }
	out := make([]LaneRef, 0, len(l.lanes))
	for _, rec := range l.lanes {
		if rec.phase == LaneActive { out = append(out, rec.ref) }
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (l *LaneLifecycle) current(ref LaneRef) (*laneRecord, error) {
	if ref.ID == 0 || ref.Generation == 0 { return nil, ErrLaneState }
	rec := l.lanes[ref.ID]
	if rec == nil { return nil, ErrLaneNotFound }
	if rec.ref.Generation != ref.Generation { return nil, fmt.Errorf("%w: lane=%d got=%d current=%d", ErrStaleLaneGeneration, ref.ID, ref.Generation, rec.ref.Generation) }
	return rec, nil
}

func (l *LaneLifecycle) newRef(id uint8) LaneRef {
	ref := LaneRef{ID: id, Generation: l.nextGeneration}
	l.nextGeneration++
	return ref
}
