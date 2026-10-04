// Package platformproxy defines the small platform-adapter envelope carried as
// an opaque inner datagram by the frozen V2.2 WBD link. It is deliberately
// above linkdata/FEC/DTLS/FakeTCP and must not alter those transport semantics.
package platformproxy

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
)

const (
	Version1 byte = 1

	// HeaderSize is fixed so an adapter can budget against the immutable WBD
	// link MTU before handing a complete frame to linkdata.Path.Encode.
	HeaderSize = 44

	// MaxPayload leaves 100 bytes of headroom under the current default 1400
	// byte link MTU. Callers using a smaller negotiated MTU must apply the
	// tighter per-link bound themselves.
	MaxPayload = 1300
)

var magic = [4]byte{'W', 'B', 'D', 'P'}

var (
	ErrMalformed   = errors.New("platformproxy: malformed frame")
	ErrUnsupported = errors.New("platformproxy: unsupported frame")
	ErrLimit       = errors.New("platformproxy: frame limit exceeded")
)

type Kind byte

const (
	KindUDPDatagram Kind = 1 + iota
	KindTCPOpen
	KindTCPData
	KindTCPAck
	KindTCPClose
)

const flagFIN byte = 1 << 0

// Frame is a platform-facing flow envelope. FlowID is chosen by the capture
// side and remains stable for one proxied flow.
//
// UDPDatagram and TCPOpen require Peer. For client->server UDP Peer is the
// original destination; for server->client UDP it is the remote source.
// TCPData uses Offset as the byte-stream offset. TCPAck uses Offset as the
// cumulative next byte expected. FIN is valid only on TCPData.
//
// This frame does not provide reliability by itself. TCP reliability belongs
// to the platform adapter using DATA offsets and ACKs, not to FakeTCP/FEC.
type Frame struct {
	Kind    Kind
	FlowID  uint64
	Offset  uint64
	FIN     bool
	Peer    netip.AddrPort
	Payload []byte
}

func Marshal(f Frame) ([]byte, error) {
	if f.FlowID == 0 {
		return nil, fmt.Errorf("%w: zero flow id", ErrMalformed)
	}
	if len(f.Payload) > MaxPayload {
		return nil, fmt.Errorf("%w: payload=%d", ErrLimit, len(f.Payload))
	}

	var flags byte
	if f.FIN {
		flags = flagFIN
	}
	family, addr, port, err := validate(f)
	if err != nil {
		return nil, err
	}

	out := make([]byte, HeaderSize+len(f.Payload))
	copy(out[0:4], magic[:])
	out[4] = Version1
	out[5] = byte(f.Kind)
	out[6] = flags
	out[7] = family
	binary.BigEndian.PutUint64(out[8:16], f.FlowID)
	binary.BigEndian.PutUint64(out[16:24], f.Offset)
	binary.BigEndian.PutUint16(out[24:26], port)
	copy(out[26:42], addr[:])
	binary.BigEndian.PutUint16(out[42:44], uint16(len(f.Payload)))
	copy(out[HeaderSize:], f.Payload)
	return out, nil
}

func Unmarshal(b []byte) (Frame, error) {
	if len(b) < HeaderSize {
		return Frame{}, fmt.Errorf("%w: short header", ErrMalformed)
	}
	if string(b[0:4]) != string(magic[:]) {
		return Frame{}, fmt.Errorf("%w: bad magic", ErrMalformed)
	}
	if b[4] != Version1 {
		return Frame{}, fmt.Errorf("%w: version=%d", ErrUnsupported, b[4])
	}
	kind := Kind(b[5])
	flags := b[6]
	family := b[7]
	flowID := binary.BigEndian.Uint64(b[8:16])
	offset := binary.BigEndian.Uint64(b[16:24])
	port := binary.BigEndian.Uint16(b[24:26])
	payloadLen := int(binary.BigEndian.Uint16(b[42:44]))
	if flowID == 0 {
		return Frame{}, fmt.Errorf("%w: zero flow id", ErrMalformed)
	}
	if payloadLen > MaxPayload || HeaderSize+payloadLen != len(b) {
		return Frame{}, fmt.Errorf("%w: payload length=%d wire=%d", ErrMalformed, payloadLen, len(b))
	}

	peer, err := decodePeer(family, port, b[26:42])
	if err != nil {
		return Frame{}, err
	}
	f := Frame{
		Kind:    kind,
		FlowID:  flowID,
		Offset:  offset,
		FIN:     flags&flagFIN != 0,
		Peer:    peer,
		Payload: append([]byte(nil), b[HeaderSize:]...),
	}
	if flags&^flagFIN != 0 {
		return Frame{}, fmt.Errorf("%w: flags=0x%x", ErrUnsupported, flags)
	}
	if _, _, _, err := validate(f); err != nil {
		return Frame{}, err
	}
	return f, nil
}

func validate(f Frame) (family byte, addr [16]byte, port uint16, err error) {
	switch f.Kind {
	case KindUDPDatagram:
		if f.FIN || f.Offset != 0 {
			return 0, addr, 0, fmt.Errorf("%w: UDP flags/offset", ErrMalformed)
		}
		return encodePeer(f.Peer)
	case KindTCPOpen:
		if f.FIN || f.Offset != 0 || len(f.Payload) != 0 {
			return 0, addr, 0, fmt.Errorf("%w: TCP_OPEN fields", ErrMalformed)
		}
		return encodePeer(f.Peer)
	case KindTCPData:
		if f.Peer.IsValid() {
			return 0, addr, 0, fmt.Errorf("%w: TCP_DATA peer", ErrMalformed)
		}
		if len(f.Payload) == 0 && !f.FIN {
			return 0, addr, 0, fmt.Errorf("%w: empty TCP_DATA without FIN", ErrMalformed)
		}
		return 0, addr, 0, nil
	case KindTCPAck:
		if f.FIN || f.Peer.IsValid() || len(f.Payload) != 0 {
			return 0, addr, 0, fmt.Errorf("%w: TCP_ACK fields", ErrMalformed)
		}
		return 0, addr, 0, nil
	case KindTCPClose:
		if f.FIN || f.Peer.IsValid() || f.Offset != 0 || len(f.Payload) != 0 {
			return 0, addr, 0, fmt.Errorf("%w: TCP_CLOSE fields", ErrMalformed)
		}
		return 0, addr, 0, nil
	default:
		return 0, addr, 0, fmt.Errorf("%w: kind=%d", ErrUnsupported, f.Kind)
	}
}

func encodePeer(peer netip.AddrPort) (family byte, addr [16]byte, port uint16, err error) {
	if !peer.IsValid() || peer.Port() == 0 {
		return 0, addr, 0, fmt.Errorf("%w: invalid peer", ErrMalformed)
	}
	ip := peer.Addr().Unmap()
	if ip.Is4() {
		family = 4
		v4 := ip.As4()
		copy(addr[:4], v4[:])
	} else if ip.Is6() {
		family = 6
		v6 := ip.As16()
		copy(addr[:], v6[:])
	} else {
		return 0, addr, 0, fmt.Errorf("%w: invalid peer address", ErrMalformed)
	}
	return family, addr, peer.Port(), nil
}

func decodePeer(family byte, port uint16, raw []byte) (netip.AddrPort, error) {
	switch family {
	case 0:
		if port != 0 {
			return netip.AddrPort{}, fmt.Errorf("%w: peer family/port mismatch", ErrMalformed)
		}
		for _, v := range raw {
			if v != 0 {
				return netip.AddrPort{}, fmt.Errorf("%w: peer bytes without family", ErrMalformed)
			}
		}
		return netip.AddrPort{}, nil
	case 4:
		if port == 0 {
			return netip.AddrPort{}, fmt.Errorf("%w: zero peer port", ErrMalformed)
		}
		var v4 [4]byte
		copy(v4[:], raw[:4])
		for _, v := range raw[4:] {
			if v != 0 {
				return netip.AddrPort{}, fmt.Errorf("%w: nonzero IPv4 padding", ErrMalformed)
			}
		}
		return netip.AddrPortFrom(netip.AddrFrom4(v4), port), nil
	case 6:
		if port == 0 {
			return netip.AddrPort{}, fmt.Errorf("%w: zero peer port", ErrMalformed)
		}
		var v6 [16]byte
		copy(v6[:], raw)
		return netip.AddrPortFrom(netip.AddrFrom16(v6), port), nil
	default:
		return netip.AddrPort{}, fmt.Errorf("%w: address family=%d", ErrUnsupported, family)
	}
}
