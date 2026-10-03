package runtimeowner

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func freshBatchTransport(t *testing.T, emit faketcp.SegmentEmitter, batch faketcp.SegmentBatchEmitter) *laneTransport {
	t.Helper()
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil { t.Fatal(err) }
	rt, err := New(owner, nil)
	if err != nil { t.Fatal(err) }
	t.Cleanup(rt.Close)
	cfg, _ := transportPair(emit, nil, 1, 1000)
	cfg.EmitBatch = batch
	snapshot, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 7), cfg)
	if err != nil { t.Fatal(err) }
	return rt.lanes[snapshot.Ref]
}

func TestFreshBatchMatchesSingleAndKeepsControlSeparate(t *testing.T) {
	var single, batched []faketcp.Segment
	capture := func(dst *[]faketcp.Segment) faketcp.SegmentEmitter {
		return func(seg faketcp.Segment) error {
			seg.Payload = append([]byte(nil), seg.Payload...)
			*dst = append(*dst, seg)
			return nil
		}
	}
	calls := 0
	batchEmit := capture(&batched)
	a := freshBatchTransport(t, capture(&single), nil)
	b := freshBatchTransport(t, batchEmit, func(segments []faketcp.Segment) (int, error) {
		calls++
		if len(segments) > freshBatchSize { t.Fatal("batch exceeded bound") }
		for _, seg := range segments { batchEmit(seg) }
		return len(segments), nil
	})
	var records []datapath.WireRecord
	for i := 0; i < 15; i++ {
		records = append(records, datapath.WireRecord{Wire: bytes.Repeat([]byte{byte(i+1)}, i+1), Control: i == 4})
	}
	now := time.Unix(100, 0)
	if err := a.send(records, now); err != nil { t.Fatal(err) }
	if err := b.send(records, now); err != nil { t.Fatal(err) }
	if calls != 3 || len(single) != len(batched) { t.Fatalf("calls=%d singles=%d batched=%d", calls, len(single), len(batched)) }
	for i := range single {
		if single[i].Seq != batched[i].Seq || single[i].Ack != batched[i].Ack ||
			single[i].Flags != batched[i].Flags || !bytes.Equal(single[i].Payload, batched[i].Payload) {
			t.Fatalf("wire/sequence differed at%d", i)
		}
	}
	if a.sendNext != b.sendNext || len(a.pending) != len(b.pending) || b.stats.FreshBatchSent != 14 {
		t.Fatal("batch changed shadow backup/sequence accounting")
	}
}

func TestFreshBatchPartialFailureDropsOnlyUnsentBackups(t *testing.T) {
	errEmit := errors.New("injected partial write")
	tr := freshBatchTransport(t, func(faketcp.Segment) error { return nil },
		func([]faketcp.Segment) (int, error) { return 2, errEmit })
	records := []datapath.WireRecord{{Wire: []byte("one")}, {Wire: []byte("two")}, {Wire: []byte("three")}, {Wire: []byte("four")}}
	if err := tr.send(records, time.Unix(100, 0)); !errors.Is(err, errEmit) { t.Fatalf("err=%v", err) }
	if len(tr.pending) != 2 || tr.stats.FreshEmitFailures != 2 || tr.stats.Abandoned != 2 || tr.stats.FreshBatchSent != 2 {
		t.Fatalf("partial accounting=%+v pending=%d", tr.stats, len(tr.pending))
	}
	for _, record := range records { clear(record.Wire) }
	if !bytes.Equal(tr.pending[1000].payload, []byte("one")) || !bytes.Equal(tr.pending[1003].payload, []byte("two")) {
		t.Fatal("repair backup lost original ciphertext ownership")
	}
	// The consumed-but-unsent sequence range is a finite gap, never a fresh
	// send barrier or a phantom payload queued for later retransmission.
	if err := tr.send([]datapath.WireRecord{{Wire: []byte("next")}}, time.Unix(101, 0)); err != nil { t.Fatal(err) }
	if tr.stats.FreshSent != 5 || len(tr.pending) != 3 { t.Fatal("partial failure blocked future fresh send") }
}
