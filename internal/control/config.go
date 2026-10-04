package control

import (
	"encoding/binary"
	"fmt"
)

const (
	TypeConfig   Type = 9
	TypeConfigOK Type = 10
)

type ProtectionMode byte

const (
	ProtectionNormal ProtectionMode = 1 + iota
	ProtectionWeak15
	ProtectionWeak2
	// ProtectionAutoReserved documents the constitution-defined Auto value.
	// It is deliberately not admitted until the later Auto milestone.
	ProtectionAutoReserved
)

type Config struct{ Mode ProtectionMode }
type ConfigOK struct{ Mode ProtectionMode }

func (m ProtectionMode) String() string {
	switch m {
	case ProtectionNormal:
		return "normal"
	case ProtectionWeak15:
		return "weak-1.5x"
	case ProtectionWeak2:
		return "weak-2x"
	case ProtectionAutoReserved:
		return "auto"
	default:
		return fmt.Sprintf("unknown(%d)", m)
	}
}

func validProtectionMode(m ProtectionMode) bool {
	return m >= ProtectionNormal && m <= ProtectionWeak2
}

// MarshalExtended preserves the M3A-D codec and adds only CONFIG/CONFIG_OK.
func MarshalExtended(frame any) ([]byte, error) {
	var typ Type
	var mode ProtectionMode
	switch f := frame.(type) {
	case Config:
		typ, mode = TypeConfig, f.Mode
	case *Config:
		if f == nil {
			return nil, fmt.Errorf("%w: nil CONFIG", ErrMalformed)
		}
		return MarshalExtended(*f)
	case ConfigOK:
		typ, mode = TypeConfigOK, f.Mode
	case *ConfigOK:
		if f == nil {
			return nil, fmt.Errorf("%w: nil CONFIG_OK", ErrMalformed)
		}
		return MarshalExtended(*f)
	default:
		return Marshal(frame)
	}
	if !validProtectionMode(mode) {
		return nil, fmt.Errorf("%w: protection mode %d", ErrUnsupported, mode)
	}
	out := make([]byte, HeaderLen+1)
	copy(out[:4], Magic[:])
	out[4] = FrameVersion1
	out[5] = byte(typ)
	binary.BigEndian.PutUint16(out[6:8], 1)
	out[8] = byte(mode)
	return out, nil
}

// UnmarshalExtended preserves M3A-D behavior and intercepts only types 9/10.
func UnmarshalExtended(data []byte) (any, error) {
	if len(data) < HeaderLen {
		return Unmarshal(data)
	}
	typ := Type(data[5])
	if typ != TypeConfig && typ != TypeConfigOK {
		return Unmarshal(data)
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
	if bodyLen != 1 || len(data) != HeaderLen+1 {
		return nil, fmt.Errorf("%w: config body/length", ErrMalformed)
	}
	mode := ProtectionMode(data[8])
	if !validProtectionMode(mode) {
		return nil, fmt.Errorf("%w: protection mode %d", ErrUnsupported, mode)
	}
	if typ == TypeConfig {
		return Config{Mode: mode}, nil
	}
	return ConfigOK{Mode: mode}, nil
}

type ConfigSessionStats struct {
	SessionStats
	Configured     bool
	ProtectionMode ProtectionMode
}

// ConfigServerSession layers one-shot product configuration over the already
// qualified M3A-D ServerSession without changing its auth/liveness/close state.
type ConfigServerSession struct {
	base          *ServerSession
	configured    bool
	mode          ProtectionMode
	configRX      uint64
	configTX      uint64
	configRXBytes uint64
	configTXBytes uint64
}

func NewConfigServerSession(minProtocol, maxProtocol uint16, expectedToken []byte) (*ConfigServerSession, error) {
	base, err := NewServerSession(minProtocol, maxProtocol, expectedToken)
	if err != nil {
		return nil, err
	}
	return &ConfigServerSession{base: base}, nil
}

func (s *ConfigServerSession) State() State { return s.base.State() }

func (s *ConfigServerSession) Stats() ConfigSessionStats {
	st := s.base.Stats()
	st.ControlRX += s.configRX
	st.ControlTX += s.configTX
	st.ControlRXBytes += s.configRXBytes
	st.ControlTXBytes += s.configTXBytes
	return ConfigSessionStats{SessionStats: st, Configured: s.configured, ProtectionMode: s.mode}
}

func (s *ConfigServerSession) HandleWire(data []byte, now uint64) ([]byte, error) {
	if len(data) >= HeaderLen && Type(data[5]) == TypeConfig {
		frame, err := UnmarshalExtended(data)
		if err != nil {
			return nil, err
		}
		s.configRX++
		s.configRXBytes += uint64(len(data))
		var reply any
		if s.base.State() == StateAwaitAuth {
			reply = Error{Code: ErrorAuthRequired, Message: "AUTH required"}
		} else if s.base.State() != StateEstablished {
			reply = Error{Code: ErrorUnexpectedState, Message: "CONFIG requires Established"}
		} else if s.configured {
			reply = Error{Code: ErrorUnexpectedState, Message: "CONFIG already applied"}
		} else {
			cfg := frame.(Config)
			s.configured, s.mode = true, cfg.Mode
			reply = ConfigOK{Mode: cfg.Mode}
		}
		wire, err := MarshalExtended(reply)
		if err != nil {
			return nil, err
		}
		s.configTX++
		s.configTXBytes += uint64(len(wire))
		return wire, nil
	}
	return s.base.HandleWire(data, now)
}
