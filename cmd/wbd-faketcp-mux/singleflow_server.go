//go:build linux

package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/singleflow"
)

type serverSingleFlowPhase uint8

const (
	serverSingleFlowBootstrap serverSingleFlowPhase = iota
	serverSingleFlowAwaitSwitch
	serverSingleFlowDatagram
)

type serverSingleFlowState struct {
	mu        sync.RWMutex
	phase     serverSingleFlowPhase
	assembler *singleflow.OrderedAssembler
	inbound   chan []byte
	ticket    realityfront.Ticket
	switchReq chan struct{}
	reqOnce   sync.Once
	startOnce sync.Once
	done      chan struct{}
	doneOnce  sync.Once
}

func newServerSingleFlowState(peerNext uint32) *serverSingleFlowState {
	return &serverSingleFlowState{
		phase: serverSingleFlowBootstrap,
		assembler: singleflow.NewOrderedAssembler(peerNext),
		inbound: make(chan []byte, 128),
		switchReq: make(chan struct{}),
		done: make(chan struct{}),
	}
}

func (sf *serverSingleFlowState) stop() {
	if sf != nil {
		sf.doneOnce.Do(func() { close(sf.done) })
	}
}

func (s *muxServer) singleFlowEnabled() bool { return s.front != nil }

func (s *muxServer) startSingleFlowBootstrap(sess *muxSession) {
	if !s.singleFlowEnabled() || sess == nil || sess.sf == nil {
		return
	}
	sess.sf.startOnce.Do(func() {
		go s.runSingleFlowBootstrap(sess)
	})
}

func (s *muxServer) runSingleFlowBootstrap(sess *muxSession) {
	app, carrier := net.Pipe()
	defer app.Close()
	defer carrier.Close()

	pumpErr := make(chan error, 2)
	go func() { pumpErr <- s.singleFlowCarrierWriter(sess, carrier) }()
	go func() { pumpErr <- s.singleFlowCarrierReader(sess, carrier) }()

	res, tlsConn, err := realityfront.HandleServerConnSimpleSingleFlow(s.ctx, app, *s.front)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_REALITY_FAIL client_port=%d err=%q\n", sess.flow.ClientPort, err)
		s.removeSessionMatch(sess.flow, sess)
		return
	}
	_ = tlsConn // deliberately no close_notify on the shared public flow
	if res.Ticket == (realityfront.Ticket{}) {
		s.removeSessionMatch(sess.flow, sess)
		return
	}

	sess.sf.mu.Lock()
	sess.sf.ticket = res.Ticket
	sess.sf.phase = serverSingleFlowAwaitSwitch
	sess.sf.mu.Unlock()
	fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_REALITY_AUTH_OK client_port=%d tls=1.3\n", sess.flow.ClientPort)

	select {
	case <-sess.sf.switchReq:
	case err := <-pumpErr:
		if err != nil && !errors.Is(err, net.ErrClosed) {
			fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_BOOTSTRAP_PUMP_FAIL client_port=%d err=%q\n", sess.flow.ClientPort, err)
		}
		s.removeSessionMatch(sess.flow, sess)
		return
	case <-time.After(15 * time.Second):
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_SWITCH_TIMEOUT client_port=%d\n", sess.flow.ClientPort)
		s.removeSessionMatch(sess.flow, sess)
		return
	case <-sess.sf.done:
		return
	case <-s.ctx.Done():
		return
	}

	// HandleSegment processes the request's ACK before the request payload is
	// delivered here. Wait until all server->client TLS/auth bytes are actually
	// out of the sender pending set before crossing the protocol boundary.
	if err := s.waitSingleFlowSenderDrained(sess, 12*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_BOOTSTRAP_DRAIN_FAIL client_port=%d err=%q\n", sess.flow.ClientPort, err)
		s.removeSessionMatch(sess.flow, sess)
		return
	}
	fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_BOOTSTRAP_TX_DRAINED client_port=%d\n", sess.flow.ClientPort)

	// The server DTLS worker must be ready before the client receives SWITCH_ACK
	// and sends its first ClientHello. This removes the old first-payload/process
	// startup race without creating another public connection.
	if err := s.startSessionWorker(sess); err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_DTLS_WORKER_FAIL client_port=%d err=%q\n", sess.flow.ClientPort, err)
		s.removeSessionMatch(sess.flow, sess)
		return
	}

	sess.sf.mu.Lock()
	ticket := sess.sf.ticket
	sess.sf.phase = serverSingleFlowDatagram
	sess.sf.assembler = nil
	sess.sf.mu.Unlock()
	_ = app.Close()
	_ = carrier.Close()

	if err := s.sendCarrierPayload(sess, singleflow.SwitchAck(ticket[:])); err != nil {
		s.removeSessionMatch(sess.flow, sess)
		return
	}
	fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_DATAGRAM_READY client_port=%d public_flow=reused hol=bootstrap-only\n", sess.flow.ClientPort)
}

func (s *muxServer) waitSingleFlowSenderDrained(sess *muxSession, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		pending := sess.assoc.SenderPending()
		if pending == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timeout with %d pending segments", pending)
		}
		select {
		case <-ticker.C:
		case <-sess.sf.done:
			return errors.New("session removed while draining bootstrap")
		case <-s.ctx.Done():
			return s.ctx.Err()
		}
	}
}

func (s *muxServer) singleFlowCarrierWriter(sess *muxSession, carrier net.Conn) error {
	for {
		select {
		case <-s.ctx.Done():
			return nil
		case <-sess.sf.done:
			return nil
		case p := <-sess.sf.inbound:
			if len(p) == 0 {
				continue
			}
			if _, err := carrier.Write(p); err != nil {
				return err
			}
		}
	}
}

func (s *muxServer) singleFlowCarrierReader(sess *muxSession, carrier net.Conn) error {
	buf := make([]byte, singleflow.BootstrapMaxPayload)
	for {
		n, err := carrier.Read(buf)
		if n > 0 {
			if werr := s.sendCarrierPayload(sess, buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			return err
		}
		select {
		case <-sess.sf.done:
			return nil
		default:
		}
	}
}

func (s *muxServer) sendCarrierPayload(sess *muxSession, payload []byte) error {
	for len(payload) != 0 {
		n := len(payload)
		if n > singleflow.BootstrapMaxPayload {
			n = singleflow.BootstrapMaxPayload
		}
		p, err := sess.assoc.Enqueue(payload[:n], time.Now())
		if err != nil {
			return err
		}
		if err := s.sendPending(sess, p); err != nil {
			return err
		}
		payload = payload[n:]
	}
	return nil
}

func (s *muxServer) handleSingleFlowPayload(sess *muxSession, seg faketcp.Segment) error {
	if sess == nil || sess.sf == nil {
		return errors.New("single-flow session state missing")
	}
	sess.sf.mu.RLock()
	phase := sess.sf.phase
	assembler := sess.sf.assembler
	ticket := sess.sf.ticket
	sess.sf.mu.RUnlock()

	switch phase {
	case serverSingleFlowBootstrap:
		if assembler == nil {
			return errors.New("single-flow bootstrap assembler missing")
		}
		contiguous := assembler.Push(seg.Seq, seg.Payload)
		if len(contiguous) != 0 {
			select {
			case sess.sf.inbound <- contiguous:
			case <-sess.sf.done:
				return errors.New("session removed during bootstrap")
			case <-s.ctx.Done():
				return s.ctx.Err()
			}
		}
		return nil
	case serverSingleFlowAwaitSwitch:
		if singleflow.IsSwitchRequest(seg.Payload, ticket[:]) {
			sess.sf.reqOnce.Do(func() { close(sess.sf.switchReq) })
			fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_SWITCH_REQUEST_RECEIVED client_port=%d\n", sess.flow.ClientPort)
		}
		return nil
	case serverSingleFlowDatagram:
		if sess.relay == nil {
			return errors.New("single-flow DTLS relay missing")
		}
		_, err := sess.relay.Write(seg.Payload)
		return err
	default:
		return errors.New("single-flow invalid server phase")
	}
}
