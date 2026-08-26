//go:build windows

package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/windowsruntime"
)

type exitTestRunner struct {
	events []string
}

func (r *exitTestRunner) Run(command windowsruntime.Command) error {
	r.events = append(r.events, "run:"+command.Name)
	return nil
}

func (r *exitTestRunner) Start(command windowsruntime.Command) (windowsruntime.Process, error) {
	r.events = append(r.events, "start:"+command.Name)
	return &exitTestProcess{runner: r, name: command.Name}, nil
}

type exitTestProcess struct {
	runner *exitTestRunner
	name   string
}

func (p *exitTestProcess) Stop() error {
	p.runner.events = append(p.runner.events, "stop:"+p.name)
	return nil
}

type exitTestDiscoverer struct{}

func (exitTestDiscoverer) Discover(windowsruntime.Profile) (windowsruntime.Underlay, error) {
	return windowsruntime.Underlay{
		SourceIP:     "192.0.2.20",
		PacketDevice: `\Device\NPF_{01234567-89AB-CDEF-0123-456789ABCDEF}`,
		SourceMAC:    "00:11:22:33:44:55",
		NextHopMAC:   "66:77:88:99:aa:bb",
	}, nil
}

type exitTestTickets struct{}

func (exitTestTickets) Clear(string) error { return nil }
func (exitTestTickets) Read(string) (string, error) {
	return strings.Repeat("ab", 32), nil
}

func exitTestProfile() windowsruntime.Profile {
	return windowsruntime.Profile{
		BinDir:      `C:\Program Files\WBD`,
		ServerFront: "198.51.100.10:40443",
		ServerName:  "front.example",
		RouteKey:    "0123456789abcdef",
		Username:    "solo",
		Password:    "shared-password",
		ServerRaw:   "198.51.100.10:40000",
		FEC:         "off",
		IfName:      "WBD",
		MTU:         1400,
		RouteMode:   "Full",
		TunnelIPv4:  "10.66.0.2/30",
		TicketPath:  `C:\ProgramData\WBD\ticket.tmp`,
		RouteState:  `C:\ProgramData\WBD\route-state.json`,
	}
}

func wantExitLifecycle() []string {
	return []string{
		"run:route-cleanup",
		"stop:tun",
		"stop:link",
		"stop:dtls",
		"stop:faketcp",
	}
}

func connectedExitTestController(t *testing.T) (*windowsruntime.Controller, *exitTestRunner) {
	t.Helper()
	runner := &exitTestRunner{}
	controller := windowsruntime.NewController(runner, exitTestDiscoverer{}, exitTestTickets{})
	if err := controller.Connect(exitTestProfile()); err != nil {
		t.Fatal(err)
	}
	runner.events = nil
	return controller, runner
}

func TestExplicitExitAutomaticallyRunsRouteCleanupBeforeProcessTeardown(t *testing.T) {
	saved := app
	defer func() { app = saved }()

	controller, runner := connectedExitTestController(t)
	app.controller = controller
	app.results = make(chan runtimeResult, 1)
	app.window = 0
	app.operation = ""
	app.exitRequested = false
	app.cleanupFailed = false

	// This is the same function reached by both the window Exit button and the
	// tray Exit command. The user does not need to click Disconnect first.
	requestExit(0)

	var result runtimeResult
	select {
	case result = <-app.results:
	case <-time.After(2 * time.Second):
		t.Fatal("Exit did not trigger runtime cleanup")
	}
	if result.action != "disconnect" || result.err != nil {
		t.Fatalf("Exit cleanup result = action=%q err=%v", result.action, result.err)
	}
	if !reflect.DeepEqual(runner.events, wantExitLifecycle()) {
		t.Fatalf("Exit lifecycle = %v want %v", runner.events, wantExitLifecycle())
	}
	if controller.State() != windowsruntime.RuntimeDisconnected {
		t.Fatalf("Exit left controller state %s", controller.State())
	}
}

func TestProcessExitFallbackAutomaticallyCleansConnectedRuntime(t *testing.T) {
	saved := app
	defer func() { app = saved }()

	controller, runner := connectedExitTestController(t)
	app.controller = controller
	app.results = make(chan runtimeResult, 1)
	app.operation = ""

	// This is the second safety net reached after the Win32 message loop ends,
	// including an error return. It must use the same route-first teardown.
	if err := cleanupBeforeProcessExit(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(runner.events, wantExitLifecycle()) {
		t.Fatalf("process-exit lifecycle = %v want %v", runner.events, wantExitLifecycle())
	}
	if controller.State() != windowsruntime.RuntimeDisconnected {
		t.Fatalf("process-exit cleanup left controller state %s", controller.State())
	}
}
