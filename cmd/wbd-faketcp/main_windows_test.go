//go:build windows

package main

import "testing"

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
