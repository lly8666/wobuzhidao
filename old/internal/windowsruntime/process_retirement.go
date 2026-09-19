package windowsruntime

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const dynamicLaneProcessStopWait = 5 * time.Second

type stoppedProcess interface {
	WaitStopped(time.Duration) error
}

type gracefulStoppedProcess interface {
	GracefulStop(time.Duration) error
}

type peerResetStoppedProcess interface {
	PeerResetStop(time.Duration) error
}

func isDynamicLaneLinkProcess(name string) bool {
	return name == "link" || strings.HasPrefix(name, "link-")
}

func isDynamicLaneFakeTCPProcess(name string) bool {
	return name == "faketcp" || strings.HasPrefix(name, "faketcp-")
}

func stopDynamicLaneProcess(name string, proc Process) error {
	if isDynamicLaneLinkProcess(name) {
		if graceful, ok := proc.(gracefulStoppedProcess); ok {
			return graceful.GracefulStop(dynamicLaneProcessStopWait)
		}
	}
	if isDynamicLaneFakeTCPProcess(name) {
		if resetter, ok := proc.(peerResetStoppedProcess); ok {
			return resetter.PeerResetStop(dynamicLaneProcessStopWait)
		}
	}
	if err := proc.Stop(); err != nil {
		return err
	}
	return waitDynamicLaneProcessStopped(proc)
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
