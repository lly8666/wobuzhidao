//go:build !windows

package faketcp

import (
	"bytes"
	"net"
	"testing"
)

func TestNonWindowsDefaultPacketPersonaIsLegacy(t *testing.T) {
	if DefaultPacketPersona != PacketPersonaLegacy {
		t.Fatalf("default persona=%s want legacy", DefaultPacketPersona)
	}
	src, _ := IPv4(net.ParseIP("10.0.0.1"))
	dst, _ := IPv4(net.ParseIP("10.0.0.2"))
	wantBuf := make([]byte, 256)
	gotBuf := make([]byte, 256)
	payload := []byte("steady-state-byte-freeze")
	want := append([]byte(nil), marshalIPv4TCPSACKBaseInto(wantBuf, src, dst, 40000, 443, 100, 200, FlagACK|FlagPSH, 65535, nil, payload, 17)...)
	got := MarshalIPv4TCPSACKInto(gotBuf, src, dst, 40000, 443, 100, 200, FlagACK|FlagPSH, 65535, nil, payload, 17)
	if !bytes.Equal(got, want) {
		t.Fatal("non-Windows default packet path diverged from mature legacy bytes")
	}
}
