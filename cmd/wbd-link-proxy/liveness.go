package main

import "time"

const (
	clientRemoteRXTimeoutWindows      = 3
	clientKeepaliveMissWindows        = 4
	clientKeepaliveAttemptsPerMiss    = 3
	clientKeepaliveTotalProbeAttempts = clientKeepaliveMissWindows * clientKeepaliveAttemptsPerMiss
)

func clientRemoteRXTimeout(keepalive time.Duration) time.Duration {
	if keepalive <= 0 {
		return 0
	}
	const maxDuration = time.Duration(1<<63 - 1)
	if keepalive > maxDuration/clientRemoteRXTimeoutWindows {
		return maxDuration
	}
	return keepalive * clientRemoteRXTimeoutWindows
}

// clientRemoteRXExpired remains a budget helper for compatibility tests. Live
// retirement is driven by the keepalive tracker: authenticated LINK data is
// peer-liveness evidence, while outbound-only traffic never refreshes it.
func clientRemoteRXExpired(lastRemoteRX, now time.Time, keepalive time.Duration) bool {
	timeout := clientRemoteRXTimeout(keepalive)
	return timeout > 0 && !now.Before(lastRemoteRX.Add(timeout))
}

func clientKeepaliveDue(nextPing, now time.Time, keepalive time.Duration) bool {
	return keepalive > 0 && !now.Before(nextPing)
}

// Twelve probes fit inside the historical 3*keepalive retirement budget. The
// first probe is sent after one keepalive interval; the remaining 2*keepalive
// budget is split into twelve response windows. At 20% independent loss in each
// direction, a probe round trip fails with probability 0.36, so all twelve
// probes failing is about 4.7e-6 instead of relying on a few bare probes.
func clientKeepaliveProbeWait(keepalive time.Duration) time.Duration {
	if keepalive <= 0 {
		return 0
	}
	wait := keepalive / 6
	if wait <= 0 {
		return time.Nanosecond
	}
	return wait
}

type clientKeepaliveTracker struct {
	nextProbe time.Time
	probing   bool
	nonce     uint64
	attempts  int
}

func newClientKeepaliveTracker(now time.Time, keepalive time.Duration) clientKeepaliveTracker {
	var t clientKeepaliveTracker
	if keepalive > 0 {
		t.nextProbe = now.Add(keepalive)
	}
	return t
}

// poll returns whether to send a PING, the current round nonce, and whether the
// full 3*keepalive budget has expired without valid authenticated remote LINK
// activity. During active receive traffic, observeRemoteActivity keeps this
// tracker out of probing state so keepalive remains an idle-path fallback.
func (t *clientKeepaliveTracker) poll(now time.Time, keepalive time.Duration) (bool, uint64, bool) {
	if keepalive <= 0 {
		return false, 0, false
	}
	if t.nextProbe.IsZero() {
		t.nextProbe = now.Add(keepalive)
		return false, 0, false
	}
	if now.Before(t.nextProbe) {
		return false, 0, false
	}
	wait := clientKeepaliveProbeWait(keepalive)
	if !t.probing {
		t.probing = true
		t.nonce = uint64(now.UnixNano())
		if t.nonce == 0 {
			t.nonce = 1
		}
		t.attempts = 1
		t.nextProbe = now.Add(wait)
		return true, t.nonce, false
	}
	if t.attempts >= clientKeepaliveTotalProbeAttempts {
		return false, 0, true
	}
	t.attempts++
	t.nextProbe = now.Add(wait)
	return true, t.nonce, false
}

// observeRemoteActivity records authenticated, successfully decoded LINK data.
// It deliberately has no outbound counterpart: local writes cannot prove that
// the peer or the path is still reachable. Valid remote activity cancels an
// outstanding idle probe and restarts the idle interval.
func (t *clientKeepaliveTracker) observeRemoteActivity(now time.Time, keepalive time.Duration) {
	if keepalive <= 0 {
		return
	}
	t.probing = false
	t.nonce = 0
	t.attempts = 0
	t.nextProbe = now.Add(keepalive)
}

func (t *clientKeepaliveTracker) observePong(nonce uint64, now time.Time, keepalive time.Duration) bool {
	if keepalive <= 0 || !t.probing || nonce != t.nonce {
		return false
	}
	t.observeRemoteActivity(now, keepalive)
	return true
}
