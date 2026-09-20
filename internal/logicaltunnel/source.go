package logicaltunnel

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
)

const MaxLeasedIPv4PacketLen = 9000

var (
	ErrSourceSpoof       = errors.New("logicaltunnel: IPv4 source does not match lease")
	ErrInvalidIPv4Packet = errors.New("logicaltunnel: invalid IPv4 packet")
)

// ValidateIPv4Source is the transport-independent lease fence for inner
// business packets. It accepts exactly one well-formed IPv4 packet whose source
// equals the Logical Tunnel /32 lease. IPv6 and another installation's source
// fail closed; no platform/TUN state is consulted here.
func ValidateIPv4Source(packet []byte, leased netip.Addr) error {
	leased = leased.Unmap()
	if !leased.IsValid() || !leased.Is4() {
		return ErrSourceSpoof
	}
	if len(packet) < 20 {
		return fmt.Errorf("%w: length=%d", ErrInvalidIPv4Packet, len(packet))
	}
	if len(packet) > MaxLeasedIPv4PacketLen {
		return fmt.Errorf("%w: length=%d limit=%d", ErrInvalidIPv4Packet, len(packet), MaxLeasedIPv4PacketLen)
	}
	if packet[0]>>4 != 4 {
		return ErrSourceSpoof
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || ihl > len(packet) {
		return fmt.Errorf("%w: header_length=%d packet_length=%d", ErrInvalidIPv4Packet, ihl, len(packet))
	}
	total := int(binary.BigEndian.Uint16(packet[2:4]))
	if total != len(packet) {
		return fmt.Errorf("%w: total_length=%d packet_length=%d", ErrInvalidIPv4Packet, total, len(packet))
	}
	var raw [4]byte
	copy(raw[:], packet[12:16])
	if netip.AddrFrom4(raw) != leased {
		return ErrSourceSpoof
	}
	return nil
}
