package session

import (
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/protocol"
)

func TestStreamReceiptNormalizesRangesAndFirstGap(t *testing.T) {
	r := NewReceiver(nil, 0)
	// Receive prefix plus two disjoint later regions. Transmission IDs and lane
	// context never enter receipt identity.
	for _, f := range []protocol.DataFrame{
		{FlowID: 100, Offset: 0, TransmissionID: 1, Payload: []byte("abcd")},
		{FlowID: 100, Offset: 8, TransmissionID: 2, Payload: []byte("ij")},
		{FlowID: 100, Offset: 12, TransmissionID: 3, FIN: true, Payload: []byte("mn")},
	} {
		if _, err := r.AcceptData(f); err != nil {
			t.Fatal(err)
		}
	}
	receipt, err := r.ReceiptFor(100)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.ACKs) != 1 {
		t.Fatalf("acks=%#v", receipt.ACKs)
	}
	ack := receipt.ACKs[0]
	want := []protocol.Range{{Start: 0, End: 4}, {Start: 8, End: 10}, {Start: 12, End: 14}}
	if !ack.FIN || ack.Kind != protocol.AckStream || !rangesEqual(ack.Ranges, want) {
		t.Fatalf("ack=%#v want=%#v", ack, want)
	}
	if receipt.Gap == nil || receipt.Gap.Start != 4 || receipt.Gap.End != 8 || receipt.Gap.Kind != protocol.AckStream {
		t.Fatalf("gap=%#v", receipt.Gap)
	}

	// Fill first gap. The first observable gap moves, then disappears after all
	// logical bytes are contiguous.
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 100, Offset: 4, TransmissionID: 4, Payload: []byte("efgh")}); err != nil {
		t.Fatal(err)
	}
	receipt, _ = r.ReceiptFor(100)
	if receipt.Gap == nil || receipt.Gap.Start != 10 || receipt.Gap.End != 12 {
		t.Fatalf("second gap=%#v", receipt.Gap)
	}
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 100, Offset: 10, TransmissionID: 5, Payload: []byte("kl")}); err != nil {
		t.Fatal(err)
	}
	receipt, _ = r.ReceiptFor(100)
	if receipt.Gap != nil || len(receipt.ACKs) != 1 || len(receipt.ACKs[0].Ranges) != 1 || receipt.ACKs[0].Ranges[0] != (protocol.Range{Start: 0, End: 14}) {
		t.Fatalf("final receipt=%#v", receipt)
	}
}

func TestEmptyStreamFINHasAckWithoutByteRange(t *testing.T) {
	r := NewReceiver(nil, 0)
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 101, Offset: 0, TransmissionID: 1, FIN: true}); err != nil {
		t.Fatal(err)
	}
	receipt, err := r.ReceiptFor(101)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.ACKs) != 1 || !receipt.ACKs[0].FIN || len(receipt.ACKs[0].Ranges) != 0 || receipt.Gap != nil {
		t.Fatalf("empty FIN receipt=%#v", receipt)
	}
}

func TestFINOnlyCanRevealStreamHole(t *testing.T) {
	r := NewReceiver(nil, 0)
	if _, err := r.AcceptData(protocol.DataFrame{FlowID: 102, Offset: 10, TransmissionID: 1, FIN: true}); err != nil {
		t.Fatal(err)
	}
	receipt, err := r.ReceiptFor(102)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Gap == nil || receipt.Gap.Start != 0 || receipt.Gap.End != 10 {
		t.Fatalf("FIN-revealed gap=%#v", receipt.Gap)
	}
}

func TestDatagramReceiptMergesLiveIDsAndFindsInternalGap(t *testing.T) {
	start := time.Unix(500, 0).UTC()
	clock := NewManualClock(start)
	r := NewReceiver(clock, 0)
	dl := start.Add(time.Second)
	for _, id := range []protocol.DatagramID{5, 6, 9, 10} {
		if _, err := r.AcceptDatagram(protocol.DatagramFrame{FlowID: 103, DatagramID: id, TransmissionID: protocol.TransmissionID(id), Payload: []byte{byte(id)}}, dl); err != nil {
			t.Fatal(err)
		}
	}
	receipt, err := r.ReceiptFor(103)
	if err != nil {
		t.Fatal(err)
	}
	want := []protocol.Range{{Start: 5, End: 7}, {Start: 9, End: 11}}
	if len(receipt.ACKs) != 1 || !rangesEqual(receipt.ACKs[0].Ranges, want) {
		t.Fatalf("acks=%#v", receipt.ACKs)
	}
	if receipt.Gap == nil || receipt.Gap.Start != 7 || receipt.Gap.End != 9 || receipt.Gap.Kind != protocol.AckDatagram {
		t.Fatalf("gap=%#v", receipt.Gap)
	}

	// Expired datagrams are no longer represented in live receipt state and an
	// already-expired arrival is never acknowledged.
	clock.Advance(time.Second)
	if _, err := r.AcceptDatagram(protocol.DatagramFrame{FlowID: 103, DatagramID: 11, TransmissionID: 20, Payload: []byte("late")}, dl); err != nil {
		t.Fatal(err)
	}
	receipt, err = r.ReceiptFor(103)
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt.ACKs) != 0 || receipt.Gap != nil {
		t.Fatalf("expired receipt leaked=%#v", receipt)
	}
}

func TestReceiptUnknownFlow(t *testing.T) {
	r := NewReceiver(nil, 0)
	if _, err := r.ReceiptFor(999); !errors.Is(err, ErrUnknownFlow) {
		t.Fatalf("unknown receipt err=%v", err)
	}
}

func TestSplitAckRangesAtWireLimit(t *testing.T) {
	ranges := make([]protocol.Range, protocol.MaxAckRanges+1)
	for i := range ranges {
		ranges[i] = protocol.Range{Start: uint64(i * 2), End: uint64(i*2 + 1)}
	}
	acks := splitACKs(1, protocol.AckStream, true, ranges)
	if len(acks) != 2 || len(acks[0].Ranges) != protocol.MaxAckRanges || len(acks[1].Ranges) != 1 || !acks[0].FIN || !acks[1].FIN {
		t.Fatalf("split ACKs=%#v", acks)
	}
	for _, ack := range acks {
		if _, err := protocol.MarshalFrame(ack); err != nil {
			t.Fatalf("split ACK not encodable: %v", err)
		}
	}
}

func rangesEqual(a, b []protocol.Range) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
