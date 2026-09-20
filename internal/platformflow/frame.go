package platformflow

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

const (
	Version1        byte = 1
	FrameHeaderSize      = 44
	IPv4HeaderSize       = 20
	MaxPayload           = logicaltunnel.MaxLeasedIPv4PacketLen - IPv4HeaderSize - FrameHeaderSize
)

var (
	frameMagic      = [4]byte{'W', 'B', 'P', 'F'}
	ErrMalformed    = errors.New("platformflow: malformed frame")
	ErrUnsupported  = errors.New("platformflow: unsupported frame")
	ErrLimit        = errors.New("platformflow: flow limit exceeded")
	ErrClosed       = errors.New("platformflow: flow closed")
	ErrWindowFull   = errors.New("platformflow: TCP send window full")
	ErrRetryLimit   = errors.New("platformflow: TCP retransmission limit reached")
	ErrNoWireOutput = errors.New("platformflow: no authoritative lane produced wire output")
)

type Kind byte

const (
	KindUDPDatagram Kind = 1 + iota
	KindTCPOpen
	KindTCPData
	KindTCPAck
	KindTCPClose
)

const flagFIN byte = 1

type Frame struct {
	Kind    Kind
	FlowID  uint64
	Offset  uint64
	FIN     bool
	Peer    netip.AddrPort
	Payload []byte
}

func MarshalFrame(f Frame) ([]byte, error) {
	if f.FlowID == 0 {
		return nil, fmt.Errorf("%w: zero flow id", ErrMalformed)
	}
	if len(f.Payload) > MaxPayload {
		return nil, fmt.Errorf("%w: payload=%d max=%d", ErrLimit, len(f.Payload), MaxPayload)
	}
	var flags byte
	if f.FIN {
		flags = flagFIN
	}
	family, addr, port, err := validateFrame(f)
	if err != nil {
		return nil, err
	}
	out := make([]byte, FrameHeaderSize+len(f.Payload))
	copy(out[:4], frameMagic[:])
	out[4] = Version1
	out[5] = byte(f.Kind)
	out[6] = flags
	out[7] = family
	binary.BigEndian.PutUint64(out[8:16], f.FlowID)
	binary.BigEndian.PutUint64(out[16:24], f.Offset)
	binary.BigEndian.PutUint16(out[24:26], port)
	copy(out[26:42], addr[:])
	binary.BigEndian.PutUint16(out[42:44], uint16(len(f.Payload)))
	copy(out[44:], f.Payload)
	return out, nil
}

func ParseFrame(b []byte) (Frame, error) {
	if len(b) < FrameHeaderSize || string(b[:4]) != string(frameMagic[:]) {
		return Frame{}, ErrMalformed
	}
	if b[4] != Version1 {
		return Frame{}, fmt.Errorf("%w: version=%d", ErrUnsupported, b[4])
	}
	if b[6]&^flagFIN != 0 {
		return Frame{}, fmt.Errorf("%w: flags=0x%x", ErrUnsupported, b[6])
	}
	n := int(binary.BigEndian.Uint16(b[42:44]))
	if n > MaxPayload || FrameHeaderSize+n != len(b) {
		return Frame{}, fmt.Errorf("%w: payload=%d wire=%d", ErrMalformed, n, len(b))
	}
	peer, err := decodePeer(b[7], binary.BigEndian.Uint16(b[24:26]), b[26:42])
	if err != nil {
		return Frame{}, err
	}
	f := Frame{
		Kind: Kind(b[5]), FlowID: binary.BigEndian.Uint64(b[8:16]),
		Offset: binary.BigEndian.Uint64(b[16:24]), FIN: b[6]&flagFIN != 0,
		Peer: peer, Payload: append([]byte(nil), b[44:]...),
	}
	if f.FlowID == 0 {
		return Frame{}, fmt.Errorf("%w: zero flow id", ErrMalformed)
	}
	if _, _, _, err := validateFrame(f); err != nil {
		return Frame{}, err
	}
	return f, nil
}

func validateFrame(f Frame) (byte, [16]byte, uint16, error) {
	var zero [16]byte
	switch f.Kind {
	case KindUDPDatagram:
		if f.FIN || f.Offset != 0 {
			return 0, zero, 0, fmt.Errorf("%w: UDP flags/offset", ErrMalformed)
		}
		return encodePeer(f.Peer)
	case KindTCPOpen:
		if f.FIN || f.Offset != 0 || len(f.Payload) != 0 {
			return 0, zero, 0, fmt.Errorf("%w: TCP open fields", ErrMalformed)
		}
		return encodePeer(f.Peer)
	case KindTCPData:
		if f.Peer.IsValid() || (len(f.Payload) == 0 && !f.FIN) {
			return 0, zero, 0, fmt.Errorf("%w: TCP data fields", ErrMalformed)
		}
		return 0, zero, 0, nil
	case KindTCPAck:
		if f.FIN || f.Peer.IsValid() || len(f.Payload) != 0 {
			return 0, zero, 0, fmt.Errorf("%w: TCP ack fields", ErrMalformed)
		}
		return 0, zero, 0, nil
	case KindTCPClose:
		if f.FIN || f.Peer.IsValid() || f.Offset != 0 || len(f.Payload) != 0 {
			return 0, zero, 0, fmt.Errorf("%w: TCP close fields", ErrMalformed)
		}
		return 0, zero, 0, nil
	default:
		return 0, zero, 0, fmt.Errorf("%w: kind=%d", ErrUnsupported, f.Kind)
	}
}

func encodePeer(peer netip.AddrPort) (byte, [16]byte, uint16, error) {
	var raw [16]byte
	if !peer.IsValid() || peer.Port() == 0 || peer.Addr().IsUnspecified() {
		return 0, raw, 0, fmt.Errorf("%w: invalid peer", ErrMalformed)
	}
	addr := peer.Addr().Unmap()
	if addr.Is4() {
		v := addr.As4()
		copy(raw[:4], v[:])
		return 4, raw, peer.Port(), nil
	}
	if addr.Is6() {
		v := addr.As16()
		copy(raw[:], v[:])
		return 6, raw, peer.Port(), nil
	}
	return 0, raw, 0, fmt.Errorf("%w: invalid peer", ErrMalformed)
}

func decodePeer(family byte, port uint16, raw []byte) (netip.AddrPort, error) {
	switch family {
	case 0:
		if port != 0 {
			return netip.AddrPort{}, ErrMalformed
		}
		for _, b := range raw {
			if b != 0 {
				return netip.AddrPort{}, ErrMalformed
			}
		}
		return netip.AddrPort{}, nil
	case 4:
		if port == 0 {
			return netip.AddrPort{}, ErrMalformed
		}
		var v [4]byte
		copy(v[:], raw[:4])
		for _, b := range raw[4:] {
			if b != 0 {
				return netip.AddrPort{}, ErrMalformed
			}
		}
		return netip.AddrPortFrom(netip.AddrFrom4(v), port), nil
	case 6:
		if port == 0 {
			return netip.AddrPort{}, ErrMalformed
		}
		var v [16]byte
		copy(v[:], raw)
		return netip.AddrPortFrom(netip.AddrFrom16(v), port), nil
	default:
		return netip.AddrPort{}, fmt.Errorf("%w: family=%d", ErrUnsupported, family)
	}
}
