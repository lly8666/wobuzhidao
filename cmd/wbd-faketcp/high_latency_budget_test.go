package main

import (
	"testing"
	"time"
)

func TestStartupBudgetsSupport300msOneWayPath(t *testing.T) {
	const targetOneWay = 300 * time.Millisecond
	const targetRTT = 2 * targetOneWay

	if fakeTCPHandshakeReadWindow < 2*targetRTT {
		t.Fatalf("FakeTCP per-attempt window=%s want >=%s for high-latency RTT+jitter margin", fakeTCPHandshakeReadWindow, 2*targetRTT)
	}
	if fakeTCPInitialRTOFloor < 2*targetRTT {
		t.Fatalf("FakeTCP initial RTO floor=%s want >=%s", fakeTCPInitialRTOFloor, 2*targetRTT)
	}
	if fakeTCPHandshakeTimeout < 20*targetRTT {
		t.Fatalf("FakeTCP handshake budget=%s want >=%s", fakeTCPHandshakeTimeout, 20*targetRTT)
	}
	if defaultRealityTimeout < 20*targetRTT {
		t.Fatalf("Reality/bootstrap budget=%s want >=%s", defaultRealityTimeout, 20*targetRTT)
	}
}
