package windowsruntime

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func soakCommandArg(args []string, key string) (string, bool) {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key {
			return args[i+1], true
		}
	}
	return "", false
}

func soakEventCount(events []string, want string) int {
	count := 0
	for _, event := range events {
		if event == want {
			count++
		}
	}
	return count
}

func soakLaneSourcesAndKeepalive(t *testing.T, c *Controller, lanes int) map[int]string {
	t.Helper()
	c.mu.Lock()
	plans := cloneLanePlans(c.lanePlans)
	c.mu.Unlock()
	if len(plans) != lanes {
		t.Fatalf("active lane count=%d want=%d", len(plans), lanes)
	}
	out := make(map[int]string, lanes)
	for id := 1; id <= lanes; id++ {
		plan, ok := plans[id]
		if !ok {
			t.Fatalf("lane %d missing from plans=%v", id, plans)
		}
		source, ok := soakCommandArg(plan.FakeTCP.Args, "--source")
		if !ok {
			t.Fatalf("lane %d FakeTCP source missing: %v", id, plan.FakeTCP.Args)
		}
		out[id] = source
		keepalive, ok := soakCommandArg(plan.Link.Args, "-keepalive")
		if !ok || keepalive != "15s" {
			t.Fatalf("lane %d keepalive=%q present=%v want=15s args=%v", id, keepalive, ok, plan.Link.Args)
		}
	}
	return out
}

func assertDormantLaneShutdown(t *testing.T, c *Controller, r *recordingRunner, lanes int, phase string) {
	t.Helper()
	if got := c.State(); got != RuntimeDormant {
		t.Fatalf("%s: runtime self-woke without payload: %s", phase, got)
	}
	if got := c.executor.DynamicLaneIDs(); len(got) != 0 {
		t.Fatalf("%s: DORMANT retained public lanes=%v", phase, got)
	}
	for id := 1; id <= lanes; id++ {
		lane := strconv.Itoa(id)
		if got := soakEventCount(r.events, "stop:link-"+lane); got != 1 {
			t.Fatalf("%s: lane %d link stop count=%d events=%v", phase, id, got, r.events)
		}
		if got := soakEventCount(r.events, "stop:dtls-"+lane); got != 1 {
			t.Fatalf("%s: lane %d DTLS stop count=%d events=%v", phase, id, got, r.events)
		}
		if got := soakEventCount(r.events, "stop:faketcp-"+lane); got != 1 {
			t.Fatalf("%s: lane %d FakeTCP stop count=%d events=%v", phase, id, got, r.events)
		}
		if got := soakEventCount(r.events, "start:link-"+lane); got != 1 {
			t.Fatalf("%s: lane %d LINK restarted while dormant count=%d events=%v", phase, id, got, r.events)
		}
		if got := soakEventCount(r.events, "start:dtls-"+lane); got != 1 {
			t.Fatalf("%s: lane %d DTLS restarted while dormant count=%d events=%v", phase, id, got, r.events)
		}
		if got := soakEventCount(r.events, "start:faketcp-"+lane); got != 1 {
			t.Fatalf("%s: lane %d FakeTCP restarted while dormant count=%d events=%v", phase, id, got, r.events)
		}
	}
}

// TestPayloadIdleTwoMinuteSoakDormantAndWake is intentionally gated because it
// uses the real product idle interval. The dedicated Windows Actions workflow
// runs lanes=1 and lanes=4 in parallel. It proves that real payload inactivity,
// not keepalive/control traffic, tears down every public Transport Lane after
// two minutes; that DORMANT stays fully quiescent for a further 105 seconds;
// and that the first later payload activity rebuilds fresh lanes after an
// underlay change without restarting the shared Game/TUN/network context.
func TestPayloadIdleTwoMinuteSoakDormantAndWake(t *testing.T) {
	if os.Getenv("WBD_IDLE_WAKE_SOAK") != "1" {
		t.Skip("set WBD_IDLE_WAKE_SOAK=1 in the dedicated real-time soak workflow")
	}
	lanes, err := strconv.Atoi(os.Getenv("WBD_IDLE_WAKE_LANES"))
	if err != nil || (lanes != 1 && lanes != 4) {
		t.Fatalf("WBD_IDLE_WAKE_LANES must be 1 or 4, got=%q", os.Getenv("WBD_IDLE_WAKE_LANES"))
	}

	r := &recordingRunner{}
	c := testController(r)
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = lanes
	p.IdleTimeoutSeconds = 120
	keepalive := 15
	p.KeepaliveSeconds = &keepalive
	if err := c.Connect(p); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := c.Disconnect(); err != nil {
			t.Errorf("disconnect: %v", err)
		}
	}()

	control, markPayload, laneSets := payloadIdleControlResponder(t)
	setControllerGameControl(c, control)
	// Restart the monitor only to anchor this soak's wall-clock observation after
	// the test control responder is installed. The timeout is the real 120s.
	c.startPayloadIdleMonitor(120 * time.Second)
	markPayload()

	beforeSources := soakLaneSourcesAndKeepalive(t, c, lanes)
	startedAt := time.Now()
	waitControllerState(t, c, RuntimeDormant, 155*time.Second)
	dormantAfter := time.Since(startedAt)
	if dormantAfter < 118*time.Second {
		t.Fatalf("entered DORMANT too early after %s; product idle contract is 120s", dormantAfter)
	}
	if got := c.executor.DynamicLaneIDs(); len(got) != 0 {
		t.Fatalf("DORMANT retained public lanes=%v", got)
	}
	select {
	case cmd := <-laneSets:
		if len(cmd.Lanes) != 0 {
			t.Fatalf("DORMANT Game barrier lanes=%v", cmd.Lanes)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("DORMANT did not publish an empty Game lane set")
	}
	assertDormantLaneShutdown(t, c, r, lanes, "entry")
	fmt.Printf("WBD_IDLE_WAKE_DORMANT_PASS lanes=%d elapsed=%s keepalive=15s idle=120s\n", lanes, dormantAfter.Round(time.Millisecond))

	// Observe the closed-link state for a full 105 seconds (1m45s) before
	// injecting any new payload. Sampling every five seconds proves the runtime
	// remains dormant and no FakeTCP/DTLS/LINK lane self-restarts because of
	// keepalive, control traffic, timers, or scheduler jitter.
	quietObservation := 105 * time.Second
	quietStarted := time.Now()
	quietDeadline := quietStarted.Add(quietObservation)
	for sample := 1; ; sample++ {
		remaining := time.Until(quietDeadline)
		if remaining <= 0 {
			break
		}
		sleep := 5 * time.Second
		if remaining < sleep {
			sleep = remaining
		}
		time.Sleep(sleep)
		assertDormantLaneShutdown(t, c, r, lanes, fmt.Sprintf("quiet-sample-%d", sample))
	}
	quietElapsed := time.Since(quietStarted)
	if quietElapsed < quietObservation {
		t.Fatalf("quiet observation ended early after %s want>=%s", quietElapsed, quietObservation)
	}
	assertDormantLaneShutdown(t, c, r, lanes, "quiet-final")
	fmt.Printf("WBD_IDLE_WAKE_QUIET_PASS lanes=%d quiet=%s state=dormant dynamic_lanes=0\n", lanes, quietElapsed.Round(time.Millisecond))

	// Simulate the physical link changing while dormant. Wake must rediscover the
	// underlay and build fresh public associations when a new payload arrives.
	discoverer, ok := c.discoverer.(*recordingUnderlayDiscoverer)
	if !ok {
		t.Fatalf("unexpected discoverer type %T", c.discoverer)
	}
	changedUnderlay := testUnderlay()
	changedUnderlay.SourceIP = "192.0.2.21"
	discoverer.underlay = changedUnderlay

	wakeStarted := time.Now()
	markPayload()
	waitControllerState(t, c, RuntimeConnected, 45*time.Second)
	wakeElapsed := time.Since(wakeStarted)
	afterSources := soakLaneSourcesAndKeepalive(t, c, lanes)
	for id := 1; id <= lanes; id++ {
		if beforeSources[id] == afterSources[id] {
			t.Fatalf("lane %d reused pre-dormant association source=%q", id, afterSources[id])
		}
		if !strings.HasPrefix(afterSources[id], "192.0.2.21:") {
			t.Fatalf("lane %d did not rediscover changed underlay source=%q", id, afterSources[id])
		}
		if got := soakEventCount(r.events, "start:faketcp-"+strconv.Itoa(id)); got != 2 {
			t.Fatalf("lane %d FakeTCP start count=%d want=2 events=%v", id, got, r.events)
		}
		if got := soakEventCount(r.events, "start:dtls-"+strconv.Itoa(id)); got != 2 {
			t.Fatalf("lane %d DTLS start count=%d want=2 events=%v", id, got, r.events)
		}
		if got := soakEventCount(r.events, "start:link-"+strconv.Itoa(id)); got != 2 {
			t.Fatalf("lane %d LINK start count=%d want=2 events=%v", id, got, r.events)
		}
	}
	if got := soakEventCount(r.events, "start:game"); got != 1 {
		t.Fatalf("wake restarted shared Game process count=%d events=%v", got, r.events)
	}
	if got := soakEventCount(r.events, "start:tun"); got != 1 {
		t.Fatalf("wake restarted shared TUN process count=%d events=%v", got, r.events)
	}

	var finalCount int
	deadline := time.After(5 * time.Second)
	for finalCount < lanes {
		select {
		case cmd := <-laneSets:
			finalCount = len(cmd.Lanes)
		case <-deadline:
			t.Fatalf("wake Game lane publication stopped at %d want=%d", finalCount, lanes)
		}
	}
	fmt.Printf("WBD_IDLE_WAKE_RECONNECT_PASS lanes=%d wake=%s quiet=%s old_sources=%v new_sources=%v\n", lanes, wakeElapsed.Round(time.Millisecond), quietElapsed.Round(time.Millisecond), beforeSources, afterSources)
}
