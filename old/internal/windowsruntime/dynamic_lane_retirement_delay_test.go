package windowsruntime

import (
	"sync"
	"testing"
	"time"
)

type delayedRetirementProcess struct {
	delay time.Duration
	done  chan struct{}
	once  sync.Once
}

func newDelayedRetirementProcess(delay time.Duration) *delayedRetirementProcess {
	return &delayedRetirementProcess{delay: delay, done: make(chan struct{})}
}

func (p *delayedRetirementProcess) Stop() error {
	p.once.Do(func() {
		go func() {
			time.Sleep(p.delay)
			close(p.done)
		}()
	})
	return nil
}

func (p *delayedRetirementProcess) WaitStopped(timeout time.Duration) error {
	t := time.NewTimer(timeout)
	defer t.Stop()
	select {
	case <-p.done:
		return nil
	case <-t.C:
		return &retirementDelayTimeoutError{}
	}
}

type retirementDelayTimeoutError struct{}

func (*retirementDelayTimeoutError) Error() string { return "delayed process did not retire" }

func TestStopDynamicLanePlanWaitsThroughDelayedChildExit(t *testing.T) {
	lane := LanePlan{
		ID:      2,
		Slot:    1,
		FakeTCP: Command{Name: "faketcp-2-candidate-s1"},
		DTLS:    Command{Name: "dtls-2-candidate-s1"},
		Link:    Command{Name: "link-2-candidate-s1"},
	}
	link := newDelayedRetirementProcess(300 * time.Millisecond)
	dtls := newDelayedRetirementProcess(20 * time.Millisecond)
	faketcp := newDelayedRetirementProcess(20 * time.Millisecond)
	e := &Executor{
		running: true,
		processes: []namedProcess{
			{name: lane.FakeTCP.Name, proc: faketcp},
			{name: lane.DTLS.Name, proc: dtls},
			{name: lane.Link.Name, proc: link},
		},
	}

	started := time.Now()
	if err := e.StopDynamicLanePlan(lane); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(started)
	// The physical slot must not become reusable merely because Stop/Kill was
	// requested. Link teardown is deliberately delayed to model Windows process
	// and socket retirement under load/high RTT. Reverse-order retirement also
	// means DTLS/FakeTCP cannot be declared gone before their own exit receipts.
	if elapsed < 300*time.Millisecond {
		t.Fatalf("retirement returned too early after %s", elapsed)
	}
	if elapsed >= dynamicLaneProcessStopWait {
		t.Fatalf("retirement exceeded bounded wait: %s", elapsed)
	}
	if len(e.processes) != 0 {
		t.Fatalf("retired process group remains registered: %v", e.processes)
	}
}
