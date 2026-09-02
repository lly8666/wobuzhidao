package windowsruntime

import (
	"errors"
	"fmt"
	"sort"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func cloneLanePlans(in map[int]LanePlan) map[int]LanePlan {
	out := make(map[int]LanePlan, len(in))
	for id, plan := range in {
		out[id] = plan
	}
	return out
}

func gameTargetsFromPlans(plans map[int]LanePlan) ([]gamelane.LaneTarget, error) {
	ids := make([]int, 0, len(plans))
	for id := range plans {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	out := make([]gamelane.LaneTarget, 0, len(ids))
	for _, id := range ids {
		addr, err := LaneGameTarget(plans[id])
		if err != nil {
			return nil, err
		}
		out = append(out, gamelane.LaneTarget{ID: uint8(id), Address: addr})
	}
	return out, nil
}

func lifecycleRefForID(l *logicaltunnel.LaneLifecycle, id int) (logicaltunnel.LaneRef, error) {
	if l == nil {
		return logicaltunnel.LaneRef{}, errors.New("Logical Tunnel lifecycle is unavailable")
	}
	for _, snap := range l.Snapshot() {
		if int(snap.Ref.ID) == id {
			return snap.Ref, nil
		}
	}
	return logicaltunnel.LaneRef{}, fmt.Errorf("logical lane %d is not active", id)
}

func (c *Controller) bootstrapRuntimeLane(profile Profile, base Underlay, expected logicaltunnel.TunnelConfig, laneID, slot int, candidate bool) (LanePlan, Process, error) {
	underlay := base
	underlay.SourcePort = nextFakeTCPSourcePort()
	var bootstrap LaneBootstrap
	var err error
	if candidate {
		bootstrap, err = BuildCandidateLaneBootstrapSlot(profile, underlay, laneID, slot)
	} else {
		bootstrap, err = BuildLaneBootstrap(profile, underlay, laneID)
	}
	if err != nil {
		return LanePlan{}, nil, err
	}
	if err := c.tickets.Clear(bootstrap.TicketPath); err != nil {
		return LanePlan{}, nil, fmt.Errorf("clear lane %d Reality ticket: %w", laneID, err)
	}
	if err := c.tickets.Clear(bootstrap.TunnelConfigPath); err != nil {
		return LanePlan{}, nil, fmt.Errorf("clear lane %d tunnel config: %w", laneID, err)
	}
	proc, err := c.runner.Start(bootstrap.FakeTCP)
	if err != nil {
		return LanePlan{}, nil, fmt.Errorf("start lane %d same-flow FakeTCP: %w", laneID, err)
	}
	owned := true
	defer func() {
		if owned {
			_ = proc.Stop()
		}
	}()
	if err := waitProcessMarker(fmt.Sprintf("lane %d Reality bootstrap", laneID), proc, singleFlowBootstrapReadyMarker, singleFlowBootstrapWait); err != nil {
		return LanePlan{}, nil, err
	}
	ticket, err := c.tickets.Read(bootstrap.TicketPath)
	if err != nil {
		return LanePlan{}, nil, fmt.Errorf("read lane %d Reality ticket: %w", laneID, err)
	}
	raw, err := c.tickets.Read(bootstrap.TunnelConfigPath)
	if err != nil {
		return LanePlan{}, nil, fmt.Errorf("read lane %d tunnel config: %w", laneID, err)
	}
	cfg, err := decodeAuthenticatedTunnelConfig(raw)
	if err != nil {
		return LanePlan{}, nil, err
	}
	bootstrap.Ticket = ticket
	bootstrap.TunnelConfig = cfg
	if err := bootstrap.ValidateAuthenticated(&expected); err != nil {
		return LanePlan{}, nil, err
	}
	var plan LanePlan
	if candidate {
		plan, err = BuildCandidateLanePlanSlot(profile, bootstrap, slot)
	} else {
		plan, err = BuildAuthenticatedLanePlan(profile, bootstrap)
	}
	if err != nil {
		return LanePlan{}, nil, err
	}
	owned = false
	return plan, proc, nil
}

// Dormant removes every public Transport Lane while retaining the authenticated
// Logical Tunnel, shared Game process, one TUN/NAT context, IPv6 kill-switch and
// routes. Game is first switched to an empty target set, so new inner packets are
// locally dropped while no public lane exists. There is no ordinary-TCP fallback.
func (c *Controller) Dormant() error {
	c.mu.Lock()
	if c.state != RuntimeConnected {
		state := c.state
		c.mu.Unlock()
		return fmt.Errorf("Windows runtime cannot enter dormant while %s", state)
	}
	c.state = RuntimeDisconnecting
	control := c.gameControl
	plans := cloneLanePlans(c.lanePlans)
	lifecycle := c.lifecycle
	c.mu.Unlock()

	if err := setGameLaneTargets(control, nil, gameControlTimeout); err != nil {
		c.mu.Lock()
		c.state = RuntimeConnected
		c.mu.Unlock()
		return fmt.Errorf("enter dormant Game barrier: %w", err)
	}
	ids := make([]int, 0, len(plans))
	for id := range plans {
		ids = append(ids, id)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(ids)))
	var errs []error
	for _, id := range ids {
		if err := c.executor.StopDynamicLanePlan(plans[id]); err != nil {
			errs = append(errs, fmt.Errorf("stop lane %d for dormant: %w", id, err))
		}
	}
	if lifecycle != nil {
		lifecycle.Dormant()
	}
	c.mu.Lock()
	c.lanePlans = map[int]LanePlan{}
	c.state = RuntimeDormant
	c.mu.Unlock()
	return errors.Join(errs...)
}

// Wake recreates the configured 1..4 Transport Lanes with fresh source ports
// and same-association Reality-like admission. Shared Game/TUN/routes are never
// restarted. The first READY lane is published to Game immediately so forwarding
// resumes without waiting for optional Game redundancy; later READY lanes are
// attached incrementally to the same Logical Tunnel race set.
func (c *Controller) Wake() error {
	c.mu.Lock()
	if c.state != RuntimeDormant {
		state := c.state
		c.mu.Unlock()
		return fmt.Errorf("Windows runtime cannot wake while %s", state)
	}
	c.state = RuntimeConnecting
	profile := c.profile
	base := c.baseUnderlay
	expected := c.tunnelConfig
	control := c.gameControl
	c.mu.Unlock()

	failDormant := func(err error) error {
		c.mu.Lock()
		c.state = RuntimeDormant
		c.mu.Unlock()
		return err
	}
	if err := logicaltunnel.ValidateProductTransportLaneCount(profile.Lanes); err != nil {
		return failDormant(err)
	}
	lifecycle, err := logicaltunnel.NewLaneLifecycle(profile.Lanes)
	if err != nil {
		return failDormant(err)
	}

	plans := make(map[int]LanePlan, profile.Lanes)
	started := make([]LanePlan, 0, profile.Lanes)
	gamePublished := false
	rollback := func() {
		if gamePublished {
			_ = setGameLaneTargets(control, nil, gameControlTimeout)
		}
		for i := len(started) - 1; i >= 0; i-- {
			_ = c.executor.StopDynamicLanePlan(started[i])
		}
	}
	rollbackDormant := func(err error) error {
		rollback()
		return failDormant(err)
	}
	for id := 1; id <= profile.Lanes; id++ {
		plan, proc, err := c.bootstrapRuntimeLane(profile, base, expected, id, id, false)
		if err != nil {
			return rollbackDormant(fmt.Errorf("wake lane %d bootstrap: %w", id, err))
		}
		if err := c.executor.StartDynamicLane(plan, proc); err != nil {
			return rollbackDormant(fmt.Errorf("wake lane %d transport: %w", id, err))
		}
		plans[id] = plan
		started = append(started, plan)
		if _, err := lifecycle.AttachInitial(uint8(id)); err != nil {
			return rollbackDormant(err)
		}
		targets, err := gameTargetsFromPlans(plans)
		if err != nil {
			return rollbackDormant(err)
		}
		if err := setGameLaneTargets(control, targets, gameControlTimeout); err != nil {
			return rollbackDormant(fmt.Errorf("wake lane %d Game promotion: %w", id, err))
		}
		gamePublished = true
	}

	c.mu.Lock()
	c.lanePlans = plans
	c.lifecycle = lifecycle
	c.state = RuntimeConnected
	c.mu.Unlock()
	return nil
}

// ReplaceLane performs same-logical-ID make-before-break. The candidate uses a
// private transport slot (normally 5) and a fresh public source port while the
// old incarnation remains authoritative and stays in Game. Only after the
// candidate passes same-flow bootstrap, DTLS and LINK readiness does Game
// atomically swap that LaneID's endpoint. The Logical Tunnel generation fence is
// then advanced, and the old transport is retired last. No fifth logical lane or
// new PacketID namespace is created.
func (c *Controller) ReplaceLane(laneID int) error {
	c.mu.Lock()
	if c.state != RuntimeConnected {
		state := c.state
		c.mu.Unlock()
		return fmt.Errorf("Windows runtime cannot replace lane while %s", state)
	}
	oldPlan, ok := c.lanePlans[laneID]
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("logical lane %d is not active", laneID)
	}
	oldRef, err := lifecycleRefForID(c.lifecycle, laneID)
	if err != nil {
		c.mu.Unlock()
		return err
	}
	c.state = RuntimeConnecting
	profile := c.profile
	base := c.baseUnderlay
	expected := c.tunnelConfig
	control := c.gameControl
	lifecycle := c.lifecycle
	oldPlans := cloneLanePlans(c.lanePlans)
	c.mu.Unlock()

	finishConnected := func(err error) error {
		c.mu.Lock()
		c.state = RuntimeConnected
		c.mu.Unlock()
		return err
	}
	slot := NextReplacementSlot(oldPlan)
	candidate, proc, err := c.bootstrapRuntimeLane(profile, base, expected, laneID, slot, true)
	if err != nil {
		return finishConnected(fmt.Errorf("candidate lane %d bootstrap: %w", laneID, err))
	}
	if err := c.executor.StartDynamicLane(candidate, proc); err != nil {
		return finishConnected(fmt.Errorf("candidate lane %d transport: %w", laneID, err))
	}
	rollbackCandidate := func() { _ = c.executor.StopDynamicLanePlan(candidate) }

	newPlans := cloneLanePlans(oldPlans)
	newPlans[laneID] = candidate
	targets, err := gameTargetsFromPlans(newPlans)
	if err != nil {
		rollbackCandidate()
		return finishConnected(err)
	}
	if err := setGameLaneTargets(control, targets, gameControlTimeout); err != nil {
		rollbackCandidate()
		return finishConnected(fmt.Errorf("candidate lane %d Game promotion: %w", laneID, err))
	}
	fresh, err := lifecycle.PromoteSameIDReplacement(oldRef)
	if err != nil {
		oldTargets, oldTargetErr := gameTargetsFromPlans(oldPlans)
		if oldTargetErr == nil {
			_ = setGameLaneTargets(control, oldTargets, gameControlTimeout)
		}
		rollbackCandidate()
		return finishConnected(fmt.Errorf("candidate lane %d lifecycle promotion: %w", laneID, err))
	}
	_ = fresh // generation is retained in lifecycle and observable via Snapshot.

	// Candidate is authoritative from here even if best-effort old cleanup fails.
	c.mu.Lock()
	c.lanePlans = newPlans
	c.state = RuntimeConnected
	c.mu.Unlock()
	if err := c.executor.StopDynamicLanePlan(oldPlan); err != nil {
		return fmt.Errorf("lane %d promoted but old transport cleanup failed: %w", laneID, err)
	}
	return nil
}
