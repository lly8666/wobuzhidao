package windowsruntime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

type Process interface { Stop() error }

type Runner interface {
	Run(Command) error
	Start(Command) (Process, error)
}

type Executor struct {
	mu             sync.Mutex
	runner         Runner
	plan           Plan
	processes      []namedProcess
	running        bool
	cleanupPending bool
}

type namedProcess struct {
	name string
	proc Process
}

func NewExecutor(runner Runner) *Executor {
	if runner == nil { runner = OSRunner{} }
	return &Executor{runner: runner}
}

// Start starts the complete process stack. It remains useful for transport-only
// and compatibility tests that do not perform product single-flow admission.
func (e *Executor) Start(plan Plan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.startLocked(plan, nil)
}

// StartAfterFakeTCP continues startup after Controller has already started the
// one public FakeTCP process and waited for its in-flow TLS/bootstrap ticket.
// The supplied process becomes lifecycle-owned by Executor and is rolled back
// together with DTLS/LINK/TUN on any later failure.
func (e *Executor) StartAfterFakeTCP(plan Plan, fake Process) error {
	if fake == nil { return errors.New("prestarted FakeTCP process is required") }
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.startLocked(plan, fake)
}

func (e *Executor) startLocked(plan Plan, prestartedFake Process) error {
	if e.cleanupPending { return errors.New("Windows runtime has pending network cleanup") }
	if e.running || len(e.processes) != 0 { return errors.New("Windows runtime is already running") }

	commands := plan.ProcessSequence()
	if prestartedFake != nil {
		e.processes = append(e.processes, namedProcess{name: plan.FakeTCP.Name, proc: prestartedFake})
		if len(commands) != 0 { commands = commands[1:] }
	}
	for _, command := range commands {
		proc, err := e.runner.Start(command)
		if err != nil {
			e.rollbackProcessesLocked()
			return fmt.Errorf("start %s: %w", command.Name, err)
		}
		e.processes = append(e.processes, namedProcess{name: command.Name, proc: proc})
	}

	if err := e.runner.Run(plan.IPv6Apply); err != nil {
		cleanupErr := e.runner.Run(plan.IPv6Cleanup)
		e.rollbackProcessesLocked()
		if cleanupErr != nil {
			e.plan = plan
			e.cleanupPending = true
			return fmt.Errorf("apply IPv6 kill switch: %w (cleanup: %v)", err, cleanupErr)
		}
		return fmt.Errorf("apply IPv6 kill switch: %w", err)
	}

	if err := e.runner.Run(plan.RouteApply); err != nil {
		routeErr := e.runner.Run(plan.RouteCleanup)
		ipv6Err := e.runner.Run(plan.IPv6Cleanup)
		e.rollbackProcessesLocked()
		if routeErr != nil || ipv6Err != nil {
			e.plan = plan
			e.cleanupPending = true
			return fmt.Errorf("apply capture routes: %w (route cleanup: %v; IPv6 cleanup: %v)", err, routeErr, ipv6Err)
		}
		return fmt.Errorf("apply capture routes: %w", err)
	}

	e.plan = plan
	e.running = true
	return nil
}

// Stop removes IPv4 steering first, then releases the device-wide IPv6 block,
// then terminates TUN/LINK/DTLS/FakeTCP in reverse order. Any cleanup failure is
// retained and retryable by a later Disconnect/Exit.
func (e *Executor) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if !e.running && len(e.processes) == 0 {
		if !e.cleanupPending { return nil }
		var retryErrs []error
		if err := e.runner.Run(e.plan.RouteCleanup); err != nil { retryErrs = append(retryErrs, fmt.Errorf("cleanup capture routes: %w", err)) }
		if err := e.runner.Run(e.plan.IPv6Cleanup); err != nil { retryErrs = append(retryErrs, fmt.Errorf("cleanup IPv6 kill switch: %w", err)) }
		if len(retryErrs) != 0 { return errors.Join(retryErrs...) }
		e.plan = Plan{}
		e.cleanupPending = false
		return nil
	}

	var errs []error
	routeCleanupErr := e.runner.Run(e.plan.RouteCleanup)
	if routeCleanupErr != nil { errs = append(errs, fmt.Errorf("cleanup capture routes: %w", routeCleanupErr)) }
	ipv6CleanupErr := e.runner.Run(e.plan.IPv6Cleanup)
	if ipv6CleanupErr != nil { errs = append(errs, fmt.Errorf("cleanup IPv6 kill switch: %w", ipv6CleanupErr)) }
	for i := len(e.processes) - 1; i >= 0; i-- {
		if err := e.processes[i].proc.Stop(); err != nil { errs = append(errs, fmt.Errorf("stop %s: %w", e.processes[i].name, err)) }
	}
	e.processes = nil
	e.running = false
	if routeCleanupErr != nil || ipv6CleanupErr != nil {
		e.cleanupPending = true
	} else {
		e.plan = Plan{}
		e.cleanupPending = false
	}
	return errors.Join(errs...)
}

func (e *Executor) Running() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.running }
func (e *Executor) CleanupPending() bool { e.mu.Lock(); defer e.mu.Unlock(); return e.cleanupPending }

func (e *Executor) rollbackProcessesLocked() {
	for i := len(e.processes) - 1; i >= 0; i-- { _ = e.processes[i].proc.Stop() }
	e.processes = nil
	e.running = false
	if !e.cleanupPending { e.plan = Plan{} }
}

type OSRunner struct{}

func (OSRunner) Run(command Command) error {
	cmd := exec.Command(command.Path, command.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (OSRunner) Start(command Command) (Process, error) {
	cmd := exec.Command(command.Path, command.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil { return nil, err }
	return &osProcess{cmd: cmd}, nil
}

type osProcess struct { cmd *exec.Cmd }

func (p *osProcess) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil { return nil }
	killErr := p.cmd.Process.Kill()
	waitErr := p.cmd.Wait()
	if errors.Is(killErr, os.ErrProcessDone) { killErr = nil }
	if _, ok := waitErr.(*exec.ExitError); ok && killErr == nil { waitErr = nil }
	return errors.Join(killErr, waitErr)
}
