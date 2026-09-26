package session

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/protocol"
)

func TestStreamOutOfOrderThenContiguousDeliveryAndFIN(t *testing.T) {
	r := NewReceiver(nil, 0)
	later, err := r.AcceptData(protocol.DataFrame{FlowID: 1, Offset: 5, TransmissionID: 1, FIN: true, Payload: []byte("world")})
	if err != nil {
		t.Fatal(err)
	}
	if len(later.Data) != 0 || later.NextOffset != 0 || later.Complete || later.BufferedBytes != 5 {
		t.Fatalf("unexpected out-of-order state: %#v", later)
	}

	first, err := r.AcceptData(protocol.DataFrame{FlowID: 1, Offset: 0, TransmissionID: 2, Payload: []byte("hello")})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Data, []byte("helloworld")) || first.NextOffset != 10 || !first.Complete || first.BufferedBytes != 0 {
		t.Fatalf("unexpected delivery: %#v", first)
	}
}

func TestStreamReinjectionAndPartialDuplicate(t *testing.T) {
	r := NewReceiver(nil, 0)
	one, err := r.AcceptData(protocol.DataFrame{FlowID: 2, Offset: 0, TransmissionID: 10, Payload: []byte("abc")})
	if err != nil {
		t.Fatal(err)
	}
	if string(one.Data) != "abc" || one.Duplicate {
		t.Fatalf("first: %#v", one)
	}

	dup, err := r.AcceptData(protocol.DataFrame{FlowID: 2, Offset: 0, TransmissionID: 99, Payload: []byte("abc")})
	if err != nil {
		t.Fatal(err)
	}
	if !dup.Duplicate || len(dup.Data) != 0 || dup.NextOffset != 3 {
		t.Fatalf("reinjected duplicate: %#v", dup)
	}

	partial, err := r.AcceptData(protocol.DataFrame{FlowID: 2, Offset: 1, TransmissionID: 100, Payload: []byte("bcdef")})
	if err != nil {
		t.Fatal(err)
	}
	if string(partial.Data) != "def" || partial.NextOffset != 6 || partial.NewUniqueBytes != 3 {
		t.Fatalf("partial overlap: %#v", partial)
	}
}

func TestStreamBufferedOverlapConflict(t *testing.T) {
	r := NewReceiver(nil, 0)
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 3, Offset: 3, TransmissionID: 1, Payload: []byte("def")}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 3, Offset: 4, TransmissionID: 2, Payload: []byte("X")}); !errors.Is(err, ErrStreamConflict) {
		t.Fatalf("want stream conflict, got %v", err)
	}
	// A failed conflicting insert must not corrupt existing buffered bytes.
	got, err := r.AcceptData(protocol.DataFrame{FlowID: 3, Offset: 0, TransmissionID: 3, Payload: []byte("abc")})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != "abcdef" {
		t.Fatalf("state corrupted after conflict: %q", got.Data)
	}
}

func TestStreamFINConsistency(t *testing.T) {
	r := NewReceiver(nil, 0)
	fin, err := r.AcceptData(protocol.DataFrame{FlowID: 4, Offset: 5, TransmissionID: 1, FIN: true})
	if err != nil {
		t.Fatal(err)
	}
	if fin.Complete || fin.NextOffset != 0 {
		t.Fatalf("premature complete: %#v", fin)
	}
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 4, Offset: 6, TransmissionID: 2, FIN: true}); !errors.Is(err, ErrFinalOffset) {
		t.Fatalf("conflicting FIN: %v", err)
	}
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 4, Offset: 5, TransmissionID: 3, Payload: []byte("x")}); !errors.Is(err, ErrFinalOffset) {
		t.Fatalf("data beyond FIN: %v", err)
	}
	got, err := r.AcceptData(protocol.DataFrame{FlowID: 4, Offset: 0, TransmissionID: 4, Payload: []byte("hello")})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != "hello" || !got.Complete {
		t.Fatalf("final completion: %#v", got)
	}
	dup, err := r.AcceptData(protocol.DataFrame{FlowID: 4, Offset: 5, TransmissionID: 5, FIN: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dup.Complete {
		t.Fatalf("consistent FIN duplicate lost completion: %#v", dup)
	}
}

func TestStreamReorderLimitIsTransactional(t *testing.T) {
	r := NewReceiver(nil, 4)
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 5, Offset: 100, TransmissionID: 1, Payload: []byte("12345")}); !errors.Is(err, ErrReorderLimit) {
		t.Fatalf("want reorder limit, got %v", err)
	}
	got, err := r.AcceptData(protocol.DataFrame{FlowID: 5, Offset: 0, TransmissionID: 2, Payload: []byte("a")})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != "a" || got.BufferedBytes != 0 {
		t.Fatalf("limit failure mutated state: %#v", got)
	}
}

func TestFlowTypeMismatch(t *testing.T) {
	clock := NewManualClock(time.Unix(0, 0).UTC())
	r := NewReceiver(clock, 0)
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 9, Offset: 0, TransmissionID: 1, Payload: []byte("x")}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.AcceptDatagram(protocol.DatagramFrame{FlowID: 9, DatagramID: 1, TransmissionID: 2, Payload: []byte("y")}, clock.Now().Add(time.Second)); !errors.Is(err, ErrFlowTypeMismatch) {
		t.Fatalf("want flow type mismatch, got %v", err)
	}
}

func TestDatagramDedupConflictAndHardDeadline(t *testing.T) {
	start := time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)
	clock := NewManualClock(start)
	r := NewReceiver(clock, 0)
	deadline := start.Add(100 * time.Millisecond)
	f := protocol.DatagramFrame{FlowID: 10, DatagramID: 7, TransmissionID: 1, Payload: []byte("voice")}

	first, err := r.AcceptDatagram(f, deadline)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Delivered || first.Duplicate || first.Expired || string(first.Payload) != "voice" {
		t.Fatalf("first: %#v", first)
	}

	reinject := f
	reinject.TransmissionID = 2
	dup, err := r.AcceptDatagram(reinject, deadline)
	if err != nil {
		t.Fatal(err)
	}
	if !dup.Duplicate || dup.Delivered || dup.Expired {
		t.Fatalf("duplicate: %#v", dup)
	}

	bad := reinject
	bad.Payload = []byte("other")
	if _, err := r.AcceptDatagram(bad, deadline); !errors.Is(err, ErrDatagramConflict) {
		t.Fatalf("conflicting duplicate: %v", err)
	}
	if _, err := r.AcceptDatagram(reinject, deadline.Add(time.Millisecond)); !errors.Is(err, ErrDeadlineConflict) {
		t.Fatalf("deadline conflict: %v", err)
	}

	clock.Advance(100 * time.Millisecond)
	late, err := r.AcceptDatagram(protocol.DatagramFrame{FlowID: 10, DatagramID: 8, TransmissionID: 3, Payload: []byte("old")}, deadline)
	if err != nil {
		t.Fatal(err)
	}
	if !late.Expired || late.Delivered || late.Duplicate {
		t.Fatalf("deadline boundary must expire: %#v", late)
	}
}

func TestDatagramPruneUsesVirtualClock(t *testing.T) {
	start := time.Unix(100, 0).UTC()
	clock := NewManualClock(start)
	r := NewReceiver(clock, 0)
	for i := uint64(1); i <= 3; i++ {
		_, err := r.AcceptDatagram(protocol.DatagramFrame{FlowID: 11, DatagramID: protocol.DatagramID(i), TransmissionID: protocol.TransmissionID(i), Payload: []byte{byte(i)}}, start.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	clock.Advance(1500 * time.Millisecond)
	if got := r.PruneExpiredDatagrams(); got != 1 {
		t.Fatalf("pruned=%d want=1", got)
	}
	clock.Advance(2 * time.Second)
	if got := r.PruneExpiredDatagrams(); got != 2 {
		t.Fatalf("pruned=%d want=2", got)
	}
}

func TestDatagramZeroDeadlineRejected(t *testing.T) {
	r := NewReceiver(NewManualClock(time.Unix(0, 0).UTC()), 0)
	_, err := r.AcceptDatagram(protocol.DatagramFrame{FlowID: 12, DatagramID: 1, TransmissionID: 1}, time.Time{})
	if !errors.Is(err, ErrInvalidFrame) {
		t.Fatalf("zero deadline: %v", err)
	}
}

func TestManualClockRejectsNegativeAdvance(t *testing.T) {
	c := NewManualClock(time.Unix(0, 0))
	defer func() {
		if recover() == nil {
			t.Fatal("negative advance did not panic")
		}
	}()
	c.Advance(-time.Nanosecond)
}

func TestRejectedFINIsTransactional(t *testing.T) {
	r := NewReceiver(nil, 4)
	// The FIN frame would establish final offset 105, but it exceeds the
	// reorder buffer. Rejection must not leave final=105 behind.
	_, err := r.AcceptData(protocol.DataFrame{FlowID: 13, Offset: 100, TransmissionID: 1, FIN: true, Payload: []byte("12345")})
	if !errors.Is(err, ErrReorderLimit) {
		t.Fatalf("want reorder limit, got %v", err)
	}
	// A later valid stream ending at 3 must still be accepted and complete.
	got, err := r.AcceptData(protocol.DataFrame{FlowID: 13, Offset: 0, TransmissionID: 2, FIN: true, Payload: []byte("abc")})
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != "abc" || !got.Complete || got.Duplicate {
		t.Fatalf("rejected FIN leaked state: %#v", got)
	}
}

func TestFINOnlyFrameIsStateChangeNotDuplicate(t *testing.T) {
	r := NewReceiver(nil, 0)
	got, err := r.AcceptData(protocol.DataFrame{FlowID: 14, Offset: 0, TransmissionID: 1, FIN: true})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Complete || got.Duplicate {
		t.Fatalf("FIN-only state change misclassified: %#v", got)
	}
	dup, err := r.AcceptData(protocol.DataFrame{FlowID: 14, Offset: 0, TransmissionID: 2, FIN: true})
	if err != nil {
		t.Fatal(err)
	}
	if !dup.Complete || !dup.Duplicate {
		t.Fatalf("second FIN should be duplicate: %#v", dup)
	}
}
