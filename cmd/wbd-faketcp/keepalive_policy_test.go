package main

import (
	"math/rand"
	"testing"
	"time"
)

func testKeepalivePolicy() keepalivePolicy {
	return keepalivePolicy{Idle: time.Millisecond, ProbeWaitMin: time.Millisecond, ProbeWaitMax: 4 * time.Millisecond, MissWindows: 4, AttemptsPerMiss: 3}
}

func TestKeepaliveRequiresAllTwelveMissedAttempts(t *testing.T) {
	p := testKeepalivePolicy()
	k := newKeepaliveTracker(p)
	now := time.Unix(1000, 0)
	s := keepaliveSnapshot{RTO: time.Millisecond}
	if got := k.observe(now, s); got != keepaliveNone {
		t.Fatalf("initial action=%v", got)
	}
	now = now.Add(p.Idle)
	probes := 0
	for {
		a := k.observe(now, s)
		switch a {
		case keepaliveSendProbe:
			probes++
			now = now.Add(p.ProbeWaitMin)
		case keepaliveDeclareDead:
			if probes != p.MissWindows*p.AttemptsPerMiss {
				t.Fatalf("declared dead after %d probes, want %d", probes, p.MissWindows*p.AttemptsPerMiss)
			}
			return
		default:
			now = now.Add(p.ProbeWaitMin)
		}
	}
}

func TestKeepaliveReplyOnLastAllowedAttemptRecovers(t *testing.T) {
	p := testKeepalivePolicy()
	k := newKeepaliveTracker(p)
	now := time.Unix(1100, 0)
	s := keepaliveSnapshot{RTO: time.Millisecond}
	k.observe(now, s)
	now = now.Add(p.Idle)
	probes := 0
	for probes < p.MissWindows*p.AttemptsPerMiss {
		a := k.observe(now, s)
		if a == keepaliveDeclareDead {
			t.Fatal("declared dead before final allowed probe could reply")
		}
		if a == keepaliveSendProbe {
			probes++
			if probes == p.MissWindows*p.AttemptsPerMiss {
				// ACK-only reply: raw RX advances while data RX does not.
				s.RawRX++
				now = now.Add(p.ProbeWaitMin / 2)
				if got := k.observe(now, s); got != keepaliveNone {
					t.Fatalf("last-attempt response action=%v want none", got)
				}
				if w, a := k.position(); w != 0 || a != 0 {
					t.Fatalf("probe state not reset after recovery: window=%d attempt=%d", w, a)
				}
				return
			}
		}
		now = now.Add(p.ProbeWaitMin)
	}
	t.Fatal("did not reach final allowed probe")
}

func TestKeepaliveACKOrSACKProgressCancelsBlackholeDetection(t *testing.T) {
	for _, mode := range []string{"ack", "sack"} {
		t.Run(mode, func(t *testing.T) {
			p := testKeepalivePolicy()
			k := newKeepaliveTracker(p)
			now := time.Unix(1200, 0)
			s := keepaliveSnapshot{Pending: 1, LastACK: 5000, RTO: time.Millisecond}
			k.observe(now, s)
			now = now.Add(p.Idle)
			if got := k.observe(now, s); got != keepaliveSendProbe {
				t.Fatalf("first action=%v want probe", got)
			}
			if mode == "ack" {
				s.LastACK++
				s.Acked++
				s.Pending = 0
			} else {
				s.SACKed++
			}
			now = now.Add(p.ProbeWaitMin / 2)
			if got := k.observe(now, s); got != keepaliveNone {
				t.Fatalf("progress action=%v want none", got)
			}
		})
	}
}

func TestKeepaliveInboundPayloadDoesNotMaskOutboundBlackholeWithPendingData(t *testing.T) {
	p := testKeepalivePolicy()
	k := newKeepaliveTracker(p)
	now := time.Unix(1300, 0)
	s := keepaliveSnapshot{Pending: 1, LastACK: 5000, RTO: time.Millisecond}
	k.observe(now, s)
	now = now.Add(p.Idle)
	probes := 0
	for {
		a := k.observe(now, s)
		if a == keepaliveSendProbe {
			probes++
			// Server->client payload keeps arriving, but client->server is blackholed:
			// no ACK/SACK progress and no ACK-only response can return from a probe.
			s.RawRX++
			s.DataRX++
		}
		if a == keepaliveDeclareDead {
			if probes != 12 {
				t.Fatalf("declared dead after %d probes want 12", probes)
			}
			return
		}
		now = now.Add(p.ProbeWaitMin)
	}
}

func TestKeepaliveProbeWaitSupports600msRTTAndCapsBackedOffRTO(t *testing.T) {
	k := newKeepaliveTracker(defaultKeepalivePolicy)
	if got := k.probeWait(600 * time.Millisecond); got != 1500*time.Millisecond {
		t.Fatalf("600ms RTT/RTO wait=%s want 1.5s floor", got)
	}
	if got := k.probeWait(2 * time.Second); got != 2*time.Second {
		t.Fatalf("2s RTO wait=%s want 2s", got)
	}
	if got := k.probeWait(60 * time.Second); got != 3*time.Second {
		t.Fatalf("backed-off 60s RTO wait=%s want 3s liveness cap", got)
	}
}

func TestKeepaliveFourByThreeFalseDeathProbabilityAt20PercentEachDirection(t *testing.T) {
	// Deterministic Monte Carlo over independent 20% loss on probe and ACK. This
	// is a policy regression, not a replacement for the namespace/netem test.
	const episodes = 1_000_000
	r := rand.New(rand.NewSource(20260908))
	falseDeaths := 0
	for e := 0; e < episodes; e++ {
		alive := false
		for probe := 0; probe < 12; probe++ {
			probeArrives := r.Float64() >= 0.20
			ackReturns := r.Float64() >= 0.20
			if probeArrives && ackReturns {
				alive = true
				break
			}
		}
		if !alive {
			falseDeaths++
		}
	}
	// Expected ~4.74 cases/million. A generous deterministic ceiling catches an
	// accidental return to four bare probes without making the test flaky.
	if falseDeaths > 20 {
		t.Fatalf("false deaths=%d/%d, policy is too aggressive for 20%%+20%% loss", falseDeaths, episodes)
	}
}
