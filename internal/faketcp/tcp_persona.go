package faketcp

import (
	"encoding/binary"
	"fmt"
	"strings"
)

// PacketPersona controls only observable IPv4/TCP presentation. It does not
// change sequence accounting, retransmission, SACK recovery, FEC, or any
// steady-state delivery semantics.
type PacketPersona uint8

const (
	PacketPersonaLegacy PacketPersona = iota
	PacketPersonaWindows11
)

func ParsePacketPersona(s string) (PacketPersona, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "legacy", "linux":
		return PacketPersonaLegacy, nil
	case "windows", "windows11", "win11":
		return PacketPersonaWindows11, nil
	default:
		return PacketPersonaLegacy, fmt.Errorf("unknown TCP packet persona %q", s)
	}
}

func (p PacketPersona) String() string {
	switch p {
	case PacketPersonaWindows11:
		return "windows11"
	default:
		return "legacy"
	}
}

// MarshalIPv4TCPSACKPersonaInto preserves the mature legacy packet builder and
// applies a presentation-only profile afterwards. This keeps ARQ/FEC and the
// non-persona wire path byte-for-byte unchanged.
//
// The Windows profile intentionally keeps WBD's path-MTU-derived MSS=1360 but
// presents the stable Windows-family traits used by passive fingerprinting:
// IPv4 TTL 128, DF with non-zero IP ID (already provided by the legacy builder),
// window 65535 supplied by callers, WS=8, and SYN option layout
// MSS,NOP,WS,NOP,NOP,SACK-permitted. The TTL is kept coherent for the whole
// client flow rather than changing after the bootstrap phase.
func MarshalIPv4TCPSACKPersonaInto(buf []byte, srcIP, dstIP [4]byte, srcPort, dstPort uint16, seq, ack uint32, flags uint8, window uint16, sacks []SACKBlock, payload []byte, ipID uint16, persona PacketPersona) []byte {
	pkt := MarshalIPv4TCPSACKInto(buf, srcIP, dstIP, srcPort, dstPort, seq, ack, flags, window, sacks, payload, ipID)
	if persona != PacketPersonaWindows11 {
		return pkt
	}

	// Windows uses an initial IPv4 TTL of 128. Keep it for every packet in the
	// same raw flow so observers do not see a persona jump at the TLS->DTLS
	// transition. TTL is outside the TCP checksum.
	ip := pkt[:20]
	ip[8] = 128
	binary.BigEndian.PutUint16(ip[10:12], 0)
	binary.BigEndian.PutUint16(ip[10:12], checksum(ip))

	if flags&FlagSYN == 0 {
		return pkt
	}

	// SYN option bytes occupy the same 12 bytes as the legacy profile, so this
	// is a pure reorder with no sequence/header-length impact:
	//   MSS(4), NOP(1), WS(3), NOP(1), NOP(1), SACK-permitted(2).
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
