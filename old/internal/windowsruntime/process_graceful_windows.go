//go:build windows

package windowsruntime

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/sys/windows"
)

const (
	linkSupervisorEventPrefix    = `Local\WBDLinkShutdown-`
	linkCloseAckMarker           = "WBD_LINK_CLOSE_ACK"
	fakeTCPSupervisorEventPrefix = `Local\WBDFakeTCPShutdown-`
	fakeTCPRetireResetMarker     = "WBD_FAKETCP_RETIRE_RESET_TX"
)

func (p *osProcess) GracefulStop(timeout time.Duration) error {
	return p.signalAndWaitStop(timeout, linkSupervisorEventPrefix, linkCloseAckMarker, "LINK")
}

// PeerResetStop asks an established Windows FakeTCP client to send an exact-flow
// peer RST before it exits. Normal replacement retirement calls this only after
// the LINK CloseNormal exchange has completed and the DTLS child is gone. The
// server mux already treats peer RST as terminal for that four-tuple, so this
// closes the retired underlay immediately instead of leaving it to idle GC.
func (p *osProcess) PeerResetStop(timeout time.Duration) error {
	return p.signalAndWaitStop(timeout, fakeTCPSupervisorEventPrefix, fakeTCPRetireResetMarker, "FakeTCP")
}

func (p *osProcess) signalAndWaitStop(timeout time.Duration, eventPrefix, marker, label string) error {
	if timeout <= 0 {
		return errors.New("graceful process retirement timeout must be positive")
	}
	deadline := time.Now().Add(timeout)

	p.mu.Lock()
	exited := p.exited
	proc := p.cmd.Process
	p.mu.Unlock()
	if exited {
		if p.out.contains(marker) {
			return nil
		}
		return fmt.Errorf("%s process exited before graceful retirement marker %q", label, marker)
	}
	if proc == nil {
		return fmt.Errorf("%s graceful process retirement has no child process", label)
	}

	name, err := windows.UTF16PtrFromString(eventPrefix + strconv.Itoa(proc.Pid))
	if err != nil {
		return p.failSignaledStop(deadline, label, fmt.Errorf("encode %s supervisor event: %w", label, err))
	}
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return p.failSignaledStop(deadline, label, fmt.Errorf("open %s supervisor event: %w", label, err))
	}
	defer windows.CloseHandle(h)
	if err := windows.SetEvent(h); err != nil {
		return p.failSignaledStop(deadline, label, fmt.Errorf("signal %s supervisor event: %w", label, err))
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		return p.failSignaledStop(deadline, label, fmt.Errorf("%s graceful retirement deadline expired before marker", label))
	}
	if err := p.WaitReady(marker, remaining); err != nil {
		return p.failSignaledStop(deadline, label, fmt.Errorf("wait %s retirement marker: %w", label, err))
	}
	remaining = time.Until(deadline)
	if remaining <= 0 {
		return p.failSignaledStop(deadline, label, fmt.Errorf("%s graceful retirement deadline expired before process exit", label))
	}
	if err := p.WaitStopped(remaining); err != nil {
		return p.failSignaledStop(deadline, label, fmt.Errorf("wait %s process exit: %w", label, err))
	}
	return nil
}

func (p *osProcess) failSignaledStop(deadline time.Time, label string, cause error) error {
	var errs []error
	errs = append(errs, cause)
	if err := p.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("%s fallback kill: %w", label, err))
	}
	if remaining := time.Until(deadline); remaining > 0 {
		if err := p.WaitStopped(remaining); err != nil {
			errs = append(errs, fmt.Errorf("wait %s fallback kill: %w", label, err))
		}
	}
	return errors.Join(errs...)
}
