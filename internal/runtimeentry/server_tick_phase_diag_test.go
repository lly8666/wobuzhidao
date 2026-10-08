package runtimeentry

import (
	"encoding/json"
	"testing"
	"time"
)

// Phase counters are opt-in observations. The tick owner remains serial and
// the read/handler queue contract is deliberately unaffected.
func TestServerPipelineTickPhaseDiagnosticSnapshot(t *testing.T) {
	var p serverPipelineTiming
	p.tick.observe(12 * time.Millisecond)
	p.tickRetransmit.observe(time.Millisecond)
	p.tickSweep.observe(2 * time.Millisecond)
	p.tickRuntime.observe(4 * time.Millisecond)
	p.tickService.observe(3 * time.Millisecond)
	p.tickServiceUDP.observe(time.Millisecond)
	p.tickServiceTCP.observe(2 * time.Millisecond)
	p.tickBookkeeping.observe(time.Millisecond)

	got := p.snapshot(true)
	if !got.Enabled || got.Tick.Samples != 1 ||
		got.TickRetransmit.TotalNS != uint64(time.Millisecond) ||
		got.TickSweep.TotalNS != uint64(2*time.Millisecond) ||
		got.TickRuntime.TotalNS != uint64(4*time.Millisecond) ||
		got.TickService.TotalNS != uint64(3*time.Millisecond) ||
		got.TickServiceUDP.TotalNS != uint64(time.Millisecond) ||
		got.TickServiceTCP.TotalNS != uint64(2*time.Millisecond) ||
		got.TickBookkeeping.TotalNS != uint64(time.Millisecond) {
		t.Fatalf("missing server tick phase counters: %+v", got)
	}
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"tick_retransmit", "tick_sweep", "tick_runtime", "tick_service", "tick_service_udp", "tick_service_tcp", "tick_bookkeeping"} {
		if len(fields[key]) == 0 {
			t.Fatalf("missing diagnostic JSON field %q", key)
		}
	}
}
