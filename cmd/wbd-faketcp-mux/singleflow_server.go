//go:build linux

package main

import (
	"errors"
	"fmt"
	"io"
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
	serverSingleFlowDatagram
)

type serverSingleFlowState struct {
	mu        sync.RWMutex
	phase     serverSingleFlowPhase
	assembler *singleflow.OrderedAssembler
	inbound   chan []byte
	ticket    realityfront.Ticket
	startOnce sync.Once
	done      chan struct{}
	doneOnce  sync.Once
}

func newServerSingleFlowState(peerNext uint32) *serverSingleFlowState {
	return &serverSingleFlowState{
		phase:     serverSingleFlowBootstrap,
		assembler: singleflow.NewOrderedAssembler(peerNext),
		inbound:   make(chan []byte, 128),
		done:      make(chan struct{}),
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

	go func() { _ = s.singleFlowCarrierWriter(sess, carrier) }()
	go func() { _ = s.singleFlowCarrierReader(sess, carrier) }()

	res, tlsConn, err := realityfront.HandleServerConnSimpleSingleFlow(s.ctx, app, *s.front)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_REALITY_FAIL client_port=%d err=%q\n", sess.flow.ClientPort, err)
		s.removeSessionMatch(sess.flow, sess)
		return
	}
	// Deliberately never call tlsConn.Close(): V3 has no TLS close_notify, FIN,
	// RST or second SYN at the boundary to the datagram phase.
	if res.Ticket == (realityfront.Ticket{}) {
		s.removeSessionMatch(sess.flow, sess)
		return
	}

	sess.sf.mu.Lock()
	sess.sf.ticket = res.Ticket
	sess.sf.mu.Unlock()
	fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_REALITY_AUTH_OK client_port=%d tls=1.3\n", sess.flow.ClientPort)

	// The mode-switch request remains encrypted TLS 1.3 application data. The
	// public wire therefore stays TLS-shaped through the entire setup phase.
	deadline := time.Now().Add(15 * time.Second)
	_ = tlsConn.SetDeadline(deadline)
	req := make([]byte, singleflow.SwitchFrameLen)
	if _, err := io.ReadFull(tlsConn, req); err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_TLS_SWITCH_READ_FAIL client_port=%d err=%q\n", sess.flow.ClientPort, err)
		s.removeSessionMatch(sess.flow, sess)
		return
	}
	if !singleflow.IsSwitchRequest(req, res.Ticket[:]) {
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_TLS_SWITCH_BAD client_port=%d\n", sess.flow.ClientPort)
		s.removeSessionMatch(sess.flow, sess)
		return
	}
	fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_TLS_SWITCH_REQUEST_RECEIVED client_port=%d\n", sess.flow.ClientPort)

	// Reading SWITCH_REQ proves all earlier client TLS bytes are contiguous at
	// the server. Drain the server's earlier TLS/auth bytes before appending the
	// final encrypted ACK record. HOL/retransmission is intentionally bounded to
	// this setup phase only.
	if err := s.waitSingleFlowSenderDrained(sess, 12*time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_BOOTSTRAP_DRAIN_FAIL client_port=%d err=%q\n", sess.flow.ClientPort, err)
		s.removeSessionMatch(sess.flow, sess)
		return
	}
	fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_BOOTSTRAP_TX_DRAINED client_port=%d\n", sess.flow.ClientPort)

	// The worker is live before the client receives its encrypted switch ACK.
	// Consequently the first DTLS ClientHello can never outrun worker startup.
	if err := s.startSessionWorker(sess); err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_DTLS_WORKER_FAIL client_port=%d err=%q\n", sess.flow.ClientPort, err)
		s.removeSessionMatch(sess.flow, sess)
		return
	}

	if _, err := tlsConn.Write(singleflow.SwitchAck(res.Ticket[:])); err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_TLS_SWITCH_WRITE_FAIL client_port=%d err=%q\n", sess.flow.ClientPort, err)
		s.removeSessionMatch(sess.flow, sess)
		return
	}
	fmt.Fprintf(os.Stderr, "WBD_SINGLEFLOW_TLS_SWITCH_ACK_SENT client_port=%d\n", sess.flow.ClientPort)

	// Switch server receive semantics immediately after the final TLS ACK record
	// has been handed to the same raw sender. If that record is lost it is still
	// retransmitted by the sender; the client cannot start DTLS until it has
	// decrypted the record. Old TLS duplicates have sequence numbers below the
	// receiver frontier and are discarded before reaching this handler.
	sess.sf.mu.Lock()
	sess.sf.phase = serverSingleFlowDatagram
	sess.sf.assembler = nil
	sess.sf.mu.Unlock()
	_ = tlsConn.SetDeadline(time.Time{})
	_ = app.Close()
	_ = carrier.Close()
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
