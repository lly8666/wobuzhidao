package faketcp

import (
	"bytes"
	"testing"
	"time"
)

func TestFullRepairWindowAdmitsFreshBulkByRetiringOldRepair(t *testing.T) {
	s := NewSenderWithRecovery(1000, time.Second, RecoverySACKRACK)
	now := time.Unix(1700000300, 0)
	var oldest *Pending
	for i := 0; i < MaxSteadyStateOutstandingDatagrams; i++ {
		p, err := s.EnqueueSteadyState(bytes.Repeat([]byte{byte(i)}, 64), now)
		if err != nil {
			t.Fatalf("fill %d: %v", i, err)
		}
		if i == 0 {
			oldest = p
		}
	}

	payload := bytes.Repeat([]byte{0x5a}, SteadyStateControlAdmissionMaxDatagramBytes+1)
	before := s.NextSeq()
	out, err := s.EnqueueSteadyStateBatch([][]byte{payload}, now.Add(time.Millisecond))
	if err != nil {
		t.Fatalf("fresh bulk admission at repair ceiling: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("fresh bulk was shed: batch=%d", len(out))
	}
	if out[0].Seq != before || out[0].End != before+uint32(len(payload)) {
		t.Fatalf("fresh sequence allocation=%d..%d want=%d..%d", out[0].Seq, out[0].End, before, before+uint32(len(payload)))
	}
	if got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams {
		t.Fatalf("pending=%d want=%d", got, MaxSteadyStateOutstandingDatagrams)
	}
	if s.Outstanding(oldest.Seq) != nil || oldest.Payload != nil || !oldest.Retired {
		t.Fatalf("oldest repair was not retired: outstanding=%v payload=%d retired=%t", s.Outstanding(oldest.Seq) != nil, len(oldest.Payload), oldest.Retired)
	}
}

func TestRepairHorizonNeverEvictsBootstrap(t *testing.T) {
	s := NewSenderWithRecovery(5000, time.Second, RecoverySACKRACK)
	now := time.Unix(1700000400, 0)
	bootstrap := s.Enqueue([]byte("ClientHello"), now)
	bootstrap.Bootstrap = true
	for i := 1; i < MaxSteadyStateOutstandingDatagrams; i++ {
		if _, err := s.EnqueueSteadyState([]byte{byte(i)}, now); err != nil {
			t.Fatalf("fill %d: %v", i, err)
		}
	}
	if _, err := s.EnqueueSteadyState([]byte{0xee}, now.Add(time.Millisecond)); err != nil {
		t.Fatalf("fresh enqueue with one protected bootstrap: %v", err)
	}
	if s.Outstanding(bootstrap.Seq) != bootstrap || bootstrap.Payload == nil || bootstrap.Retired {
		t.Fatal("bootstrap state was retired by steady-state repair pressure")
	}
}
