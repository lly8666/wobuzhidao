package platformproxy

import (
	"fmt"
	"net/netip"
	"sync"
	"time"
)

const defaultUDPClientIdleTimeout = 60 * time.Second

// UDPClientFlow is one endpoint-independent UDP mapping on the OpenWrt/client
// side. FlowID is bound only to the intercepted client source endpoint. Peer is
// the peer associated with the current lookup result; it is deliberately not
// part of mapping identity.
type UDPClientFlow struct {
	FlowID uint64
	Client netip.AddrPort
	Peer   netip.AddrPort
}

type udpClientFlowKey struct {
	client netip.AddrPort
}

type udpClientFlowState struct {
	flow     UDPClientFlow
	ipv4     bool
	lastSeen time.Time
}

type UDPClientFlowTable struct {
	idleTimeout time.Duration

	mu    sync.Mutex
	next  uint64
	byKey map[udpClientFlowKey]*udpClientFlowState
	byID  map[uint64]*udpClientFlowState
}

func NewUDPClientFlowTable(idleTimeout time.Duration) *UDPClientFlowTable {
	if idleTimeout <= 0 {
		idleTimeout = defaultUDPClientIdleTimeout
	}
	return &UDPClientFlowTable{
		idleTimeout: idleTimeout,
		byKey:       make(map[udpClientFlowKey]*udpClientFlowState),
		byID:        make(map[uint64]*udpClientFlowState),
	}
}

// Forward implements endpoint-independent mapping: one intercepted UDP source
// address:port keeps one FlowID while it talks to any number of remote peers.
// The address family is immutable for that mapping because the server-side
// egress socket is one AF-specific UDP socket.
func (t *UDPClientFlowTable) Forward(client, peer netip.AddrPort, now time.Time) (UDPClientFlow, error) {
	if !validUDPFlowEndpoint(client) || !validUDPFlowEndpoint(peer) || udpAddrIs4(client.Addr()) != udpAddrIs4(peer.Addr()) {
		return UDPClientFlow{}, fmt.Errorf("%w: invalid UDP client flow endpoint", ErrMalformed)
	}
	key := udpClientFlowKey{client: client}

	t.mu.Lock()
	defer t.mu.Unlock()
	if state := t.byKey[key]; state != nil {
		if state.ipv4 != udpAddrIs4(peer.Addr()) {
			return UDPClientFlow{}, fmt.Errorf("%w: UDP mapping address family changed", ErrMalformed)
		}
		state.lastSeen = now
		out := state.flow
		out.Peer = peer
		return out, nil
	}
	id, err := t.allocateIDLocked()
	if err != nil {
		return UDPClientFlow{}, err
	}
	state := &udpClientFlowState{
		flow:     UDPClientFlow{FlowID: id, Client: client},
		ipv4:     udpAddrIs4(client.Addr()),
		lastSeen: now,
	}
	t.byKey[key] = state
	t.byID[id] = state
	out := state.flow
	out.Peer = peer
	return out, nil
}

// Reverse implements endpoint-independent filtering for an existing mapping:
// any valid remote endpoint in the mapping's address family may send back to
// the mapped UDP socket. The remote endpoint is carried separately in WBDP and
// becomes the transparent source address on the client side.
func (t *UDPClientFlowTable) Reverse(flowID uint64, peer netip.AddrPort, now time.Time) (UDPClientFlow, error) {
	if flowID == 0 || !validUDPFlowEndpoint(peer) {
		return UDPClientFlow{}, fmt.Errorf("%w: invalid UDP reverse identity", ErrMalformed)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	state := t.byID[flowID]
	if state == nil {
		return UDPClientFlow{}, fmt.Errorf("%w: unknown UDP flow id=%d", ErrMalformed, flowID)
	}
	if state.ipv4 != udpAddrIs4(peer.Addr()) {
		return UDPClientFlow{}, fmt.Errorf("%w: UDP reverse address family changed", ErrMalformed)
	}
	state.lastSeen = now
	out := state.flow
	out.Peer = peer
	return out, nil
}

func (t *UDPClientFlowTable) Expire(now time.Time) []UDPClientFlow {
	t.mu.Lock()
	defer t.mu.Unlock()
	expired := make([]UDPClientFlow, 0)
	for key, state := range t.byKey {
		if now.Before(state.lastSeen) || now.Sub(state.lastSeen) < t.idleTimeout {
			continue
		}
		delete(t.byKey, key)
		delete(t.byID, state.flow.FlowID)
		expired = append(expired, state.flow)
	}
	return expired
}

func (t *UDPClientFlowTable) Len() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.byID)
}

func (t *UDPClientFlowTable) allocateIDLocked() (uint64, error) {
	for attempts := 0; attempts <= len(t.byID); attempts++ {
		t.next++
		if t.next == 0 {
			t.next++
		}
		if _, exists := t.byID[t.next]; !exists {
			return t.next, nil
		}
	}
	return 0, fmt.Errorf("%w: UDP flow id space exhausted", ErrLimit)
}

func validUDPFlowEndpoint(v netip.AddrPort) bool {
	return v.IsValid() && v.Port() != 0 && !v.Addr().IsUnspecified()
}

func udpAddrIs4(addr netip.Addr) bool {
	return addr.Unmap().Is4()
}
