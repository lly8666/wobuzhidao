package windowsruntime

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// Process is a long-running child owned by the Windows runtime executor.
// Stop must terminate the child and wait for it to leave.
type Process interface {
	Stop() error
}

// Runner separates lifecycle policy from os/exec so ordering and rollback can
// be qualified without requiring administrator privileges or Windows binaries.
type Runner interface {
	Run(Command) error
	Start(Command) (Process, error)
}

// Executor owns one frozen Windows runtime Plan at a time. It intentionally
// knows nothing about FakeTCP/DTLS/LINK wire semantics; those remain encoded in
// BuildPlan. Its only authority is process/route lifecycle ordering.
type Executor struct {
	mu        sync.Mutex
	runner    Runner
	plan      Plan
	processes []namedProcess
	running   bool
}

type namedProcess struct {
	name string
	proc Process
}

func NewExecutor(runner Runner) *Executor {
	if runner == nil {
		runner = OSRunner{}
	}
	return &Executor{runner: runner}
}

// Start launches the long-running stack in Plan.StartSequence order and applies
// broad capture routes only after every process has started. A partial failure
// rolls back everything started in this call. If route application was
// attempted, route cleanup is attempted before process teardown.
func (e *Executor) Start(plan Plan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.running || len(e.processes) != 0 {
		return errors.New("Windows runtime is already running")
	}

	sequence := plan.StartSequence()
	if len(sequence) == 0 || sequence[len(sequence)-1].Name != plan.RouteApply.Name {
		return errors.New("Windows runtime plan must apply routes last")
	}

	for _, command := range sequence[:len(sequence)-1] {
		proc, err := e.runner.Start(command)
		if err != nil {
			e.rollbackLocked(false, plan)
			return fmt.Errorf("start %s: %w", command.Name, err)
		}
		e.processes = append(e.processes, namedProcess{name: command.Name, proc: proc})
	}

	if err := e.runner.Run(plan.RouteApply); err != nil {
		cleanupErr := e.runner.Run(plan.RouteCleanup)
		e.rollbackLocked(false, plan)
		if cleanupErr != nil {
			return fmt.Errorf("apply capture routes: %w (cleanup: %v)", err, cleanupErr)
		}
		return fmt.Errorf("apply capture routes: %w", err)
	}

	e.plan = plan
	e.running = true
	return nil
}

// Stop removes capture routes first, then terminates TUN/LINK/DTLS/FakeTCP in
// reverse process order. It is idempotent so explicit Exit may safely call it
// after a prior Disconnect.
func (e *Executor) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.running && len(e.processes) == 0 {
		return nil
	}

	var errs []error
	if err := e.runner.Run(e.plan.RouteCleanup); err != nil {
		errs = append(errs, fmt.Errorf("cleanup capture routes: %w", err))
	}
	for i := len(e.processes) - 1; i >= 0; i-- {
		if err := e.processes[i].proc.Stop(); err != nil {
			errs = append(errs, fmt.Errorf("stop %s: %w", e.processes[i].name, err))
		}
	}
	e.processes = nil
	e.plan = Plan{}
	e.running = false
	return errors.Join(errs...)
}

func (e *Executor) Running() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.running
}

func (e *Executor) rollbackLocked(cleanRoutes bool, plan Plan) {
	if cleanRoutes {
		_ = e.runner.Run(plan.RouteCleanup)
	}
	for i := len(e.processes) - 1; i >= 0; i-- {
		_ = e.processes[i].proc.Stop()
	}
	e.processes = nil
	e.plan = Plan{}
	e.running = false
}

// OSRunner is the product runner used by the GUI controller. Long-running
// children inherit stdout/stderr so administrator diagnostics remain visible
// when the GUI is launched from a console or CI harness.
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
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &osProcess{cmd: cmd}, nil
}

type osProcess struct {
	cmd *exec.Cmd
}

func (p *osProcess) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	killErr := p.cmd.Process.Kill()
	waitErr := p.cmd.Wait()
	if errors.Is(killErr, os.ErrProcessDone) {
		killErr = nil
	}
	if _, ok := waitErr.(*exec.ExitError); ok && killErr == nil {
		waitErr = nil
	}
	return errors.Join(killErr, waitErr)
}
