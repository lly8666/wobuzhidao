package platformproxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

const defaultUDPIdleTimeout = 60 * time.Second

type udpFlowKey struct {
	servicePeer string
	flowID      uint64
}

type udpFlow struct {
	key         udpFlowKey
	servicePeer *net.UDPAddr
	target      netip.AddrPort
	upstream    *net.UDPConn

	mu       sync.Mutex
	lastSeen time.Time
	closed   bool
}

func (f *udpFlow) touch(now time.Time) {
	f.mu.Lock()
	f.lastSeen = now
	f.mu.Unlock()
}

func (f *udpFlow) idleFor(now time.Time) time.Duration {
	f.mu.Lock()
	last := f.lastSeen
	f.mu.Unlock()
	if last.IsZero() || now.Before(last) {
		return 0
	}
	return now.Sub(last)
}

func (f *udpFlow) close() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.closed = true
	f.mu.Unlock()
	_ = f.upstream.Close()
}

// UDPRelay serves UDPDatagram platform frames received from wbd-link-server-mux.
//
// wbd-link-server-mux creates one connected local UDP service socket per LiveID,
// so the relay intentionally keys state by (service source address, FlowID).
// FlowID alone is not globally unique and must never be used to route across
// independent sessions.
type UDPRelay struct {
	conn        *net.UDPConn
	idleTimeout time.Duration

	mu    sync.Mutex
	flows map[udpFlowKey]*udpFlow
}

func NewUDPRelay(conn *net.UDPConn, idleTimeout time.Duration) (*UDPRelay, error) {
	if conn == nil {
		return nil, errors.New("platformproxy: nil UDP relay socket")
	}
	if idleTimeout <= 0 {
		idleTimeout = defaultUDPIdleTimeout
	}
	return &UDPRelay{conn: conn, idleTimeout: idleTimeout, flows: make(map[udpFlowKey]*udpFlow)}, nil
}

func (r *UDPRelay) Serve(ctx context.Context) error {
	if ctx == nil {
		return errors.New("platformproxy: nil context")
	}
	buf := make([]byte, 65535)
	for {
		_ = r.conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond))
		n, servicePeer, err := r.conn.ReadFromUDP(buf)
		now := time.Now()
		if err != nil {
			if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
				select {
				case <-ctx.Done():
					r.closeAll()
					return ctx.Err()
				default:
					r.closeAll()
					return err
				}
			}
		} else {
			if err := r.handle(servicePeer, buf[:n], now); err != nil {
				// Malformed or stale application frames are isolated to that datagram.
				// The WBD session itself remains alive.
				continue
			}
		}
		r.expire(now)
		select {
		case <-ctx.Done():
			r.closeAll()
			return ctx.Err()
		default:
		}
	}
}

func (r *UDPRelay) handle(servicePeer *net.UDPAddr, packet []byte, now time.Time) error {
	frame, err := Unmarshal(packet)
	if err != nil {
		return err
	}
	if frame.Kind != KindUDPDatagram {
		return fmt.Errorf("%w: UDP relay kind=%d", ErrUnsupported, frame.Kind)
	}
	flow, err := r.flowFor(servicePeer, frame.FlowID, frame.Peer, now)
	if err != nil {
		return err
	}
	flow.touch(now)
	_, err = flow.upstream.Write(frame.Payload)
	return err
}

func (r *UDPRelay) flowFor(servicePeer *net.UDPAddr, flowID uint64, target netip.AddrPort, now time.Time) (*udpFlow, error) {
	if servicePeer == nil || flowID == 0 || !target.IsValid() {
		return nil, fmt.Errorf("%w: invalid UDP flow identity", ErrMalformed)
	}
	key := udpFlowKey{servicePeer: servicePeer.String(), flowID: flowID}

	r.mu.Lock()
	if existing := r.flows[key]; existing != nil {
		r.mu.Unlock()
		if existing.target != target {
			return nil, fmt.Errorf("%w: UDP flow target changed", ErrMalformed)
		}
		return existing, nil
	}

	targetAddr := net.UDPAddrFromAddrPort(target)
	upstream, err := net.DialUDP(networkFor(target.Addr()), nil, targetAddr)
	if err != nil {
		r.mu.Unlock()
		return nil, err
	}
	flow := &udpFlow{
		key: key, servicePeer: cloneUDPAddr(servicePeer), target: target,
		upstream: upstream, lastSeen: now,
	}
	r.flows[key] = flow
	r.mu.Unlock()

	go r.readUpstream(flow)
	return flow, nil
}

func (r *UDPRelay) readUpstream(flow *udpFlow) {
	buf := make([]byte, 65535)
	for {
		n, err := flow.upstream.Read(buf)
		if err != nil {
			r.remove(flow.key, flow)
			return
		}
		now := time.Now()
		flow.touch(now)
		frame, err := Marshal(Frame{
			Kind: KindUDPDatagram, FlowID: flow.key.flowID,
			Peer: flow.target, Payload: buf[:n],
		})
		if err != nil {
			continue
		}
		if _, err := r.conn.WriteToUDP(frame, flow.servicePeer); err != nil {
			r.remove(flow.key, flow)
			return
		}
	}
}

func (r *UDPRelay) expire(now time.Time) {
	var stale []*udpFlow
	r.mu.Lock()
	for key, flow := range r.flows {
		if flow.idleFor(now) >= r.idleTimeout {
			delete(r.flows, key)
			stale = append(stale, flow)
		}
	}
	r.mu.Unlock()
	for _, flow := range stale {
		flow.close()
	}
}

func (r *UDPRelay) remove(key udpFlowKey, want *udpFlow) {
	r.mu.Lock()
	if r.flows[key] == want {
		delete(r.flows, key)
	}
	r.mu.Unlock()
	want.close()
}

func (r *UDPRelay) closeAll() {
	r.mu.Lock()
	flows := make([]*udpFlow, 0, len(r.flows))
	for key, flow := range r.flows {
		delete(r.flows, key)
		flows = append(flows, flow)
	}
	r.mu.Unlock()
	for _, flow := range flows {
		flow.close()
	}
}

func networkFor(addr netip.Addr) string {
	if addr.Unmap().Is4() {
		return "udp4"
	}
	return "udp6"
}

func cloneUDPAddr(in *net.UDPAddr) *net.UDPAddr {
	if in == nil {
		return nil
	}
	out := *in
	if in.IP != nil {
		out.IP = append(net.IP(nil), in.IP...)
	}
	return &out
}
