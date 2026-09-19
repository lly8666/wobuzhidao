package main

import (
	"testing"
	"time"
)

func TestLaneQualificationBudgetSupports300msOneWayPath(t *testing.T) {
	const targetRTT = 600 * time.Millisecond
	if laneQualificationRetry < targetRTT {
		t.Fatalf("lane qualification retry=%s want >=%s", laneQualificationRetry, targetRTT)
	}
	if laneQualificationTimeout < 15*targetRTT {
		t.Fatalf("lane qualification budget=%s want >=%s", laneQualificationTimeout, 15*targetRTT)
	}
}
