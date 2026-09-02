//go:build windows

package main

import (
	"encoding/binary"
	"testing"
)

func TestNpcapModeSendToRxClearLockedValue(t *testing.T) {
	if npcapModeSendToRxClear != 0x0200 {
		t.Fatalf("MODE_SENDTORX_CLEAR=%#x want=0x0200", npcapModeSendToRxClear)
	}
}

func TestEthernetIPv4Payload(t *testing.T) {
	ip := []byte{0x45, 0, 0, 20}
	frame := make([]byte, 14+len(ip))
	frame[12], frame[13] = 0x08, 0x00
	copy(frame[14:], ip)
	got, ok := ethernetIPv4Payload(frame)
	if !ok || string(got) != string(ip) {
		t.Fatalf("plain Ethernet IPv4 parse failed: ok=%v got=%x", ok, got)
	}

	vlan := make([]byte, 18+len(ip))
	vlan[12], vlan[13] = 0x81, 0x00
	vlan[16], vlan[17] = 0x08, 0x00
	copy(vlan[18:], ip)
	got, ok = ethernetIPv4Payload(vlan)
	if !ok || string(got) != string(ip) {
		t.Fatalf("VLAN Ethernet IPv4 parse failed: ok=%v got=%x", ok, got)
	}
}

func TestEthernetIPv4PayloadQinQAndNonIPv4Noise(t *testing.T) {
	ip := []byte{0x45, 0, 0, 20}
	qinq := make([]byte, 22+len(ip))
	binary.BigEndian.PutUint16(qinq[12:14], 0x88a8)
	binary.BigEndian.PutUint16(qinq[16:18], 0x8100)
	binary.BigEndian.PutUint16(qinq[20:22], 0x0800)
	copy(qinq[22:], ip)
	got, ok := ethernetIPv4Payload(qinq)
	if !ok || string(got) != string(ip) {
		t.Fatalf("QinQ Ethernet IPv4 parse failed: ok=%v got=%x", ok, got)
	}

	for _, etherType := range []uint16{0x0806, 0x86dd, 0x88cc} { // ARP, IPv6, LLDP.
		frame := make([]byte, 64)
		binary.BigEndian.PutUint16(frame[12:14], etherType)
		if _, ok := ethernetIPv4Payload(frame); ok {
			t.Fatalf("non-IPv4 EtherType %#x must be ignored", etherType)
		}
	}
	for n := 0; n < 22; n++ {
		frame := make([]byte, n)
		if n >= 14 {
			binary.BigEndian.PutUint16(frame[12:14], 0x88a8)
		}
		if _, ok := ethernetIPv4Payload(frame); ok {
			t.Fatalf("truncated Ethernet/VLAN frame len=%d must be ignored", n)
		}
	}
}

func TestParseEtherMAC(t *testing.T) {
	m, err := parseEtherMAC("02:11:22:33:44:55")
	if err != nil {
		t.Fatal(err)
	}
	if m != [6]byte{0x02, 0x11, 0x22, 0x33, 0x44, 0x55} {
		t.Fatalf("unexpected MAC %x", m)
	}
	if _, err := parseEtherMAC("bad"); err == nil {
		t.Fatal("expected invalid MAC rejection")
	}
}

func testFlowIO() *npcapRawPacketIO {
	return &npcapRawPacketIO{
		sourceIP:   [4]byte{192, 168, 10, 11},
		remoteIP:   [4]byte{203, 0, 113, 7},
		sourcePort: 41001,
		remotePort: 443,
	}
}

func makeFlowPacket(proto byte, srcIP, dstIP [4]byte, srcPort, dstPort uint16) []byte {
	pkt := make([]byte, 40)
	pkt[0] = 0x45
	binary.BigEndian.PutUint16(pkt[2:4], uint16(len(pkt)))
	pkt[9] = proto
	copy(pkt[12:16], srcIP[:])
	copy(pkt[16:20], dstIP[:])
	if proto == 6 {
		binary.BigEndian.PutUint16(pkt[20:22], srcPort)
		binary.BigEndian.PutUint16(pkt[22:24], dstPort)
		pkt[32] = 5 << 4
	}
	return pkt
}

func TestMatchesInboundFlowRejectsPhysicalAdapterNoise(t *testing.T) {
	r := testFlowIO()

	valid := makeFlowPacket(6, r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort)
	if !r.matchesInboundFlow(valid) {
		t.Fatal("exact inbound WBD TCP packet must pass")
	}

	outboundSelfCapture := makeFlowPacket(6, r.sourceIP, r.remoteIP, r.sourcePort, r.remotePort)
	if r.matchesInboundFlow(outboundSelfCapture) {
		t.Fatal("outbound self-capture must not enter the FakeTCP receive state machine")
	}

	udp := makeFlowPacket(17, r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort)
	if r.matchesInboundFlow(udp) {
		t.Fatal("IPv4 UDP adapter noise must be ignored during handshake")
	}

	icmp := makeFlowPacket(1, r.remoteIP, r.sourceIP, 0, 0)
	if r.matchesInboundFlow(icmp) {
		t.Fatal("IPv4 ICMP adapter noise must be ignored during handshake")
	}

	wrongPort := makeFlowPacket(6, r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort+1)
	if r.matchesInboundFlow(wrongPort) {
		t.Fatal("unrelated inbound TCP flow must be ignored")
	}

	wrongPeer := makeFlowPacket(6, [4]byte{203, 0, 113, 8}, r.sourceIP, r.remotePort, r.sourcePort)
	if r.matchesInboundFlow(wrongPeer) {
		t.Fatal("TCP from a different peer must be ignored")
	}

	fragment := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(fragment[6:8], 0x2000)
	if r.matchesInboundFlow(fragment) {
		t.Fatal("fragmented IPv4 packet must not enter FakeTCP parser")
	}

	badTotal := append([]byte(nil), valid...)
	binary.BigEndian.PutUint16(badTotal[2:4], 39)
	if r.matchesInboundFlow(badTotal) {
		t.Fatal("truncated IPv4/TCP packet must be rejected")
	}
}

func TestMatchesInboundFlowWithIPv4Options(t *testing.T) {
	r := testFlowIO()
	pkt := make([]byte, 44)
	pkt[0] = 0x46 // IPv4 with four bytes of options.
	binary.BigEndian.PutUint16(pkt[2:4], uint16(len(pkt)))
	pkt[9] = 6
	copy(pkt[12:16], r.remoteIP[:])
	copy(pkt[16:20], r.sourceIP[:])
	binary.BigEndian.PutUint16(pkt[24:26], r.remotePort)
	binary.BigEndian.PutUint16(pkt[26:28], r.sourcePort)
	pkt[36] = 5 << 4
	if !r.matchesInboundFlow(pkt) {
		t.Fatal("exact inbound WBD TCP packet with IPv4 options must pass")
	}

	badIHL := append([]byte(nil), pkt...)
	badIHL[0] = 0x4f
	if r.matchesInboundFlow(badIHL) {
		t.Fatal("IPv4 header length beyond captured packet must be rejected")
	}
}

func TestInboundFlowMutationCorpusRejectsNoise(t *testing.T) {
	r := testFlowIO()
	valid := makeFlowPacket(6, r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort)
	for i := 0; i < 4096; i++ {
		pkt := append([]byte(nil), valid...)
		switch i % 8 {
		case 0:
			pkt[12] ^= byte(1 + i%251)
		case 1:
			pkt[16] ^= byte(1 + i%251)
		case 2:
			pkt[9] = byte(1 + i%5)
		case 3:
			binary.BigEndian.PutUint16(pkt[20:22], r.remotePort+uint16(1+i%1000))
		case 4:
			binary.BigEndian.PutUint16(pkt[22:24], r.sourcePort+uint16(1+i%1000))
		case 5:
			binary.BigEndian.PutUint16(pkt[6:8], 0x2000|uint16(i&0x1fff))
		case 6:
			binary.BigEndian.PutUint16(pkt[2:4], uint16(i%40))
		case 7:
			pkt[0] = 0x60 // IPv6-shaped noise delivered after Ethernet extraction must still be rejected.
		}
		if r.matchesInboundFlow(pkt) {
			t.Fatalf("noise mutation %d unexpectedly matched exact inbound WBD flow", i)
		}
	}
	for n := 0; n < len(valid); n++ {
		if r.matchesInboundFlow(valid[:n]) {
			t.Fatalf("truncated exact packet len=%d unexpectedly matched", n)
		}
	}
	if !r.matchesInboundFlow(valid) {
		t.Fatal("mutation corpus guard damaged exact-flow acceptance")
	}
}

func TestVLANExactFlowPassesButVLANUDPNoiseDoesNot(t *testing.T) {
	r := testFlowIO()
	wrapVLAN := func(ip []byte) []byte {
		frame := make([]byte, 18+len(ip))
		frame[12], frame[13] = 0x81, 0x00
		frame[16], frame[17] = 0x08, 0x00
		copy(frame[18:], ip)
		return frame
	}

	valid := makeFlowPacket(6, r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort)
	ip, ok := ethernetIPv4Payload(wrapVLAN(valid))
	if !ok || !r.matchesInboundFlow(ip) {
		t.Fatal("VLAN-encapsulated exact WBD TCP flow must pass")
	}

	udp := makeFlowPacket(17, r.remoteIP, r.sourceIP, r.remotePort, r.sourcePort)
	ip, ok = ethernetIPv4Payload(wrapVLAN(udp))
	if !ok {
		t.Fatal("VLAN IPv4 extraction should succeed before protocol filtering")
	}
	if r.matchesInboundFlow(ip) {
		t.Fatal("VLAN IPv4 UDP noise must be rejected")
	}
}

func TestMatchesKernelRSTExactFlow(t *testing.T) {
	r := testFlowIO()
	pkt := make([]byte, 40)
	pkt[0] = 0x45
	pkt[9] = 6
	copy(pkt[12:16], r.sourceIP[:])
	copy(pkt[16:20], r.remoteIP[:])
	binary.BigEndian.PutUint16(pkt[20:22], r.sourcePort)
	binary.BigEndian.PutUint16(pkt[22:24], r.remotePort)
	pkt[33] = 0x04
	if !r.matchesKernelRST(pkt) {
		t.Fatal("exact outbound WBD RST was not detected")
	}

	ack := append([]byte(nil), pkt...)
	ack[33] = 0x10
	if r.matchesKernelRST(ack) {
		t.Fatal("ordinary ACK must not be reported as a kernel RST")
	}

	wrongPort := append([]byte(nil), pkt...)
	binary.BigEndian.PutUint16(wrongPort[20:22], r.sourcePort+1)
	if r.matchesKernelRST(wrongPort) {
		t.Fatal("RST from a different source port must not match the WBD flow")
	}

	inbound := append([]byte(nil), pkt...)
	copy(inbound[12:16], r.remoteIP[:])
	copy(inbound[16:20], r.sourceIP[:])
	binary.BigEndian.PutUint16(inbound[20:22], r.remotePort)
	binary.BigEndian.PutUint16(inbound[22:24], r.sourcePort)
	if r.matchesKernelRST(inbound) {
		t.Fatal("inbound peer RST must not be reported as a local kernel RST")
	}
}

func TestFlowPayloadBytesExactDirection(t *testing.T) {
	r := testFlowIO()
	out := make([]byte, 45)
	out[0] = 0x45
	binary.BigEndian.PutUint16(out[2:4], uint16(len(out)))
	out[9] = 6
	copy(out[12:16], r.sourceIP[:])
	copy(out[16:20], r.remoteIP[:])
	binary.BigEndian.PutUint16(out[20:22], r.sourcePort)
	binary.BigEndian.PutUint16(out[22:24], r.remotePort)
	out[32] = 5 << 4
	copy(out[40:], []byte("hello"))
	if n, ok := r.flowPayloadBytes(out, true); !ok || n != 5 {
		t.Fatalf("outbound payload boundary mismatch n=%d ok=%v", n, ok)
	}
	if _, ok := r.flowPayloadBytes(out, false); ok {
		t.Fatal("outbound packet must not match inbound direction")
	}

	in := append([]byte(nil), out...)
	copy(in[12:16], r.remoteIP[:])
	copy(in[16:20], r.sourceIP[:])
	binary.BigEndian.PutUint16(in[20:22], r.remotePort)
	binary.BigEndian.PutUint16(in[22:24], r.sourcePort)
	if n, ok := r.flowPayloadBytes(in, false); !ok || n != 5 {
		t.Fatalf("inbound payload boundary mismatch n=%d ok=%v", n, ok)
	}

	ackOnly := append([]byte(nil), out[:40]...)
	binary.BigEndian.PutUint16(ackOnly[2:4], uint16(len(ackOnly)))
	if _, ok := r.flowPayloadBytes(ackOnly, true); ok {
		t.Fatal("ACK-only packet must not be reported as payload")
	}

	wrongPort := append([]byte(nil), out...)
	binary.BigEndian.PutUint16(wrongPort[20:22], r.sourcePort+1)
	if _, ok := r.flowPayloadBytes(wrongPort, true); ok {
		t.Fatal("different four-tuple must not match payload boundary")
	}
}

func FuzzNpcapIngressClassifierNeverPanics(f *testing.F) {
	f.Add([]byte{})
	f.Add(make([]byte, 13))
	f.Add(make([]byte, 64))
	f.Fuzz(func(t *testing.T, frame []byte) {
		r := testFlowIO()
		packet, ok := ethernetIPv4Payload(frame)
		if !ok {
			return
		}
		_ = r.matchesInboundFlow(packet)
		_ = r.matchesKernelRST(packet)
		_, _ = r.flowPayloadBytes(packet, false)
		_, _ = r.flowPayloadBytes(packet, true)
	})
}
