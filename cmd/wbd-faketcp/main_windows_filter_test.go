//go:build windows

package main

import (
	"encoding/binary"
	"testing"
)

func makeWindowsFlowPacket(srcIP, dstIP [4]byte, srcPort, dstPort uint16, proto byte) []byte {
	pkt := make([]byte, 40)
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], uint16(len(pkt)))
	pkt[9] = proto
	copy(pkt[12:16], srcIP[:])
	copy(pkt[16:20], dstIP[:])
	binary.BigEndian.PutUint16(pkt[20:22], srcPort)
	binary.BigEndian.PutUint16(pkt[22:24], dstPort)
	pkt[32] = 5 << 4
	return pkt
}

func TestMatchesInboundFlowExactTuple(t *testing.T) {
	r := testFlowIO()
	in := makeWindowsFlowPacket(r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort, 6)
	if !r.matchesInboundFlow(in) {
		t.Fatal("exact inbound WBD TCP flow was rejected")
	}

	udp := append([]byte(nil), in...)
	udp[9] = 17
	if r.matchesInboundFlow(udp) {
		t.Fatal("unrelated IPv4/UDP traffic must not reach FakeTCP")
	}

	outbound := makeWindowsFlowPacket(r.sourceIP, r.remoteIP, r.sourcePort, r.remotePort, 6)
	if r.matchesInboundFlow(outbound) {
		t.Fatal("outbound injected WBD frame must not be returned as inbound")
	}

	wrongRemotePort := append([]byte(nil), in...)
	binary.BigEndian.PutUint16(wrongRemotePort[20:22], r.remotePort+1)
	if r.matchesInboundFlow(wrongRemotePort) {
		t.Fatal("different server port must not match the WBD flow")
	}

	wrongLocalPort := append([]byte(nil), in...)
	binary.BigEndian.PutUint16(wrongLocalPort[22:24], r.sourcePort+1)
	if r.matchesInboundFlow(wrongLocalPort) {
		t.Fatal("different local source port must not match the WBD flow")
	}

	wrongRemoteIP := append([]byte(nil), in...)
	wrongRemoteIP[15] ^= 1
	if r.matchesInboundFlow(wrongRemoteIP) {
		t.Fatal("different remote IP must not match the WBD flow")
	}

	tooShort := in[:20]
	if r.matchesInboundFlow(tooShort) {
		t.Fatal("truncated packet must not match the WBD flow")
	}
}
