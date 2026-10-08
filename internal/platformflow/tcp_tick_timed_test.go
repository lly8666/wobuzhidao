package platformflow

import (
	"testing"
	"time"
)

// The diagnostic-only TCP maintenance path must safely handle empty and
// closed flow sets without triggering an emission or abort.
func TestTCPServerTickTimedEmptyAndClosedFlows(t *testing.T) {
	now := time.Unix(100, 0)
	s := &TCPServer{flows: make(map[uint64]*tcpServerFlow)}
	got := s.TickTimed(now)
	if got.Flows != 0 || got.DueFrames != 0 || got.Aborts != 0 ||
		got.Scan != 0 || got.Emit != 0 || got.Abort != 0 {
		t.Fatalf("empty tick should emit no work: %+v", got)
	}
	s.Tick(now) // Off/normal path must remain valid.
	s.flows[123] = &tcpServerFlow{closed: true}
	got = s.TickTimed(now)
	if got.Flows != 1 || got.DueFrames != 0 || got.Aborts != 0 ||
		got.Emit != 0 || got.Abort != 0 {
		t.Fatalf("closed flow must not retransmit or abort: %+v", got)
	}
	s.Tick(now)
}
