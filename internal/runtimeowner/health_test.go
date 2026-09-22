package runtimeowner

import (
	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"testing"
	"time"
)

func TestHealthAuthenticatedIndependentOfFECAndBusiness(t *testing.T) {
	for _, parity := range []int{0, 4, 10, 20} {
		lease := runtimeLease(t)
		co, e := datapath.NewLeasedTunnelOwner(lease, 1, 8)
		if e != nil {
			t.Fatal(e)
		}
		so, e := datapath.NewLeasedTunnelOwner(lease, 1, 8)
		if e != nil {
			t.Fatal(e)
		}
		var wires []faketcp.Segment
		cr, e := New(co, nil)
		if e != nil {
			t.Fatal(e)
		}
		defer cr.Close()
		deliveries := 0
		sr, e := New(so, func(p [][]byte, _ time.Time) error { deliveries += len(p); return nil })
		if e != nil {
			t.Fatal(e)
		}
		defer sr.Close()
		cc, sc := transportPair(func(s faketcp.Segment) error { wires = append(wires, s); return nil }, func(faketcp.Segment) error { return nil }, 1, 1000)
		cs, e := cr.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, parity, 7), cc)
		if e != nil {
			t.Fatal(e)
		}
		ss, e := sr.AttachInitial(1, runtimeLane(t, datapath.RoleServer, lease, parity, 7), sc)
		if e != nil {
			t.Fatal(e)
		}
		for _, pair := range []struct {
			r       *Runtime
			refType bool
		}{{cr, true}, {sr, false}} {
			ref := ss.Ref
			if pair.refType {
				ref = cs.Ref
			}
			if e := pair.r.ConfigureHealth(ref, time.Second, func(time.Time) time.Duration { return time.Minute }); e != nil {
				t.Fatal(e)
			}
		}
		now := time.Unix(1000, 0)
		if sr.PeerIdle(now, 10*time.Second) {
			t.Fatal("missing keepalive treated as idle")
		}
		if e := cr.Tick(now); e != nil {
			t.Fatal(e)
		}
		if len(wires) != 1 || len(wires[0].Payload) != 40 {
			t.Fatalf("health amplified by FEC: parity=%d wires=%d", parity, len(wires))
		}
		if e := sr.HandleSegment(ss.Ref, wires[0], now); e != nil {
			t.Fatal(e)
		}
		if deliveries != 0 || !sr.PeerIdle(now, 10*time.Second) {
			t.Fatal("health delivered as business or lost idle hint")
		}
		if sr.PeerIdle(now.Add(3*time.Second), 10*time.Second) {
			t.Fatal("stale hint allowed idle close")
		}
		if !sr.Unhealthy(ss.Ref, now.Add(91*time.Second), 90*time.Second) {
			t.Fatal("blackhole undetected")
		}
		stats, _ := cr.TransportStats(cs.Ref)
		if stats.RepairCreditBytes != steadyRepairBurstBytes {
			t.Fatal("health earned repair credit")
		}
		// Duplicate old health cannot refresh authenticated liveness.
		if e := sr.HandleSegment(ss.Ref, wires[0], now.Add(92*time.Second)); e != nil {
			t.Fatal(e)
		}
		if !sr.Unhealthy(ss.Ref, now.Add(93*time.Second), 90*time.Second) {
			t.Fatal("duplicate kept dead lane alive")
		}
	}
}

func TestAdaptivePressureKeepsReorderingGrace(t *testing.T) {
	now := time.Unix(1000, 0)
	tr := &laneTransport{received: make(map[uint32]receiveSpan), recvNext: 100}
	tr.pressure = receivePressure{rate: 1000, rtt: 100 * time.Millisecond}
	for i := 0; i < 240; i++ {
		seq := uint32(200 + i*10)
		tr.received[seq] = receiveSpan{end: seq + 10, first: now}
	}
	tr.updatePressureHole(now)
	if tr.pressureAllowsForgiveness(now.Add(99 * time.Millisecond)) {
		t.Fatal("forgave normal reordering before RTT")
	}
	if !tr.pressureAllowsForgiveness(now.Add(101 * time.Millisecond)) {
		t.Fatal("pressure did not release stale TCP gap")
	}
	tr.pressure = receivePressure{}
	if tr.pressureAllowsForgiveness(now.Add(time.Second)) {
		t.Fatal("cold start invented tiny BDP")
	}
}
