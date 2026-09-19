package main

import (
	"testing"
	"time"
)

func TestLinkStartupBudgetSupports300msOneWayPath(t *testing.T) {
	const targetRTT = 600 * time.Millisecond
	if setupRetryAfter < targetRTT {
		t.Fatalf("LINK retry interval=%s want >=%s to avoid stacking retries inside one RTT", setupRetryAfter, targetRTT)
	}
	if defaultSetupTimeout < 20*targetRTT {
		t.Fatalf("LINK startup budget=%s want >=%s", defaultSetupTimeout, 20*targetRTT)
	}
}
