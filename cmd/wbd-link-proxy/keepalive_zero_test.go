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
	case <-time.After(80 * time.Millisecond):
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
