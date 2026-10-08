package platformflow

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
)

func TestMarshalPacketSingleOwnedAllocationExactLegacyWire(t *testing.T) {
	lease := netip.MustParseAddr("10.23.7.8")
	peer := netip.MustParseAddrPort("192.0.2.7:443")
	for _, tc := range []struct {
		name string
		frame Frame
	}{
		{"udp", Frame{Kind: KindUDPDatagram, FlowID: 1, Peer: peer, Payload: []byte("udp-live")}},
		{"tcp-open", Frame{Kind: KindTCPOpen, FlowID: 2, Peer: peer}},
		{"tcp-data", Frame{Kind: KindTCPData, FlowID: 3, Offset: 7, Payload: []byte("tcp-owned-retry")}},
		{"tcp-fin", Frame{Kind: KindTCPData, FlowID: 4, Offset: 88, FIN: true}},
		{"tcp-ack", Frame{Kind: KindTCPAck, FlowID: 5, Offset: 501}},
		{"tcp-close", Frame{Kind: KindTCPClose, FlowID: 6}},
		{"max-payload", Frame{Kind: KindTCPData, FlowID: 7, Payload: bytes.Repeat([]byte{0xa5}, MaxPayload)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			frameWire, err := MarshalFrame(tc.frame)
			if err != nil { t.Fatalf("legacy frame marshal: %v", err) }
			want := make([]byte, IPv4HeaderSize+len(frameWire))
			want[0] = 0x45
			binary.BigEndian.PutUint16(want[2:4], uint16(len(want)))
			binary.BigEndian.PutUint16(want[6:8], 0x4000)
			want[8], want[9] = 64, ServiceProtocol
			ip := lease.As4()
			copy(want[12:16], ip[:])
			copy(want[16:20], ip[:])
			binary.BigEndian.PutUint16(want[10:12], ipv4Checksum(want[:IPv4HeaderSize]))
			copy(want[IPv4HeaderSize:], frameWire)

			got, err := MarshalPacket(lease, tc.frame)
			if err != nil { t.Fatalf("marshal: %v", err) }
			if !bytes.Equal(got, want) { t.Fatalf("wire mismatch: got %d bytes, want %d bytes", len(got), len(want)) }
			parsed, handled, err := ParsePacket(got, lease)
			if err != nil || !handled { t.Fatalf("roundtrip handled=%t err=%v", handled, err) }
			if parsed.Kind != tc.frame.Kind || parsed.FlowID != tc.frame.FlowID ||
				parsed.Offset != tc.frame.Offset || parsed.FIN != tc.frame.FIN ||
				parsed.Peer != tc.frame.Peer || !bytes.Equal(parsed.Payload, tc.frame.Payload) {
				t.Fatalf("roundtrip frame mismatch: %+v", parsed)
			}
			if len(tc.frame.Payload) > 0 {
				// The returned IPv4 packet must own its bytes, even after the
				// source frame buffer is reused by TCP or a Game lane.
				original := got[IPv4HeaderSize+FrameHeaderSize]
				tc.frame.Payload[0] ^= 0xff
				if got[IPv4HeaderSize+FrameHeaderSize] != original {
					t.Fatal("packet aliases source payload")
				}
			}
		})
	}
}

func TestMarshalPacketValidationPreserved(t *testing.T) {
	lease := netip.MustParseAddr("10.23.7.8")
	peer := netip.MustParseAddrPort("192.0.2.7:443")
	tests := []struct { name string; lease netip.Addr; frame Frame; want error }{
		{"bad lease", netip.MustParseAddr("2001:db8::1"), Frame{Kind: KindTCPAck, FlowID: 1}, errors.New("unused")},
		{"zero flow", lease, Frame{Kind: KindTCPAck}, ErrMalformed},
		{"oversize", lease, Frame{Kind: KindTCPData, FlowID: 1, Payload: make([]byte, MaxPayload+1)}, ErrLimit},
		{"udp invalid fin", lease, Frame{Kind: KindUDPDatagram, FlowID: 1, Peer: peer, FIN: true, Payload: []byte{1}}, ErrMalformed},
		{"no target", lease, Frame{Kind: KindTCPOpen, FlowID: 1}, ErrMalformed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := MarshalPacket(tt.lease, tt.frame)
			if err == nil { t.Fatal("invalid service frame accepted") }
			if tt.name != "bad lease" && !errors.Is(err, tt.want) { t.Fatalf("got %v; want %v", err, tt.want) }
			if tt.name == "bad lease" {
				_, refErr := MarshalFrame(tt.frame)
				if refErr != nil { t.Fatalf("valid frame rejected by MarshalFrame: %v", refErr) }
			}
		})
	}
}

// This guards the E2 objective: the old implementation allocated one frame
// plus one IPv4 packet. The exact same on-wire result now owns one buffer.
func TestMarshalPacketOneAllocation(t *testing.T) {
	lease := netip.MustParseAddr("10.23.7.8")
	frame := Frame{Kind: KindTCPData, FlowID: 77, Offset: 5, Payload: bytes.Repeat([]byte{0xab}, 1200)}
	allocs := testing.AllocsPerRun(200, func() {
		packet, err := MarshalPacket(lease, frame)
		if err != nil || len(packet) != IPv4HeaderSize+FrameHeaderSize+1200 {
			panic("bad packet")
		}
	})
	if allocs > 1 {
		t.Fatalf("expected exactly one owned packet allocation; got %.2f", allocs)
	}
}
