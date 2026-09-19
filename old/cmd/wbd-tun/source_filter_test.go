package main

import (
	"bytes"
	"io"
	"net/netip"
	"testing"
)

type packetQueueEndpoint struct {
	packets [][]byte
	writes  [][]byte
	closed  bool
}

func (e *packetQueueEndpoint) ReadPacket(p []byte) (int, error) {
	if len(e.packets) == 0 {
		return 0, io.EOF
	}
	packet := e.packets[0]
	e.packets = e.packets[1:]
	return copy(p, packet), nil
}
func (e *packetQueueEndpoint) WritePacket(p []byte) (int, error) {
	e.writes = append(e.writes, append([]byte(nil), p...))
	return len(p), nil
}
func (e *packetQueueEndpoint) Close() error { e.closed = true; return nil }

func ipv4Packet(source [4]byte) []byte {
	p := make([]byte, 20)
	p[0] = 0x45
	copy(p[12:16], source[:])
	p[16], p[17], p[18], p[19] = 1, 1, 1, 1
	return p
}

func TestSourceIPv4EndpointDropsNonLeaseUntilExpected(t *testing.T) {
	raw := &packetQueueEndpoint{packets: [][]byte{
		ipv4Packet([4]byte{169, 254, 104, 89}),
		{0x60, 0, 0, 0, 0, 0, 0, 0},
		ipv4Packet([4]byte{10, 66, 0, 1}),
	}}
	filtered, err := newSourceIPv4Endpoint(raw, netip.MustParseAddr("10.66.0.1"))
	if err != nil { t.Fatal(err) }
	buf := make([]byte, 1500)
	n, err := filtered.ReadPacket(buf)
	if err != nil { t.Fatal(err) }
	want := ipv4Packet([4]byte{10, 66, 0, 1})
	if !bytes.Equal(buf[:n], want) { t.Fatalf("got %v want %v", buf[:n], want) }
	if len(raw.packets) != 0 { t.Fatalf("packets remaining=%d", len(raw.packets)) }
}

func TestSourceIPv4EndpointInboundWriteUnchanged(t *testing.T) {
	raw := &packetQueueEndpoint{}
	filtered, err := newSourceIPv4Endpoint(raw, netip.MustParseAddr("10.66.0.1"))
	if err != nil { t.Fatal(err) }
	packet := ipv4Packet([4]byte{8, 8, 8, 8})
	n, err := filtered.WritePacket(packet)
	if err != nil { t.Fatal(err) }
	if n != len(packet) || len(raw.writes) != 1 || !bytes.Equal(raw.writes[0], packet) { t.Fatalf("write not forwarded unchanged") }
}

func TestSourceIPv4EndpointRequiresIPv4(t *testing.T) {
	raw := &packetQueueEndpoint{}
	if _, err := newSourceIPv4Endpoint(raw, netip.MustParseAddr("::1")); err == nil { t.Fatal("expected IPv6 source rejection") }
}
