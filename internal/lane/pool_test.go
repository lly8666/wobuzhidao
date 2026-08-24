package lane

import (
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/protocol"
	"github.com/lly8666/wobuzhidao/internal/session"
)

func realTCPPair(t *testing.T) (*TCP, *TCP) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	accepted := make(chan net.Conn, 1)
	errCh := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		accepted <- c
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client, err := DialTCP(ctx, ln.Addr().String())
	if err != nil {
		ln.Close()
		t.Fatal(err)
	}
	var server net.Conn
	select {
	case server = <-accepted:
	case err := <-errCh:
		client.Close()
		ln.Close()
		t.Fatal(err)
	case <-ctx.Done():
		client.Close()
		ln.Close()
		t.Fatal(ctx.Err())
	}
	ln.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))
	s := WrapTCP(server)
	_ = s.SetDeadline(time.Now().Add(5 * time.Second))
	return client, s
}

func TestPoolRoundRobinAndIndependentLaneReorder(t *testing.T) {
	client1, server1 := realTCPPair(t)
	client2, server2 := realTCPPair(t)
	defer server1.Close()
	defer server2.Close()

	pool := NewPool(32)
	// Add in reverse order; scheduling is defined by ascending LaneID.
	if err := pool.Add(2, client2); err != nil {
		t.Fatal(err)
	}
	if err := pool.Add(1, client1); err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if got := pool.ActiveLaneIDs(); !reflect.DeepEqual(got, []protocol.LaneID{1, 2}) {
		t.Fatalf("active lanes=%v", got)
	}

	// New-data scheduling is deterministic round-robin, independent of
	// receiver order. M4 does not yet react to ACK/loss signals.
	wantIDs := []protocol.LaneID{1, 2, 1, 2}
	for i, wantID := range wantIDs {
		id, err := pool.SendNext(protocol.DataFrame{FlowID: 50, Offset: protocol.StreamOffset(i), TransmissionID: protocol.TransmissionID(i + 1), Payload: []byte{byte(i)}})
		if err != nil {
			t.Fatal(err)
		}
		if id != wantID {
			t.Fatalf("send %d lane=%d want=%d", i, id, wantID)
		}
	}
	for i := 0; i < 2; i++ {
		if _, err := server1.Receive(); err != nil {
			t.Fatal(err)
		}
		if _, err := server2.Receive(); err != nil {
			t.Fatal(err)
		}
	}

	// Explicit lane selection is separate from round-robin policy.
	explicit := protocol.GapHintFrame{FlowID: 51, Kind: protocol.AckStream, Start: 10, End: 20}
	if err := pool.SendOn(2, explicit); err != nil {
		t.Fatal(err)
	}
	gotExplicit, err := server2.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotExplicit, explicit) {
		t.Fatalf("explicit got=%#v", gotExplicit)
	}

	clock := session.NewManualClock(time.Unix(0, 0).UTC())
	recv := session.NewReceiver(clock, 0)
	lateHalf := protocol.DataFrame{FlowID: 60, Offset: 5, TransmissionID: 100, FIN: true, Payload: []byte("world")}
	lateHalfDup := lateHalf
	lateHalfDup.TransmissionID = 101
	dgram := protocol.DatagramFrame{FlowID: 61, DatagramID: 1, TransmissionID: 102, Payload: []byte("voice")}

	// Lane 1 intentionally withholds the missing stream prefix. Lane 2 can
	// still deliver later stream bytes and a datagram; another copy of the same
	// logical stream range arrives on lane 1 and must dedup independently of
	// TransmissionID/LaneID.
	if err := server2.Send(lateHalf); err != nil {
		t.Fatal(err)
	}
	if err := server2.Send(dgram); err != nil {
		t.Fatal(err)
	}
	if err := server1.Send(lateHalfDup); err != nil {
		t.Fatal(err)
	}

	seenLanes := map[protocol.LaneID]bool{}
	streamUnique := 0
	streamDuplicate := 0
	datagramDelivered := false
	deadline := time.After(2 * time.Second)
	for received := 0; received < 3; received++ {
		select {
		case ev := <-pool.Events():
			if ev.Err != nil {
				t.Fatalf("unexpected lane error: lane=%d err=%v", ev.LaneID, ev.Err)
			}
			seenLanes[ev.LaneID] = true
			switch f := ev.Frame.(type) {
			case protocol.DataFrame:
				out, err := recv.AcceptData(f)
				if err != nil {
					t.Fatal(err)
				}
				if len(out.Data) != 0 || out.NextOffset != 0 {
					t.Fatalf("stream advanced across missing prefix: %#v", out)
				}
				if out.Duplicate {
					streamDuplicate++
				} else {
					streamUnique++
				}
			case protocol.DatagramFrame:
				out, err := recv.AcceptDatagram(f, clock.Now().Add(time.Second))
				if err != nil {
					t.Fatal(err)
				}
				datagramDelivered = out.Delivered && string(out.Payload) == "voice"
			default:
				t.Fatalf("unexpected frame %#v", ev.Frame)
			}
		case <-deadline:
			t.Fatal("timed out waiting for independent lane events")
		}
	}
	if !seenLanes[1] || !seenLanes[2] || streamUnique != 1 || streamDuplicate != 1 || !datagramDelivered {
		t.Fatalf("fan-in state lanes=%v unique=%d duplicate=%d datagram=%v", seenLanes, streamUnique, streamDuplicate, datagramDelivered)
	}

	// Only now release the missing prefix on lane 1. The buffered lane-2 bytes
	// drain immediately into one contiguous logical stream.
	prefix := protocol.DataFrame{FlowID: 60, Offset: 0, TransmissionID: 103, Payload: []byte("hello")}
	if err := server1.Send(prefix); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-pool.Events():
		if ev.Err != nil || ev.LaneID != 1 {
			t.Fatalf("prefix event=%#v", ev)
		}
		out, err := recv.AcceptData(ev.Frame.(protocol.DataFrame))
		if err != nil {
			t.Fatal(err)
		}
		if string(out.Data) != "helloworld" || !out.Complete || out.NextOffset != 10 {
			t.Fatalf("reassembled=%#v", out)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for delayed prefix")
	}
}

func TestPoolLaneFailureRemovesLaneAndCloseUnblocksReaders(t *testing.T) {
	client1, server1 := realTCPPair(t)
	client2, server2 := realTCPPair(t)
	defer server2.Close()
	pool := NewPool(8)
	if err := pool.Add(1, client1); err != nil {
		t.Fatal(err)
	}
	if err := pool.Add(2, client2); err != nil {
		t.Fatal(err)
	}

	// A peer close is a terminal lane event and removes it from new-data
	// scheduling; this is carrier bookkeeping, not M5/M6 recovery policy.
	if err := server1.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case ev := <-pool.Events():
		if ev.LaneID != 1 || ev.Err == nil || !(errors.Is(ev.Err, io.EOF) || errors.Is(ev.Err, net.ErrClosed)) {
			t.Fatalf("failure event=%#v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for lane failure")
	}
	if got := pool.ActiveLaneIDs(); !reflect.DeepEqual(got, []protocol.LaneID{2}) {
		t.Fatalf("active after failure=%v", got)
	}
	id, err := pool.SendNext(protocol.DataFrame{FlowID: 70, Offset: 0, TransmissionID: 1, Payload: []byte("ok")})
	if err != nil || id != 2 {
		t.Fatalf("send after lane failure id=%d err=%v", id, err)
	}
	if _, err := server2.Receive(); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- pool.Close() }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("pool.Close did not unblock lane readers")
	}
	select {
	case _, ok := <-pool.Events():
		if ok {
			for range pool.Events() {
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("events channel did not close")
	}
}

func TestPoolRejectsDuplicateAndUnknownLanes(t *testing.T) {
	left, right := net.Pipe()
	defer right.Close()
	pool := NewPool(4)
	if err := pool.Add(5, WrapTCP(left)); err != nil {
		t.Fatal(err)
	}
	if err := pool.Add(5, WrapTCP(right)); !errors.Is(err, ErrDuplicateLaneID) {
		t.Fatalf("duplicate lane err=%v", err)
	}
	if err := pool.SendOn(99, protocol.DataFrame{}); !errors.Is(err, ErrLaneUnavailable) {
		t.Fatalf("unknown lane err=%v", err)
	}
	_ = pool.Close()
}
