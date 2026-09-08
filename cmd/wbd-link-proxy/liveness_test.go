package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

func TestClientRemoteRXDeadlineUsesThreeKeepaliveWindows(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	keepalive := 10 * time.Second
	if clientRemoteRXExpired(base, base.Add(3*keepalive-time.Nanosecond), keepalive) {
		t.Fatal("remote RX expired before three keepalive windows")
	}
	if !clientRemoteRXExpired(base, base.Add(3*keepalive), keepalive) {
		t.Fatal("remote RX did not expire at three keepalive windows")
	}
	refreshed := base.Add(2 * keepalive)
	if clientRemoteRXExpired(refreshed, base.Add(4*keepalive), keepalive) {
		t.Fatal("valid remote receive did not refresh liveness deadline")
	}
}

func TestClientKeepaliveTrackerUsesTwelveAttemptsInsideHistoricalBudget(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	keepalive := 6 * time.Second
	wait := clientKeepaliveProbeWait(keepalive)
	if wait != time.Second {
		t.Fatalf("probe wait=%s want=1s", wait)
	}
	tracker := newClientKeepaliveTracker(base, keepalive)
	if send, _, dead := tracker.poll(base.Add(keepalive-time.Nanosecond), keepalive); send || dead {
		t.Fatalf("probe fired early send=%t dead=%t", send, dead)
	}
	var nonce uint64
	for attempt := 1; attempt <= clientKeepaliveTotalProbeAttempts; attempt++ {
		now := base.Add(keepalive + time.Duration(attempt-1)*wait)
		send, gotNonce, dead := tracker.poll(now, keepalive)
		if !send || dead {
			t.Fatalf("attempt %d send=%t dead=%t", attempt, send, dead)
		}
		if attempt == 1 {
			nonce = gotNonce
			if nonce == 0 {
				t.Fatal("zero nonce")
			}
		} else if gotNonce != nonce {
			t.Fatalf("attempt %d nonce=%d want=%d", attempt, gotNonce, nonce)
		}
	}
	if send, _, dead := tracker.poll(base.Add(3*keepalive), keepalive); send || !dead {
		t.Fatalf("historical deadline send=%t dead=%t", send, dead)
	}
}

func TestClientKeepaliveTrackerMatchingPongRecoversWrongPongDoesNot(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	keepalive := 60 * time.Millisecond
	tracker := newClientKeepaliveTracker(base, keepalive)
	send, nonce, dead := tracker.poll(base.Add(keepalive), keepalive)
	if !send || dead || nonce == 0 {
		t.Fatalf("first probe send=%t nonce=%d dead=%t", send, nonce, dead)
	}
	if tracker.observePong(nonce+1, base.Add(keepalive+time.Millisecond), keepalive) {
		t.Fatal("wrong nonce unexpectedly refreshed liveness")
	}
	if !tracker.observePong(nonce, base.Add(keepalive+2*time.Millisecond), keepalive) {
		t.Fatal("matching pong did not recover liveness")
	}
	if send, _, dead := tracker.poll(base.Add(2*keepalive+time.Millisecond), keepalive); send || dead {
		t.Fatalf("recovered tracker fired too early send=%t dead=%t", send, dead)
	}
}

func TestClientDataLoopOutboundTrafficCannotMaskNoRoundTrip(t *testing.T) {
	client := udp4(t)
	dtls := udp4(t)
	app := udp4(t)
	defer client.Close()
	defer dtls.Close()
	defer app.Close()

	path, err := linkdata.New(control.LinkConfig{
		FECMode: control.FECOff, Scheduler: control.FECSchedulerNone,
		MTU: 1400, LaneCount: 1,
	}, maxBlocks)
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	keepalive := 10 * time.Millisecond
	go func() {
		done <- clientDataLoop(client, addr(dtls), path, establishedStartup{}, keepalive, stop)
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		_, _ = app.WriteToUDP([]byte("outbound-does-not-prove-peer-liveness"), addr(client))
		select {
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "liveness timeout") {
				t.Fatalf("client loop error=%v", err)
			}
			return
		case <-time.After(5 * time.Millisecond):
		}
	}
	stop <- os.Interrupt
	t.Fatal("continuous outbound traffic masked missing keepalive round trip")
}

func TestClientDataLoopDownstreamPingTrafficCannotMaskUpstreamBlackhole(t *testing.T) {
	client := udp4(t)
	dtls := udp4(t)
	defer client.Close()
	defer dtls.Close()

	path, err := linkdata.New(control.LinkConfig{
		FECMode: control.FECOff, Scheduler: control.FECSchedulerNone,
		MTU: 1400, LaneCount: 1,
	}, maxBlocks)
	if err != nil {
		t.Fatal(err)
	}

	stop := make(chan os.Signal, 1)
	done := make(chan error, 1)
	keepalive := 12 * time.Millisecond
	go func() {
		done <- clientDataLoop(client, addr(dtls), path, establishedStartup{}, keepalive, stop)
	}()

	// This models a server->client-only path. The peer can inject valid LINK PING
	// frames, while everything the client sends upstream is simply ignored.
	peer := addr(client)
	ticker := time.NewTicker(3 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		select {
		case now := <-ticker.C:
			wire, err := control.MarshalLink(control.Ping{Nonce: uint64(now.UnixNano())})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := dtls.WriteToUDP(wire, peer); err != nil {
				t.Fatal(err)
			}
		case err := <-done:
			if err == nil || !strings.Contains(err.Error(), "liveness timeout") {
				t.Fatalf("client loop error=%v", err)
			}
			return
		}
	}
	stop <- os.Interrupt
	t.Fatal("downstream-only LINK traffic masked upstream blackhole")
}
