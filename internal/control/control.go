package control

import (
	"crypto/sha256"
	"crypto/subtle"
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
	MaxTokenLen               = 256
)

var Magic = [4]byte{'W', 'B', 'D', 'C'}

type Type byte

const (
	TypeHello Type = 1 + iota
	TypeAccept
	TypeError
	TypeAuth
	TypeAuthOK
	TypePing
	TypePong
	TypeClose
)

type ErrorCode uint16

const (
	ErrorNoCommonVersion ErrorCode = 1
	ErrorMalformedHello  ErrorCode = 2
	ErrorPolicy          ErrorCode = 3
	ErrorAuthRequired    ErrorCode = 4
	ErrorAuthFailed      ErrorCode = 5
	ErrorUnexpectedState ErrorCode = 6
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

type Auth struct {
	Token []byte
}

type AuthOK struct{}

type Ping struct {
	Nonce uint64
}

type Pong struct {
	Nonce uint64
}

type CloseReason uint16

const (
	CloseNormal CloseReason = 1 + iota
	CloseIdleTimeout
	CloseAuthFailure
	ClosePolicy
	CloseProtocolError
	CloseTransportTransient
)

type Close struct {
	Reason CloseReason
	Detail string
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
	case Auth:
		typ = TypeAuth
		if len(f.Token) == 0 {
			return nil, fmt.Errorf("%w: empty AUTH token", ErrMalformed)
		}
		if len(f.Token) > MaxTokenLen {
			return nil, fmt.Errorf("%w: AUTH token %d", ErrLimit, len(f.Token))
		}
		body = append([]byte(nil), f.Token...)
	case *Auth:
		if f == nil {
			return nil, fmt.Errorf("%w: nil AUTH", ErrMalformed)
		}
		return Marshal(*f)
	case AuthOK:
		typ = TypeAuthOK
	case *AuthOK:
		if f == nil {
			return nil, fmt.Errorf("%w: nil AUTH_OK", ErrMalformed)
		}
		return Marshal(*f)
	case Ping:
		typ = TypePing
		body = make([]byte, 8)
		binary.BigEndian.PutUint64(body, f.Nonce)
	case *Ping:
		if f == nil {
			return nil, fmt.Errorf("%w: nil PING", ErrMalformed)
		}
		return Marshal(*f)
	case Pong:
		typ = TypePong
		body = make([]byte, 8)
		binary.BigEndian.PutUint64(body, f.Nonce)
	case *Pong:
		if f == nil {
			return nil, fmt.Errorf("%w: nil PONG", ErrMalformed)
		}
		return Marshal(*f)
	case Close:
		typ = TypeClose
		if !validCloseReason(f.Reason) {
			return nil, fmt.Errorf("%w: close reason %d", ErrUnsupported, f.Reason)
		}
		if !utf8.ValidString(f.Detail) {
			return nil, fmt.Errorf("%w: invalid UTF-8 close detail", ErrMalformed)
		}
		if len(f.Detail) > MaxErrorMessageLen {
			return nil, fmt.Errorf("%w: close detail %d", ErrLimit, len(f.Detail))
		}
		body = make([]byte, 2+len(f.Detail))
		binary.BigEndian.PutUint16(body[:2], uint16(f.Reason))
		copy(body[2:], f.Detail)
	case *Close:
		if f == nil {
			return nil, fmt.Errorf("%w: nil CLOSE", ErrMalformed)
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
	case TypeAuth:
		if len(body) == 0 {
			return nil, fmt.Errorf("%w: empty AUTH token", ErrMalformed)
		}
		if len(body) > MaxTokenLen {
			return nil, fmt.Errorf("%w: AUTH token %d", ErrLimit, len(body))
		}
		return Auth{Token: append([]byte(nil), body...)}, nil
	case TypeAuthOK:
		if len(body) != 0 {
			return nil, fmt.Errorf("%w: AUTH_OK body %d", ErrMalformed, len(body))
		}
		return AuthOK{}, nil
	case TypePing:
		if len(body) != 8 {
			return nil, fmt.Errorf("%w: PING body %d", ErrMalformed, len(body))
		}
		return Ping{Nonce: binary.BigEndian.Uint64(body)}, nil
	case TypePong:
		if len(body) != 8 {
			return nil, fmt.Errorf("%w: PONG body %d", ErrMalformed, len(body))
		}
		return Pong{Nonce: binary.BigEndian.Uint64(body)}, nil
	case TypeClose:
		if len(body) < 2 {
			return nil, fmt.Errorf("%w: CLOSE body %d", ErrMalformed, len(body))
		}
		reason := CloseReason(binary.BigEndian.Uint16(body[:2]))
		if !validCloseReason(reason) {
			return nil, fmt.Errorf("%w: close reason %d", ErrUnsupported, reason)
		}
		detail := body[2:]
		if len(detail) > MaxErrorMessageLen {
			return nil, fmt.Errorf("%w: close detail %d", ErrLimit, len(detail))
		}
		if !utf8.Valid(detail) {
			return nil, fmt.Errorf("%w: invalid UTF-8 close detail", ErrMalformed)
		}
		return Close{Reason: reason, Detail: string(detail)}, nil
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

func validCloseReason(r CloseReason) bool {
	return r >= CloseNormal && r <= CloseTransportTransient
}

func ReconnectAllowed(reason CloseReason) bool {
	switch reason {
	case CloseIdleTimeout, CloseTransportTransient:
		return true
	default:
		return false
	}
}

func Backoff(attempt uint32, min, max uint64) (uint64, error) {
	if min == 0 || max < min {
		return 0, fmt.Errorf("%w: invalid backoff range %d..%d", ErrMalformed, min, max)
	}
	d := min
	for i := uint32(0); i < attempt && d < max; i++ {
		if d > max/2 {
			d = max
			break
		}
		d *= 2
		if d > max {
			d = max
		}
	}
	return d, nil
}

type State byte

const (
	StateAwaitHello State = iota
	StateAwaitAuth
	StateEstablished
	StateFailed
	StateClosed
)

type SessionStats struct {
	State          State
	AuthRequired   bool
	Authenticated  bool
	ControlRX      uint64
	ControlTX      uint64
	ControlRXBytes uint64
	ControlTXBytes uint64
	PingsReceived  uint64
	PongsSent      uint64
	LastActivity   uint64
	CloseReason    CloseReason
}

type ServerSession struct {
	state             State
	minProtocol       uint16
	maxProtocol       uint16
	authRequired      bool
	authenticated     bool
	expectedTokenHash [sha256.Size]byte
	stats             SessionStats
}

func NewServerSession(minProtocol, maxProtocol uint16, expectedToken []byte) (*ServerSession, error) {
	if minProtocol == 0 || maxProtocol == 0 || minProtocol > maxProtocol {
		return nil, fmt.Errorf("%w: invalid server protocol range %d..%d", ErrMalformed, minProtocol, maxProtocol)
	}
	if len(expectedToken) > MaxTokenLen {
		return nil, fmt.Errorf("%w: expected token %d", ErrLimit, len(expectedToken))
	}
	s := &ServerSession{state: StateAwaitHello, minProtocol: minProtocol, maxProtocol: maxProtocol}
	if len(expectedToken) != 0 {
		s.authRequired = true
		s.expectedTokenHash = sha256.Sum256(expectedToken)
	}
	s.syncStatsState()
	return s, nil
}

func (s *ServerSession) State() State { return s.state }

func (s *ServerSession) Stats() SessionStats {
	s.syncStatsState()
	return s.stats
}

func (s *ServerSession) IdleExpired(now, idle uint64) bool {
	if idle == 0 || s.stats.LastActivity == 0 || now < s.stats.LastActivity {
		return false
	}
	return now-s.stats.LastActivity >= idle
}

func (s *ServerSession) ExpireIdle(now, idle uint64) bool {
	if s.state != StateEstablished || !s.IdleExpired(now, idle) {
		return false
	}
	s.state = StateClosed
	s.stats.CloseReason = CloseIdleTimeout
	s.syncStatsState()
	return true
}

func (s *ServerSession) Handle(frame any) any {
	return s.handleDecoded(frame)
}

func (s *ServerSession) HandleWire(data []byte, now uint64) ([]byte, error) {
	frame, err := Unmarshal(data)
	if err != nil {
		return nil, err
	}
	s.stats.ControlRX++
	s.stats.ControlRXBytes += uint64(len(data))
	s.stats.LastActivity = now
	reply := s.handleDecoded(frame)
	wire, err := Marshal(reply)
	if err != nil {
		return nil, err
	}
	s.stats.ControlTX++
	s.stats.ControlTXBytes += uint64(len(wire))
	s.syncStatsState()
	return wire, nil
}

func (s *ServerSession) handleDecoded(frame any) any {
	switch s.state {
	case StateAwaitHello:
		h, ok := frame.(Hello)
		if !ok {
			return Error{Code: ErrorUnexpectedState, Message: "HELLO required"}
		}
		reply := Negotiate(h, s.minProtocol, s.maxProtocol)
		if _, ok := reply.(Accept); !ok {
			s.state = StateFailed
			s.syncStatsState()
			return reply
		}
		if s.authRequired {
			s.state = StateAwaitAuth
		} else {
			s.state = StateEstablished
			s.authenticated = true
		}
		s.syncStatsState()
		return reply
	case StateAwaitAuth:
		a, ok := frame.(Auth)
		if !ok {
			return Error{Code: ErrorAuthRequired, Message: "AUTH required"}
		}
		provided := sha256.Sum256(a.Token)
		if subtle.ConstantTimeCompare(provided[:], s.expectedTokenHash[:]) != 1 {
			s.state = StateFailed
			s.syncStatsState()
			return Error{Code: ErrorAuthFailed, Message: "authentication failed"}
		}
		s.state = StateEstablished
		s.authenticated = true
		s.syncStatsState()
		return AuthOK{}
	case StateEstablished:
		if p, ok := frame.(Ping); ok {
			s.stats.PingsReceived++
			s.stats.PongsSent++
			return Pong{Nonce: p.Nonce}
		}
		if c, ok := frame.(Close); ok {
			s.state = StateClosed
			s.stats.CloseReason = c.Reason
			s.syncStatsState()
			return c
		}
		return Error{Code: ErrorUnexpectedState, Message: "session already established"}
	case StateFailed:
		return Error{Code: ErrorUnexpectedState, Message: "session failed"}
	case StateClosed:
		return Error{Code: ErrorUnexpectedState, Message: "session closed"}
	default:
		return Error{Code: ErrorUnexpectedState, Message: "invalid session state"}
	}
}

func (s *ServerSession) syncStatsState() {
	s.stats.State = s.state
	s.stats.AuthRequired = s.authRequired
	s.stats.Authenticated = s.authenticated
}
