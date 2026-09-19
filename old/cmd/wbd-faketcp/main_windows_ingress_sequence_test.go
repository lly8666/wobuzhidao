//go:build windows

package main

import (
	"encoding/binary"
	"testing"
)

// TestWindowsNpcapIngressSequenceIgnoresNoiseBeforeExactPeerPacket locks the
// exact filtering order used by npcapRawPacketIO.ReadPacket: Ethernet/VLAN
// extraction first, then exact inbound IPv4/TCP four-tuple selection.  The
// physical adapter is noisy during startup; unrelated frames must never reach
// recvOne()/ParsePacket where they could turn into a fatal "not ipv4/tcp"
// handshake error.
func TestWindowsNpcapIngressSequenceIgnoresNoiseBeforeExactPeerPacket(t *testing.T) {
	r := testFlowIO()

	ethernet := func(etherType uint16, payload []byte) []byte {
		frame := make([]byte, 14+len(payload))
		binary.BigEndian.PutUint16(frame[12:14], etherType)
		copy(frame[14:], payload)
		return frame
	}
	vlan := func(payload []byte) []byte {
		frame := make([]byte, 18+len(payload))
		binary.BigEndian.PutUint16(frame[12:14], 0x8100)
		binary.BigEndian.PutUint16(frame[16:18], 0x0800)
		copy(frame[18:], payload)
		return frame
	}

	udp := makeFlowPacket(17, r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort)
	icmp := makeFlowPacket(1, r.remoteIP, r.sourceIP, 0, 0)
	outboundSelf := makeFlowPacket(6, r.sourceIP, r.remoteIP, r.sourcePort, r.remotePort)
	wrongTCP := makeFlowPacket(6, r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort+17)
	exact := makeFlowPacket(6, r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort)
	exact[33] = 0x12 // SYN|ACK, representative handshake response.

	frames := [][]byte{
		make([]byte, 8),                              // truncated Ethernet noise
		ethernet(0x0806, make([]byte, 28)),          // ARP
		ethernet(0x86dd, make([]byte, 40)),          // IPv6
		ethernet(0x0800, udp),                       // unrelated IPv4/UDP
		vlan(icmp),                                  // VLAN IPv4/ICMP
		ethernet(0x0800, outboundSelf),              // Npcap self-capture of our own TX
		ethernet(0x0800, wrongTCP),                  // unrelated inbound TCP
		vlan(exact),                                 // first frame allowed to parser
	}

	accepted := 0
	for i, frame := range frames {
		packet, ok := ethernetIPv4Payload(frame)
		if !ok {
			continue
		}
		if !r.matchesInboundFlow(packet) {
			continue
		}
		accepted++
		if i != len(frames)-1 {
			t.Fatalf("noise frame %d reached FakeTCP parser", i)
		}
		if len(packet) != len(exact) {
			t.Fatalf("accepted packet length=%d want=%d", len(packet), len(exact))
		}
	}
	if accepted != 1 {
		t.Fatalf("accepted=%d want exactly one peer IPv4/TCP frame", accepted)
	}
}
