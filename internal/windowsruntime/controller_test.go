package windowsruntime

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

type recordingTicketStore struct {
	runner   *recordingRunner
	ticket   string
	clearErr error
	readErr  error
}

func (s *recordingTicketStore) Clear(string) error {
	s.runner.events = append(s.runner.events, "ticket:clear")
	return s.clearErr
}

func (s *recordingTicketStore) Read(string) (string, error) {
	s.runner.events = append(s.runner.events, "ticket:read")
	if s.readErr != nil {
		return "", s.readErr
	}
	return s.ticket, nil
}

type recordingUnderlayDiscoverer struct {
	runner   *recordingRunner
	underlay Underlay
	err      error
}

func (d *recordingUnderlayDiscoverer) Discover(Profile) (Underlay, error) {
	d.runner.events = append(d.runner.events, "discover:underlay")
	if d.err != nil {
		return Underlay{}, d.err
	}
	return d.underlay, nil
}

func testController(r *recordingRunner) *Controller {
	return NewController(
		r,
		&recordingUnderlayDiscoverer{runner: r, underlay: testUnderlay()},
		&recordingTicketStore{runner: r, ticket: strings.Repeat("ab", 32)},
	)
}

func TestControllerConnectDisconnectPreservesCompositionOrder(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	if err := c.Connect(testProfile()); err != nil {
		t.Fatal(err)
	}
	if got := c.State(); got != RuntimeConnected {
		t.Fatalf("state after Connect = %s", got)
	}
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
	if got := c.State(); got != RuntimeDisconnected {
		t.Fatalf("state after Disconnect = %s", got)
	}

	want := []string{
		"ticket:clear",
		"run:reality-bootstrap",
		"ticket:read",
		"discover:underlay",
		"start:faketcp", "start:dtls", "start:link", "start:tun", "run:route-apply",
		"run:route-cleanup", "stop:tun", "stop:link", "stop:dtls", "stop:faketcp",
	}
	if !reflect.DeepEqual(r.events, want) {
		t.Fatalf("controller events = %v want %v", r.events, want)
	}
}

func TestControllerBootstrapFailureNeverStartsCaptureStack(t *testing.T) {
	r := &recordingRunner{fail: "reality-bootstrap"}
	c := testController(r)
	if err := c.Connect(testProfile()); err == nil {
		t.Fatal("expected bootstrap failure")
	}
	want := []string{"ticket:clear", "run:reality-bootstrap"}
	if !reflect.DeepEqual(r.events, want) {
		t.Fatalf("events = %v want %v", r.events, want)
	}
	if got := c.State(); got != RuntimeDisconnected {
		t.Fatalf("failed Connect state = %s", got)
	}
}

func TestControllerUnderlayFailureNeverStartsCaptureStack(t *testing.T) {
	r := &recordingRunner{}
	tickets := &recordingTicketStore{runner: r, ticket: strings.Repeat("cd", 32)}
	discoverer := &recordingUnderlayDiscoverer{runner: r, err: errors.New("no neighbor")}
	c := NewController(r, discoverer, tickets)
	if err := c.Connect(testProfile()); err == nil {
		t.Fatal("expected underlay failure")
	}
	want := []string{"ticket:clear", "run:reality-bootstrap", "ticket:read", "discover:underlay"}
	if !reflect.DeepEqual(r.events, want) {
		t.Fatalf("events = %v want %v", r.events, want)
	}
}

func TestControllerRejectsSecondConnectWithoutTouchingRuntime(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	if err := c.Connect(testProfile()); err != nil {
		t.Fatal(err)
	}
	before := append([]string(nil), r.events...)
	if err := c.Connect(testProfile()); err == nil {
		t.Fatal("second Connect must fail while connected")
	}
	if !reflect.DeepEqual(r.events, before) {
		t.Fatalf("second Connect changed runtime events: before=%v after=%v", before, r.events)
	}
	if err := c.Disconnect(); err != nil {
		t.Fatal(err)
	}
}

func TestControllerTicketClearFailureStopsBeforeBootstrap(t *testing.T) {
	r := &recordingRunner{}
	c := NewController(
		r,
		&recordingUnderlayDiscoverer{runner: r, underlay: testUnderlay()},
		&recordingTicketStore{runner: r, clearErr: errors.New("denied")},
	)
	if err := c.Connect(testProfile()); err == nil {
		t.Fatal("expected ticket clear failure")
	}
	want := []string{"ticket:clear"}
	if !reflect.DeepEqual(r.events, want) {
		t.Fatalf("events = %v want %v", r.events, want)
	}
}
