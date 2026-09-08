package windowsruntime

import (
	"errors"
	"fmt"
	"time"
)

const dynamicLaneProcessStopWait = 5 * time.Second

type stoppedProcess interface {
	WaitStopped(time.Duration) error
}

// waitDynamicLaneProcessStopped turns Process.Stop from a kill request into a
// retirement barrier when the concrete process can report its actual exit.
// Recording/fake processes used by unit tests remain compatible by simply not
// implementing stoppedProcess.
func waitDynamicLaneProcessStopped(proc Process) error {
	waiter, ok := proc.(stoppedProcess)
	if !ok {
		return nil
	}
	return waiter.WaitStopped(dynamicLaneProcessStopWait)
}

// WaitStopped waits until exec.Cmd.Wait has completed and the child has been
// reaped. On Windows, os.Process.Kill/TerminateProcess can return before that
// point, so a physical transport slot must not be considered reusable merely
// because Kill itself succeeded.
func (p *osProcess) WaitStopped(timeout time.Duration) error {
	if timeout <= 0 {
		return errors.New("process retirement timeout must be positive")
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-p.done:
		return nil
	case <-timer.C:
		p.mu.Lock()
		exited := p.exited
		p.mu.Unlock()
		if exited {
			return nil
		}
		return fmt.Errorf("timeout waiting %s for process exit", timeout)
	}
}
