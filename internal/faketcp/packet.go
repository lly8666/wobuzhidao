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

	DefaultWindowScale = 8
	DefaultMSS         = 1360
	DefaultIPv4PeerMSS = 536
	MaxWindowScale     = 14

	synOptionLen = 12
)

var (
	ErrShortPacket  = errors.New("faketcp: short packet")
	ErrNotIPv4TCP   = errors.New("faketcp: not ipv4/tcp")
	ErrBadTCPHeader = errors.New("faketcp: invalid tcp header")
)

// Segment is the P2 handshake/bootstrap packet view. SACK blocks and
// steady-state recovery metadata are intentionally not part of this extraction.
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
	Payload        []byte
}

// IsInitialSYN accepts a normal initial TCP SYN independently of the WBD client
// presentation. ECE/CWR bits are intentionally tolerated; ACK/FIN/RST/PSH,
// SYN payload, invalid option values, or zero ports are not.
func IsInitialSYN(s Segment) bool {
	if s.Flags&FlagSYN == 0 ||
		s.Flags&(FlagACK|FlagFIN|FlagRST|FlagPSH) != 0 ||
		len(s.Payload) != 0 ||
		s.SrcPort == 0 || s.DstPort == 0 {
		return false
	}
	if s.MSSSet && s.MSS == 0 {
		return false
	}
	if s.WindowScaleSet && s.WindowScale > MaxWindowScale {
		return false
	}
	return true
}

// IsWBDHandshakeSegment recognizes the client presentation currently emitted by
// WBD. It is an appearance helper only and is deliberately NOT a server
// admission predicate; identity is decided later from the protected TLS path.
func IsWBDHandshakeSegment(s Segment) bool {
	return IsInitialSYN(s) &&
		s.MSSSet && s.MSS == DefaultMSS &&
		s.SACKPermitted && s.WindowScaleSet && s.WindowScale == DefaultWindowScale
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

func packetOptionLen(flags uint8) int {
	if flags&FlagSYN != 0 {
		return synOptionLen
	}
	return 0
}

func PacketLen(flags uint8, payloadLen int) int {
	return 40 + packetOptionLen(flags) + payloadLen
}

// MarshalSegment serializes the options carried by Segment itself. This is
// needed for server SYN-ACK negotiation: SACK/window-scale are only offered
// when the peer offered them. The legacy MarshalIPv4TCP helpers below keep the
// fixed WBD client SYN presentation.
func MarshalSegment(seg Segment, ipID uint16, persona PacketPersona) []byte {
	opts := segmentSYNOptions(seg, persona)
	buf := make([]byte, 40+len(opts)+len(seg.Payload))
	return marshalIPv4TCPSegmentInto(buf, seg, opts, ipID, persona)
}

func MarshalIPv4TCP(srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, payload []byte, ipID uint16) []byte {
	buf := make([]byte, PacketLen(flags, len(payload)))
	return MarshalIPv4TCPPersonaInto(buf, srcIP, dstIP, srcPort, dstPort, seq, ack, flags, window, payload, ipID, DefaultPacketPersona)
}

func MarshalIPv4TCPPersonaInto(buf []byte, srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, payload []byte, ipID uint16, persona PacketPersona) []byte {
	pkt := marshalIPv4TCPBaseInto(buf, srcIP, dstIP, srcPort, dstPort, seq, ack, flags, window, payload, ipID)
	if persona != PacketPersonaWindows11 {
		return pkt
	}

	ip := pkt[:20]
	ip[8] = 128
	binary.BigEndian.PutUint16(ip[10:12], 0)
	binary.BigEndian.PutUint16(ip[10:12], checksum(ip))

	if flags&FlagSYN == 0 {
		return pkt
	}

	tcp := pkt[20:]
	o := tcp[20:32]
	clear(o)
	o[0], o[1] = 2, 4
	binary.BigEndian.PutUint16(o[2:4], DefaultMSS)
	o[4] = 1
	o[5], o[6], o[7] = 3, 3, DefaultWindowScale
	o[8], o[9] = 1, 1
	o[10], o[11] = 4, 2
	binary.BigEndian.PutUint16(tcp[16:18], 0)
	binary.BigEndian.PutUint16(tcp[16:18], tcpChecksum(srcIP, dstIP, tcp))
	return pkt
}

func marshalIPv4TCPBaseInto(buf []byte, srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, payload []byte, ipID uint16) []byte {
	optLen := packetOptionLen(flags)
	need := 40 + optLen + len(payload)
	if len(buf) < need {
		panic("faketcp: marshal buffer too small")
	}
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
		o[0], o[1] = 2, 4
		binary.BigEndian.PutUint16(o[2:4], DefaultMSS)
		o[4], o[5] = 4, 2
		o[6] = 1
		o[7], o[8], o[9] = 3, 3, DefaultWindowScale
		o[10], o[11] = 1, 1
	}

	copy(tcp[20+optLen:], payload)
	binary.BigEndian.PutUint16(tcp[16:18], tcpChecksum(srcIP, dstIP, tcp))
	return buf
}

func marshalIPv4TCPSegmentInto(buf []byte, seg Segment, opts []byte, ipID uint16, persona PacketPersona) []byte {
	need := 40 + len(opts) + len(seg.Payload)
	if len(buf) < need {
		panic("faketcp: marshal buffer too small")
	}
	buf = buf[:need]
	clear(buf[:40+len(opts)])

	ip := buf[:20]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(buf)))
	binary.BigEndian.PutUint16(ip[4:6], ipID)
	binary.BigEndian.PutUint16(ip[6:8], 0x4000)
	if persona == PacketPersonaWindows11 {
		ip[8] = 128
	} else {
		ip[8] = 64
	}
	ip[9] = 6
	copy(ip[12:16], seg.SrcIP[:])
	copy(ip[16:20], seg.DstIP[:])
	binary.BigEndian.PutUint16(ip[10:12], checksum(ip))

	tcp := buf[20:]
	binary.BigEndian.PutUint16(tcp[0:2], seg.SrcPort)
	binary.BigEndian.PutUint16(tcp[2:4], seg.DstPort)
	binary.BigEndian.PutUint32(tcp[4:8], seg.Seq)
	binary.BigEndian.PutUint32(tcp[8:12], seg.Ack)
	tcp[12] = byte((20 + len(opts)) / 4 << 4)
	tcp[13] = seg.Flags
	binary.BigEndian.PutUint16(tcp[14:16], seg.Window)
	copy(tcp[20:20+len(opts)], opts)
	copy(tcp[20+len(opts):], seg.Payload)
	binary.BigEndian.PutUint16(tcp[16:18], tcpChecksum(seg.SrcIP, seg.DstIP, tcp))
	return buf
}

func segmentSYNOptions(seg Segment, persona PacketPersona) []byte {
	if seg.Flags&FlagSYN == 0 {
		return nil
	}
	opts := make([]byte, 0, synOptionLen)
	appendMSS := func() {
		if !seg.MSSSet {
			return
		}
		opts = append(opts, 2, 4, 0, 0)
		binary.BigEndian.PutUint16(opts[len(opts)-2:], seg.MSS)
	}
	appendSACK := func() {
		if seg.SACKPermitted {
			opts = append(opts, 4, 2)
		}
	}
	appendWS := func() {
		if !seg.WindowScaleSet {
			return
		}
		opts = append(opts, 3, 3, seg.WindowScale)
	}

	if persona == PacketPersonaWindows11 {
		appendMSS()
		if seg.WindowScaleSet {
			opts = append(opts, 1)
			appendWS()
		}
		if seg.SACKPermitted {
			opts = append(opts, 1, 1)
			appendSACK()
		}
	} else {
		appendMSS()
		appendSACK()
		if seg.WindowScaleSet {
			opts = append(opts, 1)
			appendWS()
		}
	}
	for len(opts)%4 != 0 {
		opts = append(opts, 1)
	}
	return opts
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
	if total < ihl+20 {
		return s, ErrShortPacket
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
	parseTCPOptions(tcp[20:doff], &s)
	s.Payload = tcp[doff:]
	return s, nil
}

func parseTCPOptions(opts []byte, s *Segment) {
	for i := 0; i < len(opts); {
		kind := opts[i]
		if kind == 0 {
			return
		}
		if kind == 1 {
			i++
			continue
		}
		if i+2 > len(opts) {
			return
		}
		l := int(opts[i+1])
		if l < 2 || i+l > len(opts) {
			return
		}
		switch {
		case kind == 2 && l == 4:
			s.MSS = binary.BigEndian.Uint16(opts[i+2 : i+4])
			s.MSSSet = true
		case kind == 3 && l == 3:
			s.WindowScale = opts[i+2]
			s.WindowScaleSet = true
		case kind == 4 && l == 2:
			s.SACKPermitted = true
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
	if len(b) != 0 {
		sum += uint32(b[0]) << 8
	}
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
	if len(b) != 0 {
		sum += uint32(b[0]) << 8
	}
	return finishChecksum(sum)
}

func finishChecksum(sum uint32) uint16 {
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}
