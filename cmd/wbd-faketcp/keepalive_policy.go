package main

import (
	"errors"
	"time"
)

var errDeadPeer = errors.New("faketcp dead peer")

// A window contains several wire probes. Four bare probes are not robust on a
// path with independent 20% loss in each direction: a round trip fails with
// probability about 36%, so four consecutive failures are still about 1.7%.
// Three attempts in each of four miss windows require twelve consecutive failed
// round trips before retirement (about 4.7e-6 under that simple loss model).
type keepalivePolicy struct {
	Idle            time.Duration
	ProbeWaitMin    time.Duration
	ProbeWaitMax    time.Duration
	MissWindows     int
	AttemptsPerMiss int
}

var defaultKeepalivePolicy = keepalivePolicy{
	Idle:            5 * time.Second,
	ProbeWaitMin:    1500 * time.Millisecond,
	ProbeWaitMax:    3 * time.Second,
	MissWindows:     4,
	AttemptsPerMiss: 3,
}

type keepaliveSnapshot struct {
	RawRX   uint64
	DataRX  uint64
	Pending int
	LastACK uint32
	Acked   uint64
	SACKed  uint64
	RTO     time.Duration
}

type keepaliveAction uint8

const (
	keepaliveNone keepaliveAction = iota
	keepaliveSendProbe
	keepaliveDeclareDead
)

type keepaliveTracker struct {
	policy keepalivePolicy

	initialized  bool
	previous     keepaliveSnapshot
	lastProgress time.Time
	probing      bool
	window       int
	attempt      int
	deadline     time.Time
}

func newKeepaliveTracker(policy keepalivePolicy) *keepaliveTracker {
	if policy.Idle <= 0 {
		policy.Idle = defaultKeepalivePolicy.Idle
	}
	if policy.ProbeWaitMin <= 0 {
		policy.ProbeWaitMin = defaultKeepalivePolicy.ProbeWaitMin
	}
	if policy.ProbeWaitMax < policy.ProbeWaitMin {
		policy.ProbeWaitMax = policy.ProbeWaitMin
	}
	if policy.MissWindows <= 0 {
		policy.MissWindows = defaultKeepalivePolicy.MissWindows
	}
	if policy.AttemptsPerMiss <= 0 {
		policy.AttemptsPerMiss = defaultKeepalivePolicy.AttemptsPerMiss
	}
	return &keepaliveTracker{policy: policy}
}

func (k *keepaliveTracker) reset(now time.Time, s keepaliveSnapshot) {
	k.initialized = true
	k.previous = s
	k.lastProgress = now
	k.probing = false
	k.window = 0
	k.attempt = 0
	k.deadline = time.Time{}
}

func (k *keepaliveTracker) probeWait(rto time.Duration) time.Duration {
	if rto < k.policy.ProbeWaitMin {
		return k.policy.ProbeWaitMin
	}
	if rto > k.policy.ProbeWaitMax {
		return k.policy.ProbeWaitMax
	}
	return rto
}

func (k *keepaliveTracker) observe(now time.Time, s keepaliveSnapshot) keepaliveAction {
	if !k.initialized {
		k.reset(now, s)
		return keepaliveNone
	}

	progress := s.LastACK != k.previous.LastACK || s.Acked != k.previous.Acked || s.SACKed != k.previous.SACKed
	rawDelta := s.RawRX - k.previous.RawRX
	dataDelta := s.DataRX - k.previous.DataRX
	if s.Pending == 0 && rawDelta != 0 {
		// On an idle sender any exact-flow packet proves the return path is alive.
		progress = true
	}
	if k.probing && rawDelta > dataDelta {
		// During an active probe, a non-payload exact-flow response is liveness
		// evidence even when cumulative ACK cannot advance (the classic idle
		// keepalive ACK case and duplicate ACKs after a lost data segment).
		progress = true
	}
	k.previous = s

	if progress {
		k.lastProgress = now
		k.probing = false
		k.window = 0
		k.attempt = 0
		k.deadline = time.Time{}
		return keepaliveNone
	}

	if !k.probing {
		if now.Sub(k.lastProgress) < k.policy.Idle {
			return keepaliveNone
		}
		k.probing = true
		k.window = 1
		k.attempt = 1
		k.deadline = now.Add(k.probeWait(s.RTO))
		return keepaliveSendProbe
	}
	if now.Before(k.deadline) {
		return keepaliveNone
	}
	if k.attempt < k.policy.AttemptsPerMiss {
		k.attempt++
		k.deadline = now.Add(k.probeWait(s.RTO))
		return keepaliveSendProbe
	}
	if k.window < k.policy.MissWindows {
		k.window++
		k.attempt = 1
		k.deadline = now.Add(k.probeWait(s.RTO))
		return keepaliveSendProbe
	}
	return keepaliveDeclareDead
}

func (k *keepaliveTracker) position() (window, attempt int) {
	return k.window, k.attempt
}
