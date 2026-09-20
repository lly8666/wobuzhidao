package platformflow

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"
)

const (
	DefaultTCPIdleTimeout = 90 * time.Second
	DefaultTCPDialTimeout = 10 * time.Second
	DefaultMaxTCPFlows    = 4096
)

type TCPConfig struct {
	Reliability       TCPReliabilityConfig
	IdleTimeout       time.Duration
	OpenRTO           time.Duration
	MaxOpenRetransmit int
	DialTimeout       time.Duration
	MaxFlows          int
}

func DefaultTCPConfig() TCPConfig {
	return TCPConfig{
		Reliability:       DefaultTCPReliabilityConfig(),
		IdleTimeout:       DefaultTCPIdleTimeout,
		OpenRTO:           500 * time.Millisecond,
		MaxOpenRetransmit: 8,
		DialTimeout:       DefaultTCPDialTimeout,
		MaxFlows:          DefaultMaxTCPFlows,
	}
}

func (c *TCPConfig) normalize() error {
	if c.Reliability.ChunkSize == 0 {
		c.Reliability = DefaultTCPReliabilityConfig()
	}
	if err := c.Reliability.validate(); err != nil {
		return err
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = DefaultTCPIdleTimeout
	}
	if c.OpenRTO <= 0 {
		c.OpenRTO = c.Reliability.RTO
	}
	if c.MaxOpenRetransmit < 0 {
		return ErrMalformed
	}
	if c.DialTimeout <= 0 {
		c.DialTimeout = DefaultTCPDialTimeout
	}
	if c.MaxFlows <= 0 {
		c.MaxFlows = DefaultMaxTCPFlows
	}
	return nil
}

type tcpClientFlow struct {
	id     uint64
	target netip.AddrPort
	conn   net.Conn
	tunnel *TunnelFlow
	tx     *TCPTransmit
	rx     *TCPReceive

	mu          sync.Mutex
	cond        *sync.Cond
	opened      bool
	openSent    time.Time
	openRetries int
	lastSeen    time.Time
	closed      bool
}

func (f *tcpClientFlow) finishedLocked() bool {
	return f.tx.FINAcked() && f.rx.FINDelivered()
}

func (f *tcpClientFlow) close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	f.tx.Abort()
	f.rx.Close()
	f.cond.Broadcast()
	conn := f.conn
	tunnel := f.tunnel
	f.mu.Unlock()
	_ = conn.Close()
	_ = tunnel.Close()
}

type TCPClient struct {
	channel *TunnelChannel
	cfg     TCPConfig

	mu    sync.Mutex
	next  uint64
	flows map[uint64]*tcpClientFlow
}

func NewTCPClient(channel *TunnelChannel, cfg TCPConfig) (*TCPClient, error) {
	if channel == nil {
		return nil, ErrMalformed
	}
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	return &TCPClient{channel: channel, cfg: cfg, flows: make(map[uint64]*tcpClientFlow)}, nil
}

func (c *TCPClient) Add(conn net.Conn, target netip.AddrPort, now time.Time) (uint64, error) {
	if conn == nil || !validEndpoint(target) {
		return 0, ErrMalformed
	}
	c.mu.Lock()
	if len(c.flows) >= c.cfg.MaxFlows {
		c.mu.Unlock()
		return 0, ErrLimit
	}
	id, err := c.allocateIDLocked()
	if err != nil {
		c.mu.Unlock()
		return 0, err
	}
	tunnel, err := c.channel.OpenFlow()
	if err != nil {
		c.mu.Unlock()
		return 0, err
	}
	tx, err := NewTCPTransmit(id, c.cfg.Reliability)
	if err != nil {
		c.mu.Unlock()
		_ = tunnel.Close()
		return 0, err
	}
	rx, err := NewTCPReceive(id, c.cfg.Reliability)
	if err != nil {
		c.mu.Unlock()
		_ = tunnel.Close()
		return 0, err
	}
	flow := &tcpClientFlow{
		id: id, target: target, conn: conn, tunnel: tunnel, tx: tx, rx: rx,
		openSent: now, lastSeen: now,
	}
	flow.cond = sync.NewCond(&flow.mu)
	c.flows[id] = flow
	c.mu.Unlock()

	if err := tunnel.Send(Frame{Kind: KindTCPOpen, FlowID: id, Peer: target}, now); err != nil {
		c.remove(flow)
		return 0, err
	}
	go c.readLocal(flow)
	return id, nil
}

func (c *TCPClient) Handle(frame Frame, now time.Time) error {
	if frame.FlowID == 0 {
		return ErrMalformed
	}
	flow := c.get(frame.FlowID)
	if flow == nil {
		if frame.Kind == KindTCPClose {
			return nil
		}
		return fmt.Errorf("%w: unknown TCP client flow", ErrMalformed)
	}
	switch frame.Kind {
	case KindTCPAck:
		return c.handleAck(flow, frame, now)
	case KindTCPData:
		return c.handleData(flow, frame, now)
	case KindTCPClose:
		c.remove(flow)
		return nil
	default:
		return ErrUnsupported
	}
}

func (c *TCPClient) handleAck(flow *tcpClientFlow, frame Frame, now time.Time) error {
	flow.mu.Lock()
	if flow.closed {
		flow.mu.Unlock()
		return ErrClosed
	}
	if !flow.opened {
		if frame.Offset != 0 {
			flow.mu.Unlock()
			return ErrMalformed
		}
		flow.opened = true
		flow.lastSeen = now
		flow.cond.Broadcast()
		flow.mu.Unlock()
		return nil
	}
	if err := flow.tx.Ack(frame.Offset); err != nil {
		flow.mu.Unlock()
		return err
	}
	flow.lastSeen = now
	flow.cond.Broadcast()
	finished := flow.finishedLocked()
	flow.mu.Unlock()
	if finished {
		_ = flow.tunnel.Send(Frame{Kind: KindTCPClose, FlowID: flow.id}, now)
		c.remove(flow)
	}
	return nil
}

func (c *TCPClient) handleData(flow *tcpClientFlow, frame Frame, now time.Time) error {
	flow.mu.Lock()
	if flow.closed {
		flow.mu.Unlock()
		return ErrClosed
	}
	result, err := flow.rx.Push(frame)
	if err == nil {
		flow.lastSeen = now
	}
	finished := err == nil && flow.finishedLocked()
	flow.mu.Unlock()
	if err != nil {
		return err
	}
	if len(result.Delivered) > 0 {
		if err := writeFull(flow.conn, result.Delivered); err != nil {
			c.abort(flow, now)
			return err
		}
	}
	if result.FIN && !result.Duplicate {
		if cw, ok := flow.conn.(interface{ CloseWrite() error }); ok {
			if err := cw.CloseWrite(); err != nil {
				c.abort(flow, now)
				return err
			}
		}
	}
	if err := flow.tunnel.Send(result.Ack, now); err != nil {
		c.abort(flow, now)
		return err
	}
	if finished {
		_ = flow.tunnel.Send(Frame{Kind: KindTCPClose, FlowID: flow.id}, now)
		c.remove(flow)
	}
	return nil
}

func (c *TCPClient) readLocal(flow *tcpClientFlow) {
	flow.mu.Lock()
	for !flow.opened && !flow.closed {
		flow.cond.Wait()
	}
	closed := flow.closed
	flow.mu.Unlock()
	if closed {
		return
	}
	buf := make([]byte, c.cfg.Reliability.ChunkSize)
	for {
		n, err := flow.conn.Read(buf)
		if n > 0 {
			if qerr := c.queueLocal(flow, append([]byte(nil), buf[:n]...), false, time.Now()); qerr != nil {
				c.abort(flow, time.Now())
				return
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			if qerr := c.queueLocal(flow, nil, true, time.Now()); qerr != nil && !errors.Is(qerr, ErrClosed) {
				c.abort(flow, time.Now())
			}
		} else if !errors.Is(err, net.ErrClosed) {
			c.abort(flow, time.Now())
		}
		return
	}
}

func (c *TCPClient) queueLocal(flow *tcpClientFlow, data []byte, fin bool, now time.Time) error {
	flow.mu.Lock()
	for {
		if flow.closed {
			flow.mu.Unlock()
			return ErrClosed
		}
		frames, err := flow.tx.Queue(data, fin, now)
		if errors.Is(err, ErrWindowFull) {
			flow.cond.Wait()
			now = time.Now()
			continue
		}
		if err != nil {
			flow.mu.Unlock()
			return err
		}
		flow.lastSeen = now
		flow.mu.Unlock()
		for _, frame := range frames {
			if err := flow.tunnel.Send(frame, now); err != nil {
				return err
			}
		}
		return nil
	}
}

func (c *TCPClient) Tick(now time.Time) {
	for _, flow := range c.snapshot() {
		flow.mu.Lock()
		if flow.closed {
			flow.mu.Unlock()
			continue
		}
		idle := !now.Before(flow.lastSeen) && now.Sub(flow.lastSeen) >= c.cfg.IdleTimeout
		if !flow.opened {
			if idle {
				flow.mu.Unlock()
				c.abort(flow, now)
				continue
			}
			if now.Before(flow.openSent) || now.Sub(flow.openSent) < c.cfg.OpenRTO {
				flow.mu.Unlock()
				continue
			}
			if flow.openRetries >= c.cfg.MaxOpenRetransmit {
				flow.mu.Unlock()
				c.abort(flow, now)
				continue
			}
			flow.openRetries++
			flow.openSent = now
			target := flow.target
			id := flow.id
			flow.mu.Unlock()
			if err := flow.tunnel.Send(Frame{Kind: KindTCPOpen, FlowID: id, Peer: target}, now); err != nil {
				c.abort(flow, now)
			}
			continue
		}
		due, err := flow.tx.RetransmitDue(now)
		flow.mu.Unlock()
		if err != nil || idle {
			c.abort(flow, now)
			continue
		}
		for _, frame := range due {
			if err := flow.tunnel.Send(frame, now); err != nil {
				c.abort(flow, now)
				break
			}
		}
	}
}

func (c *TCPClient) Close() {
	for _, flow := range c.snapshot() {
		c.remove(flow)
	}
}

func (c *TCPClient) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.flows)
}

func (c *TCPClient) abort(flow *tcpClientFlow, now time.Time) {
	_ = flow.tunnel.Send(Frame{Kind: KindTCPClose, FlowID: flow.id}, now)
	c.remove(flow)
}

func (c *TCPClient) allocateIDLocked() (uint64, error) {
	for attempts := 0; attempts <= len(c.flows); attempts++ {
		c.next++
		if c.next == 0 {
			c.next++
		}
		if c.flows[c.next] == nil {
			return c.next, nil
		}
	}
	return 0, ErrLimit
}

func (c *TCPClient) get(id uint64) *tcpClientFlow {
	c.mu.Lock()
	flow := c.flows[id]
	c.mu.Unlock()
	return flow
}

func (c *TCPClient) snapshot() []*tcpClientFlow {
	c.mu.Lock()
	out := make([]*tcpClientFlow, 0, len(c.flows))
	for _, flow := range c.flows {
		out = append(out, flow)
	}
	c.mu.Unlock()
	return out
}

func (c *TCPClient) remove(want *tcpClientFlow) {
	c.mu.Lock()
	if c.flows[want.id] == want {
		delete(c.flows, want.id)
	}
	c.mu.Unlock()
	want.close()
}

type tcpServerFlow struct {
	id       uint64
	target   netip.AddrPort
	conn     net.Conn
	tunnel   *TunnelFlow
	tx       *TCPTransmit
	rx       *TCPReceive
	lastSeen time.Time

	mu     sync.Mutex
	cond   *sync.Cond
	closed bool
}

func (f *tcpServerFlow) finishedLocked() bool {
	return f.tx.FINAcked() && f.rx.FINDelivered()
}

func (f *tcpServerFlow) close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	f.tx.Abort()
	f.rx.Close()
	f.cond.Broadcast()
	conn := f.conn
	tunnel := f.tunnel
	f.mu.Unlock()
	_ = conn.Close()
	_ = tunnel.Close()
}

type TCPServer struct {
	channel *TunnelChannel
	cfg     TCPConfig

	mu    sync.Mutex
	flows map[uint64]*tcpServerFlow
	dial  func(context.Context, string, string) (net.Conn, error)
}

func NewTCPServer(channel *TunnelChannel, cfg TCPConfig) (*TCPServer, error) {
	if channel == nil {
		return nil, ErrMalformed
	}
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	dialer := &net.Dialer{Timeout: cfg.DialTimeout}
	return &TCPServer{
		channel: channel, cfg: cfg, flows: make(map[uint64]*tcpServerFlow),
		dial: dialer.DialContext,
	}, nil
}

func (s *TCPServer) Handle(frame Frame, now time.Time) error {
	if frame.FlowID == 0 {
		return ErrMalformed
	}
	switch frame.Kind {
	case KindTCPOpen:
		return s.handleOpen(frame, now)
	case KindTCPData, KindTCPAck, KindTCPClose:
	default:
		return ErrUnsupported
	}
	flow := s.get(frame.FlowID)
	if flow == nil {
		if frame.Kind == KindTCPClose {
			return nil
		}
		return fmt.Errorf("%w: unknown TCP server flow", ErrMalformed)
	}
	switch frame.Kind {
	case KindTCPData:
		return s.handleData(flow, frame, now)
	case KindTCPAck:
		return s.handleAck(flow, frame, now)
	case KindTCPClose:
		s.remove(flow)
		return nil
	}
	return ErrUnsupported
}

func (s *TCPServer) handleOpen(frame Frame, now time.Time) error {
	if !validEndpoint(frame.Peer) {
		return ErrMalformed
	}
	s.mu.Lock()
	if current := s.flows[frame.FlowID]; current != nil {
		if current.target != frame.Peer {
			s.mu.Unlock()
			return ErrMalformed
		}
		current.mu.Lock()
		current.lastSeen = now
		current.mu.Unlock()
		tunnel := current.tunnel
		s.mu.Unlock()
		return tunnel.Send(Frame{Kind: KindTCPAck, FlowID: frame.FlowID, Offset: 0}, now)
	}
	if len(s.flows) >= s.cfg.MaxFlows {
		s.mu.Unlock()
		return ErrLimit
	}
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.DialTimeout)
	conn, err := s.dial(ctx, networkFor(frame.Peer.Addr(), "tcp"), frame.Peer.String())
	cancel()
	if err != nil {
		return err
	}
	tunnel, err := s.channel.OpenFlow()
	if err != nil {
		_ = conn.Close()
		return err
	}
	tx, err := NewTCPTransmit(frame.FlowID, s.cfg.Reliability)
	if err != nil {
		_ = conn.Close()
		_ = tunnel.Close()
		return err
	}
	rx, err := NewTCPReceive(frame.FlowID, s.cfg.Reliability)
	if err != nil {
		_ = conn.Close()
		_ = tunnel.Close()
		return err
	}
	flow := &tcpServerFlow{
		id: frame.FlowID, target: frame.Peer, conn: conn, tunnel: tunnel,
		tx: tx, rx: rx, lastSeen: now,
	}
	flow.cond = sync.NewCond(&flow.mu)

	s.mu.Lock()
	if existing := s.flows[frame.FlowID]; existing != nil {
		s.mu.Unlock()
		flow.close()
		if existing.target != frame.Peer {
			return ErrMalformed
		}
		return existing.tunnel.Send(Frame{Kind: KindTCPAck, FlowID: frame.FlowID, Offset: 0}, now)
	}
	s.flows[frame.FlowID] = flow
	s.mu.Unlock()

	if err := tunnel.Send(Frame{Kind: KindTCPAck, FlowID: frame.FlowID, Offset: 0}, now); err != nil {
		s.remove(flow)
		return err
	}
	go s.readUpstream(flow)
	return nil
}

func (s *TCPServer) handleData(flow *tcpServerFlow, frame Frame, now time.Time) error {
	flow.mu.Lock()
	if flow.closed {
		flow.mu.Unlock()
		return ErrClosed
	}
	result, err := flow.rx.Push(frame)
	if err == nil {
		flow.lastSeen = now
	}
	finished := err == nil && flow.finishedLocked()
	flow.mu.Unlock()
	if err != nil {
		return err
	}
	if len(result.Delivered) > 0 {
		if err := writeFull(flow.conn, result.Delivered); err != nil {
			s.abort(flow, now)
			return err
		}
	}
	if result.FIN && !result.Duplicate {
		if cw, ok := flow.conn.(interface{ CloseWrite() error }); ok {
			if err := cw.CloseWrite(); err != nil {
				s.abort(flow, now)
				return err
			}
		}
	}
	if err := flow.tunnel.Send(result.Ack, now); err != nil {
		s.abort(flow, now)
		return err
	}
	if finished {
		_ = flow.tunnel.Send(Frame{Kind: KindTCPClose, FlowID: flow.id}, now)
		s.remove(flow)
	}
	return nil
}

func (s *TCPServer) handleAck(flow *tcpServerFlow, frame Frame, now time.Time) error {
	flow.mu.Lock()
	if flow.closed {
		flow.mu.Unlock()
		return ErrClosed
	}
	if err := flow.tx.Ack(frame.Offset); err != nil {
		flow.mu.Unlock()
		return err
	}
	flow.lastSeen = now
	flow.cond.Broadcast()
	finished := flow.finishedLocked()
	flow.mu.Unlock()
	if finished {
		_ = flow.tunnel.Send(Frame{Kind: KindTCPClose, FlowID: flow.id}, now)
		s.remove(flow)
	}
	return nil
}

func (s *TCPServer) readUpstream(flow *tcpServerFlow) {
	buf := make([]byte, s.cfg.Reliability.ChunkSize)
	for {
		n, err := flow.conn.Read(buf)
		if n > 0 {
			if qerr := s.queueUpstream(flow, append([]byte(nil), buf[:n]...), false, time.Now()); qerr != nil {
				s.abort(flow, time.Now())
				return
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			if qerr := s.queueUpstream(flow, nil, true, time.Now()); qerr != nil && !errors.Is(qerr, ErrClosed) {
				s.abort(flow, time.Now())
			}
		} else if !errors.Is(err, net.ErrClosed) {
			s.abort(flow, time.Now())
		}
		return
	}
}

func (s *TCPServer) queueUpstream(flow *tcpServerFlow, data []byte, fin bool, now time.Time) error {
	flow.mu.Lock()
	for {
		if flow.closed {
			flow.mu.Unlock()
			return ErrClosed
		}
		frames, err := flow.tx.Queue(data, fin, now)
		if errors.Is(err, ErrWindowFull) {
			flow.cond.Wait()
			now = time.Now()
			continue
		}
		if err != nil {
			flow.mu.Unlock()
			return err
		}
		flow.lastSeen = now
		flow.mu.Unlock()
		for _, frame := range frames {
			if err := flow.tunnel.Send(frame, now); err != nil {
				return err
			}
		}
		return nil
	}
}

func (s *TCPServer) Tick(now time.Time) {
	for _, flow := range s.snapshot() {
		flow.mu.Lock()
		if flow.closed {
			flow.mu.Unlock()
			continue
		}
		idle := !now.Before(flow.lastSeen) && now.Sub(flow.lastSeen) >= s.cfg.IdleTimeout
		due, err := flow.tx.RetransmitDue(now)
		flow.mu.Unlock()
		if err != nil || idle {
			s.abort(flow, now)
			continue
		}
		for _, frame := range due {
			if err := flow.tunnel.Send(frame, now); err != nil {
				s.abort(flow, now)
				break
			}
		}
	}
}

func (s *TCPServer) Close() {
	for _, flow := range s.snapshot() {
		s.remove(flow)
	}
}

func (s *TCPServer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.flows)
}

func (s *TCPServer) abort(flow *tcpServerFlow, now time.Time) {
	_ = flow.tunnel.Send(Frame{Kind: KindTCPClose, FlowID: flow.id}, now)
	s.remove(flow)
}

func (s *TCPServer) get(id uint64) *tcpServerFlow {
	s.mu.Lock()
	flow := s.flows[id]
	s.mu.Unlock()
	return flow
}

func (s *TCPServer) snapshot() []*tcpServerFlow {
	s.mu.Lock()
	out := make([]*tcpServerFlow, 0, len(s.flows))
	for _, flow := range s.flows {
		out = append(out, flow)
	}
	s.mu.Unlock()
	return out
}

func (s *TCPServer) remove(want *tcpServerFlow) {
	s.mu.Lock()
	if s.flows[want.id] == want {
		delete(s.flows, want.id)
	}
	s.mu.Unlock()
	want.close()
}

func networkFor(addr netip.Addr, prefix string) string {
	if addr.Unmap().Is4() {
		return prefix + "4"
	}
	return prefix + "6"
}

func writeFull(conn net.Conn, data []byte) error {
	for len(data) > 0 {
		n, err := conn.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrUnexpectedEOF
		}
		data = data[n:]
	}
	return nil
}
