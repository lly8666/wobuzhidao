package faketcp

import (
	"encoding/binary"
	"errors"
	"net"
)

const (
	FlagFIN = 0x01
	FlagSYN = 0x02
	FlagRST = 0x04
	FlagPSH = 0x08
	FlagACK = 0x10
)

var (
	ErrShortPacket  = errors.New("faketcp: short packet")
	ErrNotIPv4TCP  = errors.New("faketcp: not ipv4/tcp")
	ErrBadTCPHeader = errors.New("faketcp: invalid tcp header")
)

type Segment struct {
	SrcIP   [4]byte
	DstIP   [4]byte
	SrcPort uint16
	DstPort uint16
	Seq     uint32
	Ack     uint32
	Flags   uint8
	Window  uint16
	Payload []byte
}

func IPv4(ip net.IP) ([4]byte, bool) {
	var out [4]byte
	v := ip.To4()
	if v == nil {
		return out, false
	}
	copy(out[:], v)
	return out, true
}

// MarshalIPv4TCP constructs one IPv4/TCP packet. The raw carrier deliberately
// keeps options minimal on the steady-state path: SYN advertises MSS and SACK
// permitted, while data/ACK packets use a 20-byte TCP header. This avoids the
// allocation-heavy generic packet builders in the hot path.
func MarshalIPv4TCP(srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, payload []byte, ipID uint16) []byte {
	optLen := 0
	if flags&FlagSYN != 0 {
		// MSS 1360 + SACK permitted + 2 NOPs => 8 bytes.
		optLen = 8
	}
	tcpLen := 20 + optLen + len(payload)
	buf := make([]byte, 20+tcpLen)

	ip := buf[:20]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(buf)))
	binary.BigEndian.PutUint16(ip[4:6], ipID)
	binary.BigEndian.PutUint16(ip[6:8], 0x4000) // DF
	ip[8] = 64
	ip[9] = 6
	copy(ip[12:16], srcIP[:])
	copy(ip[16:20], dstIP[:])
	binary.BigEndian.PutUint16(ip[10:12], checksum(ip))

	tcp := buf[20:]
	binary.BigEndian.PutUint16(tcp[0:2], srcPort)
	binary.BigEndian.PutUint16(tcp[2:4], dstPort)
	binary.BigEndian.PutUint32(tcp[4:8], seq)
	binary.BigEndian.PutUint32(tcp[8:12], ack)
	tcp[12] = byte((20 + optLen) / 4 << 4)
	tcp[13] = flags
	binary.BigEndian.PutUint16(tcp[14:16], window)
	if optLen != 0 {
		o := tcp[20:28]
		o[0], o[1] = 2, 4
		binary.BigEndian.PutUint16(o[2:4], 1360)
		o[4], o[5], o[6], o[7] = 4, 2, 1, 1
	}
	copy(tcp[20+optLen:], payload)
	binary.BigEndian.PutUint16(tcp[16:18], tcpChecksum(srcIP, dstIP, tcp))
	return buf
}

func ParseIPv4TCP(packet []byte) (Segment, error) {
	var s Segment
	if len(packet) < 40 {
		return s, ErrShortPacket
	}
	if packet[0]>>4 != 4 || packet[9] != 6 {
		return s, ErrNotIPv4TCP
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+20 {
		return s, ErrShortPacket
	}
	total := int(binary.BigEndian.Uint16(packet[2:4]))
	if total == 0 || total > len(packet) {
		total = len(packet)
	}
	tcp := packet[ihl:total]
	doff := int(tcp[12]>>4) * 4
	if doff < 20 || doff > len(tcp) {
		return s, ErrBadTCPHeader
	}
	copy(s.SrcIP[:], packet[12:16])
	copy(s.DstIP[:], packet[16:20])
	s.SrcPort = binary.BigEndian.Uint16(tcp[0:2])
	s.DstPort = binary.BigEndian.Uint16(tcp[2:4])
	s.Seq = binary.BigEndian.Uint32(tcp[4:8])
	s.Ack = binary.BigEndian.Uint32(tcp[8:12])
	s.Flags = tcp[13]
	s.Window = binary.BigEndian.Uint16(tcp[14:16])
	s.Payload = tcp[doff:]
	return s, nil
}

func checksum(b []byte) uint16 {
	var sum uint32
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b[:2]))
		b = b[2:]
	}
	if len(b) != 0 {
		sum += uint32(b[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func tcpChecksum(srcIP, dstIP [4]byte, tcp []byte) uint16 {
	pseudo := make([]byte, 12+len(tcp))
	copy(pseudo[0:4], srcIP[:])
	copy(pseudo[4:8], dstIP[:])
	pseudo[9] = 6
	binary.BigEndian.PutUint16(pseudo[10:12], uint16(len(tcp)))
	copy(pseudo[12:], tcp)
	return checksum(pseudo)
}
