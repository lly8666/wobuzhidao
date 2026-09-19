package windowsruntime

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

const testTunnelConfigJSON = `{"tunnel_id":"11223344556677889900aabbccddeeff","address4":"10.66.0.1/32","routes4":["0.0.0.0/0"]}`

type recordingTicketStore struct {
	runner         *recordingRunner
	ticket         string
	tunnelConfig   string
	clearErr       error
	ticketReadErr  error
	configReadErr  error
}

func (s *recordingTicketStore) Clear(path string) error {
	if strings.Contains(strings.ToLower(path), "tunnel-config") {
		s.runner.events = append(s.runner.events, "state:clear:tunnel")
	} else {
		s.runner.events = append(s.runner.events, "state:clear:ticket")
	}
	return s.clearErr
}

func (s *recordingTicketStore) Read(path string) (string, error) {
	if strings.Contains(strings.ToLower(path), "tunnel-config") {
		s.runner.events = append(s.runner.events, "state:read:tunnel")
		if s.configReadErr != nil { return "", s.configReadErr }
		if s.tunnelConfig == "" { return testTunnelConfigJSON, nil }
		return s.tunnelConfig, nil
	}
	s.runner.events = append(s.runner.events, "state:read:ticket")
	if s.ticketReadErr != nil { return "", s.ticketReadErr }
	return s.ticket, nil
}

type recordingUnderlayDiscoverer struct {
	runner   *recordingRunner
	underlay Underlay
	err      error
}

func (d *recordingUnderlayDiscoverer) Discover(Profile) (Underlay, error) {
	d.runner.events = append(d.runner.events, "discover:underlay")
	if d.err != nil { return Underlay{}, d.err }
	return d.underlay, nil
}

type recordingPreflightDiscoverer struct {
	*recordingUnderlayDiscoverer
	preflightErr error
}

func (d *recordingPreflightDiscoverer) Preflight(Profile) error {
	d.runner.events = append(d.runner.events, "preflight:dependencies")
	return d.preflightErr
}

func testController(r *recordingRunner) *Controller {
	return NewController(
		r,
		&recordingUnderlayDiscoverer{runner: r, underlay: testUnderlay()},
		&recordingTicketStore{runner: r, ticket: strings.Repeat("ab", 32), tunnelConfig: testTunnelConfigJSON},
	)
}

func startupRecoveryEvents() []string { return []string{"run:route-cleanup", "run:ipv6-cleanup"} }
func withStartupRecovery(rest ...string) []string { return append(startupRecoveryEvents(), rest...) }

func TestControllerConnectDisconnectUsesAuthenticatedPerLaneSingleFlowLease(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile()
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil { t.Fatal(err) }
	if got := c.State(); got != RuntimeConnected { t.Fatalf("state after Connect = %s", got) }
	if err := c.Disconnect(); err != nil { t.Fatal(err) }
	if got := c.State(); got != RuntimeDisconnected { t.Fatalf("state after Disconnect = %s", got) }

	want := withStartupRecovery(
		"discover:underlay",
		"state:clear:ticket", "state:clear:tunnel",
		"start:faketcp-1", "ready:faketcp-1:bootstrap", "state:read:ticket", "state:read:tunnel",
		"ready:faketcp-1", "start:dtls-1", "ready:dtls-1", "start:link-1", "ready:link-1",
		"start:game", "ready:game", "start:tun", "ready:tun",
		"run:ipv6-apply", "run:route-apply",
		"run:route-cleanup", "run:ipv6-cleanup",
		"stop:tun", "stop:game", "stop:link-1", "stop:dtls-1", "stop:faketcp-1",
	)
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("controller events = %v want %v", r.events, want) }
	for _, e := range r.events {
		if strings.Contains(e, "reality-bootstrap") { t.Fatalf("separate public Reality bootstrap reintroduced: %v", r.events) }
	}
}

func TestControllerTwoLaneLifecycleKeepsIndependentLaneFlowsBehindOneGameAndTUN(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile()
	p.Lanes = 2
	p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil { t.Fatal(err) }
	if err := c.Disconnect(); err != nil { t.Fatal(err) }

	want := withStartupRecovery(
		"discover:underlay",
		"state:clear:ticket", "state:clear:tunnel", "start:faketcp-1", "ready:faketcp-1:bootstrap", "state:read:ticket", "state:read:tunnel",
		"state:clear:ticket", "state:clear:tunnel", "start:faketcp-2", "ready:faketcp-2:bootstrap", "state:read:ticket", "state:read:tunnel",
		"ready:faketcp-1", "start:dtls-1", "ready:dtls-1", "start:link-1", "ready:link-1",
		"ready:faketcp-2", "start:dtls-2", "ready:dtls-2", "start:link-2", "ready:link-2",
		"start:game", "ready:game", "start:tun", "ready:tun",
		"run:ipv6-apply", "run:route-apply",
		"run:route-cleanup", "run:ipv6-cleanup",
		"stop:tun", "stop:game",
		"stop:link-2", "stop:dtls-2", "stop:faketcp-2",
		"stop:link-1", "stop:dtls-1", "stop:faketcp-1",
	)
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("two-lane lifecycle=%v want=%v", r.events, want) }
}

func TestDecodeAuthenticatedTunnelConfigRejectsInvalidLease(t *testing.T) {
	if _, err := decodeAuthenticatedTunnelConfig(`{"tunnel_id":"bad","address4":"10.66.0.1/32","routes4":[]}`); err == nil { t.Fatal("invalid TunnelID accepted") }
	if _, err := decodeAuthenticatedTunnelConfig(`{"tunnel_id":"11223344556677889900aabbccddeeff","address4":"10.66.0.1/30","routes4":[]}`); err == nil { t.Fatal("non-/32 lease accepted") }
}

func TestControllerDependencyPreflightFailsBeforePublicFlow(t *testing.T) {
	r := &recordingRunner{}
	d := &recordingPreflightDiscoverer{recordingUnderlayDiscoverer: &recordingUnderlayDiscoverer{runner: r, underlay: testUnderlay()}, preflightErr: errors.New("Npcap missing")}
	c := NewController(r, d, &recordingTicketStore{runner: r, ticket: strings.Repeat("ef", 32), tunnelConfig: testTunnelConfigJSON})
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err == nil { t.Fatal("expected dependency preflight failure") }
	want := []string{"preflight:dependencies"}
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("events=%v want=%v", r.events, want) }
	if got := c.State(); got != RuntimeDisconnected { t.Fatalf("failed preflight state=%s", got) }
}

func TestControllerStartupRecoveryFailureStopsBeforeBootstrapStateOrPublicFlow(t *testing.T) {
	r := &recordingRunner{fail: "route-cleanup"}
	c := testController(r)
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err == nil { t.Fatal("expected startup cleanup failure") }
	want := []string{"run:route-cleanup"}
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("events=%v want=%v", r.events, want) }
	if got := c.State(); got != RuntimeDisconnected { t.Fatalf("failed recovery state=%s", got) }
}

func TestControllerFakeTCPStartFailureNeverStartsDTLSOrCapture(t *testing.T) {
	r := &recordingRunner{fail: "faketcp-1"}
	c := testController(r)
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err == nil { t.Fatal("expected FakeTCP failure") }
	want := withStartupRecovery("discover:underlay", "state:clear:ticket", "state:clear:tunnel", "start:faketcp-1")
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("events=%v want=%v", r.events, want) }
	if got := c.State(); got != RuntimeDisconnected { t.Fatalf("failed Connect state=%s", got) }
}

func TestControllerBootstrapReadinessFailureNeverPollsState(t *testing.T) {
	r := &recordingRunner{failMarker: singleFlowBootstrapReadyMarker}
	c := testController(r)
	p := testProfile(); p.TunnelIPv4 = ""
	err := c.Connect(p)
	if err == nil { t.Fatal("expected bootstrap readiness failure") }
	if !strings.Contains(err.Error(), "lane 1 Reality bootstrap") { t.Fatalf("bootstrap error lost lane context: %v", err) }
	want := withStartupRecovery("discover:underlay", "state:clear:ticket", "state:clear:tunnel", "start:faketcp-1", "ready:faketcp-1:bootstrap", "stop:faketcp-1")
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("events=%v want=%v", r.events, want) }
	for _, event := range r.events {
		if strings.HasPrefix(event, "state:read:") { t.Fatalf("state polling must not hide an exited bootstrap child: %v", r.events) }
	}
}

func TestControllerSecondLaneBootstrapFailureRollsBackBothPublicFlowsBeforeDTLS(t *testing.T) {
	r := &recordingRunner{failReady: "faketcp-2"}
	c := testController(r)
	p := testProfile(); p.Lanes = 2; p.TunnelIPv4 = ""
	if err := c.Connect(p); err == nil { t.Fatal("expected lane 2 bootstrap readiness failure") }
	want := withStartupRecovery(
		"discover:underlay",
		"state:clear:ticket", "state:clear:tunnel", "start:faketcp-1", "ready:faketcp-1:bootstrap", "state:read:ticket", "state:read:tunnel",
		"state:clear:ticket", "state:clear:tunnel", "start:faketcp-2", "ready:faketcp-2:bootstrap",
		"stop:faketcp-2", "stop:faketcp-1",
	)
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("lane bootstrap rollback=%v want=%v", r.events, want) }
	for _, event := range r.events {
		if strings.HasPrefix(event, "start:dtls-") || event == "run:ipv6-apply" || event == "run:route-apply" { t.Fatalf("network/data plane started after bootstrap failure: %v", r.events) }
	}
}

func TestControllerTicketFailureStopsOnlyPrestartedLane(t *testing.T) {
	r := &recordingRunner{}
	states := &recordingTicketStore{runner: r, ticket: strings.Repeat("cd", 32), tunnelConfig: testTunnelConfigJSON, ticketReadErr: errors.New("TLS auth failed")}
	c := NewController(r, &recordingUnderlayDiscoverer{runner: r, underlay: testUnderlay()}, states)
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err == nil { t.Fatal("expected ticket failure") }
	want := withStartupRecovery("discover:underlay", "state:clear:ticket", "state:clear:tunnel", "start:faketcp-1", "ready:faketcp-1:bootstrap", "state:read:ticket", "stop:faketcp-1")
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("events=%v want=%v", r.events, want) }
}

func TestControllerTunnelConfigFailureStopsBeforeDTLS(t *testing.T) {
	r := &recordingRunner{}
	states := &recordingTicketStore{runner: r, ticket: strings.Repeat("cd", 32), configReadErr: errors.New("missing authenticated config")}
	c := NewController(r, &recordingUnderlayDiscoverer{runner: r, underlay: testUnderlay()}, states)
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err == nil { t.Fatal("expected tunnel config failure") }
	want := withStartupRecovery("discover:underlay", "state:clear:ticket", "state:clear:tunnel", "start:faketcp-1", "ready:faketcp-1:bootstrap", "state:read:ticket", "state:read:tunnel", "stop:faketcp-1")
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("events=%v want=%v", r.events, want) }
}

func TestControllerExecutorRollbackStopsTransferredFakeTCPExactlyOnce(t *testing.T) {
	r := &recordingRunner{fail: "route-apply"}
	c := testController(r)
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err == nil { t.Fatal("expected route apply failure") }
	stops := 0
	for _, event := range r.events { if event == "stop:faketcp-1" { stops++ } }
	if stops != 1 { t.Fatalf("transferred FakeTCP stop count=%d events=%v", stops, r.events) }
	if got := c.State(); got != RuntimeDisconnected { t.Fatalf("failed Connect state=%s", got) }
}

func TestControllerUnderlayFailureNeverStartsPublicFlow(t *testing.T) {
	r := &recordingRunner{}
	states := &recordingTicketStore{runner: r, ticket: strings.Repeat("cd", 32), tunnelConfig: testTunnelConfigJSON}
	d := &recordingUnderlayDiscoverer{runner: r, err: errors.New("no neighbor")}
	c := NewController(r, d, states)
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err == nil { t.Fatal("expected underlay failure") }
	want := withStartupRecovery("discover:underlay")
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("events=%v want=%v", r.events, want) }
}

func TestControllerRejectsSecondConnectWithoutTouchingRuntime(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil { t.Fatal(err) }
	before := append([]string(nil), r.events...)
	if err := c.Connect(p); err == nil { t.Fatal("second Connect must fail while connected") }
	if !reflect.DeepEqual(r.events, before) { t.Fatalf("second Connect changed runtime events") }
	if err := c.Disconnect(); err != nil { t.Fatal(err) }
}

func TestControllerStateClearFailureStopsBeforePublicFlow(t *testing.T) {
	r := &recordingRunner{}
	c := NewController(r, &recordingUnderlayDiscoverer{runner: r, underlay: testUnderlay()}, &recordingTicketStore{runner: r, clearErr: errors.New("denied")})
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err == nil { t.Fatal("expected state clear failure") }
	want := withStartupRecovery("discover:underlay", "state:clear:ticket")
	if !reflect.DeepEqual(r.events, want) { t.Fatalf("events=%v want=%v", r.events, want) }
}

func TestControllerDisconnectedDisconnectRetriesPendingNetworkCleanup(t *testing.T) {
	r := &recordingRunner{}
	c := testController(r)
	p := testProfile(); p.TunnelIPv4 = ""
	if err := c.Connect(p); err != nil { t.Fatal(err) }
	r.failOnce = "route-cleanup"
	if err := c.Disconnect(); err == nil { t.Fatal("expected first cleanup failure") }
	if got := c.State(); got != RuntimeDisconnected { t.Fatalf("state after failed cleanup=%s", got) }
	if err := c.Connect(p); err == nil { t.Fatal("Connect must remain blocked until pending cleanup succeeds") }
	if err := c.Disconnect(); err != nil { t.Fatalf("disconnected Disconnect must retry cleanup: %v", err) }
	if err := c.Connect(p); err != nil { t.Fatalf("Connect after cleanup retry: %v", err) }
	if err := c.Disconnect(); err != nil { t.Fatal(err) }
}
