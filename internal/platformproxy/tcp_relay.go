package platformproxy

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

const defaultTCPRelayIdleTimeout = 90 * time.Second
const defaultTCPRelayDialTimeout = 10 * time.Second

type TCPRelayConfig struct {
	Reliability TCPReliabilityConfig
	IdleTimeout time.Duration
	DialTimeout time.Duration
}

func DefaultTCPRelayConfig() TCPRelayConfig {
	return TCPRelayConfig{
		Reliability: DefaultTCPReliabilityConfig(),
		IdleTimeout: defaultTCPRelayIdleTimeout,
		DialTimeout: defaultTCPRelayDialTimeout,
	}
}

type tcpRelayKey struct {
	servicePeer string
	flowID      uint64
}

type tcpRelayFlow struct {
	key         tcpRelayKey
	servicePeer *net.UDPAddr
	target      netip.AddrPort
	upstream    net.Conn
	tx          *TCPTransmit
	rx          *TCPReceive

	mu       sync.Mutex
	cond     *sync.Cond
	lastSeen time.Time
	closed   bool
}

func (f *tcpRelayFlow) touchLocked(now time.Time) {
	f.lastSeen = now
}

func (f *tcpRelayFlow) finishedLocked() bool {
	return f.tx.FINAcked() && f.rx.FINDelivered()
}

func (f *tcpRelayFlow) close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	f.cond.Broadcast()
	f.mu.Unlock()
	_ = f.upstream.Close()
}

// TCPRelay owns server-side platform TCP egress. wbd-link-server-mux gives
// every LiveID a distinct connected service UDP peer, so FlowID is scoped by
// that peer exactly as it is for UDP. The same numeric FlowID in two LiveIDs
// therefore cannot share TCP reliability or upstream socket state.
//
// HandleFrame is expected to be serialized by the platform-server datagram
// dispatcher. Per-flow upstream readers and Tick may run concurrently; each
// flow protects its TCPTransmit/TCPReceive state independently.
type TCPRelay struct {
	conn *net.UDPConn
	cfg  TCPRelayConfig
	dial func(context.Context, string, string) (net.Conn, error)

	mu    sync.Mutex
	flows map[tcpRelayKey]*tcpRelayFlow
}

func NewTCPRelay(conn *net.UDPConn, cfg TCPRelayConfig) (*TCPRelay, error) {
	if conn == nil {
		return nil, errors.New("platformproxy: nil TCP relay socket")
	}
	if cfg.Reliability.ChunkSize == 0 {
		cfg.Reliability = DefaultTCPReliabilityConfig()
	}
	if err := cfg.Reliability.validate(); err != nil {
		return nil, err
	}
	if cfg.IdleTimeout <= 0 {
		cfg.IdleTimeout = defaultTCPRelayIdleTimeout
	}
	if cfg.DialTimeout <= 0 {
		cfg.DialTimeout = defaultTCPRelayDialTimeout
	}
	dialer := &net.Dialer{Timeout: cfg.DialTimeout}
	return &TCPRelay{
		conn: conn,
		cfg:  cfg,
		dial: dialer.DialContext,
		flows: make(map[tcpRelayKey]*tcpRelayFlow),
	}, nil
}

func (r *TCPRelay) HandlePacket(servicePeer *net.UDPAddr, packet []byte, now time.Time) error {
	frame, err := Unmarshal(packet)
	if err != nil {
		return err
	}
	return r.HandleFrame(servicePeer, frame, now)
}

func (r *TCPRelay) HandleFrame(servicePeer *net.UDPAddr, frame Frame, now time.Time) error {
	if servicePeer == nil || frame.FlowID == 0 {
		return fmt.Errorf("%w: invalid TCP relay identity", ErrMalformed)
	}
	if frame.Kind == KindTCPOpen {
		return r.handleOpen(servicePeer, frame, now)
	}
	if frame.Kind != KindTCPData && frame.Kind != KindTCPAck && frame.Kind != KindTCPClose {
		return fmt.Errorf("%w: TCP relay kind=%d", ErrUnsupported, frame.Kind)
	}

	key := tcpRelayKey{servicePeer: servicePeer.String(), flowID: frame.FlowID}
	flow := r.get(key)
	if flow == nil {
		if frame.Kind == KindTCPClose {
			return nil // terminal close is idempotent
		}
		return fmt.Errorf("%w: unknown TCP relay flow", ErrMalformed)
	}
	switch frame.Kind {
	case KindTCPData:
		return r.handleData(flow, frame, now)
	case KindTCPAck:
		return r.handleAck(flow, frame, now)
	case KindTCPClose:
		r.remove(key, flow)
	}
	return nil
}

func (r *TCPRelay) handleOpen(servicePeer *net.UDPAddr, frame Frame, now time.Time) error {
	if !validTCPRelayEndpoint(frame.Peer) {
		return fmt.Errorf("%w: invalid TCP OPEN target", ErrMalformed)
	}
	key := tcpRelayKey{servicePeer: servicePeer.String(), flowID: frame.FlowID}
	if flow := r.get(key); flow != nil {
		flow.mu.Lock()
		same := flow.target == frame.Peer && !flow.closed
		if same {
			flow.touchLocked(now)
		}
		flow.mu.Unlock()
		if !same {
			return fmt.Errorf("%w: TCP OPEN target changed", ErrMalformed)
		}
		// OPEN is idempotent. ACK(0) is the explicit establishment response;
		// a later duplicate ACK(0) is stale and harmless to the byte sender.
		return r.sendFrame(flow.servicePeer, Frame{Kind: KindTCPAck, FlowID: frame.FlowID, Offset: 0})
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.DialTimeout)
	defer cancel()
	upstream, err := r.dial(ctx, tcpNetworkFor(frame.Peer.Addr()), frame.Peer.String())
	if err != nil {
		_ = r.sendFrame(servicePeer, Frame{Kind: KindTCPClose, FlowID: frame.FlowID})
		return err
	}
	tx, err := NewTCPTransmit(frame.FlowID, r.cfg.Reliability)
	if err != nil {
		_ = upstream.Close()
		return err
	}
	rx, err := NewTCPReceive(frame.FlowID, r.cfg.Reliability)
	if err != nil {
		_ = upstream.Close()
		return err
	}
	flow := &tcpRelayFlow{
		key: key, servicePeer: cloneUDPAddr(servicePeer), target: frame.Peer,
		upstream: upstream, tx: tx, rx: rx, lastSeen: now,
	}
	flow.cond = sync.NewCond(&flow.mu)

	r.mu.Lock()
	if existing := r.flows[key]; existing != nil {
		r.mu.Unlock()
		_ = upstream.Close()
		return r.handleOpen(servicePeer, frame, now)
	}
	r.flows[key] = flow
	r.mu.Unlock()

	go r.readUpstream(flow)
	if err := r.sendFrame(flow.servicePeer, Frame{Kind: KindTCPAck, FlowID: frame.FlowID, Offset: 0}); err != nil {
		r.remove(key, flow)
		return err
	}
	return nil
}

func (r *TCPRelay) handleData(flow *tcpRelayFlow, frame Frame, now time.Time) error {
	flow.mu.Lock()
	if flow.closed {
		flow.mu.Unlock()
		return ErrTCPClosed
	}
	result, err := flow.rx.Push(frame)
	if err == nil {
		flow.touchLocked(now)
	}
	finished := err == nil && flow.finishedLocked()
	flow.mu.Unlock()
	if err != nil {
		return err
	}

	if len(result.Delivered) > 0 {
		if err := writeFull(flow.upstream, result.Delivered); err != nil {
			r.remove(flow.key, flow)
			return err
		}
	}
	if result.FIN && !result.Duplicate {
		if cw, ok := flow.upstream.(interface{ CloseWrite() error }); ok {
			if err := cw.CloseWrite(); err != nil {
				r.remove(flow.key, flow)
				return err
			}
		}
	}
	if err := r.sendFrame(flow.servicePeer, result.Ack); err != nil {
		r.remove(flow.key, flow)
		return err
	}
	if finished {
		_ = r.sendFrame(flow.servicePeer, Frame{Kind: KindTCPClose, FlowID: flow.key.flowID})
		r.remove(flow.key, flow)
	}
	return nil
}

func (r *TCPRelay) handleAck(flow *tcpRelayFlow, frame Frame, now time.Time) error {
	flow.mu.Lock()
	if flow.closed {
		flow.mu.Unlock()
		return ErrTCPClosed
	}
	if err := flow.tx.Ack(frame.Offset); err != nil {
		flow.mu.Unlock()
		return err
	}
	flow.touchLocked(now)
	flow.cond.Broadcast()
	finished := flow.finishedLocked()
	flow.mu.Unlock()
	if finished {
		_ = r.sendFrame(flow.servicePeer, Frame{Kind: KindTCPClose, FlowID: flow.key.flowID})
		r.remove(flow.key, flow)
	}
	return nil
}

func (r *TCPRelay) readUpstream(flow *tcpRelayFlow) {
	buf := make([]byte, r.cfg.Reliability.ChunkSize)
	for {
		n, err := flow.upstream.Read(buf)
		if n > 0 {
			if qerr := r.queueOutbound(flow, append([]byte(nil), buf[:n]...), false, time.Now()); qerr != nil {
				r.remove(flow.key, flow)
				return
			}
		}
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			if qerr := r.queueOutbound(flow, nil, true, time.Now()); qerr != nil && !errors.Is(qerr, ErrTCPClosed) {
				r.remove(flow.key, flow)
			}
		} else {
			_ = r.sendFrame(flow.servicePeer, Frame{Kind: KindTCPClose, FlowID: flow.key.flowID})
			r.remove(flow.key, flow)
		}
		return
	}
}

func (r *TCPRelay) queueOutbound(flow *tcpRelayFlow, data []byte, fin bool, now time.Time) error {
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
		flow.touchLocked(now)
		flow.mu.Unlock()
		for _, f := range frames {
			if err := r.sendFrame(flow.servicePeer, f); err != nil {
				return err
			}
		}
		return nil
	}
}

// Tick performs per-flow retransmission and idle reclamation. It never holds a
// global relay lock while touching a TCP reliability window, so one slow flow
// cannot block ACK/retry progress for a different LiveID/FlowID.
func (r *TCPRelay) Tick(now time.Time) {
	for _, flow := range r.snapshot() {
		flow.mu.Lock()
		if flow.closed {
			flow.mu.Unlock()
			continue
		}
		due, err := flow.tx.RetransmitDue(now)
		idle := !flow.lastSeen.IsZero() && !now.Before(flow.lastSeen) && now.Sub(flow.lastSeen) >= r.cfg.IdleTimeout
		flow.mu.Unlock()
		if err != nil || idle {
			_ = r.sendFrame(flow.servicePeer, Frame{Kind: KindTCPClose, FlowID: flow.key.flowID})
			r.remove(flow.key, flow)
			continue
		}
		for _, f := range due {
			if err := r.sendFrame(flow.servicePeer, f); err != nil {
				r.remove(flow.key, flow)
				break
			}
		}
	}
}

func (r *TCPRelay) Close() {
	for _, flow := range r.snapshot() {
		r.remove(flow.key, flow)
	}
}

func (r *TCPRelay) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.flows)
}

func (r *TCPRelay) get(key tcpRelayKey) *tcpRelayFlow {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flows[key]
}

func (r *TCPRelay) snapshot() []*tcpRelayFlow {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*tcpRelayFlow, 0, len(r.flows))
	for _, flow := range r.flows {
		out = append(out, flow)
	}
	return out
}

func (r *TCPRelay) remove(key tcpRelayKey, want *tcpRelayFlow) {
	r.mu.Lock()
	if r.flows[key] == want {
		delete(r.flows, key)
	}
	r.mu.Unlock()
	want.close()
}

func (r *TCPRelay) sendFrame(peer *net.UDPAddr, frame Frame) error {
	wire, err := Marshal(frame)
	if err != nil {
		return err
	}
	_, err = r.conn.WriteToUDP(wire, peer)
	return err
}

func validTCPRelayEndpoint(peer netip.AddrPort) bool {
	return peer.IsValid() && peer.Port() != 0 && !peer.Addr().IsUnspecified()
}

func tcpNetworkFor(addr netip.Addr) string {
	if addr.Unmap().Is4() {
		return "tcp4"
	}
	return "tcp6"
}

func writeFull(w io.Writer, payload []byte) error {
	for len(payload) > 0 {
		n, err := w.Write(payload)
		if err != nil {
			return err
		}
		if n <= 0 {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}
