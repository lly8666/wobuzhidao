package windowsruntime

import (
	"errors"
	"fmt"
	"strings"
)

func validateDynamicLanePlan(lane LanePlan) error {
	if lane.ID < 1 || lane.ID > 4 {
		return fmt.Errorf("dynamic lane id=%d out of range", lane.ID)
	}
	if lane.Slot == 0 {
		lane.Slot = lane.ID
	}
	if lane.Slot != lane.ID && lane.Slot != makeBeforeBreakCandidateSlot {
		return fmt.Errorf("dynamic lane slot=%d invalid for logical lane id=%d", lane.Slot, lane.ID)
	}
	if !strings.HasPrefix(lane.FakeTCP.Name, "faketcp-") || !strings.HasPrefix(lane.DTLS.Name, "dtls-") || !strings.HasPrefix(lane.Link.Name, "link-") {
		return errors.New("dynamic lane commands require FakeTCP/DTLS/LINK process names")
	}
	if lane.FakeTCP.Name == lane.DTLS.Name || lane.FakeTCP.Name == lane.Link.Name || lane.DTLS.Name == lane.Link.Name {
		return errors.New("dynamic lane process names must be distinct")
	}
	return nil
}

func laneProcessNameSet(lane LanePlan) map[string]bool {
	return map[string]bool{
		lane.FakeTCP.Name: true,
		lane.DTLS.Name:    true,
		lane.Link.Name:    true,
	}
}

// StartDynamicLane takes ownership of a same-flow FakeTCP process whose bounded
// Reality-like bootstrap has already authenticated. It then brings up only this
// transport incarnation's DTLS and LINK. Shared Game/TUN/routes remain alive.
// Ownership begins at function entry: every rejection before executor admission
// stops the supplied FakeTCP child; after admission rollback owns all three lane
// children. Callers therefore never need a second cleanup path for a rejected
// candidate.
//
// Independent logical LaneIDs 1..4 may coexist. During make-before-break, one
// replacement candidate may also coexist with the old incarnation of the same
// logical LaneID by using private transport slot 5 and distinct process names.
// Slot 5 is overlap capacity, not a fifth logical lane or PacketID namespace.
func (e *Executor) StartDynamicLane(lane LanePlan, prestartedFake Process) error {
	if prestartedFake == nil {
		return errors.New("dynamic lane requires prestarted FakeTCP")
	}
	owned := true
	defer func() {
		if owned {
			_ = prestartedFake.Stop()
		}
	}()
	if err := validateDynamicLanePlan(lane); err != nil {
		return err
	}
	wanted := laneProcessNameSet(lane)

	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return errors.New("shared Windows runtime is not running")
	}
	if e.cleanupPending {
		return errors.New("Windows runtime has pending network cleanup")
	}
	for _, p := range e.processes {
		if wanted[p.name] {
			return fmt.Errorf("dynamic lane process %s already exists", p.name)
		}
	}
	base := len(e.processes)
	rollback := func() {
		for i := len(e.processes) - 1; i >= base; i-- {
			_ = e.processes[i].proc.Stop()
		}
		e.processes = e.processes[:base]
	}

	e.processes = append(e.processes, namedProcess{name: lane.FakeTCP.Name, proc: prestartedFake})
	owned = false
	if err := waitProcessReady(lane.FakeTCP.Name, prestartedFake); err != nil {
		rollback()
		return err
	}
	dtls, err := e.runner.Start(lane.DTLS)
	if err != nil {
		rollback()
		return fmt.Errorf("start %s: %w", lane.DTLS.Name, err)
	}
	e.processes = append(e.processes, namedProcess{name: lane.DTLS.Name, proc: dtls})
	if err := waitProcessReady(lane.DTLS.Name, dtls); err != nil {
		rollback()
		return err
	}
	link, err := e.runner.Start(lane.Link)
	if err != nil {
		rollback()
		return fmt.Errorf("start %s: %w", lane.Link.Name, err)
	}
	e.processes = append(e.processes, namedProcess{name: lane.Link.Name, proc: link})
	if err := waitProcessReady(lane.Link.Name, link); err != nil {
		rollback()
		return err
	}
	return nil
}

func (e *Executor) StopDynamicLanePlan(lane LanePlan) error {
	if err := validateDynamicLanePlan(lane); err != nil {
		return err
	}
	wanted := laneProcessNameSet(lane)
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running {
		return errors.New("shared Windows runtime is not running")
	}
	found := 0
	var errs []error
	for i := len(e.processes) - 1; i >= 0; i-- {
		p := e.processes[i]
		if !wanted[p.name] {
			continue
		}
		found++
		if stopErr := p.proc.Stop(); stopErr != nil {
			errs = append(errs, fmt.Errorf("stop %s: %w", p.name, stopErr))
		}
		e.processes = append(e.processes[:i], e.processes[i+1:]...)
	}
	if found == 0 {
		return fmt.Errorf("dynamic lane process group is not running")
	}
	if found != 3 {
		errs = append(errs, fmt.Errorf("dynamic lane process group incomplete: found=%d want=3", found))
	}
	return errors.Join(errs...)
}

// StopDynamicLane is the normal-slot compatibility wrapper.
func (e *Executor) StopDynamicLane(laneID int) error {
	fake, dtls, link, err := normalLaneCommandsForStop(laneID)
	if err != nil {
		return err
	}
	return e.StopDynamicLanePlan(LanePlan{ID: laneID, Slot: laneID, FakeTCP: fake, DTLS: dtls, Link: link})
}

func normalLaneCommandsForStop(laneID int) (Command, Command, Command, error) {
	if laneID < 1 || laneID > 4 {
		return Command{}, Command{}, Command{}, fmt.Errorf("dynamic lane id=%d out of range", laneID)
	}
	return Command{Name: fmt.Sprintf("faketcp-%d", laneID)}, Command{Name: fmt.Sprintf("dtls-%d", laneID)}, Command{Name: fmt.Sprintf("link-%d", laneID)}, nil
}

// DynamicLaneIDs reports logical active lane identities only. A make-before-
// break candidate in private slot 5 is attributed to its existing logical
// LaneID and therefore never appears as a fifth product lane.
func (e *Executor) DynamicLaneIDs() []int {
	e.mu.Lock()
	defer e.mu.Unlock()
	seen := map[int]bool{}
	for _, p := range e.processes {
		for id := 1; id <= 4; id++ {
			normal := fmt.Sprintf("link-%d", id)
			if p.name == normal || strings.HasPrefix(p.name, normal+"-candidate-s") {
				seen[id] = true
			}
		}
	}
	out := make([]int, 0, len(seen))
	for id := 1; id <= 4; id++ {
		if seen[id] {
			out = append(out, id)
		}
	}
	return out
}
