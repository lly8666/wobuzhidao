package logicaltunnel

import (
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
)

func leasedIPv4Packet(src, dst netip.Addr, payload []byte) []byte {
	packet := make([]byte, 20+len(payload))
	packet[0] = 0x45
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(packet)))
	s := src.As4()
	d := dst.As4()
	copy(packet[12:16], s[:])
	copy(packet[16:20], d[:])
	copy(packet[20:], payload)
	return packet
}

func TestValidateIPv4SourceAcceptsExactLease(t *testing.T) {
	lease := netip.MustParseAddr("10.66.0.17")
	packet := leasedIPv4Packet(lease, netip.MustParseAddr("1.1.1.1"), []byte("business"))
	if err := ValidateIPv4Source(packet, lease); err != nil {
		t.Fatal(err)
	}
	if err := ValidateIPv4Source(packet, netip.MustParseAddr("::ffff:10.66.0.17")); err != nil {
		t.Fatalf("IPv4-mapped lease rejected: %v", err)
	}
}

func TestValidateIPv4SourceRejectsOtherLeaseAndNonIPv4(t *testing.T) {
	packet := leasedIPv4Packet(
		netip.MustParseAddr("10.66.0.18"),
		netip.MustParseAddr("1.1.1.1"),
		nil,
	)
	if err := ValidateIPv4Source(packet, netip.MustParseAddr("10.66.0.17")); !errors.Is(err, ErrSourceSpoof) {
		t.Fatalf("other lease source err=%v", err)
	}

	ipv6 := make([]byte, 40)
	ipv6[0] = 0x60
	if err := ValidateIPv4Source(ipv6, netip.MustParseAddr("10.66.0.17")); !errors.Is(err, ErrSourceSpoof) {
		t.Fatalf("IPv6 err=%v", err)
	}
	if err := ValidateIPv4Source(packet, netip.MustParseAddr("2001:db8::1")); !errors.Is(err, ErrSourceSpoof) {
		t.Fatalf("non-IPv4 lease err=%v", err)
	}
}

func TestValidateIPv4SourceRejectsMalformedPacket(t *testing.T) {
	lease := netip.MustParseAddr("10.66.0.17")
	for name, packet := range map[string][]byte{
		"short":       make([]byte, 19),
		"bad-ihl":     append([]byte{0x44}, make([]byte, 19)...),
		"length-zero": append([]byte{0x45}, make([]byte, 19)...),
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateIPv4Source(packet, lease); !errors.Is(err, ErrInvalidIPv4Packet) {
				t.Fatalf("err=%v", err)
			}
		})
	}

	oversize := make([]byte, MaxLeasedIPv4PacketLen+1)
	oversize[0] = 0x45
	if err := ValidateIPv4Source(oversize, lease); !errors.Is(err, ErrInvalidIPv4Packet) {
		t.Fatalf("oversize err=%v", err)
	}
}
