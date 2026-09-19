//go:build linux

package main

import (
	"testing"
	"time"
)

func TestServerBootstrapBudgetSupports300msOneWayPath(t *testing.T) {
	const targetRTT = 600 * time.Millisecond
	if defaultBootstrapTimeout < 20*targetRTT {
		t.Fatalf("server bootstrap budget=%s want >=%s", defaultBootstrapTimeout, 20*targetRTT)
	}
	if halfOpenTimeout <= defaultBootstrapTimeout {
		t.Fatalf("half-open budget=%s must outlive bootstrap budget=%s", halfOpenTimeout, defaultBootstrapTimeout)
	}
}
