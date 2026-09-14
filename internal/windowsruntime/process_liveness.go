package windowsruntime

import (
	"fmt"
	"sort"
	"time"
)

const laneExitRetryDelay = time.Second

type processExitState interface {
	Done() <-chan struct{}
	ExitErr() error
}

type LaneProcessExit struct {
	Name string
	Err  error
}

type laneExitRetry struct {
	slot int
	next time.Time
}

type laneExitRetryState struct {
	lanes map[int]laneExitRetry
	path  *underlayPathState
}

func newLaneExitRetryState() *laneExitRetryState {
	return &laneExitRetryState{lanes: map[int]laneExitRetry{}, path: newUnderlayPathState()}
}

func (s *laneExitRetryState) clear() {
	clear(s.lanes)
	if s.path != nil {
		s.path.clear()
	}
}

func (s *laneExitRetryState) reconcile(plans map[int]LanePlan) {
	for id, retry := range s.lanes {
		plan, ok := plans[id]
		if !ok || normalizedLaneSlot(plan) != retry.slot {
			delete(s.lanes, id)
		}
	}
}

func (s *laneExitRetryState) allow(laneID, slot int, now time.Time) bool {
	retry, ok := s.lanes[laneID]
	if ok && retry.slot == slot && now.Before(retry.next) {
		return false
	}
	s.lanes[laneID] = laneExitRetry{slot: slot, next: now.Add(laneExitRetryDelay)}
	return true
}

// osProcess publishes completion only after wait() has stored the terminal
// error under p.mu, so callers that observe Done closed can safely read ExitErr.
func (p *osProcess) Done() <-chan struct{} { return p.done }

func (p *osProcess) ExitErr() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// LaneProcessExit reports terminal state only for the exact physical process
// names in lane. Retired incarnations therefore cannot poison a newer logical
// LaneID after lanePlans has switched to a different transport slot.
func (e *Executor) LaneProcessExit(lane LanePlan) (LaneProcessExit, bool, error) {
	if err := validateDynamicLanePlan(lane); err != nil {
		return LaneProcessExit{}, false, err
	}
	wanted := laneProcessNameSet(lane)
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, named := range e.processes {
		if !wanted[named.name] {
			continue
		}
		state, ok := named.proc.(processExitState)
		if !ok || state.Done() == nil {
			continue
		}
		select {
		case <-state.Done():
			return LaneProcessExit{Name: named.name, Err: state.ExitErr()}, true, nil
		default:
		}
	}
	return LaneProcessExit{}, false, nil
}

func firstExitedAuthoritativeLane(executor *Executor, plans map[int]LanePlan) (int, LaneProcessExit, bool) {
	ids := make([]int, 0, len(plans))
	for id := range plans {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		exit, exited, err := executor.LaneProcessExit(plans[id])
		if err != nil || !exited {
			continue
		}
		return id, exit, true
	}
	return 0, LaneProcessExit{}, false
}

func shouldDormantOnKeepaliveZeroExit(profile Profile, plan LanePlan, exit LaneProcessExit) bool {
	return explicitKeepaliveDisabled(profile) && exit.Name == plan.Link.Name
}

// wakeDormantOnPayload implements the on-demand half of keepalive=0. The shared
// Game/TUN process remains alive in DORMANT and continues to count real inner
// payload ingress even though its public lane target set is empty. The first
// payload sequence advance therefore becomes the signal to rebuild all desired
// public lanes. Failed wake attempts remain dormant and retry at a bounded rate.
func (c *Controller) wakeDormantOnPayload(baseline uint64) {
	ticker := time.NewTicker(maxPayloadIdlePollInterval)
	defer ticker.Stop()
	var nextRetry time.Time
	for range ticker.C {
		if c.State() != RuntimeDormant {
			return
		}
		activity, err := c.PayloadActivity()
		if err != nil || activity.Sequence <= baseline {
			continue
		}
		now := time.Now()
		if now.Before(nextRetry) {
			continue
		}
		if err := c.Wake(); err != nil {
			nextRetry = now.Add(laneExitRetryDelay)
			fmt.Printf("WBD_KEEPALIVE_ZERO_WAKE_RETRY baseline_payload_seq=%d payload_seq=%d retry=%s err=%v\n", baseline, activity.Sequence, laneExitRetryDelay, err)
			continue
		}
		fmt.Printf("WBD_KEEPALIVE_ZERO_WAKE_PASS baseline_payload_seq=%d payload_seq=%d\n", baseline, activity.Sequence)
		return
	}
}

func (c *Controller) currentProfile() Profile {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.profile
}

func (c *Controller) enterDormantForKeepaliveZero(laneID int, plan LanePlan, exit LaneProcessExit) bool {
	if !shouldDormantOnKeepaliveZeroExit(c.currentProfile(), plan, exit) {
		return false
	}
	activity, err := c.PayloadActivity()
	if err != nil {
		fmt.Printf("WBD_KEEPALIVE_ZERO_DORMANT_FALLBACK lane=%d process=%s err=%v\n", laneID, exit.Name, err)
		return false
	}
	if err := c.Dormant(); err != nil && c.State() != RuntimeDormant {
		fmt.Printf("WBD_KEEPALIVE_ZERO_DORMANT_FALLBACK lane=%d process=%s err=%v\n", laneID, exit.Name, err)
		return false
	} else if err != nil {
		fmt.Printf("WBD_KEEPALIVE_ZERO_DORMANT_WARN lane=%d process=%s err=%v\n", laneID, exit.Name, err)
	}
	fmt.Printf("WBD_KEEPALIVE_ZERO_DORMANT lane=%d process=%s baseline_payload_seq=%d\n", laneID, exit.Name, activity.Sequence)
	go c.wakeDormantOnPayload(activity.Sequence)
	return true
}

func (c *Controller) runAuthoritativeLaneExitTick(retries *laneExitRetryState, now time.Time) bool {
	state, plans := c.laneAgePlans()
	if state == RuntimeDormant {
		retries.clear()
		return false
	}
	if state != RuntimeConnected {
		return false
	}

	// Path change has priority over child-exit replacement. Once a new physical
	// underlay is observed, keep stale-child recovery from rebuilding on the old
	// baseline while logical lanes are migrated one at a time.
	if retries.path != nil && c.runUnderlayPathTick(retries.path, now) {
		return true
	}

	retries.reconcile(plans)
	laneID, exit, ok := firstExitedAuthoritativeLane(c.executor, plans)
	if !ok {
		return false
	}
	plan := plans[laneID]
	if c.enterDormantForKeepaliveZero(laneID, plan, exit) {
		retries.clear()
		return true
	}
	if !retries.allow(laneID, normalizedLaneSlot(plan), now) {
		// Keep transport failure above idle/age policy while waiting for the
		// bounded retry deadline; do not spin same-flow bootstrap every poll.
		return true
	}
	_ = c.ReplaceLane(laneID)
	return true
}
