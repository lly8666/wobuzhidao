package control

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
)

var ErrLinkStartupFailed = errors.New("WBD link startup failed")

type LinkClientState byte

const (
	LinkClientAwaitAccept LinkClientState = iota
	LinkClientAwaitAuthOK
	LinkClientEstablished
	LinkClientFailed
)

// LinkClientSession drives the reliable startup transaction over DTLS
// application datagrams. Retries repeat the exact LINK_INIT or AUTH bytes; they
// never change link parameters. A different configuration requires constructing
// a new session over a fresh association.
type LinkClientSession struct {
	init       LinkInit
	token      []byte
	state      LinkClientState
	accept     LinkAccept
	startWire  []byte
	authWire   []byte
}

func NewLinkClientSession(init LinkInit, token []byte) (*LinkClientSession, error) {
	start, err := MarshalLink(init)
	if err != nil {
		return nil, err
	}
	if len(token) > MaxTokenLen {
		return nil, fmt.Errorf("%w: token %d", ErrLimit, len(token))
	}
	return &LinkClientSession{
		init: init,
		token: append([]byte(nil), token...),
		state: LinkClientAwaitAccept,
		startWire: start,
	}, nil
}

func (s *LinkClientSession) State() LinkClientState { return s.state }
func (s *LinkClientSession) Established() bool { return s.state == LinkClientEstablished }
func (s *LinkClientSession) Accept() (LinkAccept, bool) {
	return s.accept, s.state == LinkClientAwaitAuthOK || s.state == LinkClientEstablished
}

// RetryWire returns the exact control datagram that should be retransmitted at
// the current startup state. Once established there is no startup traffic.
func (s *LinkClientSession) RetryWire() ([]byte, error) {
	switch s.state {
	case LinkClientAwaitAccept:
		return append([]byte(nil), s.startWire...), nil
	case LinkClientAwaitAuthOK:
		return append([]byte(nil), s.authWire...), nil
	case LinkClientEstablished:
		return nil, nil
	default:
		return nil, ErrLinkStartupFailed
	}
}

// HandleWire consumes a server startup response and optionally returns the next
// client control datagram (AUTH). Duplicate LINK_ACCEPT/AUTH_OK responses are
// harmless; any server error or config rewrite fails the association.
func (s *LinkClientSession) HandleWire(data []byte) ([]byte, error) {
	if s.state == LinkClientFailed {
		return nil, ErrLinkStartupFailed
	}
	frame, err := UnmarshalLink(data)
	if err != nil {
		s.state = LinkClientFailed
		return nil, err
	}
	if e, ok := frame.(Error); ok {
		s.state = LinkClientFailed
		return nil, fmt.Errorf("%w: server code=%d message=%q", ErrLinkStartupFailed, e.Code, e.Message)
	}

	switch s.state {
	case LinkClientAwaitAccept:
		accept, ok := frame.(LinkAccept)
		if !ok {
			s.state = LinkClientFailed
			return nil, fmt.Errorf("%w: expected LINK_ACCEPT, got %T", ErrLinkStartupFailed, frame)
		}
		if err := ValidateLinkAccept(s.init, accept); err != nil {
			s.state = LinkClientFailed
			return nil, err
		}
		s.accept = accept
		if !accept.AuthRequired {
			s.state = LinkClientEstablished
			return nil, nil
		}
		if len(s.token) == 0 {
			s.state = LinkClientFailed
			return nil, fmt.Errorf("%w: server requires AUTH but token is empty", ErrLinkStartupFailed)
		}
		auth, err := MarshalLink(Auth{Token: s.token})
		if err != nil {
			s.state = LinkClientFailed
			return nil, err
		}
		s.authWire = auth
		s.state = LinkClientAwaitAuthOK
		return append([]byte(nil), auth...), nil

	case LinkClientAwaitAuthOK:
		if accept, ok := frame.(LinkAccept); ok {
			if err := ValidateLinkAccept(s.init, accept); err != nil || accept != s.accept {
				s.state = LinkClientFailed
				if err != nil {
					return nil, err
				}
				return nil, fmt.Errorf("%w: LINK_ACCEPT changed during retry", ErrLinkStartupFailed)
			}
			return append([]byte(nil), s.authWire...), nil
		}
		if _, ok := frame.(AuthOK); !ok {
			s.state = LinkClientFailed
			return nil, fmt.Errorf("%w: expected AUTH_OK, got %T", ErrLinkStartupFailed, frame)
		}
		s.state = LinkClientEstablished
		return nil, nil

	case LinkClientEstablished:
		// A delayed duplicate LINK_ACCEPT or AUTH_OK is startup noise. Validate
		// any LINK_ACCEPT so a rewritten packet can never be silently ignored.
		if accept, ok := frame.(LinkAccept); ok {
			if err := ValidateLinkAccept(s.init, accept); err != nil || accept != s.accept {
				s.state = LinkClientFailed
				if err != nil {
					return nil, err
				}
				return nil, fmt.Errorf("%w: delayed LINK_ACCEPT changed", ErrLinkStartupFailed)
			}
			return nil, nil
		}
		if _, ok := frame.(AuthOK); ok && s.accept.AuthRequired {
			return nil, nil
		}
		return nil, fmt.Errorf("%w: unexpected post-startup control %T", ErrLinkStartupFailed, frame)
	default:
		return nil, ErrLinkStartupFailed
	}
}

// ReliableLinkServerSession adds idempotent retransmission semantics to the
// immutable LinkServerSession. Repeating the exact accepted LINK_INIT or AUTH
// is allowed because DTLS application datagrams can be lost. A different
// LINK_INIT remains a parameter change and poisons the association.
type ReliableLinkServerSession struct {
	inner     *LinkServerSession
	accepted  *LinkAccept
	acceptWire []byte
	authOKWire []byte
}

func NewReliableLinkServerSession(minProtocol, maxProtocol uint16, expectedToken []byte, policy LinkPolicy) (*ReliableLinkServerSession, error) {
	inner, err := NewLinkServerSession(minProtocol, maxProtocol, expectedToken, policy)
	if err != nil {
		return nil, err
	}
	return &ReliableLinkServerSession{inner: inner}, nil
}

func (s *ReliableLinkServerSession) State() State { return s.inner.State() }
func (s *ReliableLinkServerSession) Stats() LinkSessionStats { return s.inner.Stats() }

func (s *ReliableLinkServerSession) HandleWire(data []byte, now uint64) ([]byte, error) {
	frame, err := UnmarshalLink(data)
	if err != nil {
		return nil, err
	}

	if init, ok := frame.(LinkInit); ok && s.accepted != nil {
		if init.Config != s.accepted.Config || s.accepted.Protocol < init.MinProtocol || s.accepted.Protocol > init.MaxProtocol {
			s.inner.fail()
			return MarshalLink(Error{Code: ErrorUnexpectedState, Message: "link parameters changed; reconnect required"})
		}
		// Same immutable proposal: return the byte-identical acceptance. This is
		// a transport retry, not a new negotiation.
		return append([]byte(nil), s.acceptWire...), nil
	}

	if auth, ok := frame.(Auth); ok && s.accepted != nil && s.inner.State() == StateEstablished && s.accepted.AuthRequired {
		provided := sha256.Sum256(auth.Token)
		if subtle.ConstantTimeCompare(provided[:], s.inner.base.expectedTokenHash[:]) != 1 {
			s.inner.fail()
			return MarshalLink(Error{Code: ErrorAuthFailed, Message: "authentication failed; reconnect required"})
		}
		if len(s.authOKWire) == 0 {
			s.authOKWire, err = MarshalLink(AuthOK{})
			if err != nil {
				return nil, err
			}
		}
		return append([]byte(nil), s.authOKWire...), nil
	}

	wire, err := s.inner.HandleWire(data, now)
	if err != nil {
		return nil, err
	}
	out, err := UnmarshalLink(wire)
	if err != nil {
		return nil, err
	}
	if accept, ok := out.(LinkAccept); ok {
		a := accept
		s.accepted = &a
		s.acceptWire = append([]byte(nil), wire...)
	}
	if _, ok := out.(AuthOK); ok {
		s.authOKWire = append([]byte(nil), wire...)
	}
	return wire, nil
}
