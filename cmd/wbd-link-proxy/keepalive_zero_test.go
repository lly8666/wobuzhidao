package main

import (
	"testing"
	"time"
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
}
