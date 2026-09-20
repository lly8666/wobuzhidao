package datapath

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

var (
	ErrGameLaneMode     = errors.New("datapath: game mode requires 2..4 logical lanes")
	ErrGameLaneMismatch = errors.New("datapath: game envelope lane does not match transport lane")
)

type GameLaneRecords struct {
	Ref     logicaltunnel.LaneRef
	Records []WireRecord
}

type GameLaneFailure struct {
	Ref logicaltunnel.LaneRef
	Err error
}

type GameOutboundResult struct {
	PacketID uint64
	Lanes    []GameLaneRecords
	Failures []GameLaneFailure
}

type gameTunnelState struct {
	encoder *gamelane.Encoder
	decoder *gamelane.Decoder

	logicalOutbound uint64
	laneCopies      uint64
	delivered       uint64
	duplicates      uint64
	stale           uint64
	laneMismatches  uint64
}

// GameOutbound assigns one tunnel-wide PacketID, then races a lane-distinct
// envelope through every currently authoritative logical lane. It never creates
// a lane and never stripes different packets across lanes. Per-lane failures are
// returned in result.Failures so one failed lane does not suppress healthy
// sibling copies.
func (o *TunnelOwner) GameOutbound(packet []byte, now time.Time) (GameOutboundResult, error) {
	if o == nil {
		return GameOutboundResult{}, ErrTunnelOwnerClosed
	}

	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return GameOutboundResult{}, ErrTunnelOwnerClosed
	}
	if o.desired < 2 || o.desired > gamelane.MaxLanes {
		o.mu.Unlock()
		return GameOutboundResult{}, ErrGameLaneMode
	}
	if len(o.active) == 0 {
		o.mu.Unlock()
		return GameOutboundResult{}, ErrTunnelDormant
	}
	role := o.role
	hasLease := o.hasLease
	o.mu.Unlock()

	// Client-originated business packets must satisfy the same lease fence as
	// Normal mode before Game allocates a PacketID or touches any Lane state.
	if role == RoleClient && hasLease {
		if err := o.validateLeasedIPv4Source(packet); err != nil {
			return GameOutboundResult{}, err
		}
	}

	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return GameOutboundResult{}, ErrTunnelOwnerClosed
	}
	if o.desired < 2 || o.desired > gamelane.MaxLanes {
		o.mu.Unlock()
		return GameOutboundResult{}, ErrGameLaneMode
	}
	if len(o.active) == 0 {
		o.mu.Unlock()
		return GameOutboundResult{}, ErrTunnelDormant
	}
	state, err := o.ensureGameLocked()
	if err != nil {
		o.mu.Unlock()
		return GameOutboundResult{}, err
	}

	ids := make([]uint8, 0, len(o.active))
	for id := range o.active {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	packetID, copies, err := state.encoder.WrapCopies(packet, ids)
	if err != nil {
		o.mu.Unlock()
		return GameOutboundResult{}, err
	}
	bindings := make([]tunnelLaneBinding, len(ids))
	for i, id := range ids {
		bindings[i] = o.active[id]
	}
	state.logicalOutbound++
	state.laneCopies += uint64(len(copies))
	o.mu.Unlock()

	out := GameOutboundResult{
		PacketID: packetID,
		Lanes:    make([]GameLaneRecords, 0, len(copies)),
	}
	for i, copy := range copies {
		binding := bindings[i]
		if copy.LaneID != binding.ref.ID {
			out.Failures = append(out.Failures, GameLaneFailure{Ref: binding.ref, Err: ErrGameLaneMismatch})
			continue
		}
		records, err := binding.lane.Outbound(copy.Wire, now)
		if err != nil {
			out.Failures = append(out.Failures, GameLaneFailure{Ref: binding.ref, Err: err})
			continue
		}
		records, err = o.FenceOutbound(binding.ref, records)
		if err != nil {
			out.Failures = append(out.Failures, GameLaneFailure{Ref: binding.ref, Err: err})
			continue
		}
		out.Lanes = append(out.Lanes, GameLaneRecords{Ref: binding.ref, Records: records})
	}
	return out, nil
}

type gameInboundCandidate struct {
	wire []byte
}

// GameInboundPayload is the Game receive owner boundary. The Lane performs its
// existing record/FEC/LINK decode first; this method then validates the Game
// session and LaneID, applies the server lease source fence to the inner packet,
// and only then commits PacketID dedupe. Invalid/spoofed copies therefore cannot
// poison the shared first-arrival window.
func (o *TunnelOwner) GameInboundPayload(ref logicaltunnel.LaneRef, payload []byte, now time.Time) (InboundResult, error) {
	if o == nil {
		return InboundResult{}, ErrTunnelOwnerClosed
	}
	o.mu.Lock()
	if o.closed {
		o.mu.Unlock()
		return InboundResult{}, ErrTunnelOwnerClosed
	}
	if o.desired < 2 || o.desired > gamelane.MaxLanes {
		o.mu.Unlock()
		return InboundResult{}, ErrGameLaneMode
	}
	binding, ok := o.active[ref.ID]
	if !ok {
		o.generationDiscards++
		o.mu.Unlock()
		return InboundResult{}, fmt.Errorf("%w: lane=%d got=%d current=none", logicaltunnel.ErrStaleLaneGeneration, ref.ID, ref.Generation)
	}
	if binding.ref != ref {
		o.generationDiscards++
		current := binding.ref
		o.mu.Unlock()
		return InboundResult{}, staleGeneration(ref, current)
	}
	state, err := o.ensureGameLocked()
	if err != nil {
		o.mu.Unlock()
		return InboundResult{}, err
	}
	sessionID := state.decoder.SessionID()
	role := o.role
	lease := o.lease.Clone()
	hasLease := o.hasLease
	o.mu.Unlock()

	laneResult, err := binding.lane.InboundPayload(payload, now)
	if err != nil {
		return InboundResult{}, err
	}
	if err := o.ValidateGeneration(ref); err != nil {
		return InboundResult{}, err
	}

	out := InboundResult{
		RecordErrors: append([]error(nil), laneResult.RecordErrors...),
		PathErrors:   append([]error(nil), laneResult.PathErrors...),
	}
	candidates := make([]gameInboundCandidate, 0, len(laneResult.Datagrams))
	var leasedAddrErr error
	var leasedAddr = lease.Config
	for _, wire := range laneResult.Datagrams {
		header, inner, err := gamelane.Parse(wire)
		if err != nil {
			out.PathErrors = append(out.PathErrors, err)
			continue
		}
		if header.SessionID != sessionID {
			out.PathErrors = append(out.PathErrors, gamelane.ErrWrongSession)
			continue
		}
		if header.LaneID != ref.ID {
			out.PathErrors = append(out.PathErrors, ErrGameLaneMismatch)
			o.noteGameLaneMismatch()
			continue
		}
		if role == RoleServer && hasLease {
			leased, err := leasedAddr.LeaseIPv4()
			if err != nil {
				leasedAddrErr = err
				break
			}
			if err := logicaltunnel.ValidateIPv4Source(inner, leased); err != nil {
				out.PathErrors = append(out.PathErrors, err)
				o.noteSourceDiscard()
				continue
			}
		}
		candidates = append(candidates, gameInboundCandidate{wire: wire})
	}
	if leasedAddrErr != nil {
		return InboundResult{}, leasedAddrErr
	}

	o.mu.Lock()
	current, ok := o.active[ref.ID]
	if !ok || current.ref != ref {
		o.generationDiscards++
		o.mu.Unlock()
		if ok {
			return InboundResult{}, staleGeneration(ref, current.ref)
		}
		return InboundResult{}, fmt.Errorf("%w: lane=%d got=%d current=none", logicaltunnel.ErrStaleLaneGeneration, ref.ID, ref.Generation)
	}
	state, err = o.ensureGameLocked()
	if err != nil {
		o.mu.Unlock()
		return InboundResult{}, err
	}
	for _, candidate := range candidates {
		decoded, err := state.decoder.Add(candidate.wire)
		if errors.Is(err, gamelane.ErrReplayTooOld) {
			state.stale++
			out.PathErrors = append(out.PathErrors, err)
			continue
		}
		if err != nil {
			out.PathErrors = append(out.PathErrors, err)
			continue
		}
		if decoded.Duplicate {
			state.duplicates++
			continue
		}
		if decoded.Deliver {
			state.delivered++
			out.Datagrams = append(out.Datagrams, decoded.Payload)
		}
	}
	o.mu.Unlock()
	return out, nil
}

func (o *TunnelOwner) ensureGameLocked() (*gameTunnelState, error) {
	if o.desired < 2 || o.desired > gamelane.MaxLanes {
		return nil, ErrGameLaneMode
	}
	if !o.identified || len(o.tunnelID) != len(gamelane.SessionID{}) {
		return nil, ErrTunnelMismatch
	}
	if o.game != nil {
		return o.game, nil
	}
	var sessionID gamelane.SessionID
	copy(sessionID[:], o.tunnelID)
	encoder, err := gamelane.NewEncoder(sessionID, 1)
	if err != nil {
		return nil, err
	}
	decoder, err := gamelane.NewDecoder(sessionID, gamelane.DefaultReplayWindow)
	if err != nil {
		return nil, err
	}
	o.game = &gameTunnelState{encoder: encoder, decoder: decoder}
	return o.game, nil
}

func (o *TunnelOwner) noteGameLaneMismatch() {
	o.mu.Lock()
	if o.game != nil {
		o.game.laneMismatches++
	}
	o.mu.Unlock()
}
