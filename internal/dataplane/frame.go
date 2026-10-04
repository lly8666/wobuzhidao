package dataplane

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	FrameVersion1 byte = 1
	HeaderLen          = 8
	MaxPacketLen       = 9000
	TypeIP        byte = 1
)

var (
	Magic          = [4]byte{'W', 'B', 'D', 'P'}
	ErrMalformed   = errors.New("malformed WBD data frame")
	ErrUnsupported = errors.New("unsupported WBD data frame")
	ErrLimit       = errors.New("WBD data frame limit exceeded")
)

func MarshalIP(packet []byte) ([]byte, error) {
	if err := ValidateIPPacket(packet); err != nil {
		return nil, err
	}
	if len(packet) > MaxPacketLen {
		return nil, fmt.Errorf("%w: IP packet %d", ErrLimit, len(packet))
	}

	out := make([]byte, HeaderLen+len(packet))
	copy(out[:4], Magic[:])
	out[4] = FrameVersion1
	out[5] = TypeIP
	binary.BigEndian.PutUint16(out[6:8], uint16(len(packet)))
	copy(out[HeaderLen:], packet)
	return out, nil
}

func UnmarshalIP(frame []byte) ([]byte, error) {
	if len(frame) < HeaderLen {
		return nil, fmt.Errorf("%w: short header", ErrMalformed)
	}
	if string(frame[:4]) != string(Magic[:]) {
		return nil, fmt.Errorf("%w: bad magic", ErrMalformed)
	}
	if frame[4] != FrameVersion1 {
		return nil, fmt.Errorf("%w: version %d", ErrUnsupported, frame[4])
	}
	if frame[5] != TypeIP {
		return nil, fmt.Errorf("%w: type %d", ErrUnsupported, frame[5])
	}
	n := int(binary.BigEndian.Uint16(frame[6:8]))
	if n > MaxPacketLen {
		return nil, fmt.Errorf("%w: IP packet %d", ErrLimit, n)
	}
	if len(frame) != HeaderLen+n {
		return nil, fmt.Errorf("%w: encoded=%d actual=%d", ErrMalformed, n, len(frame)-HeaderLen)
	}
	packet := frame[HeaderLen:]
	if err := ValidateIPPacket(packet); err != nil {
		return nil, err
	}
	return append([]byte(nil), packet...), nil
}

func ValidateIPPacket(packet []byte) error {
	if len(packet) == 0 {
		return fmt.Errorf("%w: empty IP packet", ErrMalformed)
	}
	if len(packet) > MaxPacketLen {
		return fmt.Errorf("%w: IP packet %d", ErrLimit, len(packet))
	}

	switch packet[0] >> 4 {
	case 4:
		if len(packet) < 20 {
			return fmt.Errorf("%w: short IPv4 packet", ErrMalformed)
		}
		ihl := int(packet[0]&0x0f) * 4
		if ihl < 20 || ihl > len(packet) {
			return fmt.Errorf("%w: invalid IPv4 header length %d", ErrMalformed, ihl)
		}
		total := int(binary.BigEndian.Uint16(packet[2:4]))
		if total != len(packet) {
			return fmt.Errorf("%w: IPv4 total length %d != %d", ErrMalformed, total, len(packet))
		}
	case 6:
		if len(packet) < 40 {
			return fmt.Errorf("%w: short IPv6 packet", ErrMalformed)
		}
		payload := int(binary.BigEndian.Uint16(packet[4:6]))
		if payload == 0 && len(packet) != 40 {
			return fmt.Errorf("%w: IPv6 jumbogram unsupported", ErrUnsupported)
		}
		if 40+payload != len(packet) {
			return fmt.Errorf("%w: IPv6 payload length %d != %d", ErrMalformed, payload, len(packet)-40)
		}
	default:
		return fmt.Errorf("%w: IP version %d", ErrUnsupported, packet[0]>>4)
	}
	return nil
}
