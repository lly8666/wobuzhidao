package control

import (
	"encoding/binary"
	"fmt"
)

const (
	TypeLinkInit   Type = 11
	TypeLinkAccept Type = 12
)

type FECMode byte

const (
	FECOff FECMode = iota
	FECFixed
)

type FECScheduler byte

const (
	FECSchedulerNone FECScheduler = iota
	FECSchedulerTailRS
	FECSchedulerMicroRS
	FECSchedulerCausal
)

// LinkConfig is immutable for the lifetime of one WBD association. A client
// that wants different parameters tears down the association and creates a new
// one; there is deliberately no mid-session config epoch/control path.
type LinkConfig struct {
	FECMode      FECMode
	Scheduler    FECScheduler
	DataShards   uint8
	ParityShards uint8
	LaneCount    uint8
	FlushMillis  uint16
	MTU          uint16
}

type LinkInit struct {
	MinProtocol uint16
	MaxProtocol uint16
	Config      LinkConfig
}

type LinkAccept struct {
	Protocol     uint16
	AuthRequired bool
	Config       LinkConfig
}

type FixedFECProfile struct {
	DataShards   uint8
	ParityShards uint8
	Scheduler    FECScheduler
}

type LinkPolicy struct {
	AllowFECOff     bool
	MinMTU          uint16
	MaxMTU          uint16
	MaxFlushMillis  uint16
	MaxLaneCount    uint8
	AllowedFixedFEC []FixedFECProfile
}

// CurrentLinkPolicy mirrors the live transport, not simulator research. The
// live WBD codec is still fixed systematic 20+20 tail-RS and one raw lane.
func CurrentLinkPolicy() LinkPolicy {
	return LinkPolicy{
		AllowFECOff:     true,
		MinMTU:          576,
		MaxMTU:          1500,
		MaxFlushMillis:  100,
		MaxLaneCount:    1,
		AllowedFixedFEC: []FixedFECProfile{{DataShards: 20, ParityShards: 20, Scheduler: FECSchedulerTailRS}},
	}
}

func validateLinkConfigShape(c LinkConfig) error {
	if c.LaneCount == 0 {
		return fmt.Errorf("%w: lane count 0", ErrMalformed)
	}
	if c.MTU == 0 {
		return fmt.Errorf("%w: mtu 0", ErrMalformed)
	}
	switch c.FECMode {
	case FECOff:
		if c.Scheduler != FECSchedulerNone || c.DataShards != 0 || c.ParityShards != 0 || c.FlushMillis != 0 {
			return fmt.Errorf("%w: fec off carries fixed-fec fields", ErrMalformed)
		}
	case FECFixed:
		if c.Scheduler == FECSchedulerNone || c.DataShards == 0 || c.ParityShards == 0 || c.FlushMillis == 0 {
			return fmt.Errorf("%w: incomplete fixed fec", ErrMalformed)
		}
	default:
		return fmt.Errorf("%w: fec mode %d", ErrUnsupported, c.FECMode)
	}
	return nil
}

func (p LinkPolicy) Validate(c LinkConfig) error {
	if err := validateLinkConfigShape(c); err != nil {
		return err
	}
	if p.MinMTU == 0 || p.MaxMTU < p.MinMTU || p.MaxLaneCount == 0 {
		return fmt.Errorf("%w: invalid link policy", ErrMalformed)
	}
	if c.MTU < p.MinMTU || c.MTU > p.MaxMTU {
		return fmt.Errorf("%w: mtu %d outside %d..%d", ErrLimit, c.MTU, p.MinMTU, p.MaxMTU)
	}
	if c.LaneCount > p.MaxLaneCount {
		return fmt.Errorf("%w: lanes %d > %d", ErrLimit, c.LaneCount, p.MaxLaneCount)
	}
	if c.FECMode == FECOff {
		if !p.AllowFECOff {
			return fmt.Errorf("%w: fec off disabled", ErrUnsupported)
		}
		return nil
	}
	if c.FlushMillis > p.MaxFlushMillis {
		return fmt.Errorf("%w: flush %d > %d", ErrLimit, c.FlushMillis, p.MaxFlushMillis)
	}
	for _, allowed := range p.AllowedFixedFEC {
		if c.DataShards == allowed.DataShards && c.ParityShards == allowed.ParityShards && c.Scheduler == allowed.Scheduler {
			return nil
		}
	}
	return fmt.Errorf("%w: fixed fec %d:%d scheduler=%d", ErrUnsupported, c.DataShards, c.ParityShards, c.Scheduler)
}

func marshalLinkConfig(dst []byte, c LinkConfig) {
	dst[0] = byte(c.FECMode)
	dst[1] = byte(c.Scheduler)
	dst[2] = c.DataShards
	dst[3] = c.ParityShards
	dst[4] = c.LaneCount
	dst[5] = 0
	binary.BigEndian.PutUint16(dst[6:8], c.FlushMillis)
	binary.BigEndian.PutUint16(dst[8:10], c.MTU)
}

func unmarshalLinkConfig(src []byte) (LinkConfig, error) {
	if len(src) != 10 || src[5] != 0 {
		return LinkConfig{}, fmt.Errorf("%w: link config body", ErrMalformed)
	}
	c := LinkConfig{
		FECMode:      FECMode(src[0]),
		Scheduler:    FECScheduler(src[1]),
		DataShards:   src[2],
		ParityShards: src[3],
		LaneCount:    src[4],
		FlushMillis:  binary.BigEndian.Uint16(src[6:8]),
		MTU:          binary.BigEndian.Uint16(src[8:10]),
	}
	if err := validateLinkConfigShape(c); err != nil {
		return LinkConfig{}, err
	}
	return c, nil
}

// MarshalLink extends the existing WBDC envelope only for immutable startup
// negotiation. Existing auth/liveness/close frames retain their exact codec.
func MarshalLink(frame any) ([]byte, error) {
	var typ Type
	var body []byte
	switch f := frame.(type) {
	case LinkInit:
		if err := validateHello(Hello{MinProtocol: f.MinProtocol, MaxProtocol: f.MaxProtocol}); err != nil {
			return nil, err
		}
		if err := validateLinkConfigShape(f.Config); err != nil {
			return nil, err
		}
		typ = TypeLinkInit
		body = make([]byte, 14)
		binary.BigEndian.PutUint16(body[0:2], f.MinProtocol)
		binary.BigEndian.PutUint16(body[2:4], f.MaxProtocol)
		marshalLinkConfig(body[4:14], f.Config)
	case LinkAccept:
		if f.Protocol == 0 {
			return nil, fmt.Errorf("%w: LINK_ACCEPT protocol 0", ErrMalformed)
		}
		if err := validateLinkConfigShape(f.Config); err != nil {
			return nil, err
		}
		typ = TypeLinkAccept
		body = make([]byte, 13)
		binary.BigEndian.PutUint16(body[0:2], f.Protocol)
		if f.AuthRequired {
			body[2] = 1
		}
		marshalLinkConfig(body[3:13], f.Config)
	default:
		return MarshalExtended(frame)
	}
	out := make([]byte, HeaderLen+len(body))
	copy(out[:4], Magic[:])
	out[4] = FrameVersion1
	out[5] = byte(typ)
	binary.BigEndian.PutUint16(out[6:8], uint16(len(body)))
	copy(out[HeaderLen:], body)
	return out, nil
}

func UnmarshalLink(data []byte) (any, error) {
	if len(data) < HeaderLen {
		return UnmarshalExtended(data)
	}
	typ := Type(data[5])
	if typ != TypeLinkInit && typ != TypeLinkAccept {
		return UnmarshalExtended(data)
	}
	if string(data[:4]) != string(Magic[:]) || data[4] != FrameVersion1 {
		return nil, fmt.Errorf("%w: link envelope", ErrMalformed)
	}
	bodyLen := int(binary.BigEndian.Uint16(data[6:8]))
	if len(data) != HeaderLen+bodyLen {
		return nil, fmt.Errorf("%w: link length", ErrMalformed)
	}
	body := data[HeaderLen:]
	if typ == TypeLinkInit {
		if len(body) != 14 {
			return nil, fmt.Errorf("%w: LINK_INIT body %d", ErrMalformed, len(body))
		}
		cfg, err := unmarshalLinkConfig(body[4:14])
		if err != nil {
			return nil, err
		}
		f := LinkInit{MinProtocol: binary.BigEndian.Uint16(body[0:2]), MaxProtocol: binary.BigEndian.Uint16(body[2:4]), Config: cfg}
		if err := validateHello(Hello{MinProtocol: f.MinProtocol, MaxProtocol: f.MaxProtocol}); err != nil {
			return nil, err
		}
		return f, nil
	}
	if len(body) != 13 || body[2]&^byte(1) != 0 {
		return nil, fmt.Errorf("%w: LINK_ACCEPT body/flags", ErrMalformed)
	}
	cfg, err := unmarshalLinkConfig(body[3:13])
	if err != nil {
		return nil, err
	}
	protocol := binary.BigEndian.Uint16(body[0:2])
	if protocol == 0 {
		return nil, fmt.Errorf("%w: LINK_ACCEPT protocol 0", ErrMalformed)
	}
	return LinkAccept{Protocol: protocol, AuthRequired: body[2]&1 != 0, Config: cfg}, nil
}

// ValidateLinkAccept prevents silent server-side parameter rewriting. A WBD
// association either runs exactly the client's immutable proposal or is torn
// down and re-established with a different proposal.
func ValidateLinkAccept(init LinkInit, accept LinkAccept) error {
	if accept.Protocol < init.MinProtocol || accept.Protocol > init.MaxProtocol {
		return fmt.Errorf("%w: accepted protocol %d outside proposal", ErrUnsupported, accept.Protocol)
	}
	if accept.Config != init.Config {
		return fmt.Errorf("%w: server rewrote immutable link config", ErrUnsupported)
	}
	return nil
}

type LinkSessionStats struct {
	SessionStats
	Configured bool
	Config     LinkConfig
}

// LinkServerSession is the product startup state machine. It consumes exactly
// one LINK_INIT before authentication/Established and never accepts a link
// configuration change after that point. A rejected proposal also poisons the
// WBD session: trying another parameter set requires a fresh association.
type LinkServerSession struct {
	base   *ServerSession
	policy LinkPolicy
	config LinkConfig
	set    bool
	rx     uint64
	tx     uint64
	rxB    uint64
	txB    uint64
}

func NewLinkServerSession(minProtocol, maxProtocol uint16, expectedToken []byte, policy LinkPolicy) (*LinkServerSession, error) {
	base, err := NewServerSession(minProtocol, maxProtocol, expectedToken)
	if err != nil {
		return nil, err
	}
	if policy.MinMTU == 0 || policy.MaxMTU < policy.MinMTU || policy.MaxLaneCount == 0 {
		return nil, fmt.Errorf("%w: invalid link policy", ErrMalformed)
	}
	return &LinkServerSession{base: base, policy: policy}, nil
}

func (s *LinkServerSession) State() State { return s.base.State() }

func (s *LinkServerSession) Stats() LinkSessionStats {
	st := s.base.Stats()
	st.ControlRX += s.rx
	st.ControlTX += s.tx
	st.ControlRXBytes += s.rxB
	st.ControlTXBytes += s.txB
	return LinkSessionStats{SessionStats: st, Configured: s.set, Config: s.config}
}

func (s *LinkServerSession) fail() {
	s.base.state = StateFailed
	s.base.syncStatsState()
}

func (s *LinkServerSession) HandleWire(data []byte, now uint64) ([]byte, error) {
	frame, err := UnmarshalLink(data)
	if err != nil {
		return nil, err
	}
	s.rx++
	s.rxB += uint64(len(data))
	s.base.stats.LastActivity = now

	var reply any
	if s.base.State() == StateAwaitHello {
		init, ok := frame.(LinkInit)
		if !ok {
			s.fail()
			reply = Error{Code: ErrorUnexpectedState, Message: "LINK_INIT required; reconnect required"}
		} else if err := s.policy.Validate(init.Config); err != nil {
			s.fail()
			reply = Error{Code: ErrorPolicy, Message: err.Error() + "; reconnect required"}
		} else {
			neg := s.base.Handle(Hello{MinProtocol: init.MinProtocol, MaxProtocol: init.MaxProtocol})
			if a, ok := neg.(Accept); ok {
				s.config, s.set = init.Config, true
				reply = LinkAccept{Protocol: a.Protocol, AuthRequired: s.base.authRequired, Config: init.Config}
			} else {
				reply = neg
			}
		}
	} else {
		switch frame.(type) {
		case LinkInit, LinkAccept, Config, ConfigOK:
			s.fail()
			reply = Error{Code: ErrorUnexpectedState, Message: "link parameters are immutable; reconnect required"}
		default:
			reply = s.base.Handle(frame)
		}
	}

	wire, err := MarshalLink(reply)
	if err != nil {
		return nil, err
	}
	s.tx++
	s.txB += uint64(len(wire))
	return wire, nil
}
