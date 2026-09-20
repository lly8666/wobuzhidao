package platformflow

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

const (
	DefaultUDPIdleTimeout = 60 * time.Second
	DefaultMaxUDPFlows    = 4096
)

type UDPReply func(peer, client netip.AddrPort, payload []byte) error

type udpClientState struct {
	id       uint64
	client   netip.AddrPort
	ipv4     bool
	lastSeen time.Time
	tunnel   *TunnelFlow
}

type UDPClient struct {
	channel     *TunnelChannel
	reply       UDPReply
	idleTimeout time.Duration
	maxFlows    int

	mu    sync.Mutex
	next  uint64
	byKey map[netip.AddrPort]*udpClientState
	byID  map[uint64]*udpClientState
}

func NewUDPClient(channel *TunnelChannel, idle time.Duration, maxFlows int, reply UDPReply) (*UDPClient, error) {
	if channel == nil || reply == nil {
		return nil, ErrMalformed
	}
	if idle <= 0 {
		idle = DefaultUDPIdleTimeout
	}
	if maxFlows <= 0 {
		maxFlows = DefaultMaxUDPFlows
	}
	return &UDPClient{
		channel: channel, reply: reply, idleTimeout: idle, maxFlows: maxFlows,
		byKey: make(map[netip.AddrPort]*udpClientState),
		byID:  make(map[uint64]*udpClientState),
	}, nil
}

func (c *UDPClient) Forward(client, peer netip.AddrPort, payload []byte, now time.Time) error {
	if !validEndpoint(client) || !validEndpoint(peer) || client.Addr().Unmap().Is4() != peer.Addr().Unmap().Is4() {
		return fmt.Errorf("%w: invalid UDP endpoints", ErrMalformed)
	}
	if len(payload) > MaxPayload {
		return fmt.Errorf("%w: UDP payload=%d", ErrLimit, len(payload))
	}
	c.mu.Lock()
	state := c.byKey[client]
	if state != nil {
		if state.ipv4 != peer.Addr().Unmap().Is4() {
			c.mu.Unlock()
			return fmt.Errorf("%w: UDP family changed", ErrMalformed)
		}
		state.lastSeen = now
		id := state.id
		tunnel := state.tunnel
		c.mu.Unlock()
		return tunnel.Send(Frame{Kind: KindUDPDatagram, FlowID: id, Peer: peer, Payload: append([]byte(nil), payload...)}, now)
	}
	if len(c.byID) >= c.maxFlows {
		c.mu.Unlock()
		return ErrLimit
	}
	id, err := c.allocateIDLocked()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	tunnel, err := c.channel.OpenFlow()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	state = &udpClientState{id: id, client: client, ipv4: client.Addr().Unmap().Is4(), lastSeen: now, tunnel: tunnel}
	c.byKey[client] = state
	c.byID[id] = state
	c.mu.Unlock()
	if err := tunnel.Send(Frame{Kind: KindUDPDatagram, FlowID: id, Peer: peer, Payload: append([]byte(nil), payload...)}, now); err != nil {
		c.remove(state)
		return err
	}
	return nil
}

func (c *UDPClient) Handle(frame Frame, now time.Time) error {
	if frame.Kind != KindUDPDatagram || frame.FlowID == 0 || !validEndpoint(frame.Peer) {
		return ErrMalformed
	}
	c.mu.Lock()
	state := c.byID[frame.FlowID]
	if state == nil {
		c.mu.Unlock()
		return fmt.Errorf("%w: unknown UDP flow", ErrMalformed)
	}
	if state.ipv4 != frame.Peer.Addr().Unmap().Is4() {
		c.mu.Unlock()
		return fmt.Errorf("%w: UDP reverse family changed", ErrMalformed)
	}
	state.lastSeen = now
	client := state.client
	c.mu.Unlock()
	return c.reply(frame.Peer, client, append([]byte(nil), frame.Payload...))
}

func (c *UDPClient) Tick(now time.Time) {
	var stale []*udpClientState
	c.mu.Lock()
	for key, state := range c.byKey {
		if now.Before(state.lastSeen) || now.Sub(state.lastSeen) < c.idleTimeout {
			continue
		}
		delete(c.byKey, key)
		delete(c.byID, state.id)
		stale = append(stale, state)
	}
	c.mu.Unlock()
	for _, state := range stale {
		_ = state.tunnel.Close()
	}
}

func (c *UDPClient) Close() {
	c.mu.Lock()
	states := make([]*udpClientState, 0, len(c.byID))
	for key, state := range c.byKey {
		delete(c.byKey, key)
		delete(c.byID, state.id)
		states = append(states, state)
	}
	c.mu.Unlock()
	for _, state := range states {
		_ = state.tunnel.Close()
	}
}

func (c *UDPClient) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.byID)
}

func (c *UDPClient) allocateIDLocked() (uint64, error) {
	for attempts := 0; attempts <= len(c.byID); attempts++ {
		c.next++
		if c.next == 0 {
			c.next++
		}
		if c.byID[c.next] == nil {
			return c.next, nil
		}
	}
	return 0, ErrLimit
}

func (c *UDPClient) remove(want *udpClientState) {
	c.mu.Lock()
	if c.byID[want.id] == want {
		delete(c.byID, want.id)
		delete(c.byKey, want.client)
	}
	c.mu.Unlock()
	_ = want.tunnel.Close()
}

type udpServerState struct {
	id       uint64
	ipv4     bool
	lastSeen time.Time
	upstream *net.UDPConn
	tunnel   *TunnelFlow
	closeOnce sync.Once
}

func (s *udpServerState) close() {
	s.closeOnce.Do(func() {
		_ = s.upstream.Close()
		_ = s.tunnel.Close()
	})
}

type UDPServer struct {
	channel     *TunnelChannel
	idleTimeout time.Duration
	maxFlows    int

	mu    sync.Mutex
	flows map[uint64]*udpServerState
	listen func(bool) (*net.UDPConn, error)
}

func NewUDPServer(channel *TunnelChannel, idle time.Duration, maxFlows int) (*UDPServer, error) {
	if channel == nil {
		return nil, ErrMalformed
	}
	if idle <= 0 {
		idle = DefaultUDPIdleTimeout
	}
	if maxFlows <= 0 {
		maxFlows = DefaultMaxUDPFlows
	}
	return &UDPServer{
		channel: channel, idleTimeout: idle, maxFlows: maxFlows,
		flows: make(map[uint64]*udpServerState),
		listen: listenUDPMapping,
	}, nil
}

func (s *UDPServer) Handle(frame Frame, now time.Time) error {
	if frame.Kind != KindUDPDatagram || frame.FlowID == 0 || !validEndpoint(frame.Peer) {
		return ErrMalformed
	}
	ipv4 := frame.Peer.Addr().Unmap().Is4()
	s.mu.Lock()
	state := s.flows[frame.FlowID]
	if state != nil {
		if state.ipv4 != ipv4 {
			s.mu.Unlock()
			return fmt.Errorf("%w: UDP mapping family changed", ErrMalformed)
		}
		state.lastSeen = now
		s.mu.Unlock()
		_, err := state.upstream.WriteToUDPAddrPort(frame.Payload, frame.Peer)
		return err
	}
	if len(s.flows) >= s.maxFlows {
		s.mu.Unlock()
		return ErrLimit
	}
	upstream, err := s.listen(ipv4)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	tunnel, err := s.channel.OpenFlow()
	if err != nil {
		s.mu.Unlock()
		_ = upstream.Close()
		return err
	}
	state = &udpServerState{id: frame.FlowID, ipv4: ipv4, lastSeen: now, upstream: upstream, tunnel: tunnel}
	s.flows[frame.FlowID] = state
	s.mu.Unlock()
	go s.readUpstream(state)
	if _, err := upstream.WriteToUDPAddrPort(frame.Payload, frame.Peer); err != nil {
		s.remove(state)
		return err
	}
	return nil
}

func (s *UDPServer) readUpstream(state *udpServerState) {
	buf := make([]byte, MaxPayload)
	for {
		n, peer, err := state.upstream.ReadFromUDPAddrPort(buf)
		if err != nil {
			s.remove(state)
			return
		}
		now := time.Now()
		s.mu.Lock()
		if s.flows[state.id] != state {
			s.mu.Unlock()
			return
		}
		state.lastSeen = now
		s.mu.Unlock()
		if err := state.tunnel.Send(Frame{
			Kind: KindUDPDatagram, FlowID: state.id, Peer: peer,
			Payload: append([]byte(nil), buf[:n]...),
		}, now); err != nil {
			s.remove(state)
			return
		}
	}
}

func (s *UDPServer) Tick(now time.Time) {
	var stale []*udpServerState
	s.mu.Lock()
	for id, state := range s.flows {
		if now.Before(state.lastSeen) || now.Sub(state.lastSeen) < s.idleTimeout {
			continue
		}
		delete(s.flows, id)
		stale = append(stale, state)
	}
	s.mu.Unlock()
	for _, state := range stale {
		state.close()
	}
}

func (s *UDPServer) Close() {
	s.mu.Lock()
	states := make([]*udpServerState, 0, len(s.flows))
	for id, state := range s.flows {
		delete(s.flows, id)
		states = append(states, state)
	}
	s.mu.Unlock()
	for _, state := range states {
		state.close()
	}
}

func (s *UDPServer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.flows)
}

func (s *UDPServer) remove(want *udpServerState) {
	s.mu.Lock()
	if s.flows[want.id] == want {
		delete(s.flows, want.id)
	}
	s.mu.Unlock()
	want.close()
}

func listenUDPMapping(ipv4 bool) (*net.UDPConn, error) {
	if ipv4 {
		return net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	}
	return net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6unspecified})
}

func validEndpoint(v netip.AddrPort) bool {
	return v.IsValid() && v.Port() != 0 && !v.Addr().IsUnspecified()
}

