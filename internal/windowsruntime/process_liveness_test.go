package windowsruntime

import (
	"errors"
	"testing"
)

type exitStateProcess struct {
	done chan struct{}
	err  error
}

func newExitStateProcess() *exitStateProcess { return &exitStateProcess{done: make(chan struct{})} }
func (p *exitStateProcess) Stop() error       { return nil }
func (p *exitStateProcess) Done() <-chan struct{} { return p.done }
func (p *exitStateProcess) ExitErr() error    { return p.err }

func livenessLanePlan(slot int) LanePlan {
	suffix := "1"
	if slot == makeBeforeBreakCandidateSlot {
		suffix = "1-candidate-s5"
	}
	return LanePlan{
		ID: 1, Slot: slot,
		FakeTCP: Command{Name: "faketcp-" + suffix},
		DTLS:    Command{Name: "dtls-" + suffix},
		Link:    Command{Name: "link-" + suffix},
	}
}

func TestAuthoritativeExitIgnoresRetiredIncarnation(t *testing.T) {
	oldPlan := livenessLanePlan(1)
	newPlan := livenessLanePlan(makeBeforeBreakCandidateSlot)
	oldLink := newExitStateProcess()
	newLink := newExitStateProcess()
	executor := &Executor{running: true, processes: []namedProcess{
		{name: oldPlan.Link.Name, proc: oldLink},
		{name: newPlan.Link.Name, proc: newLink},
	}}

	close(oldLink.done)
	if laneID, exit, ok := firstExitedAuthoritativeLane(executor, map[int]LanePlan{1: newPlan}); ok {
		t.Fatalf("retired exit selected lane=%d exit=%+v", laneID, exit)
	}

	wantErr := errors.New("candidate link exited")
	newLink.err = wantErr
	close(newLink.done)
	laneID, exit, ok := firstExitedAuthoritativeLane(executor, map[int]LanePlan{1: newPlan})
	if !ok || laneID != 1 || exit.Name != newPlan.Link.Name || !errors.Is(exit.Err, wantErr) {
		t.Fatalf("authoritative exit lane=%d exit=%+v ok=%v", laneID, exit, ok)
	}
}

func TestLaneProcessExitTreatsCleanUnexpectedExitAsFailureSignal(t *testing.T) {
	plan := livenessLanePlan(1)
	link := newExitStateProcess()
	executor := &Executor{running: true, processes: []namedProcess{{name: plan.Link.Name, proc: link}}}
	close(link.done)
	exit, ok, err := executor.LaneProcessExit(plan)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || exit.Name != plan.Link.Name || exit.Err != nil {
		t.Fatalf("exit=%+v ok=%v", exit, ok)
	}
}
