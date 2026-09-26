package recovery

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/lane"
	"github.com/lly8666/wobuzhidao/internal/protocol"
	"github.com/lly8666/wobuzhidao/internal/session"
)

type sentFrame struct {
	lane  protocol.LaneID
	frame protocol.DataFrame
}

type fakeLanes struct {
	active []protocol.LaneID
	fail   map[protocol.LaneID]bool
	sent   []sentFrame
}

func (f *fakeLanes) ActiveLaneIDs() []protocol.LaneID {
	return append([]protocol.LaneID(nil), f.active...)
}

func (f *fakeLanes) SendOn(id protocol.LaneID, frame any) error {
	if f.fail[id] {
		return errors.New("synthetic lane failure")
	}
	d, ok := frame.(protocol.DataFrame)
	if !ok {
		return errors.New("not DATA")
	}
	d.Payload = append([]byte(nil), d.Payload...)
	f.sent = append(f.sent, sentFrame{lane: id, frame: d})
	return nil
}

func TestGapReinjectsSameLogicalBytesOnAlternateLane(t *testing.T) {
	s := NewStreamSender(100)
	original := protocol.DataFrame{FlowID: 1, Offset: 0, TransmissionID: 7, Payload: []byte("hello")}
	if err := s.Track(original, 1); err != nil {
		t.Fatal(err)
	}
	// Mutating the caller's payload after Track must not alter recovery bytes.
	original.Payload[0] = 'X'
	lanes := &fakeLanes{active: []protocol.LaneID{1, 2}}
	got, err := s.ReinjectGap(protocol.GapHintFrame{FlowID: 1, Kind: protocol.AckStream, Start: 0, End: 5}, lanes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].LaneID != 2 || got[0].Frame.TransmissionID != 100 || string(got[0].Frame.Payload) != "hello" {
		t.Fatalf("reinjection=%#v", got)
	}
	if got[0].Frame.FlowID != 1 || got[0].Frame.Offset != 0 {
		t.Fatalf("logical identity changed: %#v", got[0].Frame)
	}
}

func TestGapSlicesSourceAndPreservesFINOnlyAtLogicalEnd(t *testing.T) {
	s := NewStreamSender(200)
	if err := s.Track(protocol.DataFrame{FlowID: 2, Offset: 10, TransmissionID: 1, FIN: true, Payload: []byte("abcdefghij")}, 1); err != nil {
		t.Fatal(err)
	}
	lanes := &fakeLanes{active: []protocol.LaneID{1, 2}}
	first, err := s.ReinjectGap(protocol.GapHintFrame{FlowID: 2, Kind: protocol.AckStream, Start: 12, End: 15}, lanes)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Frame.Offset != 12 || string(first[0].Frame.Payload) != "cde" || first[0].Frame.FIN {
		t.Fatalf("partial=%#v", first)
	}
	last, err := s.ReinjectGap(protocol.GapHintFrame{FlowID: 2, Kind: protocol.AckStream, Start: 17, End: 20}, lanes)
	if err != nil {
		t.Fatal(err)
	}
	if len(last) != 1 || last[0].Frame.Offset != 17 || string(last[0].Frame.Payload) != "hij" || !last[0].Frame.FIN {
		t.Fatalf("tail=%#v", last)
	}
}

func TestACKPrunesAndStaleGapIsNoop(t *testing.T) {
	s := NewStreamSender(10)
	if err := s.Track(protocol.DataFrame{FlowID: 3, Offset: 0, TransmissionID: 1, Payload: []byte("hello")}, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 3, Kind: protocol.AckStream, Ranges: []protocol.Range{{Start: 0, End: 5}}}); err != nil {
		t.Fatal(err)
	}
	if records, bytes := s.Outstanding(3); records != 0 || bytes != 0 {
		t.Fatalf("outstanding records=%d bytes=%d", records, bytes)
	}
	got, err := s.ReinjectGap(protocol.GapHintFrame{FlowID: 3, Kind: protocol.AckStream, Start: 0, End: 5}, &fakeLanes{active: []protocol.LaneID{1, 2}})
	if err != nil || len(got) != 0 {
		t.Fatalf("stale gap got=%#v err=%v", got, err)
	}
}

func TestPartialACKPreventsRedundantRescue(t *testing.T) {
	s := NewStreamSender(50)
	if err := s.Track(protocol.DataFrame{FlowID: 4, Offset: 0, TransmissionID: 1, Payload: []byte("abcdefghij")}, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 4, Kind: protocol.AckStream, Ranges: []protocol.Range{{Start: 0, End: 4}}}); err != nil {
		t.Fatal(err)
	}
	got, err := s.ReinjectGap(protocol.GapHintFrame{FlowID: 4, Kind: protocol.AckStream, Start: 0, End: 8}, &fakeLanes{active: []protocol.LaneID{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Frame.Offset != 4 || string(got[0].Frame.Payload) != "efgh" {
		t.Fatalf("ACK subtraction failed: %#v", got)
	}
}

func TestFailedAlternateConsumesTransmissionAndTriesNext(t *testing.T) {
	s := NewStreamSender(300)
	if err := s.Track(protocol.DataFrame{FlowID: 5, Offset: 0, TransmissionID: 1, Payload: []byte("x")}, 1); err != nil {
		t.Fatal(err)
	}
	lanes := &fakeLanes{active: []protocol.LaneID{1, 2, 3}, fail: map[protocol.LaneID]bool{2: true}}
	got, err := s.ReinjectGap(protocol.GapHintFrame{FlowID: 5, Kind: protocol.AckStream, Start: 0, End: 1}, lanes)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].LaneID != 3 || got[0].Frame.TransmissionID != 301 {
		t.Fatalf("fallback=%#v sent=%#v", got, lanes.sent)
	}
}

func TestNoAlternateAndUnknownGap(t *testing.T) {
	s := NewStreamSender(1)
	if err := s.Track(protocol.DataFrame{FlowID: 6, Offset: 0, TransmissionID: 1, Payload: []byte("x")}, 1); err != nil {
		t.Fatal(err)
	}
	_, err := s.ReinjectGap(protocol.GapHintFrame{FlowID: 6, Kind: protocol.AckStream, Start: 0, End: 1}, &fakeLanes{active: []protocol.LaneID{1}})
	if !errors.Is(err, ErrNoAlternateLane) {
		t.Fatalf("no alternate err=%v", err)
	}
	_, err = s.ReinjectGap(protocol.GapHintFrame{FlowID: 999, Kind: protocol.AckStream, Start: 0, End: 1}, &fakeLanes{active: []protocol.LaneID{1, 2}})
	if !errors.Is(err, ErrUnknownGap) {
		t.Fatalf("unknown gap err=%v", err)
	}
}

func TestTrackRejectsOverlappingSourceRecords(t *testing.T) {
	s := NewStreamSender(1)
	if err := s.Track(protocol.DataFrame{FlowID: 7, Offset: 0, TransmissionID: 1, Payload: []byte("abcd")}, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.Track(protocol.DataFrame{FlowID: 7, Offset: 3, TransmissionID: 2, Payload: []byte("de")}, 2); !errors.Is(err, ErrTrackedOverlap) {
		t.Fatalf("overlap err=%v", err)
	}
}

func tcpPair(t *testing.T) (*lane.TCP, *lane.TCP) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan net.Conn, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			accepted <- c
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := lane.DialTCP(ctx, ln.Addr().String())
	if err != nil {
		ln.Close()
		t.Fatal(err)
	}
	var raw net.Conn
	select {
	case raw = <-accepted:
	case <-ctx.Done():
		client.Close()
		ln.Close()
		t.Fatal(ctx.Err())
	}
	ln.Close()
	server := lane.WrapTCP(raw)
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	_ = server.SetDeadline(time.Now().Add(5 * time.Second))
	return client, server
}

func TestRealTCPGapReinjectionCompletesBeforeOriginalLaneIsReleased(t *testing.T) {
	c1, s1 := tcpPair(t)
	c2, s2 := tcpPair(t)
	senderPool := lane.NewPool(32)
	receiverPool := lane.NewPool(32)
	if err := senderPool.Add(1, c1); err != nil {
		t.Fatal(err)
	}
	if err := senderPool.Add(2, c2); err != nil {
		t.Fatal(err)
	}
	// Deliberately do NOT add server lane 1 yet. Its TCP bytes can queue in the
	// kernel, but the logical receiver cannot observe them. Lane 2 remains live.
	if err := receiverPool.Add(2, s2); err != nil {
		t.Fatal(err)
	}
	defer senderPool.Close()
	defer receiverPool.Close()

	score := NewStreamSender(1000)
	prefix := protocol.DataFrame{FlowID: 80, Offset: 0, TransmissionID: 1, Payload: []byte("hello")}
	tail := protocol.DataFrame{FlowID: 80, Offset: 5, TransmissionID: 2, FIN: true, Payload: []byte("world")}
	if err := senderPool.SendOn(1, prefix); err != nil {
		t.Fatal(err)
	}
	if err := score.Track(prefix, 1); err != nil {
		t.Fatal(err)
	}
	if err := senderPool.SendOn(2, tail); err != nil {
		t.Fatal(err)
	}
	if err := score.Track(tail, 2); err != nil {
		t.Fatal(err)
	}

	recv := session.NewReceiver(nil, 0)
	ev := <-receiverPool.Events()
	if ev.LaneID != 2 || ev.Err != nil || !reflect.DeepEqual(ev.Frame, tail) {
		t.Fatalf("tail event=%#v", ev)
	}
	out, err := recv.AcceptData(ev.Frame.(protocol.DataFrame))
	if err != nil {
		t.Fatal(err)
	}
	if out.NextOffset != 0 || len(out.Data) != 0 {
		t.Fatalf("tail crossed logical gap: %#v", out)
	}
	receipt, err := recv.ReceiptFor(80)
	if err != nil || receipt.Gap == nil || receipt.Gap.Start != 0 || receipt.Gap.End != 5 {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
	// Control returns over healthy lane 2; it is not tied to the stalled lane.
	if err := receiverPool.SendOn(2, *receipt.Gap); err != nil {
		t.Fatal(err)
	}
	control := <-senderPool.Events()
	gap := control.Frame.(protocol.GapHintFrame)
	rescues, err := score.ReinjectGap(gap, senderPool)
	if err != nil {
		t.Fatal(err)
	}
	if len(rescues) != 1 || rescues[0].LaneID != 2 || rescues[0].Frame.TransmissionID != 1000 || !EqualLogicalData(rescues[0].Frame, prefix) {
		t.Fatalf("rescues=%#v", rescues)
	}
	// The rescue arrives and completes the stream while original lane 1 is
	// still absent from receiverPool. This is the M6 HOL-reduction invariant.
	rescued := <-receiverPool.Events()
	if rescued.LaneID != 2 || rescued.Err != nil {
		t.Fatalf("rescue event=%#v", rescued)
	}
	complete, err := recv.AcceptData(rescued.Frame.(protocol.DataFrame))
	if err != nil {
		t.Fatal(err)
	}
	if string(complete.Data) != "helloworld" || !complete.Complete {
		t.Fatalf("rescued delivery=%#v", complete)
	}

	finalReceipt, err := recv.ReceiptFor(80)
	if err != nil {
		t.Fatal(err)
	}
	for _, ack := range finalReceipt.ACKs {
		if err := score.ApplyACK(ack); err != nil {
			t.Fatal(err)
		}
	}
	if records, bytes := score.Outstanding(80); records != 0 || bytes != 0 {
		t.Fatalf("ACK did not prune scoreboard records=%d bytes=%d", records, bytes)
	}

	// Release original lane only after logical completion. The original TCP
	// copy is still valid and later becomes a harmless logical duplicate.
	if err := receiverPool.Add(1, s1); err != nil {
		t.Fatal(err)
	}
	late := <-receiverPool.Events()
	if late.LaneID != 1 || late.Err != nil {
		t.Fatalf("late original=%#v", late)
	}
	dup, err := recv.AcceptData(late.Frame.(protocol.DataFrame))
	if err != nil {
		t.Fatal(err)
	}
	if !dup.Duplicate || len(dup.Data) != 0 || !dup.Complete {
		t.Fatalf("late original not deduped: %#v", dup)
	}
}

func TestUnknownACKRejectedAndFINRequiresFINAck(t *testing.T) {
	s := NewStreamSender(20)
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 999, Kind: protocol.AckStream, Ranges: []protocol.Range{{Start: 0, End: 1}}}); !errors.Is(err, ErrUnknownACK) {
		t.Fatalf("unknown ACK err=%v", err)
	}
	if err := s.Track(protocol.DataFrame{FlowID: 8, Offset: 0, TransmissionID: 1, FIN: true, Payload: []byte("x")}, 1); err != nil {
		t.Fatal(err)
	}
	// Payload acknowledgement alone must not forget a FIN that has not been
	// observed by the receiver yet.
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 8, Kind: protocol.AckStream, Ranges: []protocol.Range{{Start: 0, End: 1}}}); err != nil {
		t.Fatal(err)
	}
	if records, _ := s.Outstanding(8); records != 1 {
		t.Fatalf("FIN record pruned before FIN ACK: %d", records)
	}
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 8, Kind: protocol.AckStream, FIN: true}); err != nil {
		t.Fatal(err)
	}
	if records, bytes := s.Outstanding(8); records != 0 || bytes != 0 {
		t.Fatalf("FIN ACK did not prune record=%d bytes=%d", records, bytes)
	}
}
