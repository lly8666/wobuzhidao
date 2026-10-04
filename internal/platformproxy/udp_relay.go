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
	ipv4        bool
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
// Each mapping is keyed only by (LiveID-facing service peer, FlowID). The
// upstream socket is deliberately unconnected, which gives endpoint-independent
// mapping and endpoint-independent filtering inside one address family: one
// mapped source socket may send to many remote endpoints and may receive from
// remote endpoints that were never used as an outbound destination.
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

// Serve is retained for the UDP-only executable/tests. The combined platform
// server uses HandleFrame through Relay so UDP and TCP share the same LiveID
// service socket and therefore the same service-peer isolation boundary.
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
					r.Close()
					return ctx.Err()
				default:
					r.Close()
					return err
				}
			}
		} else {
			frame, ferr := Unmarshal(buf[:n])
			if ferr == nil {
				_ = r.HandleFrame(servicePeer, frame, now)
			}
		}
		r.Tick(now)
		select {
		case <-ctx.Done():
			r.Close()
			return ctx.Err()
		default:
		}
	}
}

func (r *UDPRelay) HandlePacket(servicePeer *net.UDPAddr, packet []byte, now time.Time) error {
	frame, err := Unmarshal(packet)
	if err != nil {
		return err
	}
	return r.HandleFrame(servicePeer, frame, now)
}

func (r *UDPRelay) HandleFrame(servicePeer *net.UDPAddr, frame Frame, now time.Time) error {
	if frame.Kind != KindUDPDatagram {
		return fmt.Errorf("%w: UDP relay kind=%d", ErrUnsupported, frame.Kind)
	}
	flow, err := r.flowFor(servicePeer, frame.FlowID, frame.Peer, now)
	if err != nil {
		return err
	}
	flow.touch(now)
	_, err = flow.upstream.WriteToUDPAddrPort(frame.Payload, frame.Peer)
	return err
}

func (r *UDPRelay) flowFor(servicePeer *net.UDPAddr, flowID uint64, target netip.AddrPort, now time.Time) (*udpFlow, error) {
	if servicePeer == nil || flowID == 0 || !validUDPFlowEndpoint(target) {
		return nil, fmt.Errorf("%w: invalid UDP flow identity", ErrMalformed)
	}
	key := udpFlowKey{servicePeer: servicePeer.String(), flowID: flowID}
	ipv4 := udpAddrIs4(target.Addr())

	r.mu.Lock()
	if existing := r.flows[key]; existing != nil {
		r.mu.Unlock()
		if existing.ipv4 != ipv4 {
			return nil, fmt.Errorf("%w: UDP mapping address family changed", ErrMalformed)
		}
		return existing, nil
	}

	upstream, err := listenUDPMapping(ipv4)
	if err != nil {
		r.mu.Unlock()
		return nil, err
	}
	flow := &udpFlow{
		key: key, servicePeer: cloneUDPAddr(servicePeer), ipv4: ipv4,
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
		n, remote, err := flow.upstream.ReadFromUDPAddrPort(buf)
		if err != nil {
			r.remove(flow.key, flow)
			return
		}
		now := time.Now()
		flow.touch(now)
		frame, err := Marshal(Frame{
			Kind: KindUDPDatagram, FlowID: flow.key.flowID,
			Peer: remote, Payload: buf[:n],
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

func (r *UDPRelay) Tick(now time.Time) {
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

func (r *UDPRelay) Close() {
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

func (r *UDPRelay) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.flows)
}

func listenUDPMapping(ipv4 bool) (*net.UDPConn, error) {
	if ipv4 {
		return net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	}
	return net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6unspecified, Port: 0})
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
