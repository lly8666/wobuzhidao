package lane

import (
	"bytes"
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

func TestSingleRealTCPLaneFragmentedBidirectionalAndHalfClose(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	serverConn := make(chan *TCP, 1)
	acceptErr := make(chan error, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			acceptErr <- err
			return
		}
		serverConn <- WrapTCP(c)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := DialTCP(ctx, ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	_ = client.SetDeadline(time.Now().Add(5 * time.Second))

	var server *TCP
	select {
	case server = <-serverConn:
	case err := <-acceptErr:
		t.Fatal(err)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	defer server.Close()
	_ = server.SetDeadline(time.Now().Add(5 * time.Second))

	data := protocol.DataFrame{FlowID: 1, Offset: 0, TransmissionID: 1, FIN: true, Payload: []byte("hello")}
	dgram := protocol.DatagramFrame{FlowID: 2, DatagramID: 7, TransmissionID: 2, Payload: []byte("voice")}
	ack := protocol.AckFrame{FlowID: 1, Kind: protocol.AckStream, Ranges: []protocol.Range{{Start: 0, End: 5}}}
	gap := protocol.GapHintFrame{FlowID: 3, Kind: protocol.AckStream, Start: 10, End: 20}

	// Deliberately concatenate two frames, then fragment that byte stream into
	// irregular writes. The receiver must care only about WBD framing, not TCP
	// packet/write boundaries.
	b1, _ := protocol.MarshalFrame(data)
	b2, _ := protocol.MarshalFrame(dgram)
	wire := append(append([]byte(nil), b1...), b2...)
	steps := []int{1, 2, 5, 3, 11}
	for i, off := 0, 0; off < len(wire); i++ {
		n := steps[i%len(steps)]
		if off+n > len(wire) {
			n = len(wire) - off
		}
		written, err := client.conn.Write(wire[off : off+n])
		if err != nil {
			t.Fatal(err)
		}
		if written != n {
			t.Fatalf("short socket write %d/%d", written, n)
		}
		off += n
	}

	clock := session.NewManualClock(time.Unix(0, 0).UTC())
	recv := session.NewReceiver(clock, 0)
	got1, err := server.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got1, data) {
		t.Fatalf("DATA got=%#v want=%#v", got1, data)
	}
	delivery, err := recv.AcceptData(got1.(protocol.DataFrame))
	if err != nil {
		t.Fatal(err)
	}
	if string(delivery.Data) != "hello" || !delivery.Complete {
		t.Fatalf("stream delivery=%#v", delivery)
	}

	got2, err := server.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got2, dgram) {
		t.Fatalf("DATAGRAM got=%#v want=%#v", got2, dgram)
	}
	dg, err := recv.AcceptDatagram(got2.(protocol.DatagramFrame), clock.Now().Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if !dg.Delivered || string(dg.Payload) != "voice" {
		t.Fatalf("datagram delivery=%#v", dg)
	}

	// Server->client uses the framed lane writer. Consecutive sends are allowed
	// to coalesce inside the kernel TCP stream.
	if err := server.Send(ack); err != nil {
		t.Fatal(err)
	}
	if err := server.Send(gap); err != nil {
		t.Fatal(err)
	}
	back1, err := client.Receive()
	if err != nil {
		t.Fatal(err)
	}
	back2, err := client.Receive()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back1, ack) || !reflect.DeepEqual(back2, gap) {
		t.Fatalf("responses got=%#v %#v", back1, back2)
	}

	if err := client.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if _, err := server.Receive(); !errors.Is(err, io.EOF) {
		t.Fatalf("server after client CloseWrite: %v", err)
	}
	if err := server.CloseWrite(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Receive(); !errors.Is(err, io.EOF) {
		t.Fatalf("client after server CloseWrite: %v", err)
	}
}

func TestTCPConcurrentSendKeepsFrameBoundaries(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	lane := WrapTCP(left)

	frames := []protocol.DataFrame{
		{FlowID: 20, Offset: 0, TransmissionID: 1, Payload: bytes.Repeat([]byte{'a'}, 257)},
		{FlowID: 21, Offset: 0, TransmissionID: 2, Payload: bytes.Repeat([]byte{'b'}, 257)},
	}
	errCh := make(chan error, len(frames))
	for _, frame := range frames {
		frame := frame
		go func() { errCh <- lane.Send(frame) }()
	}

	seen := map[protocol.FlowID]bool{}
	for range frames {
		got, err := protocol.ReadFrame(right)
		if err != nil {
			t.Fatal(err)
		}
		f := got.(protocol.DataFrame)
		seen[f.FlowID] = true
	}
	for range frames {
		if err := <-errCh; err != nil {
			t.Fatal(err)
		}
	}
	if !seen[20] || !seen[21] {
		t.Fatalf("missing concurrent frame: %#v", seen)
	}
}
