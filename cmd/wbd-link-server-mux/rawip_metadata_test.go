package main

import (
	"bytes"
	"net"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func TestRawIPBackendReceivesTunnelMetadataBeforePayload(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	addr := listener.LocalAddr().(*net.UDPAddr)
	s := &server{rawIPServiceAddr: addr}

	tunnelID, err := logicaltunnel.ParseTunnelID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	lease := netip.MustParseAddr("10.66.0.42")
	ps := &peerSession{sid: string(tunnelID)[:8]}
	peerTunnelBindings.Store(ps, realityfront.TicketBinding{
		Account: "metadata-test",
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: netip.PrefixFrom(lease, 32).String(),
			Routes4:  []string{"0.0.0.0/0"},
		},
	})
	defer forgetPeerTunnel(ps)

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
	buf := make([]byte, 128)
	n, _, err := listener.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	meta, ok := rawipbackend.UnmarshalTunnelMeta(buf[:n])
	if !ok {
		t.Fatalf("first backend datagram is not Logical Tunnel metadata: %x", buf[:n])
	}
	if meta.TunnelID != tunnelID || meta.Address4 != lease {
		t.Fatalf("metadata=%+v want tunnel=%s lease=%s", meta, tunnelID, lease)
	}
	if ps.backend != backendRawIP {
		t.Fatalf("backend=%q want rawip", ps.backend)
	}

	if _, err := ps.service.Write(wire); err != nil {
		t.Fatal(err)
	}
	_ = listener.SetReadDeadline(time.Now().Add(time.Second))
	n, _, err = listener.ReadFromUDP(buf)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf[:n], wire) {
		t.Fatalf("second backend datagram=%x want payload=%x", buf[:n], wire)
	}
}
