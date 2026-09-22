package faketcp

import (
	"bytes"
	"net"
	"testing"
)

func TestMarshalParseIPv4TCPBootstrapShape(t *testing.T) {
	src, _ := IPv4(net.ParseIP("10.0.0.1"))
	dst, _ := IPv4(net.ParseIP("10.0.0.2"))
	payload := []byte("tls-bootstrap")
	pkt := MarshalIPv4TCP(src, dst, 12345, 443, 1001, 9001, FlagACK|FlagPSH, 65535, payload, 7)

	seg, err := ParseIPv4TCP(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if seg.SrcPort != 12345 || seg.DstPort != 443 || seg.Seq != 1001 || seg.Ack != 9001 || seg.Flags != FlagACK|FlagPSH {
		t.Fatalf("bad segment: %#v", seg)
	}
	if !bytes.Equal(seg.Payload, payload) {
		t.Fatalf("payload=%q", seg.Payload)
	}
	if checksum(pkt[:20]) != 0 {
		t.Fatal("invalid IPv4 header checksum")
	}
	if tcpChecksum(src, dst, pkt[20:]) != 0 {
		t.Fatal("invalid TCP checksum")
	}
}

func TestSYNOptionsAndFingerprint(t *testing.T) {
	src, _ := IPv4(net.ParseIP("192.0.2.1"))
	dst, _ := IPv4(net.ParseIP("192.0.2.2"))
	pkt := MarshalIPv4TCP(src, dst, 40000, 443, 9, 0, FlagSYN, 65535, nil, 1)

	seg, err := ParseIPv4TCP(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if seg.Flags != FlagSYN || len(seg.Payload) != 0 {
		t.Fatalf("bad SYN %#v", seg)
	}
	if pkt[20+12]>>4 != 8 {
		t.Fatalf("expected 32-byte TCP header, doff=%d", pkt[32]>>4)
	}
	if !seg.MSSSet || seg.MSS != DefaultMSS || !seg.SACKPermitted ||
		!seg.WindowScaleSet || seg.WindowScale != DefaultWindowScale {
		t.Fatalf("parsed SYN options %#v", seg)
	}
	if !IsWBDHandshakeSegment(seg) {
		t.Fatalf("WBD SYN fingerprint not recognized: %#v", seg)
	}

	ordinary := Segment{
		Flags: FlagSYN, MSSSet: true, MSS: 1460,
		SACKPermitted: true, WindowScaleSet: true, WindowScale: 7,
	}
	if IsWBDHandshakeSegment(ordinary) {
		t.Fatal("ordinary kernel-like SYN misclassified as WBD")
	}
}

func TestWindows11PersonaSYNShape(t *testing.T) {
	src, _ := IPv4(net.ParseIP("192.0.2.10"))
	dst, _ := IPv4(net.ParseIP("198.51.100.20"))
	buf := make([]byte, 128)
	pkt := MarshalIPv4TCPPersonaInto(
		buf, src, dst, 41001, 443, 1234, 0,
		FlagSYN, 65535, nil, 9, PacketPersonaWindows11,
	)

	if got := pkt[8]; got != 128 {
		t.Fatalf("ttl=%d want 128", got)
	}
	if got := pkt[6] & 0x40; got == 0 {
		t.Fatal("DF bit is not set")
	}
	wantOptions := []byte{2, 4, 0x05, 0x50, 1, 3, 3, 8, 1, 1, 4, 2}
	if got := []byte(pkt[40:52]); !bytes.Equal(got, wantOptions) {
		t.Fatalf("SYN options=%v want=%v", got, wantOptions)
	}
	seg, err := ParseIPv4TCP(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if !IsWBDHandshakeSegment(seg) {
		t.Fatalf("Windows persona not recognized: %#v", seg)
	}
	if checksum(pkt[:20]) != 0 || tcpChecksum(src, dst, pkt[20:]) != 0 {
		t.Fatal("persona rewrite did not repair checksums")
	}
}

func TestLegacyPersonaIsByteIdenticalToBase(t *testing.T) {
	src, _ := IPv4(net.ParseIP("10.0.0.1"))
	dst, _ := IPv4(net.ParseIP("10.0.0.2"))
	wantBuf := make([]byte, 128)
	gotBuf := make([]byte, 128)
	want := append([]byte(nil), marshalIPv4TCPBaseInto(
		wantBuf, src, dst, 40000, 443, 1, 0,
		FlagSYN, 65535, nil, 5,
	)...)
	got := MarshalIPv4TCPPersonaInto(
		gotBuf, src, dst, 40000, 443, 1, 0,
		FlagSYN, 65535, nil, 5, PacketPersonaLegacy,
	)
	if !bytes.Equal(got, want) {
		t.Fatal("legacy persona changed mature packet bytes")
	}
}


func TestMarshalSegmentSACKRoundTrip(t *testing.T) {
	src, _ := IPv4(net.ParseIP("192.0.2.20"))
	dst, _ := IPv4(net.ParseIP("198.51.100.30"))
	seg := Segment{
		SrcIP: src, DstIP: dst,
		SrcPort: 42000, DstPort: 443,
		Seq: 7000, Ack: 9000, Flags: FlagACK, Window: 65535,
		SACKN: 2,
		SACK: [MaxSACKBlocks]SACKBlock{
			{Start: 9100, End: 9300},
			{Start: 9500, End: 9700},
		},
	}
	pkt := MarshalSegment(seg, 11, PacketPersonaLegacy)
	got, err := ParseIPv4TCP(pkt)
	if err != nil {
		t.Fatal(err)
	}
	if got.SACKN != 2 || got.SACK[0] != seg.SACK[0] || got.SACK[1] != seg.SACK[1] {
		t.Fatalf("SACK round-trip got=%+v want=%+v", got.SACK[:got.SACKN], seg.SACK[:seg.SACKN])
	}
	if tcpHeaderLen := int(pkt[20+12]>>4) * 4; tcpHeaderLen != 40 {
		t.Fatalf("TCP header len=%d want 40 with two SACK blocks", tcpHeaderLen)
	}
	if checksum(pkt[:20]) != 0 || tcpChecksum(src, dst, pkt[20:]) != 0 {
		t.Fatal("SACK packet checksum invalid")
	}
}
