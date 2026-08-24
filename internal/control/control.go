package control

import (
	"encoding/binary"
	"errors"
	"fmt"
	"unicode/utf8"
)

const (
	FrameVersion1      byte   = 1
	ProtocolVersion1   uint16 = 1
	HeaderLen                 = 8
	MaxBodyLen                = 1024
	MaxErrorMessageLen        = 256
)

var Magic = [4]byte{'W', 'B', 'D', 'C'}

type Type byte

const (
	TypeHello Type = 1 + iota
	TypeAccept
	TypeError
)

type ErrorCode uint16

const (
	ErrorNoCommonVersion ErrorCode = 1
	ErrorMalformedHello  ErrorCode = 2
	ErrorPolicy          ErrorCode = 3
)

type Hello struct {
	MinProtocol uint16
	MaxProtocol uint16
}

type Accept struct {
	Protocol uint16
}

type Error struct {
	Code    ErrorCode
	Message string
}

var (
	ErrMalformed   = errors.New("malformed WBD control frame")
	ErrUnsupported = errors.New("unsupported WBD control frame")
	ErrLimit       = errors.New("WBD control frame limit exceeded")
)

func Marshal(frame any) ([]byte, error) {
	var typ Type
	var body []byte

	switch f := frame.(type) {
	case Hello:
		typ = TypeHello
		if err := validateHello(f); err != nil {
			return nil, err
		}
		body = make([]byte, 4)
		binary.BigEndian.PutUint16(body[0:2], f.MinProtocol)
		binary.BigEndian.PutUint16(body[2:4], f.MaxProtocol)
	case *Hello:
		if f == nil {
			return nil, fmt.Errorf("%w: nil HELLO", ErrMalformed)
		}
		return Marshal(*f)
	case Accept:
		typ = TypeAccept
		if f.Protocol == 0 {
			return nil, fmt.Errorf("%w: ACCEPT protocol 0", ErrMalformed)
		}
		body = make([]byte, 2)
		binary.BigEndian.PutUint16(body, f.Protocol)
	case *Accept:
		if f == nil {
			return nil, fmt.Errorf("%w: nil ACCEPT", ErrMalformed)
		}
		return Marshal(*f)
	case Error:
		typ = TypeError
		if f.Code == 0 {
			return nil, fmt.Errorf("%w: ERROR code 0", ErrMalformed)
		}
		if !utf8.ValidString(f.Message) {
			return nil, fmt.Errorf("%w: invalid UTF-8 error message", ErrMalformed)
		}
		if len(f.Message) > MaxErrorMessageLen {
			return nil, fmt.Errorf("%w: error message %d", ErrLimit, len(f.Message))
		}
		body = make([]byte, 2+len(f.Message))
		binary.BigEndian.PutUint16(body[:2], uint16(f.Code))
		copy(body[2:], f.Message)
	case *Error:
		if f == nil {
			return nil, fmt.Errorf("%w: nil ERROR", ErrMalformed)
		}
		return Marshal(*f)
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupported, frame)
	}

	if len(body) > MaxBodyLen {
		return nil, fmt.Errorf("%w: body %d", ErrLimit, len(body))
	}
	out := make([]byte, HeaderLen+len(body))
	copy(out[:4], Magic[:])
	out[4] = FrameVersion1
	out[5] = byte(typ)
	binary.BigEndian.PutUint16(out[6:8], uint16(len(body)))
	copy(out[HeaderLen:], body)
	return out, nil
}

func Unmarshal(data []byte) (any, error) {
	if len(data) < HeaderLen {
		return nil, fmt.Errorf("%w: short header", ErrMalformed)
	}
	if string(data[:4]) != string(Magic[:]) {
		return nil, fmt.Errorf("%w: bad magic", ErrMalformed)
	}
	if data[4] != FrameVersion1 {
		return nil, fmt.Errorf("%w: frame version %d", ErrUnsupported, data[4])
	}
	bodyLen := int(binary.BigEndian.Uint16(data[6:8]))
	if bodyLen > MaxBodyLen {
		return nil, fmt.Errorf("%w: body %d", ErrLimit, bodyLen)
	}
	if len(data) < HeaderLen+bodyLen {
		return nil, fmt.Errorf("%w: truncated body", ErrMalformed)
	}
	if len(data) != HeaderLen+bodyLen {
		return nil, fmt.Errorf("%w: trailing bytes", ErrMalformed)
	}
	body := data[HeaderLen:]

	switch Type(data[5]) {
	case TypeHello:
		if len(body) != 4 {
			return nil, fmt.Errorf("%w: HELLO body %d", ErrMalformed, len(body))
		}
		f := Hello{MinProtocol: binary.BigEndian.Uint16(body[:2]), MaxProtocol: binary.BigEndian.Uint16(body[2:4])}
		if err := validateHello(f); err != nil {
			return nil, err
		}
		return f, nil
	case TypeAccept:
		if len(body) != 2 {
			return nil, fmt.Errorf("%w: ACCEPT body %d", ErrMalformed, len(body))
		}
		f := Accept{Protocol: binary.BigEndian.Uint16(body)}
		if f.Protocol == 0 {
			return nil, fmt.Errorf("%w: ACCEPT protocol 0", ErrMalformed)
		}
		return f, nil
	case TypeError:
		if len(body) < 2 {
			return nil, fmt.Errorf("%w: ERROR body %d", ErrMalformed, len(body))
		}
		code := ErrorCode(binary.BigEndian.Uint16(body[:2]))
		if code == 0 {
			return nil, fmt.Errorf("%w: ERROR code 0", ErrMalformed)
		}
		msg := body[2:]
		if len(msg) > MaxErrorMessageLen {
			return nil, fmt.Errorf("%w: error message %d", ErrLimit, len(msg))
		}
		if !utf8.Valid(msg) {
			return nil, fmt.Errorf("%w: invalid UTF-8 error message", ErrMalformed)
		}
		return Error{Code: code, Message: string(msg)}, nil
	default:
		return nil, fmt.Errorf("%w: type %d", ErrUnsupported, data[5])
	}
}

func Negotiate(h Hello, serverMin, serverMax uint16) any {
	if validateHello(h) != nil || serverMin == 0 || serverMax == 0 || serverMin > serverMax {
		return Error{Code: ErrorMalformedHello, Message: "invalid version range"}
	}
	min := h.MinProtocol
	if serverMin > min {
		min = serverMin
	}
	max := h.MaxProtocol
	if serverMax < max {
		max = serverMax
	}
	if min > max {
		return Error{Code: ErrorNoCommonVersion, Message: "no common protocol version"}
	}
	return Accept{Protocol: max}
}

func validateHello(h Hello) error {
	if h.MinProtocol == 0 || h.MaxProtocol == 0 || h.MinProtocol > h.MaxProtocol {
		return fmt.Errorf("%w: invalid HELLO range %d..%d", ErrMalformed, h.MinProtocol, h.MaxProtocol)
	}
	return nil
}
