package windowsruntime

import (
	"errors"
	"reflect"
	"testing"
)

type recordingRunner struct {
	events   []string
	fail     string
	failOnce string
}

func (r *recordingRunner) shouldFail(name string) bool {
	if r.fail == name { return true }
	if r.failOnce == name { r.failOnce = ""; return true }
	return false
}
func (r *recordingRunner) Run(command Command) error {
	r.events = append(r.events, "run:"+command.Name)
	if r.shouldFail(command.Name) { return errors.New("injected failure") }
	return nil
}
func (r *recordingRunner) Start(command Command) (Process, error) {
	r.events = append(r.events, "start:"+command.Name)
	if r.shouldFail(command.Name) { return nil, errors.New("injected failure") }
	return &recordingProcess{runner: r, name: command.Name}, nil
}
type recordingProcess struct { runner *recordingRunner; name string }
func (p *recordingProcess) Stop() error { p.runner.events = append(p.runner.events, "stop:"+p.name); return nil }

func testExecutorPlan() Plan {
	return Plan{
		FakeTCP: Command{Name: "faketcp"}, DTLS: Command{Name: "dtls"}, Link: Command{Name: "link"}, TUN: Command{Name: "tun"},
		IPv6Apply: Command{Name: "ipv6-apply"}, RouteApply: Command{Name: "route-apply"},
		RouteCleanup: Command{Name: "route-cleanup"}, IPv6Cleanup: Command{Name: "ipv6-cleanup"},
	}
}

func TestExecutorStartStopPreservesFrozenLifecycleOrder(t *testing.T) {
	r := &recordingRunner{}
	e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err != nil { t.Fatal(err) }
	if !e.Running() { t.Fatal("executor must report running after routes are applied") }
	if err := e.Stop(); err != nil { t.Fatal(err) }
	want := []string{
		"start:faketcp", "start:dtls", "start:link", "start:tun", "run:ipv6-apply", "run:route-apply",
		"run:route-cleanup", "run:ipv6-cleanup", "stop:tun", "stop:link", "stop:dtls", "stop:faketcp",
	}
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("lifecycle events = %v want %v", r.events, want) }
	if err := e.Stop(); err != nil { t.Fatalf("second Stop must be idempotent: %v", err) }
}

func TestExecutorProcessStartFailureRollsBackWithoutNetworkMutation(t *testing.T) {
	r := &recordingRunner{fail: "link"}
	e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err == nil { t.Fatal("expected injected start failure") }
	want := []string{"start:faketcp", "start:dtls", "start:link", "stop:dtls", "stop:faketcp"}
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("rollback events = %v want %v", r.events, want) }
}

func TestExecutorIPv6FailureCleansBeforeProcessRollback(t *testing.T) {
	r := &recordingRunner{fail: "ipv6-apply"}
	e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err == nil { t.Fatal("expected injected IPv6 failure") }
	want := []string{
		"start:faketcp", "start:dtls", "start:link", "start:tun", "run:ipv6-apply", "run:ipv6-cleanup",
		"stop:tun", "stop:link", "stop:dtls", "stop:faketcp",
	}
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("IPv6 rollback events = %v want %v", r.events, want) }
}

func TestExecutorRouteFailureCleansRoutesThenIPv6BeforeProcessRollback(t *testing.T) {
	r := &recordingRunner{fail: "route-apply"}
	e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err == nil { t.Fatal("expected injected route failure") }
	want := []string{
		"start:faketcp", "start:dtls", "start:link", "start:tun", "run:ipv6-apply", "run:route-apply",
		"run:route-cleanup", "run:ipv6-cleanup", "stop:tun", "stop:link", "stop:dtls", "stop:faketcp",
	}
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("route rollback events = %v want %v", r.events, want) }
}

func TestExecutorRejectsSecondStart(t *testing.T) {
	r := &recordingRunner{}; e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err != nil { t.Fatal(err) }
	defer e.Stop()
	if err := e.Start(testExecutorPlan()); err == nil { t.Fatal("second Start must fail while runtime is active") }
}

func TestExecutorRetriesFailedCleanupBeforeAllowingRestart(t *testing.T) {
	r := &recordingRunner{}; e := NewExecutor(r)
	if err := e.Start(testExecutorPlan()); err != nil { t.Fatal(err) }
	r.failOnce = "route-cleanup"
	if err := e.Stop(); err == nil { t.Fatal("expected first cleanup to fail") }
	if e.Running() { t.Fatal("children must be stopped even when cleanup fails") }
	if !e.CleanupPending() { t.Fatal("failed cleanup must remain pending") }
	if err := e.Start(testExecutorPlan()); err == nil { t.Fatal("restart must be blocked while cleanup is pending") }
	if err := e.Stop(); err != nil { t.Fatalf("second Stop must retry cleanup: %v", err) }
	if e.CleanupPending() { t.Fatal("successful cleanup retry must clear pending state") }
	if err := e.Start(testExecutorPlan()); err != nil { t.Fatalf("restart after cleanup retry: %v", err) }
	if err := e.Stop(); err != nil { t.Fatal(err) }

	wantPrefix := []string{
		"start:faketcp", "start:dtls", "start:link", "start:tun", "run:ipv6-apply", "run:route-apply",
		"run:route-cleanup", "run:ipv6-cleanup", "stop:tun", "stop:link", "stop:dtls", "stop:faketcp",
		"run:route-cleanup", "run:ipv6-cleanup",
	}
	if len(r.events) < len(wantPrefix) || !reflect.DeepEqual(r.events[:len(wantPrefix)], wantPrefix) {
		t.Fatalf("cleanup retry events = %v want prefix %v", r.events, wantPrefix)
	}
}
