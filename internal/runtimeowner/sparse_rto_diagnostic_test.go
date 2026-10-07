package runtimeowner

import (
	"bytes"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// This is deliberately a pre-fix diagnostic. It captures the existing sparse
// loss behavior on an established low-RTT lane: once there is no later payload
// to create SACK/RACK evidence, RTT estimation cannot reduce the repair timer
// below InitialRTO (1s). The follow-up fix must change this expectation rather
// than deleting the reproducer.
func TestSparseLowRTTLossWaitsInitialRTOPreFix(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 16)
	if err != nil {
		t.Fatal(err)
	}
	var wire []faketcp.Segment
	rt, err := New(owner, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()

	cfg, _ := transportPair(func(seg faketcp.Segment) error {
		wire = append(wire, seg)
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 70000)
	cfg.InitialRTO = time.Second
	cfg.RepairHorizon = 3 * time.Second
	snap, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 101), cfg)
	if err != nil {
		t.Fatal(err)
	}
	tr := rt.lanes[snap.Ref]
	t0 := time.Unix(12000, 0)

	if err := tr.send([]datapath.WireRecord{{Wire: []byte("low-rtt-sample")}}, t0); err != nil {
		t.Fatal(err)
	}
	first := wire[len(wire)-1]
	ack := faketcp.Segment{
		SrcIP: cfg.PeerIP, DstIP: cfg.LocalIP,
		SrcPort: cfg.PeerPort, DstPort: cfg.LocalPort,
		Seq: cfg.ReceiveNext,
		Ack: first.Seq + uint32(len(first.Payload)),
		Flags: faketcp.FlagACK, Window: 65535,
	}
	if err := rt.HandleSegment(snap.Ref, ack, t0.Add(50*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	stats, _ := rt.TransportStatsAt(snap.Ref, t0.Add(50*time.Millisecond))
	if stats.SRTT != 50*time.Millisecond || stats.RTO != time.Second {
		t.Fatalf("pre-fix low RTT did not remain clamped: srtt=%s rto=%s", stats.SRTT, stats.RTO)
	}

	t1 := t0.Add(100 * time.Millisecond)
	if err := tr.send([]datapath.WireRecord{{Wire: []byte("sparse-tail-record")}}, t1); err != nil {
		t.Fatal(err)
	}
	lost := wire[len(wire)-1]
	before := len(wire)

	if err := rt.Tick(t1.Add(999 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != before {
		t.Fatalf("pre-fix sparse repair fired before 1s: before=%d after=%d", before, len(wire))
	}
	if err := rt.Tick(t1.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != before+1 {
		t.Fatalf("pre-fix sparse repair missing at 1s: before=%d after=%d", before, len(wire))
	}
	retry := wire[len(wire)-1]
	if retry.Seq != lost.Seq || !bytes.Equal(retry.Payload, lost.Payload) {
		t.Fatalf("retry changed seq/ciphertext: lost=%+v retry=%+v", lost, retry)
	}
	stats, _ = rt.TransportStatsAt(snap.Ref, t1.Add(time.Second))
	if stats.RTORepairs != 1 {
		t.Fatalf("pre-fix sparse repair stats=%+v", stats)
	}
	t.Logf("WBD_SPARSE_RTO_PREFX srtt_ms=%d rto_ms=%d first_repair_ms=1000 same_seq_ciphertext=1",
		stats.SRTT.Milliseconds(), time.Second.Milliseconds())
}
