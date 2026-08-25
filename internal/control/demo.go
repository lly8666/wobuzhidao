package control

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	TypeDemoBind   Type = 13
	TypeDemoBindOK Type = 14
	DemoWitnessLen      = 32
)

var ErrDemoBindFailed = errors.New("WBD demo preflight binding failed")

type DemoBind struct {
	Witness [DemoWitnessLen]byte
}

type DemoBindOK struct {
	Witness [DemoWitnessLen]byte
}

func MarshalDemo(frame any) ([]byte, error) {
	var typ Type
	var witness [DemoWitnessLen]byte
	switch f := frame.(type) {
	case DemoBind:
		typ, witness = TypeDemoBind, f.Witness
	case *DemoBind:
		if f == nil {
			return nil, fmt.Errorf("%w: nil DEMO_BIND", ErrMalformed)
		}
		return MarshalDemo(*f)
	case DemoBindOK:
		typ, witness = TypeDemoBindOK, f.Witness
	case *DemoBindOK:
		if f == nil {
			return nil, fmt.Errorf("%w: nil DEMO_BIND_OK", ErrMalformed)
		}
		return MarshalDemo(*f)
	default:
		return MarshalLink(frame)
	}
	if allZero(witness[:]) {
		return nil, fmt.Errorf("%w: zero demo witness", ErrMalformed)
	}
	out := make([]byte, HeaderLen+DemoWitnessLen)
	copy(out[:4], Magic[:])
	out[4] = FrameVersion1
	out[5] = byte(typ)
	binary.BigEndian.PutUint16(out[6:8], DemoWitnessLen)
	copy(out[HeaderLen:], witness[:])
	return out, nil
}

func UnmarshalDemo(data []byte) (any, error) {
	if len(data) < HeaderLen || Type(data[5]) != TypeDemoBind && Type(data[5]) != TypeDemoBindOK {
		return UnmarshalLink(data)
	}
	if string(data[:4]) != string(Magic[:]) || data[4] != FrameVersion1 {
		return nil, fmt.Errorf("%w: demo envelope", ErrMalformed)
	}
	if binary.BigEndian.Uint16(data[6:8]) != DemoWitnessLen || len(data) != HeaderLen+DemoWitnessLen {
		return nil, fmt.Errorf("%w: demo witness body", ErrMalformed)
	}
	var witness [DemoWitnessLen]byte
	copy(witness[:], data[HeaderLen:])
	if allZero(witness[:]) {
		return nil, fmt.Errorf("%w: zero demo witness", ErrMalformed)
	}
	if Type(data[5]) == TypeDemoBind {
		return DemoBind{Witness: witness}, nil
	}
	return DemoBindOK{Witness: witness}, nil
}

func allZero(b []byte) bool {
	var v byte
	for _, x := range b {
		v |= x
	}
	return v == 0
}

type DemoLinkClientSession struct {
	inner    *LinkClientSession
	witness  [DemoWitnessLen]byte
	bindWire []byte
	bound    bool
	failed   bool
}

func NewDemoLinkClientSession(init LinkInit, token []byte, witness [DemoWitnessLen]byte) (*DemoLinkClientSession, error) {
	if allZero(witness[:]) {
		return nil, ErrDemoBindFailed
	}
	inner, err := NewLinkClientSession(init, token)
	if err != nil {
		return nil, err
	}
	wire, err := MarshalDemo(DemoBind{Witness: witness})
	if err != nil {
		return nil, err
	}
	return &DemoLinkClientSession{inner: inner, witness: witness, bindWire: wire}, nil
}

func (s *DemoLinkClientSession) Established() bool { return s.bound && !s.failed && s.inner.Established() }
func (s *DemoLinkClientSession) State() LinkClientState {
	if s.failed {
		return LinkClientFailed
	}
	if !s.bound {
		return LinkClientAwaitAccept
	}
	return s.inner.State()
}
func (s *DemoLinkClientSession) Accept() (LinkAccept, bool) { return s.inner.Accept() }

func (s *DemoLinkClientSession) RetryWire() ([]byte, error) {
	if s.failed {
		return nil, ErrDemoBindFailed
	}
	if !s.bound {
		return append([]byte(nil), s.bindWire...), nil
	}
	return s.inner.RetryWire()
}

func (s *DemoLinkClientSession) HandleWire(data []byte) ([]byte, error) {
	if s.failed {
		return nil, ErrDemoBindFailed
	}
	frame, err := UnmarshalDemo(data)
	if err != nil {
		s.failed = true
		return nil, err
	}
	if !s.bound {
		ok, good := frame.(DemoBindOK)
		if !good || ok.Witness != s.witness {
			s.failed = true
			return nil, fmt.Errorf("%w: expected matching DEMO_BIND_OK, got %T", ErrDemoBindFailed, frame)
		}
		s.bound = true
		return s.inner.RetryWire()
	}
	if ok, good := frame.(DemoBindOK); good {
		if ok.Witness != s.witness {
			s.failed = true
			return nil, fmt.Errorf("%w: delayed DEMO_BIND_OK changed", ErrDemoBindFailed)
		}
		return s.inner.RetryWire()
	}
	return s.inner.HandleWire(data)
}

type DemoWitnessVerifier func([DemoWitnessLen]byte) error

type DemoReliableLinkServerSession struct {
	inner      *ReliableLinkServerSession
	verify     DemoWitnessVerifier
	witness    [DemoWitnessLen]byte
	bindOKWire []byte
	bound      bool
	failed     bool
}

func NewDemoReliableLinkServerSession(minProtocol, maxProtocol uint16, expectedToken []byte, policy LinkPolicy, verify DemoWitnessVerifier) (*DemoReliableLinkServerSession, error) {
	if verify == nil {
		return nil, ErrDemoBindFailed
	}
	inner, err := NewReliableLinkServerSession(minProtocol, maxProtocol, expectedToken, policy)
	if err != nil {
		return nil, err
	}
	return &DemoReliableLinkServerSession{inner: inner, verify: verify}, nil
}

func (s *DemoReliableLinkServerSession) State() State {
	if s.failed {
		return StateFailed
	}
	if !s.bound {
		return StateAwaitHello
	}
	return s.inner.State()
}
func (s *DemoReliableLinkServerSession) Stats() LinkSessionStats { return s.inner.Stats() }

func (s *DemoReliableLinkServerSession) HandleWire(data []byte, now uint64) ([]byte, error) {
	if s.failed {
		return nil, ErrDemoBindFailed
	}
	frame, err := UnmarshalDemo(data)
	if err != nil {
		s.failed = true
		return nil, err
	}
	if bind, ok := frame.(DemoBind); ok {
		if !s.bound {
			if err := s.verify(bind.Witness); err != nil {
				s.failed = true
				return MarshalLink(Error{Code: ErrorAuthFailed, Message: "demo preflight witness rejected; reconnect required"})
			}
			s.bound = true
			s.witness = bind.Witness
			s.bindOKWire, err = MarshalDemo(DemoBindOK{Witness: bind.Witness})
			if err != nil {
				return nil, err
			}
			return append([]byte(nil), s.bindOKWire...), nil
		}
		if bind.Witness != s.witness {
			s.failed = true
			return MarshalLink(Error{Code: ErrorUnexpectedState, Message: "demo witness changed; reconnect required"})
		}
		return append([]byte(nil), s.bindOKWire...), nil
	}
	if !s.bound {
		s.failed = true
		return MarshalLink(Error{Code: ErrorAuthRequired, Message: "DEMO_BIND required before LINK_INIT"})
	}
	if _, ok := frame.(DemoBindOK); ok {
		s.failed = true
		return nil, fmt.Errorf("%w: client sent DEMO_BIND_OK", ErrDemoBindFailed)
	}
	return s.inner.HandleWire(data, now)
}
