package faketcp

import (
	"errors"
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

func TestEnqueueSteadyStateHardBoundsPending(t *testing.T) {
	s := NewSenderWithRecovery(100, time.Second, RecoveryLegacy)
	now := time.Unix(1, 0)
	for i := 0; i < MaxSteadyStateOutstandingDatagrams; i++ {
		if _, err := s.EnqueueSteadyState([]byte{byte(i)}, now); err != nil {
			t.Fatalf("enqueue %d: %v", i, err)
		}
	}
	if got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams {
		t.Fatalf("pending=%d want=%d", got, MaxSteadyStateOutstandingDatagrams)
	}
	if _, err := s.EnqueueSteadyState([]byte{0xff}, now); !errors.Is(err, ErrSteadyStateOutstandingFull) {
		t.Fatalf("overflow err=%v want %v", err, ErrSteadyStateOutstandingFull)
	}
	if got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams {
		t.Fatalf("overflow changed pending=%d", got)
	}
	if got := s.Stats().PeakPending; got != MaxSteadyStateOutstandingDatagrams {
		t.Fatalf("peak pending=%d want=%d", got, MaxSteadyStateOutstandingDatagrams)
	}

	// Cumulative ACK of the first one-byte datagram reopens exactly one slot.
	s.Ack(101, now.Add(time.Millisecond))
	if got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams-1 {
		t.Fatalf("pending after ACK=%d", got)
	}
	if _, err := s.EnqueueSteadyState([]byte{0xee}, now.Add(2*time.Millisecond)); err != nil {
		t.Fatalf("enqueue after ACK: %v", err)
	}
	if got := s.Pending(); got != MaxSteadyStateOutstandingDatagrams {
		t.Fatalf("pending after refill=%d", got)
	}
}
