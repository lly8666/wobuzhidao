package main

import (
	"net"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
)

func TestRawIPBackendReceivesSessionMetadataBeforePayload(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	addr := listener.LocalAddr().(*net.UDPAddr)
	s := &server{rawIPServiceAddr: addr}
	ps := &peerSession{sid: "7c31a9"}
	packet := make([]byte, 20)
	packet[0] = 0x45
	packet[2], packet[3] = 0, 20
	wire, err := dataplane.MarshalIP(packet)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ensureService(ps, wire); err != nil {
		t.Fatal(err)
	}
	defer ps.service.Close()
	_ = listener.SetReadDeadline(time.Now().Add(time.Second))
	buf := make([]byte, 64)
	n, _, err := listener.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	meta, ok := rawipbackend.UnmarshalSessionMeta(buf[:n])
	if !ok || meta.SID != ps.sid {
		t.Fatalf("metadata=%+v ok=%v want sid=%s", meta, ok, ps.sid)
	}
	if ps.backend != backendRawIP {
		t.Fatalf("backend=%q want rawip", ps.backend)
	}
}
