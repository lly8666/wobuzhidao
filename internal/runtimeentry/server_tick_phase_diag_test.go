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
	p.tickTCPFlowSnapshot.observe(300 * time.Microsecond)
	p.tickTCPScan.observe(500 * time.Microsecond)
	p.tickTCPEmit.observe(2 * time.Millisecond)
	p.tickTCPAbort.observe(100 * time.Microsecond)
	p.tickTCPFlows.Add(304)
	p.tickTCPDueFrames.Add(12)
	p.tickTCPAborts.Add(3)
	p.tickTCPAsyncScheduled.Add(11)
	p.tickTCPAsyncCoalesced.Add(2)
	p.tickBookkeeping.observe(time.Millisecond)

	got := p.snapshot(true)
	if !got.Enabled || got.Tick.Samples != 1 ||
		got.TickRetransmit.TotalNS != uint64(time.Millisecond) ||
		got.TickSweep.TotalNS != uint64(2*time.Millisecond) ||
		got.TickRuntime.TotalNS != uint64(4*time.Millisecond) ||
		got.TickService.TotalNS != uint64(3*time.Millisecond) ||
		got.TickServiceUDP.TotalNS != uint64(time.Millisecond) ||
		got.TickServiceTCP.TotalNS != uint64(2*time.Millisecond) ||
		got.TickTCPFlowSnapshot.TotalNS != uint64(300*time.Microsecond) ||
		got.TickTCPScan.TotalNS != uint64(500*time.Microsecond) ||
		got.TickTCPEmit.TotalNS != uint64(2*time.Millisecond) ||
		got.TickTCPAbort.TotalNS != uint64(100*time.Microsecond) ||
		got.TickTCPFlows != 304 || got.TickTCPDueFrames != 12 || got.TickTCPAborts != 3 ||
		got.TickTCPAsyncScheduled != 11 || got.TickTCPAsyncCoalesced != 2 ||
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
	for _, key := range []string{"tick_retransmit", "tick_sweep", "tick_runtime", "tick_service", "tick_service_udp", "tick_service_tcp", "tick_tcp_flow_snapshot", "tick_tcp_scan", "tick_tcp_emit", "tick_tcp_abort", "tick_tcp_flows", "tick_tcp_due_frames", "tick_tcp_aborts", "tick_tcp_async_scheduled", "tick_tcp_async_coalesced", "tick_bookkeeping"} {
		if len(fields[key]) == 0 {
			t.Fatalf("missing diagnostic JSON field %q", key)
		}
	}
}
