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
	if pkt[20+12]>>4 != 7 { t.Fatalf("expected 28-byte TCP header, doff=%d", pkt[32]>>4) }
}
