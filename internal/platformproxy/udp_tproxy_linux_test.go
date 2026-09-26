//go:build linux

package platformproxy

import (
	"encoding/binary"
	"net/netip"
	"testing"
)

func TestDecodeOrigDst4(t *testing.T) {
	raw := make([]byte, 16)
	binary.BigEndian.PutUint16(raw[2:4], 5353)
	copy(raw[4:8], []byte{198, 51, 100, 7})
	got, err := decodeOrigDst4(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := netip.MustParseAddrPort("198.51.100.7:5353")
	if got != want {
		t.Fatalf("got=%s want=%s", got, want)
	}
}

func TestDecodeOrigDst6(t *testing.T) {
	raw := make([]byte, 28)
	binary.BigEndian.PutUint16(raw[2:4], 443)
	addr := netip.MustParseAddr("2001:db8::7").As16()
	copy(raw[8:24], addr[:])
	got, err := decodeOrigDst6(raw)
	if err != nil {
		t.Fatal(err)
	}
	want := netip.MustParseAddrPort("[2001:db8::7]:443")
	if got != want {
		t.Fatalf("got=%s want=%s", got, want)
	}
}

func TestDecodeOrigDstRejectsShortOrUnspecified(t *testing.T) {
	if _, err := decodeOrigDst4(make([]byte, 7)); err == nil {
		t.Fatal("accepted short IPv4 sockaddr")
	}
	raw := make([]byte, 16)
	binary.BigEndian.PutUint16(raw[2:4], 53)
	if _, err := decodeOrigDst4(raw); err == nil {
		t.Fatal("accepted unspecified IPv4 destination")
	}
	if _, err := decodeOrigDst6(make([]byte, 23)); err == nil {
		t.Fatal("accepted short IPv6 sockaddr")
	}
}
