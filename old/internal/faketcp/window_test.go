package faketcp

import (
	"testing"
	"time"
)

func TestSteadyStateOutstandingWindowMatchesPinnedDTLSReplayWindow(t *testing.T) {
	if got, want := PinnedDTLSReplayWindowRecords, MaxSteadyStateOutstandingDatagrams; got != want {
		t.Fatalf("DTLS replay records=%d want FakeTCP outstanding limit=%d", got, want)
	}
	if PinnedDTLSReplayWindowWords != 128 || PinnedDTLSReplayWindowRecords != 4096 {
		t.Fatalf("unexpected pinned replay contract words=%d records=%d", PinnedDTLSReplayWindowWords, PinnedDTLSReplayWindowRecords)
	}
}

func TestSteadyStateOutstandingWindowCovers40Mbps600msBDP(t *testing.T) {
	const (
		targetBitsPerSecond     int64 = 40_000_000
		conservativeRecordBytes int64 = 900
	)
	const targetRTT = 600 * time.Millisecond
	bdpBytes := targetBitsPerSecond / 8 * int64(targetRTT) / int64(time.Second)
	required := (bdpBytes + conservativeRecordBytes - 1) / conservativeRecordBytes
	if int64(MaxSteadyStateOutstandingDatagrams) < required {
		t.Fatalf("outstanding limit=%d cannot cover target BDP: need at least %d records", MaxSteadyStateOutstandingDatagrams, required)
	}
}

func TestEnqueueSteadyStateHardBoundsRepairDebtWithoutBlockingFresh(t *testing.T) {
	s := NewSenderWithRecovery(100, time.Second, RecoverySACKRACK)
	now := time.Unix(1, 0)
	var oldest *Pending
	for i := 0; i < MaxSteadyStateOutstandingDatagrams; i++ {
		p, err := s.EnqueueSteadyState([]byte{byte(i)}, now)
		if err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
		if i == 0 {
			oldest = p
		}
	}
	if got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams {
		t.Fatalf("pending=%d want=%d", got, MaxSteadyStateOutstandingDatagrams)
	}

	fresh, err := s.EnqueueSteadyState([]byte{0xff}, now.Add(time.Millisecond))
	if err != nil {
		t.Fatalf("fresh enqueue at repair ceiling: %v", err)
	}
	if fresh == nil {
		t.Fatal("fresh enqueue returned nil")
	}
	if got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams {
		t.Fatalf("repair ceiling changed pending=%d", got)
	}
	if s.Outstanding(oldest.Seq) != nil || oldest.Payload != nil || !oldest.Retired {
		t.Fatalf("old repair state was not retired: outstanding=%v payload=%d retired=%t", s.Outstanding(oldest.Seq) != nil, len(oldest.Payload), oldest.Retired)
	}
	if got := s.Stats().PeakPending; got != MaxSteadyStateOutstandingDatagrams {
		t.Fatalf("peak pending=%d want=%d", got, MaxSteadyStateOutstandingDatagrams)
	}

	s.Ack(fresh.End, now.Add(2*time.Millisecond))
	if got := s.Pending(); got != 0 {
		t.Fatalf("pending after cumulative ACK=%d", got)
	}
	if p := s.RetransmitDue(now.Add(3 * time.Second)); p != nil {
		t.Fatalf("retired repair reappeared after cumulative ACK: %#v", p)
	}
}
