package platformproxy

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"sync"
	"time"
)

const defaultTCPClientIdleTimeout = 90 * time.Second

type TCPClientConfig struct {
	Reliability       TCPReliabilityConfig
	IdleTimeout       time.Duration
	OpenRTO           time.Duration
	MaxOpenRetransmit int
}

func DefaultTCPClientConfig() TCPClientConfig {
	return TCPClientConfig{
		Reliability:       DefaultTCPReliabilityConfig(),
		IdleTimeout:       defaultTCPClientIdleTimeout,
		OpenRTO:           500 * time.Millisecond,
		MaxOpenRetransmit: 8,
	}
}

type tcpClientFlow struct {
	id     uint64
	target netip.AddrPort
	conn   net.Conn
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
	f.cond.Broadcast()
	f.mu.Unlock()
	_ = f.conn.Close()
}

// TCPClient owns transparent client-side platform TCP flows. It converts one
// accepted kernel TCP connection into reliable WBDP DATA/ACK datagrams without
// changing the WBD transport below it. The callback must only enqueue/send one
// complete WBDP frame and may be called concurrently by independent flows.
type TCPClient struct {
	cfg  TCPClientConfig
	send func(Frame) error

	mu    sync.Mutex
	next  uint64
	flows map[uint64]*tcpClientFlow
}

func NewTCPClient(send func(Frame) error, cfg TCPClientConfig) (*TCPClient, error) {
	if send == nil {
		return nil, errors.New("platformproxy: nil TCP client frame sender")
	}
	if cfg.Reliability.ChunkSize == 0 {
		cfg.Reliability = DefaultTCPReliabilityConfig()
	}
	if err := cfg.Reliability.validate(); err != nil {
		return nil, err
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = defaultTCPClientIdleTimeout
	}
	if cfg.OpenRTO <= 0 {
		cfg.OpenRTO = cfg.Reliability.RTO
	}
	if cfg.MaxOpenRetransmit < 0 {
		return nil, fmt.Errorf("%w: negative TCP OPEN retransmit limit", ErrMalformed)
	}
	return &TCPClient{cfg: cfg, send: send, flows: make(map[uint64]*tcpClientFlow)}, nil
}

func (c *TCPClient) Add(conn net.Conn, target netip.AddrPort, now time.Time) (uint64, error) {
	if conn == nil || !validTCPRelayEndpoint(target) {
		return 0, fmt.Errorf("%w: invalid transparent TCP client flow", ErrMalformed)
	}
	id, err := c.allocateID()
	if err != nil {
		return 0, err
	}
	tx, err := NewTCPTransmit(id, c.cfg.Reliability)
	if err != nil {
		return 0, err
	}
	rx, err := NewTCPReceive(id, c.cfg.Reliability)
	if err != nil {
		return 0, err
	}
	flow := &tcpClientFlow{
		id: id, target: target, conn: conn, tx: tx, rx: rx,
		openSent: now, openRetries: 0, lastSeen: now,
	}
	flow.cond = sync.NewCond(&flow.mu)
	c.mu.Lock()
	c.flows[id] = flow
	c.mu.Unlock()

	if err := c.send(Frame{Kind: KindTCPOpen, FlowID: id, Peer: target}); err != nil {
		c.remove(id, flow)
		return 0, err
	}
	go c.readLocal(flow)
	return id, nil
}

func (c *TCPClient) HandleFrame(frame Frame, now time.Time) error {
	if frame.FlowID == 0 {
		return fmt.Errorf("%w: zero TCP client flow id", ErrMalformed)
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
		c.remove(flow.id, flow)
		return nil
	default:
		return fmt.Errorf("%w: TCP client kind=%d", ErrUnsupported, frame.Kind)
	}
}

func (c *TCPClient) handleAck(flow *tcpClientFlow, frame Frame, now time.Time) error {
	flow.mu.Lock()
	if flow.closed {
		flow.mu.Unlock()
		return ErrTCPClosed
	}
	if !flow.opened {
		if frame.Offset != 0 {
			flow.mu.Unlock()
			return fmt.Errorf("%w: TCP establishment ACK=%d", ErrMalformed, frame.Offset)
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
		_ = c.send(Frame{Kind: KindTCPClose, FlowID: flow.id})
		c.remove(flow.id, flow)
	}
	return nil
}

func (c *TCPClient) handleData(flow *tcpClientFlow, frame Frame, now time.Time) error {
	flow.mu.Lock()
	if flow.closed {
		flow.mu.Unlock()
		return ErrTCPClosed
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
			_ = c.send(Frame{Kind: KindTCPClose, FlowID: flow.id})
			c.remove(flow.id, flow)
			return err
		}
	}
	if result.FIN && !result.Duplicate {
		if cw, ok := flow.conn.(interface{ CloseWrite() error }); ok {
			if err := cw.CloseWrite(); err != nil {
				c.remove(flow.id, flow)
				return err
			}
		}
	}
	if err := c.send(result.Ack); err != nil {
		c.remove(flow.id, flow)
		return err
	}
	if finished {
		_ = c.send(Frame{Kind: KindTCPClose, FlowID: flow.id})
		c.remove(flow.id, flow)
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
				c.abort(flow)
				return
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			if qerr := c.queueLocal(flow, nil, true, time.Now()); qerr != nil && !errors.Is(qerr, ErrTCPClosed) {
				c.abort(flow)
			}
		} else if !errors.Is(err, net.ErrClosed) {
			c.abort(flow)
		}
		return
	}
}

func (c *TCPClient) queueLocal(flow *tcpClientFlow, data []byte, fin bool, now time.Time) error {
	flow.mu.Lock()
	for {
		if flow.closed {
			flow.mu.Unlock()
			return ErrTCPClosed
		}
		frames, err := flow.tx.Queue(data, fin, now)
		if errors.Is(err, ErrTCPWindowFull) {
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
			if err := c.send(frame); err != nil {
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
		idle := !flow.lastSeen.IsZero() && !now.Before(flow.lastSeen) && now.Sub(flow.lastSeen) >= c.cfg.IdleTimeout
		if !flow.opened {
			if idle {
				flow.mu.Unlock()
				c.abort(flow)
				continue
			}
			if now.Before(flow.openSent) || now.Sub(flow.openSent) < c.cfg.OpenRTO {
				flow.mu.Unlock()
				continue
			}
			if flow.openRetries >= c.cfg.MaxOpenRetransmit {
				flow.mu.Unlock()
				c.abort(flow)
				continue
			}
			flow.openRetries++
			flow.openSent = now
			flow.mu.Unlock()
			if err := c.send(Frame{Kind: KindTCPOpen, FlowID: flow.id, Peer: flow.target}); err != nil {
				c.abort(flow)
			}
			continue
		}
		due, err := flow.tx.RetransmitDue(now)
		flow.mu.Unlock()
		if err != nil || idle {
			c.abort(flow)
			continue
		}
		for _, frame := range due {
			if err := c.send(frame); err != nil {
				c.abort(flow)
				break
			}
		}
	}
}

func (c *TCPClient) Close() {
	for _, flow := range c.snapshot() {
		c.remove(flow.id, flow)
	}
}

func (c *TCPClient) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.flows)
}

func (c *TCPClient) abort(flow *tcpClientFlow) {
	_ = c.send(Frame{Kind: KindTCPClose, FlowID: flow.id})
	c.remove(flow.id, flow)
}

func (c *TCPClient) allocateID() (uint64, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for attempts := 0; attempts <= len(c.flows); attempts++ {
		c.next++
		if c.next == 0 {
			c.next++
		}
		if c.flows[c.next] == nil {
			return c.next, nil
		}
	}
	return 0, fmt.Errorf("%w: TCP client flow id space exhausted", ErrLimit)
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

func (c *TCPClient) remove(id uint64, want *tcpClientFlow) {
	c.mu.Lock()
	if c.flows[id] == want {
		delete(c.flows, id)
	}
	c.mu.Unlock()
	want.close()
}
