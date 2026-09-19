package platformproxy

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestRelayDispatchesUDPAndTCPOnOneServiceSocket(t *testing.T) {
	udpEcho := startUDPEcho(t)
	tcpLn, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer tcpLn.Close()
	go func() {
		conn, err := tcpLn.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		_, _ = conn.Write([]byte("server-hi"))
		select {
		case <-time.After(500 * time.Millisecond):
		}
	}()

	serviceConn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer serviceConn.Close()
	cfg := DefaultRelayConfig()
	cfg.TCP.DialTimeout = time.Second
	cfg.TCP.IdleTimeout = 2 * time.Second
	relay, err := NewRelay(serviceConn, cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- relay.Serve(ctx) }()
	defer func() {
		cancel()
		_ = serviceConn.Close()
		select {
		case err := <-done:
			if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
				t.Errorf("relay exit: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("relay did not stop")
		}
	}()

	client := dialRelay(t, serviceConn.LocalAddr().(*net.UDPAddr))
	defer client.Close()

	sendUDPFrame(t, client, Frame{Kind: KindUDPDatagram, FlowID: 1, Peer: udpEcho.LocalAddr().(*net.UDPAddr).AddrPort(), Payload: []byte("udp")})
	udpReply := readUDPFrame(t, client)
	if udpReply.Kind != KindUDPDatagram || string(udpReply.Payload) != "udp" {
		t.Fatalf("udp reply=%+v payload=%q", udpReply, udpReply.Payload)
	}

	tcpPeer := tcpLn.Addr().(*net.TCPAddr).AddrPort()
	sendUDPFrame(t, client, Frame{Kind: KindTCPOpen, FlowID: 2, Peer: tcpPeer})
	seenOpenACK := false
	seenData := false
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && (!seenOpenACK || !seenData) {
		frame := readUDPFrame(t, client)
		switch frame.Kind {
		case KindTCPAck:
			if frame.FlowID == 2 && frame.Offset == 0 {
				seenOpenACK = true
			}
		case KindTCPData:
			if frame.FlowID == 2 && string(frame.Payload) == "server-hi" {
				seenData = true
				ack := frame.Offset + uint64(len(frame.Payload))
				sendUDPFrame(t, client, Frame{Kind: KindTCPAck, FlowID: 2, Offset: ack})
			}
		}
	}
	if !seenOpenACK || !seenData {
		t.Fatalf("tcp dispatch missing openAck=%t data=%t", seenOpenACK, seenData)
	}
}
