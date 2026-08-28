//go:build windows

package main

import (
	"encoding/binary"
	"testing"
)

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

func TestMatchesKernelRSTExactFlow(t *testing.T) {
	r := &npcapRawPacketIO{
		sourceIP:   [4]byte{192, 168, 10, 11},
		remoteIP:   [4]byte{203, 0, 113, 7},
		sourcePort: 41001,
		remotePort: 443,
	}
	pkt := make([]byte, 40)
	pkt[0] = 0x45
	pkt[9] = 6
	copy(pkt[12:16], r.sourceIP[:])
	copy(pkt[16:20], r.remoteIP[:])
	binary.BigEndian.PutUint16(pkt[20:22], r.sourcePort)
	binary.BigEndian.PutUint16(pkt[22:24], r.remotePort)
	pkt[33] = 0x04 // RST
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
