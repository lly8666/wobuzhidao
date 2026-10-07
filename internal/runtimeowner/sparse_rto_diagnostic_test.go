package runtimeowner

import (
	"bytes"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// This preserves the pre-fix sparse-loss reproducer while asserting the narrow
// repair: startup still uses 1s, but one clean low-RTT sample lets the standard
// SRTT+4*RTTVAR estimator use its separate established floor.
func TestSparseLowRTTLossUsesEstablishedMinimumRTO(t *testing.T) {
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
	initial, _ := rt.TransportStatsAt(snap.Ref, t0)
	if initial.RTO != time.Second || initial.MinimumRTO != DefaultMinimumRepairRTO {
		t.Fatalf("startup timers rto=%s minimum=%s", initial.RTO, initial.MinimumRTO)
	}

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
	if stats.SRTT != 50*time.Millisecond || stats.RTO != DefaultMinimumRepairRTO ||
		stats.MinimumRTO != DefaultMinimumRepairRTO {
		t.Fatalf("established low RTT timer mismatch: srtt=%s rto=%s minimum=%s", stats.SRTT, stats.RTO, stats.MinimumRTO)
	}

	t1 := t0.Add(100 * time.Millisecond)
	if err := tr.send([]datapath.WireRecord{{Wire: []byte("sparse-tail-record")}}, t1); err != nil {
		t.Fatal(err)
	}
	lost := wire[len(wire)-1]
	before := len(wire)

	if err := rt.Tick(t1.Add(DefaultMinimumRepairRTO - time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != before {
		t.Fatalf("sparse repair fired before established RTO: before=%d after=%d", before, len(wire))
	}
	if err := rt.Tick(t1.Add(DefaultMinimumRepairRTO)); err != nil {
		t.Fatal(err)
	}
	if len(wire) != before+1 {
		t.Fatalf("sparse repair missing at established RTO: before=%d after=%d", before, len(wire))
	}
	retry := wire[len(wire)-1]
	if retry.Seq != lost.Seq || !bytes.Equal(retry.Payload, lost.Payload) {
		t.Fatalf("retry changed seq/ciphertext: lost=%+v retry=%+v", lost, retry)
	}
	stats, _ = rt.TransportStatsAt(snap.Ref, t1.Add(DefaultMinimumRepairRTO))
	if stats.RTORepairs != 1 {
		t.Fatalf("sparse repair stats=%+v", stats)
	}
	t.Logf("WBD_SPARSE_RTO_FIXED srtt_ms=%d minimum_rto_ms=%d first_repair_ms=%d same_seq_ciphertext=1",
		stats.SRTT.Milliseconds(), stats.MinimumRTO.Milliseconds(), DefaultMinimumRepairRTO.Milliseconds())
}

func TestTransportMinimumRTOValidationAndSmallExplicitStartup(t *testing.T) {
	cfg := TransportConfig{
		LocalIP: [4]byte{192, 0, 2, 1}, PeerIP: [4]byte{192, 0, 2, 2},
		LocalPort: 1234, PeerPort: 443, InitialRTO: time.Second,
		MinimumRTO: 2 * time.Second, RepairHorizon: 3 * time.Second,
		Emit: func(faketcp.Segment) error { return nil },
	}
	if err := cfg.normalize(); !errors.Is(err, ErrTransportConfig) {
		t.Fatalf("minimum above initial accepted: %v", err)
	}
	cfg.MinimumRTO = 0
	cfg.InitialRTO = 100 * time.Millisecond
	if err := cfg.normalize(); err != nil {
		t.Fatal(err)
	}
	if cfg.MinimumRTO != cfg.InitialRTO {
		t.Fatalf("small explicit startup should cap default minimum: initial=%s minimum=%s", cfg.InitialRTO, cfg.MinimumRTO)
	}
}
