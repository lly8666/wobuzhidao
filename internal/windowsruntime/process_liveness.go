package windowsruntime

import "sort"

type processExitState interface {
	Done() <-chan struct{}
	ExitErr() error
}

type LaneProcessExit struct {
	Name string
	Err  error
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

func (c *Controller) runAuthoritativeLaneExitTick() bool {
	state, plans := c.laneAgePlans()
	if state != RuntimeConnected {
		return false
	}
	laneID, _, ok := firstExitedAuthoritativeLane(c.executor, plans)
	if !ok {
		return false
	}
	_ = c.ReplaceLane(laneID)
	return true
}
