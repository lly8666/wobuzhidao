package faketcp

import (
	"testing"
	"time"
)

// The ARQ sender intentionally has no retry ceiling. Blackhole retirement is
// owned by the existing LINK keepalive/liveness contract above FakeTCP, so a
// silent peer must not silently change the mature ARQ wire behavior.
func TestSenderRTOBackoffKeepsRetryingWithoutCeiling(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	s := NewSender(100, time.Second)
	p := s.Enqueue([]byte("x"), now)

	for i := 0; i < 20; i++ {
		now = now.Add(s.RTO())
		if got := s.RetransmitDue(now); got != p {
			t.Fatalf("retry %d got=%p want=%p", i+1, got, p)
		}
	}
	if p.Retries != 20 {
		t.Fatalf("retries=%d want=20", p.Retries)
	}
	if got, want := s.RTO(), 60*time.Second; got != want {
		t.Fatalf("RTO=%s want=%s", got, want)
	}
	if s.Pending() != 1 {
		t.Fatalf("pending=%d want=1 before LINK liveness retires the transport", s.Pending())
	}
}
