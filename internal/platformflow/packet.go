package platformflow

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

const ServiceProtocol byte = 253

func MarshalPacket(lease netip.Addr, frame Frame) ([]byte, error) {
	lease = lease.Unmap()
	if !lease.IsValid() || !lease.Is4() {
		return nil, logicaltunnel.ErrSourceSpoof
	}
	wire, err := MarshalFrame(frame)
	if err != nil {
		return nil, err
	}
	total := IPv4HeaderSize + len(wire)
	if total > logicaltunnel.MaxLeasedIPv4PacketLen {
		return nil, fmt.Errorf("%w: service packet=%d", ErrLimit, total)
	}
	out := make([]byte, total)
	out[0] = 0x45
	binary.BigEndian.PutUint16(out[2:4], uint16(total))
	binary.BigEndian.PutUint16(out[6:8], 0x4000)
	out[8] = 64
	out[9] = ServiceProtocol
	ip := lease.As4()
	copy(out[12:16], ip[:])
	copy(out[16:20], ip[:])
	binary.BigEndian.PutUint16(out[10:12], ipv4Checksum(out[:20]))
	copy(out[20:], wire)
	return out, nil
}

// ParsePacket returns handled=false only when the packet is not reserved service
// traffic. Once IPv4 protocol 253 is visible, malformed data is handled=true
// with an error so callers cannot accidentally forward it to the shared TUN.
func ParsePacket(packet []byte, lease netip.Addr) (frame Frame, handled bool, err error) {
	lease = lease.Unmap()
	if !lease.IsValid() || !lease.Is4() {
		return Frame{}, false, logicaltunnel.ErrSourceSpoof
	}
	if len(packet) < 10 || packet[0]>>4 != 4 {
		return Frame{}, false, nil
	}
	if packet[9] != ServiceProtocol {
		return Frame{}, false, nil
	}
	handled = true
	if len(packet) < IPv4HeaderSize {
		return Frame{}, true, fmt.Errorf("%w: short service IPv4", ErrMalformed)
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl != IPv4HeaderSize || ihl > len(packet) ||
		int(binary.BigEndian.Uint16(packet[2:4])) != len(packet) ||
		len(packet) > logicaltunnel.MaxLeasedIPv4PacketLen {
		return Frame{}, true, fmt.Errorf("%w: service IPv4 shape", ErrMalformed)
	}
	if binary.BigEndian.Uint16(packet[6:8])&0x3fff != 0 {
		return Frame{}, true, fmt.Errorf("%w: fragmented service packet", ErrMalformed)
	}
	if ipv4Checksum(packet[:20]) != 0 {
		return Frame{}, true, fmt.Errorf("%w: IPv4 checksum", ErrMalformed)
	}
	want := lease.As4()
	var src, dst [4]byte
	copy(src[:], packet[12:16])
	copy(dst[:], packet[16:20])
	if src != want || dst != want {
		return Frame{}, true, fmt.Errorf("%w: service lease endpoint", ErrMalformed)
	}
	frame, err = ParseFrame(packet[20:])
	if err != nil {
		return Frame{}, true, err
	}
	return frame, true, nil
}

func ipv4Checksum(header []byte) uint16 {
	var sum uint32
	for i := 0; i+1 < len(header); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(header[i : i+2]))
	}
	if len(header)%2 != 0 {
		sum += uint32(header[len(header)-1]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}
