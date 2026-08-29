package windowsruntime

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type Process interface { Stop() error }

type readyProcess interface {
	WaitReady(marker string, timeout time.Duration) error
}

type Runner interface {
	Run(Command) error
	Start(Command) (Process, error)
}

type readinessSpec struct {
	marker  string
	timeout time.Duration
}

func commandReadiness(name string) (readinessSpec, bool) {
	switch name {
	case "faketcp":
		return readinessSpec{marker: "READY role=client", timeout: 25 * time.Second}, true
	case "dtls":
		return readinessSpec{marker: "READY role=client", timeout: 25 * time.Second}, true
	case "link":
		return readinessSpec{marker: "WBD_LINK_READY role=client", timeout: 12 * time.Second}, true
	case "tun":
		return readinessSpec{marker: "WBD_TUN_READY mode=client", timeout: 10 * time.Second}, true
	default:
		return readinessSpec{}, false
	}
}

func hasArg(args []string, key string) bool {
	for _, arg := range args {
		if arg == key { return true }
	}
	return false
}

func validTicketHex(raw string) bool {
	if len(raw) != 64 { return false }
	for _, c := range raw {
		if !strings.ContainsRune("0123456789abcdefABCDEF", c) { return false }
	}
	return true
}

// prepareProcessCommand materializes only the ephemeral V3 LINK ticket. The
// persistent Plan contains a file path, not the ticket value. FakeTCP writes
// that file only after real TLS 1.3/Reality-like admission and the same-flow
// SWITCH barrier are complete, and readiness ordering guarantees this helper
// runs afterwards. Legacy V2 plans that already embed a ticket are untouched.
func prepareProcessCommand(plan Plan, command Command) (Command, error) {
	if command.Name != "link" || strings.TrimSpace(plan.TicketPath) == "" || hasArg(command.Args, "-demo-reality-ticket") {
		return command, nil
	}
	body, err := os.ReadFile(plan.TicketPath)
	if err != nil {
		return Command{}, fmt.Errorf("read single-flow Reality ticket: %w", err)
	}
	ticket := strings.TrimSpace(string(body))
	if !validTicketHex(ticket) {
		return Command{}, errors.New("single-flow Reality ticket file must contain exactly 64 hex characters")
	}
	command.Args = append(append([]string(nil), command.Args...), "-demo-reality-ticket", ticket)
	return command, nil
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

// Start brings up each transport layer only after the layer below has emitted
// its real readiness marker. Device IPv6 is then fail-closed and IPv4 capture
// is applied last. V3 FakeTCP READY additionally means the Reality-like TLS
// bootstrap and same-flow mode-switch barrier have completed, so DTLS never
// creates a second public flow and cannot race the setup phase.
func (e *Executor) Start(plan Plan) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.cleanupPending { return errors.New("Windows runtime has pending network cleanup") }
	if e.running || len(e.processes) != 0 { return errors.New("Windows runtime is already running") }

	for _, baseCommand := range plan.ProcessSequence() {
		command, err := prepareProcessCommand(plan, baseCommand)
		if err != nil {
			e.rollbackProcessesLocked()
			return fmt.Errorf("prepare %s: %w", baseCommand.Name, err)
		}
		proc, err := e.runner.Start(command)
		if err != nil {
			e.rollbackProcessesLocked()
			return fmt.Errorf("start %s: %w", command.Name, err)
		}
		e.processes = append(e.processes, namedProcess{name: command.Name, proc: proc})
		if spec, ok := commandReadiness(command.Name); ok {
			rp, ok := proc.(readyProcess)
			if !ok {
				e.rollbackProcessesLocked()
				return fmt.Errorf("wait %s ready: process runner does not expose readiness", command.Name)
			}
			if err := rp.WaitReady(spec.marker, spec.timeout); err != nil {
				e.rollbackProcessesLocked()
				return fmt.Errorf("wait %s ready: %w", command.Name, err)
			}
		}
	}

	if err := e.runner.Run(plan.IPv6Apply); err != nil {
		// The script self-cleans partial rules; retry cleanup for crash-safe
		// idempotence before stopping the already-started process stack.
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
	out := newProcessOutput()
	stdout := &readyLineWriter{dst: os.Stdout, out: out}
	stderr := &readyLineWriter{dst: os.Stderr, out: out}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil { return nil, err }
	p := &osProcess{cmd: cmd, out: out, stdout: stdout, stderr: stderr, done: make(chan struct{})}
	go p.wait()
	return p, nil
}

type processOutput struct {
	mu     sync.Mutex
	lines  []string
	notify chan struct{}
}

func newProcessOutput() *processOutput { return &processOutput{notify: make(chan struct{}, 1)} }

func (o *processOutput) observe(line string) {
	line = strings.TrimSpace(line)
	if line == "" { return }
	o.mu.Lock()
	if len(o.lines) >= 256 { copy(o.lines, o.lines[len(o.lines)-128:]); o.lines = o.lines[:128] }
	o.lines = append(o.lines, line)
	o.mu.Unlock()
	select { case o.notify <- struct{}{}: default: }
}

func (o *processOutput) contains(marker string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	for _, line := range o.lines { if strings.Contains(line, marker) { return true } }
	return false
}

type readyLineWriter struct {
	mu  sync.Mutex
	dst *os.File
	out *processOutput
	buf []byte
}

func (w *readyLineWriter) Write(p []byte) (int, error) {
	if w.dst != nil { _, _ = w.dst.Write(p) }
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 { break }
		w.out.observe(strings.TrimRight(string(w.buf[:i]), "\r"))
		w.buf = append(w.buf[:0], w.buf[i+1:]...)
	}
	return len(p), nil
}

func (w *readyLineWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.buf) == 0 { return }
	w.out.observe(strings.TrimRight(string(w.buf), "\r\n"))
	w.buf = nil
}

type osProcess struct {
	cmd            *exec.Cmd
	out            *processOutput
	stdout, stderr *readyLineWriter
	done           chan struct{}
	mu             sync.Mutex
	exitErr        error
}

func (p *osProcess) wait() {
	err := p.cmd.Wait()
	p.stdout.Flush()
	p.stderr.Flush()
	p.mu.Lock()
	p.exitErr = err
	p.mu.Unlock()
	close(p.done)
}

func (p *osProcess) getExitErr() error { p.mu.Lock(); defer p.mu.Unlock(); return p.exitErr }

func (p *osProcess) WaitReady(marker string, timeout time.Duration) error {
	if p == nil { return errors.New("nil process") }
	marker = strings.TrimSpace(marker)
	if marker == "" { return nil }
	if timeout <= 0 { timeout = 25 * time.Second }
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		if p.out.contains(marker) { return nil }
		select {
		case <-p.out.notify:
			continue
		case <-p.done:
			if p.out.contains(marker) { return nil }
			if err := p.getExitErr(); err != nil { return fmt.Errorf("process exited before marker %q: %w", marker, err) }
			return fmt.Errorf("process exited before marker %q", marker)
		case <-timer.C:
			return fmt.Errorf("timeout waiting for marker %q", marker)
		}
	}
}

func (p *osProcess) Stop() error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil { return nil }
	select { case <-p.done: return nil; default: }
	killErr := p.cmd.Process.Kill()
	<-p.done
	if errors.Is(killErr, os.ErrProcessDone) { killErr = nil }
	if p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited() { killErr = nil }
	waitErr := p.getExitErr()
	if _, ok := waitErr.(*exec.ExitError); ok && killErr == nil { waitErr = nil }
	return errors.Join(killErr, waitErr)
}
