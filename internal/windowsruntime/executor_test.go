package windowsruntime

import (
	"errors"
	"reflect"
	"testing"
)

type recordingRunner struct {
	events []string
	fail   string
}

func (r *recordingRunner) Run(command Command) error {
	r.events = append(r.events, "run:"+command.Name)
	if r.fail == command.Name {
		return errors.New("injected failure")
	}
	return nil
}

func (r *recordingRunner) Start(command Command) (Process, error) {
	r.events = append(r.events, "start:"+command.Name)
	if r.fail == command.Name {
		return nil, errors.New("injected failure")
	}
	return &recordingProcess{runner: r, name: command.Name}, nil
}

type recordingProcess struct {
	runner *recordingRunner
	name   string
}

func (p *recordingProcess) Stop() error {
	p.runner.events = append(p.runner.events, "stop:"+p.name)
	return nil
}

func testExecutorPlan() Plan {
	return Plan{
		FakeTCP:      Command{Name: "faketcp"},
		DTLS:         Command{Name: "dtls"},
		Link:         Command{Name: "link"},
		TUN:          Command{Name: "tun"},
		RouteApply:   Command{Name: "route-apply"},
		RouteCleanup: Command{Name: "route-cleanup"},
	}
}

func TestExecutorStartStopPreservesFrozenLifecycleOrder(t *testing.T) {
	r := &recordingRunner{}
	e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err != nil {
		t.Fatal(err)
	}
	if !e.Running() {
		t.Fatal("executor must report running after routes are applied")
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	if e.Running() {
		t.Fatal("executor must report stopped after cleanup")
	}
	want := []string{
		"start:faketcp", "start:dtls", "start:link", "start:tun", "run:route-apply",
		"run:route-cleanup", "stop:tun", "stop:link", "stop:dtls", "stop:faketcp",
	}
	if !reflect.DeepEqual(r.events, want) {
		t.Fatalf("lifecycle events = %v want %v", r.events, want)
	}
	if err := e.Stop(); err != nil {
		t.Fatalf("second Stop must be idempotent: %v", err)
	}
}

func TestExecutorProcessStartFailureRollsBackWithoutCaptureMutation(t *testing.T) {
	r := &recordingRunner{fail: "link"}
	e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err == nil {
		t.Fatal("expected injected start failure")
	}
	want := []string{"start:faketcp", "start:dtls", "start:link", "stop:dtls", "stop:faketcp"}
	if !reflect.DeepEqual(r.events, want) {
		t.Fatalf("rollback events = %v want %v", r.events, want)
	}
	if e.Running() {
		t.Fatal("failed start must not leave executor running")
	}
}

func TestExecutorRouteFailureCleansRoutesBeforeProcessRollback(t *testing.T) {
	r := &recordingRunner{fail: "route-apply"}
	e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err == nil {
		t.Fatal("expected injected route failure")
	}
	want := []string{
		"start:faketcp", "start:dtls", "start:link", "start:tun", "run:route-apply",
		"run:route-cleanup", "stop:tun", "stop:link", "stop:dtls", "stop:faketcp",
	}
	if !reflect.DeepEqual(r.events, want) {
		t.Fatalf("route rollback events = %v want %v", r.events, want)
	}
}

func TestExecutorRejectsSecondStart(t *testing.T) {
	r := &recordingRunner{}
	e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err != nil {
		t.Fatal(err)
	}
	defer e.Stop()
	if err := e.Start(testExecutorPlan()); err == nil {
		t.Fatal("second Start must fail while runtime is active")
	}
}
