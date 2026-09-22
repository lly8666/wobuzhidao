package faketcp

import (
	"bytes"
	"context"
	"testing"
	"time"
)

func TestClientAssociationBootstrapAndDetachHandoff(t *testing.T) {
	flow := ClientFlow{
		LocalIP: [4]byte{192, 0, 2, 10}, PeerIP: [4]byte{192, 0, 2, 20},
		LocalPort: 41000, PeerPort: 443,
	}
	emitted := make(chan Segment, 16)
	assoc, err := NewClientAssociation(flow, 1000, time.Second, func(seg Segment) error {
		copySeg := seg
		copySeg.Payload = append([]byte(nil), seg.Payload...)
		emitted <- copySeg
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer assoc.Close()

	now := time.Unix(100, 0)
	if err := assoc.Start(now); err != nil {
		t.Fatal(err)
	}
	syn := <-emitted
	if !IsWBDHandshakeSegment(syn) || syn.Seq != 1000 {
		t.Fatalf("SYN=%#v", syn)
	}

	synack := Segment{
		SrcIP: flow.PeerIP, DstIP: flow.LocalIP,
		SrcPort: flow.PeerPort, DstPort: flow.LocalPort,
		Seq: 9000, Ack: 1001, Flags: FlagSYN | FlagACK, Window: 32000,
		MSS: 1280, MSSSet: true, SACKPermitted: true,
		WindowScale: 4, WindowScaleSet: true,
	}
	if err := assoc.HandleSegment(synack, now.Add(10*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := assoc.WaitEstablished(context.Background()); err != nil {
		t.Fatal(err)
	}
	ack := <-emitted
	wantWindow := uint16(MaxBootstrapBufferedBytes >> DefaultWindowScale)
	if ack.Seq != 1001 || ack.Ack != 9001 || ack.Flags != FlagACK || ack.Window != wantWindow {
		t.Fatalf("handshake ACK=%#v want_window=%d", ack, wantWindow)
	}
	peer := assoc.PeerTCPProfile()
	if !peer.AdvertisedMSS || peer.MSS != 1280 || !peer.WindowScaleSet || peer.WindowScale != 4 {
		t.Fatalf("peer=%#v", peer)
	}

	conn := assoc.BootstrapConn()
	writeDone := make(chan error, 1)
	go func() {
		_, err := conn.Write([]byte("client-bootstrap"))
		writeDone <- err
	}()
	data := <-emitted
	if !bytes.Equal(data.Payload, []byte("client-bootstrap")) || data.Seq != 1001 {
		t.Fatalf("client data=%#v", data)
	}
	if err := assoc.HandleSegment(Segment{
		SrcIP: flow.PeerIP, DstIP: flow.LocalIP,
		SrcPort: flow.PeerPort, DstPort: flow.LocalPort,
		Seq: 9001, Ack: data.Seq + uint32(len(data.Payload)),
		Flags: FlagACK, Window: 32000,
	}, now.Add(20*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := <-writeDone; err != nil {
		t.Fatal(err)
	}

	serverPayload := []byte("server-bootstrap")
	if err := assoc.HandleSegment(Segment{
		SrcIP: flow.PeerIP, DstIP: flow.LocalIP,
		SrcPort: flow.PeerPort, DstPort: flow.LocalPort,
		Seq: 9001, Ack: data.Seq + uint32(len(data.Payload)),
		Flags: FlagACK | FlagPSH, Window: 32000, Payload: serverPayload,
	}, now.Add(30*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	payloadACK := <-emitted
	if payloadACK.Ack != 9001+uint32(len(serverPayload)) {
		t.Fatalf("payload ACK=%#v", payloadACK)
	}
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf[:n], serverPayload) {
		t.Fatalf("read=%q", buf[:n])
	}

	handoff, err := assoc.Detach()
	if err != nil {
		t.Fatal(err)
	}
	if handoff.SendNext != data.Seq+uint32(len(data.Payload)) ||
		handoff.ReceiveNext != 9001+uint32(len(serverPayload)) ||
		handoff.Peer.MSS != 1280 ||
		handoff.AdvertisedWindow != wantWindow ||
		!handoff.WindowScaleSet || handoff.WindowScale != DefaultWindowScale {
		t.Fatalf("handoff=%#v want_window=%d", handoff, wantWindow)
	}
}


func TestClientDetachSteadyWindowIgnoresBootstrapOccupancy(t *testing.T) {
	flow := ClientFlow{
		LocalIP: [4]byte{192, 0, 2, 30}, PeerIP: [4]byte{192, 0, 2, 40},
		LocalPort: 42000, PeerPort: 443,
	}
	emitted := make(chan Segment, 8)
	assoc, err := NewClientAssociation(flow, 2000, time.Second, func(seg Segment) error {
		emitted <- seg
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer assoc.Close()

	now := time.Unix(200, 0)
	if err := assoc.Start(now); err != nil {
		t.Fatal(err)
	}
	<-emitted // SYN
	synack := Segment{
		SrcIP: flow.PeerIP, DstIP: flow.LocalIP,
		SrcPort: flow.PeerPort, DstPort: flow.LocalPort,
		Seq: 10000, Ack: 2001, Flags: FlagSYN | FlagACK, Window: 32000,
		MSS: 1280, MSSSet: true, SACKPermitted: true,
		WindowScale: 4, WindowScaleSet: true,
	}
	if err := assoc.HandleSegment(synack, now.Add(time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := assoc.WaitEstablished(context.Background()); err != nil {
		t.Fatal(err)
	}
	<-emitted // final handshake ACK

	assoc.bootstrap.Feed(10001, bytes.Repeat([]byte{0x7a}, MaxBootstrapBufferedBytes))
	assoc.mu.Lock()
	bootstrapWindow := assoc.advertisedWindowLocked()
	assoc.mu.Unlock()
	if bootstrapWindow != 0 {
		t.Fatalf("bootstrap window=%d want=0 at full buffer", bootstrapWindow)
	}

	handoff, err := assoc.Detach()
	if err != nil {
		t.Fatal(err)
	}
	want := steadyAdvertisedWindow(true)
	if want != uint16(MaxBootstrapBufferedBytes>>DefaultWindowScale) {
		t.Fatalf("steady helper=%d unexpected", want)
	}
	if handoff.AdvertisedWindow != want || !handoff.WindowScaleSet ||
		handoff.WindowScale != DefaultWindowScale {
		t.Fatalf("handoff=%+v want steady window=%d scale=%d", handoff, want, DefaultWindowScale)
	}
	if got := steadyAdvertisedWindow(false); got != 65535 {
		t.Fatalf("unscaled steady window=%d want=65535", got)
	}
}
