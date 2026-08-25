package dataplane

import (
	"encoding/binary"
	"errors"
	"testing"
)

func ipv4Packet(payload []byte) []byte {
	p := make([]byte, 20+len(payload))
	p[0] = 0x45
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)))
	p[8] = 64
	p[9] = 17
	copy(p[20:], payload)
	return p
}

func ipv6Packet(payload []byte) []byte {
	p := make([]byte, 40+len(payload))
	p[0] = 0x60
	binary.BigEndian.PutUint16(p[4:6], uint16(len(payload)))
	p[6] = 17
	p[7] = 64
	copy(p[40:], payload)
	return p
}

func TestRoundTripIPv4AndIPv6(t *testing.T) {
	for _, packet := range [][]byte{
		ipv4Packet([]byte("hello-v4")),
		ipv6Packet([]byte("hello-v6")),
	} {
		wire, err := MarshalIP(packet)
		if err != nil {
			t.Fatal(err)
		}
		got, err := UnmarshalIP(wire)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(packet) {
			t.Fatalf("round trip mismatch")
		}
	}
}

func TestRejectTrailingAndLengthMismatch(t *testing.T) {
	wire, err := MarshalIP(ipv4Packet(nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := UnmarshalIP(append(wire, 0)); !errors.Is(err, ErrMalformed) {
		t.Fatalf("expected malformed trailing byte, got %v", err)
	}

	p := ipv4Packet(nil)
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)+1))
	if _, err := MarshalIP(p); !errors.Is(err, ErrMalformed) {
		t.Fatalf("expected malformed IPv4 length, got %v", err)
	}
}

func TestRejectUnknownVersionAndOversize(t *testing.T) {
	p := ipv4Packet(nil)
	p[0] = 0x70
	if _, err := MarshalIP(p); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("expected unsupported IP version, got %v", err)
	}

	p = make([]byte, MaxPacketLen+1)
	p[0] = 0x45
	if _, err := MarshalIP(p); !errors.Is(err, ErrLimit) {
		t.Fatalf("expected limit, got %v", err)
	}
}
