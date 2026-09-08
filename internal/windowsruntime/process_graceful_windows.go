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
	linkSupervisorEventPrefix = `Local\WBDLinkShutdown-`
	linkCloseAckMarker        = "WBD_LINK_CLOSE_ACK"
)

func (p *osProcess) GracefulStop(timeout time.Duration) error {
	if timeout <= 0 {
		return errors.New("graceful process retirement timeout must be positive")
	}
	deadline := time.Now().Add(timeout)

	p.mu.Lock()
	exited := p.exited
	proc := p.cmd.Process
	p.mu.Unlock()
	if exited {
		if p.out.contains(linkCloseAckMarker) {
			return nil
		}
		return errors.New("LINK process exited before graceful retirement ACK")
	}
	if proc == nil {
		return errors.New("graceful process retirement has no child process")
	}

	name, err := windows.UTF16PtrFromString(linkSupervisorEventPrefix + strconv.Itoa(proc.Pid))
	if err != nil {
		return p.failGracefulStop(deadline, fmt.Errorf("encode LINK supervisor event: %w", err))
	}
	h, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return p.failGracefulStop(deadline, fmt.Errorf("open LINK supervisor event: %w", err))
	}
	defer windows.CloseHandle(h)
	if err := windows.SetEvent(h); err != nil {
		return p.failGracefulStop(deadline, fmt.Errorf("signal LINK supervisor event: %w", err))
	}

	remaining := time.Until(deadline)
	if remaining <= 0 {
		return p.failGracefulStop(deadline, errors.New("LINK graceful retirement deadline expired before close ACK"))
	}
	if err := p.WaitReady(linkCloseAckMarker, remaining); err != nil {
		return p.failGracefulStop(deadline, fmt.Errorf("wait LINK close ACK: %w", err))
	}
	remaining = time.Until(deadline)
	if remaining <= 0 {
		return p.failGracefulStop(deadline, errors.New("LINK graceful retirement deadline expired before process exit"))
	}
	if err := p.WaitStopped(remaining); err != nil {
		return p.failGracefulStop(deadline, fmt.Errorf("wait LINK process exit: %w", err))
	}
	return nil
}

func (p *osProcess) failGracefulStop(deadline time.Time, cause error) error {
	var errs []error
	errs = append(errs, cause)
	if err := p.Stop(); err != nil {
		errs = append(errs, fmt.Errorf("fallback kill: %w", err))
	}
	if remaining := time.Until(deadline); remaining > 0 {
		if err := p.WaitStopped(remaining); err != nil {
			errs = append(errs, fmt.Errorf("wait fallback kill: %w", err))
		}
	}
	return errors.Join(errs...)
}
