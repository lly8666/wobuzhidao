//go:build windows

package main

import (
	"encoding/binary"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func ethernetFrameForTest(etherType uint16, payload []byte) []byte {
	frame := make([]byte, 14+len(payload))
	binary.BigEndian.PutUint16(frame[12:14], etherType)
	copy(frame[14:], payload)
	return frame
}

func vlanEthernetFrameForTest(etherType uint16, payload []byte) []byte {
	frame := make([]byte, 18+len(payload))
	binary.BigEndian.PutUint16(frame[12:14], 0x8100)
	binary.BigEndian.PutUint16(frame[14:16], 7)
	binary.BigEndian.PutUint16(frame[16:18], etherType)
	copy(frame[18:], payload)
	return frame
}

func acceptedInboundWBDFrameForTest(r *npcapRawPacketIO, frame []byte) ([]byte, bool) {
	packet, ok := ethernetIPv4Payload(frame)
	if !ok || !r.matchesFlowTCP(packet, false) {
		return nil, false
	}
	return packet, true
}

// Regression for the physical Windows failure that used to surface as
// "wbd-faketcp handshake: faketcp: not ipv4/tcp". Npcap captures an adapter,
// not a socket, so background Ethernet/IP traffic must be discarded at the raw
// backend boundary before the strict FakeTCP handshake parser sees it.
func TestNpcapCaptureBoundaryRejectsAdapterNoiseStorm(t *testing.T) {
	r := testFlowIO()

	inbound := faketcp.MarshalIPv4TCP(
		r.remoteIP, r.sourceIP,
		r.remotePort, r.sourcePort,
		0x11223344, 0x55667788,
		faketcp.FlagSYN|faketcp.FlagACK,
		65535, nil, 1,
	)
	valid := ethernetFrameForTest(0x0800, inbound)
	validVLAN := vlanEthernetFrameForTest(0x0800, inbound)

	outbound := faketcp.MarshalIPv4TCP(
		r.sourceIP, r.remoteIP,
		r.sourcePort, r.remotePort,
		7, 0, faketcp.FlagSYN,
		65535, nil, 2,
	)
	udpSameIPs := append([]byte(nil), inbound...)
	udpSameIPs[9] = 17
	wrongPort := append([]byte(nil), inbound...)
	binary.BigEndian.PutUint16(wrongPort[20:22], r.remotePort+1)
	wrongIP := append([]byte(nil), inbound...)
	wrongIP[12] ^= 0x01

	noise := [][]byte{
		ethernetFrameForTest(0x0806, make([]byte, 28)),
		ethernetFrameForTest(0x86dd, make([]byte, 40)),
		ethernetFrameForTest(0x0800, udpSameIPs),
		ethernetFrameForTest(0x0800, outbound),
		ethernetFrameForTest(0x0800, wrongPort),
		ethernetFrameForTest(0x0800, wrongIP),
		vlanEthernetFrameForTest(0x86dd, make([]byte, 40)),
		vlanEthernetFrameForTest(0x0800, udpSameIPs),
		[]byte{0, 1, 2, 3, 4, 5},
		ethernetFrameForTest(0x0800, []byte{0x45, 0, 0, 20}),
	}

	for round := 0; round < 2000; round++ {
		for i, frame := range noise {
			if packet, ok := acceptedInboundWBDFrameForTest(r, frame); ok {
				t.Fatalf("noise escaped Npcap boundary round=%d case=%d packet=%x", round, i, packet)
			}
		}
	}

	for name, frame := range map[string][]byte{"plain": valid, "vlan": validVLAN} {
		packet, ok := acceptedInboundWBDFrameForTest(r, frame)
		if !ok {
			t.Fatalf("valid %s inbound WBD SYNACK was rejected after noise storm", name)
		}
		seg, err := faketcp.ParseIPv4TCP(packet)
		if err != nil {
			t.Fatalf("valid %s WBD SYNACK reached parser but failed: %v", name, err)
		}
		if !faketcp.IsWBDHandshakeSegment(seg) || seg.Flags&(faketcp.FlagSYN|faketcp.FlagACK) != faketcp.FlagSYN|faketcp.FlagACK {
			t.Fatalf("valid %s WBD SYNACK lost handshake identity: %+v", name, seg)
		}
	}
}
