package faketcp

import (
	"bytes"
	"net"
	"testing"
)

func TestWindows11PersonaSYNShape(t *testing.T) {
	src, _ := IPv4(net.ParseIP("192.0.2.10"))
	dst, _ := IPv4(net.ParseIP("198.51.100.20"))
	buf := make([]byte, 128)
	pkt := MarshalIPv4TCPSACKPersonaInto(buf, src, dst, 41001, 443, 1234, 0, FlagSYN, 65535, nil, nil, 9, PacketPersonaWindows11)

	if got := pkt[8]; got != 128 { t.Fatalf("ttl=%d want 128", got) }
	if got := pkt[6] & 0x40; got == 0 { t.Fatal("DF bit is not set") }
	if got := []byte(pkt[40:52]); !bytes.Equal(got, []byte{2,4,0x05,0x50,1,3,3,8,1,1,4,2}) {
		t.Fatalf("SYN options=%v want MSS,NOP,WS8,NOP,NOP,SACK", got)
	}
	seg, err := ParseIPv4TCP(pkt)
	if err != nil { t.Fatal(err) }
	if seg.Window != 65535 || !seg.MSSSet || seg.MSS != DefaultMSS || !seg.WindowScaleSet || seg.WindowScale != 8 || !seg.SACKPermitted {
		t.Fatalf("parsed Windows persona %#v", seg)
	}
	if !IsWBDHandshakeSegment(seg) { t.Fatal("Windows option ordering must remain a valid WBD handshake") }
	if checksum(pkt[:20]) != 0 { t.Fatal("invalid IPv4 checksum") }
	if tcpChecksum(src, dst, pkt[20:]) != 0 { t.Fatal("invalid TCP checksum") }
}

func TestWindows11PersonaKeepsTTLAcrossDataPhase(t *testing.T) {
	src, _ := IPv4(net.ParseIP("192.0.2.10"))
	dst, _ := IPv4(net.ParseIP("198.51.100.20"))
	payload := []byte("post-bootstrap-dtls-datagram")
	buf := make([]byte, 256)
	pkt := MarshalIPv4TCPSACKPersonaInto(buf, src, dst, 41001, 443, 1235, 9000, FlagACK|FlagPSH, 65535, nil, payload, 10, PacketPersonaWindows11)
	if pkt[8] != 128 { t.Fatalf("data ttl=%d want 128", pkt[8]) }
	seg, err := ParseIPv4TCP(pkt)
	if err != nil { t.Fatal(err) }
	if !bytes.Equal(seg.Payload, payload) { t.Fatalf("payload=%q", seg.Payload) }
	if checksum(pkt[:20]) != 0 { t.Fatal("invalid IPv4 checksum") }
	if tcpChecksum(src, dst, pkt[20:]) != 0 { t.Fatal("invalid TCP checksum") }
}

func TestLegacyPersonaIsByteIdentical(t *testing.T) {
	src, _ := IPv4(net.ParseIP("10.0.0.1"))
	dst, _ := IPv4(net.ParseIP("10.0.0.2"))
	wantBuf := make([]byte, 128)
	gotBuf := make([]byte, 128)
	want := append([]byte(nil), marshalIPv4TCPSACKBaseInto(wantBuf, src, dst, 40000, 443, 1, 0, FlagSYN, 65535, nil, nil, 5)...)
	got := MarshalIPv4TCPSACKPersonaInto(gotBuf, src, dst, 40000, 443, 1, 0, FlagSYN, 65535, nil, nil, 5, PacketPersonaLegacy)
	if !bytes.Equal(got, want) { t.Fatal("legacy persona changed existing packet bytes") }
}
