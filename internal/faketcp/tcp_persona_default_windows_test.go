//go:build windows

package faketcp

import (
	"bytes"
	"net"
	"testing"
)

func TestWindowsDefaultPacketPersona(t *testing.T) {
	if DefaultPacketPersona != PacketPersonaWindows11 {
		t.Fatalf("default persona=%s want windows11", DefaultPacketPersona)
	}
	src, _ := IPv4(net.ParseIP("192.0.2.10"))
	dst, _ := IPv4(net.ParseIP("198.51.100.20"))
	buf := make([]byte, 128)
	pkt := MarshalIPv4TCPSACKInto(buf, src, dst, 41001, 443, 100, 0, FlagSYN, 65535, nil, nil, 7)
	if pkt[8] != 128 { t.Fatalf("default ttl=%d want 128", pkt[8]) }
	want := []byte{2,4,0x05,0x50,1,3,3,8,1,1,4,2}
	if !bytes.Equal(pkt[40:52], want) { t.Fatalf("default options=%v want %v", pkt[40:52], want) }
	if checksum(pkt[:20]) != 0 || tcpChecksum(src, dst, pkt[20:]) != 0 { t.Fatal("default Windows persona checksum invalid") }
}
