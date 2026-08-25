package faketcp

import (
	"bytes"
	"net"
	"testing"
)

func TestMarshalParseIPv4TCP(t *testing.T) {
	src, _ := IPv4(net.ParseIP("10.0.0.1"))
	dst, _ := IPv4(net.ParseIP("10.0.0.2"))
	payload := []byte("hello-faketcp")
	pkt := MarshalIPv4TCP(src, dst, 12345, 443, 1000, 2000, FlagACK|FlagPSH, 65535, payload, 7)
	seg, err := ParseIPv4TCP(pkt)
	if err != nil { t.Fatal(err) }
	if seg.SrcPort != 12345 || seg.DstPort != 443 || seg.Seq != 1000 || seg.Ack != 2000 || seg.Flags != FlagACK|FlagPSH {
		t.Fatalf("bad segment: %#v", seg)
	}
	if !bytes.Equal(seg.Payload, payload) { t.Fatalf("payload=%q", seg.Payload) }
	if checksum(pkt[:20]) != 0 { t.Fatal("invalid IPv4 header checksum") }
	if tcpChecksum(src, dst, pkt[20:]) != 0 { t.Fatal("invalid TCP checksum") }
}

func TestSYNOptionsParsePayloadOffset(t *testing.T) {
	src, _ := IPv4(net.ParseIP("192.0.2.1"))
	dst, _ := IPv4(net.ParseIP("192.0.2.2"))
	pkt := MarshalIPv4TCP(src, dst, 40000, 443, 9, 0, FlagSYN, 64240, nil, 1)
	seg, err := ParseIPv4TCP(pkt)
	if err != nil { t.Fatal(err) }
	if seg.Flags != FlagSYN || len(seg.Payload) != 0 { t.Fatalf("bad SYN %#v", seg) }
	if pkt[20+12]>>4 != 8 { t.Fatalf("expected 32-byte TCP header, doff=%d", pkt[32]>>4) }
	if !seg.SACKPermitted { t.Fatal("SYN did not parse SACK-permitted") }
	if !seg.WindowScaleSet || seg.WindowScale != DefaultWindowScale {
		t.Fatalf("window scale parsed as set=%v value=%d", seg.WindowScaleSet, seg.WindowScale)
	}
}

func TestSACKRoundTrip(t *testing.T) {
	src, _ := IPv4(net.ParseIP("198.51.100.1"))
	dst, _ := IPv4(net.ParseIP("198.51.100.2"))
	want := []SACKBlock{{Start:1200, End:1400}, {Start:1600, End:1800}}
	buf := make([]byte, 256)
	pkt := MarshalIPv4TCPSACKInto(buf, src, dst, 2222, 3333, 900, 1000, FlagACK, 65535, want, nil, 9)
	seg, err := ParseIPv4TCP(pkt)
	if err != nil { t.Fatal(err) }
	if seg.SACKN != len(want) { t.Fatalf("SACKN=%d", seg.SACKN) }
	for i := range want {
		if seg.SACK[i] != want[i] { t.Fatalf("sack[%d]=%#v want %#v", i, seg.SACK[i], want[i]) }
	}
	if checksum(pkt[:20]) != 0 { t.Fatal("invalid IPv4 checksum with SACK") }
	if tcpChecksum(src, dst, pkt[20:]) != 0 { t.Fatal("invalid TCP checksum with SACK") }
}
