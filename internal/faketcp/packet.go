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

	// A scale of 8 makes the steady-state advertised 65535 window represent
	// about 16 MiB. That covers the project's 200-Mbit/600-ms BDP without ever
	// using receive-window pressure to throttle the performance-first inner path.
	DefaultWindowScale = 8
	DefaultMSS         = 1360

	// IPv4 20 + TCP 20 + maximum RFC 2018 SACK option (36 bytes).
	MaxHeaderSize = 76
)

var (
	ErrShortPacket  = errors.New("faketcp: short packet")
	ErrNotIPv4TCP  = errors.New("faketcp: not ipv4/tcp")
	ErrBadTCPHeader = errors.New("faketcp: invalid tcp header")
)

type SACKBlock struct {
	Start uint32
	End   uint32
}

type Segment struct {
	SrcIP          [4]byte
	DstIP          [4]byte
	SrcPort        uint16
	DstPort        uint16
	Seq            uint32
	Ack            uint32
	Flags          uint8
	Window         uint16
	MSS            uint16
	MSSSet         bool
	SACKPermitted  bool
	WindowScale    uint8
	WindowScaleSet bool
	SACK           [4]SACKBlock
	SACKN          int
	Payload        []byte
}

// IsWBDHandshakeSegment recognizes the SYN option profile WBD already emits.
// It deliberately does not add a new wire marker: the helper only turns the
// existing MSS/SACK/window-scale tuple into a demultiplexing guard so a normal
// kernel TCP/TLS listener may share the same numeric public port with the raw
// FakeTCP lane without the raw lane adopting ordinary browser/kernel SYNs.
func IsWBDHandshakeSegment(s Segment) bool {
	return s.Flags&FlagSYN != 0 && len(s.Payload) == 0 &&
		s.MSSSet && s.MSS == DefaultMSS &&
		s.SACKPermitted && s.WindowScaleSet && s.WindowScale == DefaultWindowScale
}

func IPv4(ip net.IP) ([4]byte, bool) {
	var out [4]byte
	v := ip.To4()
	if v == nil { return out, false }
	copy(out[:], v)
	return out, true
}

func optionLen(flags uint8, sacks []SACKBlock) int {
	if flags&FlagSYN != 0 { return 12 }
	if len(sacks) == 0 { return 0 }
	n := len(sacks)
	if n > 4 { n = 4 }
	// RFC 2018 option is 2+8*n bytes, padded to a 32-bit TCP header boundary.
	l := 2 + 8*n
	return (l + 3) &^ 3
}

func PacketLen(flags uint8, payloadLen int) int {
	return PacketLenSACK(flags, payloadLen, nil)
}

func PacketLenSACK(flags uint8, payloadLen int, sacks []SACKBlock) int {
	return 40 + optionLen(flags, sacks) + payloadLen
}

func MarshalIPv4TCP(srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, payload []byte, ipID uint16) []byte {
	buf := make([]byte, PacketLen(flags, len(payload)))
	return MarshalIPv4TCPInto(buf, srcIP, dstIP, srcPort, dstPort, seq, ack, flags, window, payload, ipID)
}

func MarshalIPv4TCPInto(buf []byte, srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, payload []byte, ipID uint16) []byte {
	return MarshalIPv4TCPSACKInto(buf, srcIP, dstIP, srcPort, dstPort, seq, ack, flags, window, nil, payload, ipID)
}

// MarshalIPv4TCPSACKInto constructs one IPv4/TCP packet without allocation.
// SYN advertises MSS + SACK-permitted + window-scale. ACK packets may carry up
// to four RFC 2018 SACK blocks. The steady-state data path has no TCP options
// unless SACK state is actually needed.
func MarshalIPv4TCPSACKInto(buf []byte, srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, sacks []SACKBlock, payload []byte, ipID uint16) []byte {
	optLen := optionLen(flags, sacks)
	need := 40 + optLen + len(payload)
	if len(buf) < need { panic("faketcp: marshal buffer too small") }
	buf = buf[:need]
	clear(buf[:40+optLen])

	ip := buf[:20]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(buf)))
	binary.BigEndian.PutUint16(ip[4:6], ipID)
	binary.BigEndian.PutUint16(ip[6:8], 0x4000)
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
	if flags&FlagSYN != 0 {
		o := tcp[20:32]
		// MSS 1360, SACK permitted, NOP, window scale 8, NOP/NOP padding.
		o[0], o[1] = 2, 4
		binary.BigEndian.PutUint16(o[2:4], DefaultMSS)
		o[4], o[5] = 4, 2
		o[6] = 1
		o[7], o[8], o[9] = 3, 3, DefaultWindowScale
		o[10], o[11] = 1, 1
	} else if len(sacks) != 0 {
		n := len(sacks)
		if n > 4 { n = 4 }
		o := tcp[20:20+optLen]
		o[0], o[1] = 5, byte(2+8*n)
		for i := 0; i < n; i++ {
			binary.BigEndian.PutUint32(o[2+i*8:6+i*8], sacks[i].Start)
			binary.BigEndian.PutUint32(o[6+i*8:10+i*8], sacks[i].End)
		}
		// Remaining bytes are already EOL/padding zeroes.
	}
	copy(tcp[20+optLen:], payload)
	binary.BigEndian.PutUint16(tcp[16:18], tcpChecksum(srcIP, dstIP, tcp))
	return buf
}

func ParseIPv4TCP(packet []byte) (Segment, error) {
	var s Segment
	if len(packet) < 40 { return s, ErrShortPacket }
	if packet[0]>>4 != 4 || packet[9] != 6 { return s, ErrNotIPv4TCP }
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+20 { return s, ErrShortPacket }
	total := int(binary.BigEndian.Uint16(packet[2:4]))
	if total == 0 || total > len(packet) { total = len(packet) }
	tcp := packet[ihl:total]
	doff := int(tcp[12]>>4) * 4
	if doff < 20 || doff > len(tcp) { return s, ErrBadTCPHeader }
	copy(s.SrcIP[:], packet[12:16])
	copy(s.DstIP[:], packet[16:20])
	s.SrcPort = binary.BigEndian.Uint16(tcp[0:2])
	s.DstPort = binary.BigEndian.Uint16(tcp[2:4])
	s.Seq = binary.BigEndian.Uint32(tcp[4:8])
	s.Ack = binary.BigEndian.Uint32(tcp[8:12])
	s.Flags = tcp[13]
	s.Window = binary.BigEndian.Uint16(tcp[14:16])
	parseTCPOptions(tcp[20:doff], &s)
	s.Payload = tcp[doff:]
	return s, nil
}

func parseTCPOptions(opts []byte, s *Segment) {
	for i := 0; i < len(opts); {
		kind := opts[i]
		if kind == 0 { return }
		if kind == 1 { i++; continue }
		if i+2 > len(opts) { return }
		l := int(opts[i+1])
		if l < 2 || i+l > len(opts) { return }
		switch {
		case kind == 2 && l == 4:
			s.MSS = binary.BigEndian.Uint16(opts[i+2:i+4])
			s.MSSSet = true
		case kind == 3 && l == 3:
			s.WindowScale = opts[i+2]
			s.WindowScaleSet = true
		case kind == 4 && l == 2:
			s.SACKPermitted = true
		case kind == 5 && l >= 10 && (l-2)%8 == 0:
			n := (l-2)/8
			if n > 4 { n = 4 }
			for j := 0; j < n; j++ {
				off := i+2+j*8
				s.SACK[j] = SACKBlock{Start: binary.BigEndian.Uint32(opts[off:off+4]), End: binary.BigEndian.Uint32(opts[off+4:off+8])}
			}
			s.SACKN = n
		}
		i += l
	}
}

func checksum(b []byte) uint16 {
	var sum uint32
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b[:2]))
		b = b[2:]
	}
	if len(b) != 0 { sum += uint32(b[0]) << 8 }
	return finishChecksum(sum)
}

func tcpChecksum(srcIP, dstIP [4]byte, tcp []byte) uint16 {
	var sum uint32
	sum += uint32(binary.BigEndian.Uint16(srcIP[0:2]))
	sum += uint32(binary.BigEndian.Uint16(srcIP[2:4]))
	sum += uint32(binary.BigEndian.Uint16(dstIP[0:2]))
	sum += uint32(binary.BigEndian.Uint16(dstIP[2:4]))
	sum += 6
	sum += uint32(len(tcp))
	b := tcp
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b[:2]))
		b = b[2:]
	}
	if len(b) != 0 { sum += uint32(b[0]) << 8 }
	return finishChecksum(sum)
}

func finishChecksum(sum uint32) uint16 {
	for sum>>16 != 0 { sum = (sum & 0xffff) + (sum >> 16) }
	return ^uint16(sum)
}
