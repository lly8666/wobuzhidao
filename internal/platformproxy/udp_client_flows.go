package platformproxy

import (
	"fmt"
	"net/netip"
	"sync"
	"time"
)

const defaultUDPClientIdleTimeout = 60 * time.Second

type UDPClientFlow struct {
	FlowID uint64
	Client netip.AddrPort
	Peer   netip.AddrPort
}

type udpClientFlowKey struct {
	client netip.AddrPort
	peer   netip.AddrPort
}

type udpClientFlowState struct {
	flow     UDPClientFlow
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

func (t *UDPClientFlowTable) Forward(client, peer netip.AddrPort, now time.Time) (UDPClientFlow, error) {
	if !validUDPFlowEndpoint(client) || !validUDPFlowEndpoint(peer) {
		return UDPClientFlow{}, fmt.Errorf("%w: invalid UDP client flow endpoint", ErrMalformed)
	}
	key := udpClientFlowKey{client: client, peer: peer}

	t.mu.Lock()
	defer t.mu.Unlock()
	if state := t.byKey[key]; state != nil {
		state.lastSeen = now
		return state.flow, nil
	}
	id, err := t.allocateIDLocked()
	if err != nil {
		return UDPClientFlow{}, err
	}
	state := &udpClientFlowState{
		flow:     UDPClientFlow{FlowID: id, Client: client, Peer: peer},
		lastSeen: now,
	}
	t.byKey[key] = state
	t.byID[id] = state
	return state.flow, nil
}

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
	if state.flow.Peer != peer {
		return UDPClientFlow{}, fmt.Errorf("%w: UDP reverse peer changed", ErrMalformed)
	}
	state.lastSeen = now
	return state.flow, nil
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
