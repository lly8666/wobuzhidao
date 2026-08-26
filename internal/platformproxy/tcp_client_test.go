package platformproxy

import (
	"net"
	"net/netip"
	"testing"
	"time"
)

func TestTCPClientWaitsForOpenAckAndCarriesBothDirections(t *testing.T) {
	ln, err := net.ListenTCP("tcp4", &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	appCh := make(chan *net.TCPConn, 1)
	go func() {
		conn, _ := net.DialTCP("tcp4", nil, ln.Addr().(*net.TCPAddr))
		appCh <- conn
	}()
	proxyConn, err := ln.AcceptTCP()
	if err != nil {
		t.Fatal(err)
	}
	defer proxyConn.Close()
	app := <-appCh
	if app == nil {
		t.Fatal("app dial failed")
	}
	defer app.Close()

	frames := make(chan Frame, 32)
	cfg := DefaultTCPClientConfig()
	cfg.OpenRTO = 50 * time.Millisecond
	client, err := NewTCPClient(func(f Frame) error {
		frames <- cloneTCPFrame(f)
		return nil
	}, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	target := netip.MustParseAddrPort("198.51.100.20:443")
	flowID, err := client.Add(proxyConn, target, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	open := mustFrameKind(t, frames, KindTCPOpen)
	if open.FlowID != flowID || open.Peer != target {
		t.Fatalf("open=%+v", open)
	}

	if _, err := app.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	select {
	case f := <-frames:
		t.Fatalf("DATA escaped before OPEN ACK: %+v", f)
	case <-time.After(30 * time.Millisecond):
	}

	if err := client.HandleFrame(Frame{Kind: KindTCPAck, FlowID: flowID, Offset: 0}, time.Now()); err != nil {
		t.Fatal(err)
	}
	data := mustFrameKind(t, frames, KindTCPData)
	if data.FlowID != flowID || data.Offset != 0 || string(data.Payload) != "hello" || data.FIN {
		t.Fatalf("client data=%+v payload=%q", data, data.Payload)
	}

	if err := client.HandleFrame(Frame{Kind: KindTCPData, FlowID: flowID, Offset: 0, Payload: []byte("world")}, time.Now()); err != nil {
		t.Fatal(err)
	}
	ack := mustFrameKind(t, frames, KindTCPAck)
	if ack.Offset != 5 {
		t.Fatalf("ack=%+v", ack)
	}
	if err := app.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 5)
	if _, err := app.Read(buf); err != nil {
		t.Fatal(err)
	}
	if string(buf) != "world" {
		t.Fatalf("app read=%q", buf)
	}
}

func TestTCPClientRetransmitsOpenAndDropsUnknownClose(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	frames := make(chan Frame, 16)
	cfg := DefaultTCPClientConfig()
	cfg.OpenRTO = 10 * time.Millisecond
	cfg.MaxOpenRetransmit = 1
	client, err := NewTCPClient(func(f Frame) error { frames <- f; return nil }, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	start := time.Unix(100, 0)
	id, err := client.Add(left, netip.MustParseAddrPort("203.0.113.8:80"), start)
	if err != nil {
		t.Fatal(err)
	}
	_ = mustFrameKind(t, frames, KindTCPOpen)
	client.Tick(start.Add(10 * time.Millisecond))
	retry := mustFrameKind(t, frames, KindTCPOpen)
	if retry.FlowID != id {
		t.Fatalf("retry flow=%d want=%d", retry.FlowID, id)
	}
	client.Tick(start.Add(20 * time.Millisecond))
	closeFrame := mustFrameKind(t, frames, KindTCPClose)
	if closeFrame.FlowID != id || client.Len() != 0 {
		t.Fatalf("close=%+v len=%d", closeFrame, client.Len())
	}
	if err := client.HandleFrame(Frame{Kind: KindTCPClose, FlowID: id + 100}, time.Now()); err != nil {
		t.Fatalf("unknown close should be idempotent: %v", err)
	}
}

func mustFrameKind(t *testing.T, ch <-chan Frame, kind Kind) Frame {
	t.Helper()
	select {
	case frame := <-ch:
		if frame.Kind != kind {
			t.Fatalf("frame kind=%d want=%d frame=%+v", frame.Kind, kind, frame)
		}
		return frame
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for frame kind=%d", kind)
		return Frame{}
	}
}
