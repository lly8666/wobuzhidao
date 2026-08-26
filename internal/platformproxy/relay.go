package platformproxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

const defaultRelayTCPQueue = 64

type RelayConfig struct {
	UDPIdleTimeout time.Duration
	TCP            TCPRelayConfig
	TCPQueue       int
}

func DefaultRelayConfig() RelayConfig {
	return RelayConfig{
		UDPIdleTimeout: defaultUDPIdleTimeout,
		TCP:            DefaultTCPRelayConfig(),
		TCPQueue:       defaultRelayTCPQueue,
	}
}

// Relay is the single service-socket dispatcher used behind
// wbd-link-server-mux. UDP and TCP deliberately share one UDP socket so the
// existing per-LiveID service source peer remains part of every flow key.
//
// TCP frames are serialized per (service peer, FlowID), but different flows use
// independent workers. A slow TCP dial or upstream write therefore cannot hold
// the service socket reader and cannot block unrelated UDP/TCP flows.
type Relay struct {
	conn *net.UDPConn
	udp  *UDPRelay
	tcp  *TCPRelay
	cfg  RelayConfig

	mu      sync.Mutex
	workers map[tcpRelayKey]*relayTCPWorker
	stop    chan struct{}
	once    sync.Once
	wg      sync.WaitGroup
}

type relayTCPWorker struct {
	key  tcpRelayKey
	peer *net.UDPAddr
	ch   chan Frame
}

func NewRelay(conn *net.UDPConn, cfg RelayConfig) (*Relay, error) {
	if conn == nil {
		return nil, errors.New("platformproxy: nil relay socket")
	}
	if cfg.UDPIdleTimeout <= 0 {
		cfg.UDPIdleTimeout = defaultUDPIdleTimeout
	}
	if cfg.TCPQueue <= 0 {
		cfg.TCPQueue = defaultRelayTCPQueue
	}
	if cfg.TCP.Reliability.ChunkSize == 0 {
		cfg.TCP = DefaultTCPRelayConfig()
	}
	udpRelay, err := NewUDPRelay(conn, cfg.UDPIdleTimeout)
	if err != nil {
		return nil, err
	}
	tcpRelay, err := NewTCPRelay(conn, cfg.TCP)
	if err != nil {
		return nil, err
	}
	return &Relay{
		conn: conn, udp: udpRelay, tcp: tcpRelay, cfg: cfg,
		workers: make(map[tcpRelayKey]*relayTCPWorker),
		stop:    make(chan struct{}),
	}, nil
}

func (r *Relay) Serve(ctx context.Context) error {
	if ctx == nil {
		return errors.New("platformproxy: nil context")
	}
	buf := make([]byte, 65535)
	for {
		_ = r.conn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
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
				_ = r.dispatch(servicePeer, frame, now)
			}
		}

		r.udp.Tick(now)
		r.tcp.Tick(now)
		select {
		case <-ctx.Done():
			r.Close()
			return ctx.Err()
		default:
		}
	}
}

func (r *Relay) dispatch(servicePeer *net.UDPAddr, frame Frame, now time.Time) error {
	switch frame.Kind {
	case KindUDPDatagram:
		return r.udp.HandleFrame(servicePeer, frame, now)
	case KindTCPOpen, KindTCPData, KindTCPAck, KindTCPClose:
		return r.dispatchTCP(servicePeer, frame)
	default:
		return fmt.Errorf("%w: platform relay kind=%d", ErrUnsupported, frame.Kind)
	}
}

func (r *Relay) dispatchTCP(servicePeer *net.UDPAddr, frame Frame) error {
	if servicePeer == nil || frame.FlowID == 0 {
		return fmt.Errorf("%w: invalid relay TCP identity", ErrMalformed)
	}
	key := tcpRelayKey{servicePeer: servicePeer.String(), flowID: frame.FlowID}

	r.mu.Lock()
	worker := r.workers[key]
	if worker == nil {
		worker = &relayTCPWorker{
			key: key, peer: cloneUDPAddr(servicePeer),
			ch: make(chan Frame, r.cfg.TCPQueue),
		}
		r.workers[key] = worker
		r.wg.Add(1)
		go r.runTCPWorker(worker)
	}
	r.mu.Unlock()

	select {
	case worker.ch <- frame:
		return nil
	default:
		// WBD is a datagram transport: dropping here is equivalent to datagram
		// loss. The platform TCP sender will retransmit because its cumulative
		// ACK does not advance. Do not block the global service reader.
		return fmt.Errorf("%w: TCP relay worker queue full", ErrLimit)
	}
}

func (r *Relay) runTCPWorker(worker *relayTCPWorker) {
	defer r.wg.Done()
	idle := r.cfg.TCP.IdleTimeout + r.cfg.TCP.DialTimeout
	if idle < time.Second {
		idle = time.Second
	}
	timer := time.NewTimer(idle)
	defer timer.Stop()
	for {
		select {
		case frame := <-worker.ch:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			_ = r.tcp.HandleFrame(worker.peer, frame, time.Now())
			if frame.Kind == KindTCPClose {
				r.removeTCPWorker(worker)
				return
			}
			timer.Reset(idle)
		case <-timer.C:
			r.removeTCPWorker(worker)
			return
		case <-r.stop:
			return
		}
	}
}

func (r *Relay) removeTCPWorker(worker *relayTCPWorker) {
	r.mu.Lock()
	if r.workers[worker.key] == worker {
		delete(r.workers, worker.key)
	}
	r.mu.Unlock()
}

func (r *Relay) Close() {
	r.once.Do(func() {
		close(r.stop)
		r.tcp.Close()
		r.udp.Close()
		r.wg.Wait()
		r.mu.Lock()
		clear(r.workers)
		r.mu.Unlock()
	})
}
