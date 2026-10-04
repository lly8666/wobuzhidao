package scheduler

import (
	"context"
	"errors"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/lane"
	"github.com/lly8666/wobuzhidao/internal/protocol"
	"github.com/lly8666/wobuzhidao/internal/recovery"
	"github.com/lly8666/wobuzhidao/internal/session"
)

type fakeCarrier struct {
	mu      sync.Mutex
	active  []protocol.LaneID
	sent    []fakeSend
	block   map[protocol.LaneID]chan struct{}
	started chan protocol.LaneID
	fail    map[protocol.LaneID]bool
}

type fakeSend struct {
	lane  protocol.LaneID
	frame any
}

func (f *fakeCarrier) ActiveLaneIDs() []protocol.LaneID {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]protocol.LaneID(nil), f.active...)
}

func (f *fakeCarrier) setActive(ids ...protocol.LaneID) {
	f.mu.Lock()
	f.active = append([]protocol.LaneID(nil), ids...)
	f.mu.Unlock()
}

func (f *fakeCarrier) SendOn(id protocol.LaneID, frame any) error {
	f.mu.Lock()
	if f.fail[id] {
		f.mu.Unlock()
		return errors.New("synthetic send failure")
	}
	var wait chan struct{}
	if f.block != nil {
		wait = f.block[id]
	}
	started := f.started
	f.mu.Unlock()
	if started != nil {
		select {
		case started <- id:
		default:
		}
	}
	if wait != nil {
		<-wait
	}
	f.mu.Lock()
	f.sent = append(f.sent, fakeSend{lane: id, frame: frame})
	f.mu.Unlock()
	return nil
}

func TestBulkCreditAndACKRelease(t *testing.T) {
	carrier := &fakeCarrier{active: []protocol.LaneID{1, 2, 3}}
	s, err := New(carrier, Config{BulkLaneIDs: []protocol.LaneID{2, 1}, RescueLaneID: 3, MaxBulkFlightBytes: 5, MaxRescueFlightBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	one := protocol.DataFrame{FlowID: 1, Offset: 0, TransmissionID: 1, Payload: []byte("hello")}
	two := protocol.DataFrame{FlowID: 1, Offset: 5, TransmissionID: 2, Payload: []byte("world")}
	if id, err := s.SendBulk(one); err != nil || id != 1 {
		t.Fatalf("first id=%d err=%v", id, err)
	}
	if id, err := s.SendBulk(two); err != nil || id != 2 {
		t.Fatalf("second id=%d err=%v", id, err)
	}
	if s.FlightBytes(1) != 5 || s.FlightBytes(2) != 5 || s.FlightBytes(3) != 0 {
		t.Fatalf("flight=%d/%d/%d", s.FlightBytes(1), s.FlightBytes(2), s.FlightBytes(3))
	}
	if _, err := s.SendBulk(protocol.DataFrame{FlowID: 1, Offset: 10, TransmissionID: 3, Payload: []byte("!")}); !errors.Is(err, ErrNoBulkCredit) {
		t.Fatalf("want no bulk credit, got %v", err)
	}
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 1, Kind: protocol.AckStream, Ranges: []protocol.Range{{Start: 0, End: 5}}}); err != nil {
		t.Fatal(err)
	}
	if s.FlightBytes(1) != 0 || s.FlightBytes(2) != 5 {
		t.Fatalf("ACK release=%d/%d", s.FlightBytes(1), s.FlightBytes(2))
	}
	if id, err := s.SendBulk(protocol.DataFrame{FlowID: 1, Offset: 10, TransmissionID: 3, Payload: []byte("!")}); err != nil || id != 1 {
		t.Fatalf("re-admit id=%d err=%v", id, err)
	}
}

func TestRescueCreditSeparateAndLogicalACKFreesAllCopies(t *testing.T) {
	carrier := &fakeCarrier{active: []protocol.LaneID{1, 2, 3}}
	s, err := New(carrier, Config{BulkLaneIDs: []protocol.LaneID{1, 2}, RescueLaneID: 3, MaxBulkFlightBytes: 8, MaxRescueFlightBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	frame := protocol.DataFrame{FlowID: 2, Offset: 0, TransmissionID: 1, Payload: []byte("hello")}
	id, err := s.SendBulk(frame)
	if err != nil {
		t.Fatal(err)
	}
	rescue := frame
	rescue.TransmissionID = 99
	if err := s.RescueSender().SendOn(3, rescue); err != nil {
		t.Fatal(err)
	}
	if s.FlightBytes(id) != 5 || s.FlightBytes(3) != 5 {
		t.Fatalf("copy flight bulk=%d rescue=%d", s.FlightBytes(id), s.FlightBytes(3))
	}
	if err := s.RescueSender().SendOn(3, protocol.DataFrame{FlowID: 2, Offset: 5, TransmissionID: 100, Payload: []byte("x")}); !errors.Is(err, ErrNoRescueCredit) {
		t.Fatalf("want rescue cap, got %v", err)
	}
	// Tiny control traffic is allowed on the low-occupancy rescue carrier even
	// while its DATA flight is full.
	if err := s.SendControl(protocol.GapHintFrame{FlowID: 2, Kind: protocol.AckStream, Start: 5, End: 6}); err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 2, Kind: protocol.AckStream, Ranges: []protocol.Range{{Start: 0, End: 5}}}); err != nil {
		t.Fatal(err)
	}
	if s.FlightBytes(id) != 0 || s.FlightBytes(3) != 0 {
		t.Fatalf("logical ACK did not free copies bulk=%d rescue=%d", s.FlightBytes(id), s.FlightBytes(3))
	}
}

func TestACKCannotReleaseCreditWhileSocketWriteIsPending(t *testing.T) {
	gate := make(chan struct{})
	carrier := &fakeCarrier{
		active:  []protocol.LaneID{1, 3},
		block:   map[protocol.LaneID]chan struct{}{1: gate},
		started: make(chan protocol.LaneID, 1),
	}
	s, err := New(carrier, Config{BulkLaneIDs: []protocol.LaneID{1}, RescueLaneID: 3, MaxBulkFlightBytes: 5, MaxRescueFlightBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.SendBulk(protocol.DataFrame{FlowID: 3, Offset: 0, TransmissionID: 1, Payload: []byte("hello")})
		done <- err
	}()
	select {
	case id := <-carrier.started:
		if id != 1 {
			t.Fatalf("started lane=%d", id)
		}
	case <-time.After(time.Second):
		t.Fatal("write did not start")
	}
	if got := s.FlightBytes(1); got != 5 {
		t.Fatalf("pending reservation=%d", got)
	}
	// An ACK for an equivalent copy must not free this reservation until the
	// blocked Write returns; otherwise another producer could over-admit lane 1.
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 3, Kind: protocol.AckStream, Ranges: []protocol.Range{{Start: 0, End: 5}}}); err != nil {
		t.Fatal(err)
	}
	if got := s.FlightBytes(1); got != 5 {
		t.Fatalf("pending write credit prematurely released=%d", got)
	}
	if _, err := s.SendBulk(protocol.DataFrame{FlowID: 3, Offset: 5, TransmissionID: 2, Payload: []byte("x")}); !errors.Is(err, ErrNoBulkCredit) {
		t.Fatalf("want no credit while write pending, got %v", err)
	}
	close(gate)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("write did not finish")
	}
	if got := s.FlightBytes(1); got != 0 {
		t.Fatalf("ACKed write not released after completion=%d", got)
	}
}

func TestDropInactiveReleasesSchedulerOnlyAccounting(t *testing.T) {
	carrier := &fakeCarrier{active: []protocol.LaneID{1, 2, 3}}
	s, _ := New(carrier, Config{BulkLaneIDs: []protocol.LaneID{1, 2}, RescueLaneID: 3, MaxBulkFlightBytes: 10, MaxRescueFlightBytes: 5})
	if _, err := s.SendBulk(protocol.DataFrame{FlowID: 4, Offset: 0, TransmissionID: 1, Payload: []byte("abcd")}); err != nil {
		t.Fatal(err)
	}
	carrier.setActive(2, 3)
	if got := s.DropInactive(); got != 4 || s.FlightBytes(1) != 0 {
		t.Fatalf("released=%d flight1=%d", got, s.FlightBytes(1))
	}
}

func realPair(t *testing.T) (*lane.TCP, *lane.TCP) {
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

func TestThreeRealTCPLanesReserveRescueForGapRecovery(t *testing.T) {
	c1, s1 := realPair(t)
	c2, s2 := realPair(t)
	c3, s3 := realPair(t)
	senderPool := lane.NewPool(32)
	receiverPool := lane.NewPool(32)
	for id, c := range map[protocol.LaneID]*lane.TCP{1: c1, 2: c2, 3: c3} {
		if err := senderPool.Add(id, c); err != nil {
			t.Fatal(err)
		}
	}
	// Bulk lane 1 is intentionally withheld from the receiver. Bulk lane 2 and
	// the dedicated rescue/control lane 3 are live.
	if err := receiverPool.Add(2, s2); err != nil {
		t.Fatal(err)
	}
	if err := receiverPool.Add(3, s3); err != nil {
		t.Fatal(err)
	}
	defer senderPool.Close()
	defer receiverPool.Close()

	sched, err := New(senderPool, Config{BulkLaneIDs: []protocol.LaneID{1, 2}, RescueLaneID: 3, MaxBulkFlightBytes: 5, MaxRescueFlightBytes: 5})
	if err != nil {
		t.Fatal(err)
	}
	score := recovery.NewStreamSender(1000)
	prefix := protocol.DataFrame{FlowID: 80, Offset: 0, TransmissionID: 1, Payload: []byte("hello")}
	tail := protocol.DataFrame{FlowID: 80, Offset: 5, TransmissionID: 2, FIN: true, Payload: []byte("world")}
	id1, err := sched.SendBulk(prefix)
	if err != nil || id1 != 1 {
		t.Fatalf("prefix lane=%d err=%v", id1, err)
	}
	if err := score.Track(prefix, id1); err != nil {
		t.Fatal(err)
	}
	id2, err := sched.SendBulk(tail)
	if err != nil || id2 != 2 {
		t.Fatalf("tail lane=%d err=%v", id2, err)
	}
	if err := score.Track(tail, id2); err != nil {
		t.Fatal(err)
	}
	if _, err := sched.SendBulk(protocol.DataFrame{FlowID: 80, Offset: 10, TransmissionID: 3, Payload: []byte("!")}); !errors.Is(err, ErrNoBulkCredit) {
		t.Fatalf("bulk should be full, got %v", err)
	}
	if sched.FlightBytes(3) != 0 {
		t.Fatalf("rescue lane polluted by bulk: %d", sched.FlightBytes(3))
	}

	recv := session.NewReceiver(nil, 0)
	ev := <-receiverPool.Events()
	if ev.LaneID != 2 || ev.Err != nil || !reflect.DeepEqual(ev.Frame, tail) {
		t.Fatalf("tail event=%#v", ev)
	}
	if _, err := recv.AcceptData(ev.Frame.(protocol.DataFrame)); err != nil {
		t.Fatal(err)
	}
	receipt, err := recv.ReceiptFor(80)
	if err != nil || receipt.Gap == nil || receipt.Gap.Start != 0 || receipt.Gap.End != 5 {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
	// GAP travels on the reserved control/rescue carrier while both bulk credits
	// are full.
	if err := receiverPool.SendOn(3, *receipt.Gap); err != nil {
		t.Fatal(err)
	}
	control := <-senderPool.Events()
	if control.LaneID != 3 || control.Err != nil {
		t.Fatalf("control=%#v", control)
	}
	rescues, err := score.ReinjectGap(control.Frame.(protocol.GapHintFrame), sched.RescueSender())
	if err != nil {
		t.Fatal(err)
	}
	if len(rescues) != 1 || rescues[0].LaneID != 3 || rescues[0].Frame.TransmissionID != 1000 || sched.FlightBytes(3) != 5 {
		t.Fatalf("rescue=%#v rescueFlight=%d", rescues, sched.FlightBytes(3))
	}

	rescued := <-receiverPool.Events()
	if rescued.LaneID != 3 || rescued.Err != nil {
		t.Fatalf("rescued=%#v", rescued)
	}
	complete, err := recv.AcceptData(rescued.Frame.(protocol.DataFrame))
	if err != nil {
		t.Fatal(err)
	}
	if string(complete.Data) != "helloworld" || !complete.Complete {
		t.Fatalf("complete=%#v", complete)
	}

	finalReceipt, err := recv.ReceiptFor(80)
	if err != nil {
		t.Fatal(err)
	}
	for _, ack := range finalReceipt.ACKs {
		if err := receiverPool.SendOn(3, ack); err != nil {
			t.Fatal(err)
		}
		ackEvent := <-senderPool.Events()
		gotAck := ackEvent.Frame.(protocol.AckFrame)
		if err := score.ApplyACK(gotAck); err != nil {
			t.Fatal(err)
		}
		if err := sched.ApplyACK(gotAck); err != nil {
			t.Fatal(err)
		}
	}
	if sched.FlightBytes(1) != 0 || sched.FlightBytes(2) != 0 || sched.FlightBytes(3) != 0 {
		t.Fatalf("flight not released: %d/%d/%d", sched.FlightBytes(1), sched.FlightBytes(2), sched.FlightBytes(3))
	}

	// Only after logical completion do we release the gated original bulk lane.
	if err := receiverPool.Add(1, s1); err != nil {
		t.Fatal(err)
	}
	late := <-receiverPool.Events()
	if late.LaneID != 1 || late.Err != nil {
		t.Fatalf("late=%#v", late)
	}
	dup, err := recv.AcceptData(late.Frame.(protocol.DataFrame))
	if err != nil {
		t.Fatal(err)
	}
	if !dup.Duplicate || len(dup.Data) != 0 || !dup.Complete {
		t.Fatalf("late original not duplicate=%#v", dup)
	}
}

func TestConfigAndFINUnitAccounting(t *testing.T) {
	carrier := &fakeCarrier{active: []protocol.LaneID{1, 3}}
	if _, err := New(carrier, Config{BulkLaneIDs: []protocol.LaneID{1, 3}, RescueLaneID: 3, MaxBulkFlightBytes: 1, MaxRescueFlightBytes: 1}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("overlap config err=%v", err)
	}
	s, err := New(carrier, Config{BulkLaneIDs: []protocol.LaneID{1}, RescueLaneID: 3, MaxBulkFlightBytes: 1, MaxRescueFlightBytes: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SendBulk(protocol.DataFrame{FlowID: 9, Offset: 0, TransmissionID: 1, FIN: true}); err != nil {
		t.Fatal(err)
	}
	if got := s.FlightBytes(1); got != 1 {
		t.Fatalf("FIN unit cost=%d", got)
	}
	if err := s.ApplyACK(protocol.AckFrame{FlowID: 9, Kind: protocol.AckStream, FIN: true}); err != nil {
		t.Fatal(err)
	}
	if got := s.FlightBytes(1); got != 0 {
		t.Fatalf("FIN ACK release=%d", got)
	}
}
