package windowsclient

import (
	"errors"
	"time"
)

// ErrTUNDirectBackendUnavailable prevents a local/CN/underlay decision from
// silently falling back to a protected tunnel lane or leaking via an unbound
// socket. Real Windows TCP/UDP system-stack forwarding is a separate N4 gate.
var ErrTUNDirectBackendUnavailable = errors.New("windowsclient: no generation and physical-interface bound direct TCP/UDP backend")

// TUNDirectSink must be supplied by a separately qualified native system-stack
// redirector. The interface/generation values are mandatory context, not an
// assertion that the implementation has already bound an OS socket.
type TUNDirectSink interface {
	ForwardDirectIPv4(packet []byte, interfaceIndex uint32, generation uint64) error
}

type TUNDispatchResult struct {
	Decision  TUNDecision
	Protected RoutedOutbound
}

// TUNPacketDispatcher bridges the already tested classifier and authenticated
// tunnel Router. No native Wintun reader or direct Windows socket is activated
// by this object. It never sends a protected packet on a direct backend or
// reclassifies direct as protected when direct forwarding is unavailable.
type TUNPacketDispatcher struct {
	router *Router
	flows  *TUNScopedFlows
	direct TUNDirectSink
}

func NewTUNPacketDispatcher(router *Router, flows *TUNScopedFlows, direct TUNDirectSink) (*TUNPacketDispatcher, error) {
	if router == nil || flows == nil || flows.policy == nil || router.owner == nil || router.tun == nil ||
		router.addr != flows.policy.lease {
		return nil, ErrInvalidBinding
	}
	return &TUNPacketDispatcher{router: router, flows: flows, direct: direct}, nil
}

func (d *TUNPacketDispatcher) Dispatch(packet []byte, interfaceIndex uint32, generation uint64, now time.Time) (TUNDispatchResult, error) {
	if d == nil || d.router == nil || d.flows == nil {
		return TUNDispatchResult{}, ErrInvalidBinding
	}
	decision, err := d.flows.Classify(packet, interfaceIndex, generation, now)
	if err != nil {
		return TUNDispatchResult{}, err
	}
	switch decision {
	case TUNDirect:
		if d.direct == nil {
			return TUNDispatchResult{}, ErrTUNDirectBackendUnavailable
		}
		// Direct backends are not allowed to retain or mutate a borrowed Wintun
		// ring buffer. Preserve the packet before crossing the OS boundary.
		owned := append([]byte(nil), packet...)
		if err := d.direct.ForwardDirectIPv4(owned, interfaceIndex, generation); err != nil {
			return TUNDispatchResult{}, err
		}
		return TUNDispatchResult{Decision: decision}, nil
	case TUNProtected, TUNDNSProtected:
		// Retain authenticated lease, source validation, and Normal/Game
		// generation fences in the existing owner; do not seal twice here.
		routed, err := d.router.RouteFromTUN(packet, now)
		if err != nil {
			return TUNDispatchResult{}, err
		}
		return TUNDispatchResult{Decision: decision, Protected: routed}, nil
	default:
		return TUNDispatchResult{}, ErrTUNClassifier
	}
}
