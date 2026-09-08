package main

import (
	"os"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

func TestClientKeepaliveDueDisabledAtZero(t *testing.T) {
	now := time.Unix(1000, 0)
	if clientKeepaliveDue(now.Add(-time.Hour), now, 0) {
		t.Fatal("keepalive=0 must not schedule LINK PING")
	}
	if clientKeepaliveDue(now.Add(-time.Hour), now, -time.Second) {
		t.Fatal("non-positive keepalive must stay disabled")
	}
}

func TestClientKeepaliveDueAtPositiveInterval(t *testing.T) {
	now := time.Unix(1000, 0)
	if clientKeepaliveDue(now.Add(time.Millisecond), now, time.Second) {
		t.Fatal("positive keepalive fired before deadline")
	}
	if !clientKeepaliveDue(now, now, time.Second) {
		t.Fatal("positive keepalive did not fire at deadline")
	}
}

func TestKeepaliveZeroNeverForcesRemoteRXExpiry(t *testing.T) {
	last := time.Unix(1000, 0)
	if clientRemoteRXExpired(last, last.Add(24*time.Hour), 0) {
		t.Fatal("keepalive=0 must not force blackhole retirement")
	}
	if clientRemoteRXExpired(last, last.Add(24*time.Hour), -time.Second) {
		t.Fatal("negative keepalive must remain disabled")
	}
}

func TestClientKeepaliveTrackerZeroNeverProbesOrExpires(t *testing.T) {
	base := time.Unix(1000, 0)
	tracker := newClientKeepaliveTracker(base, 0)
	for _, delta := range []time.Duration{time.Second, time.Hour, 24 * time.Hour} {
		send, nonce, dead := tracker.poll(base.Add(delta), 0)
		if send || nonce != 0 || dead {
			t.Fatalf("keepalive=0 delta=%s send=%t nonce=%d dead=%t", delta, send, nonce, dead)
		}
	}
}

func TestClientDataLoopKeepaliveZeroDoesNotRetireSilentPeer(t *testing.T) {
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
	go func() {
		done <- clientDataLoop(client, addr(dtls), path, establishedStartup{}, 0, stop)
	}()

	select {
	case err := <-done:
		t.Fatalf("keepalive=0 retired silent peer unexpectedly: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	stop <- os.Interrupt
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("clientDataLoop did not stop with keepalive disabled")
	}
}

func TestClientDataLoopKeepaliveZeroDoesNotRetireDownstreamOnlyPeer(t *testing.T) {
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
	go func() {
		done <- clientDataLoop(client, addr(dtls), path, establishedStartup{}, 0, stop)
	}()

	peer := addr(client)
	for i := 0; i < 10; i++ {
		wire, err := control.MarshalLink(control.Ping{Nonce: uint64(i + 1)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := dtls.WriteToUDP(wire, peer); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("keepalive=0 retired downstream-only peer unexpectedly: %v", err)
	default:
	}

	stop <- os.Interrupt
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("shutdown error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("clientDataLoop did not stop with keepalive disabled")
	}
}
