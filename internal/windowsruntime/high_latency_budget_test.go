package windowsruntime

import (
	"testing"
	"time"
)

func TestSupervisorBudgetsEnvelopeHighLatencyProtocolStages(t *testing.T) {
	cases := []struct {
		name string
		minimum time.Duration
	}{
		{"faketcp-1", 45 * time.Second},
		{"dtls-1", 30 * time.Second},
		{"link-1", 25 * time.Second},
	}
	for _, tc := range cases {
		spec, ok := commandReadiness(tc.name)
		if !ok { t.Fatalf("missing readiness spec for %s", tc.name) }
		if spec.timeout < tc.minimum {
			t.Fatalf("%s readiness=%s want >=%s", tc.name, spec.timeout, tc.minimum)
		}
	}
	const targetRTT = 600 * time.Millisecond
	if gameControlTimeout < 20*targetRTT {
		t.Fatalf("Game control budget=%s want >=%s", gameControlTimeout, 20*targetRTT)
	}
	// Replacement must not close the old Game association before the bounded
	// high-latency LINK/FEC recovery tail has had a chance to drain.
	if replacementGameOverlapWindow < 3*time.Second {
		t.Fatalf("replacement Game overlap=%s want >=3s", replacementGameOverlapWindow)
	}
}
