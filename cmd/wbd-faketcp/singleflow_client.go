package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/singleflow"
)

type singleFlowPhase uint8

const (
	singleFlowDisabled singleFlowPhase = iota
	singleFlowBootstrap
	singleFlowAwaitSwitch
	singleFlowDatagram
)

type singleFlowClientState struct {
	mu        sync.RWMutex
	phase     singleFlowPhase
	assembler *singleflow.OrderedAssembler
	inbound   chan []byte
	ticket    realityfront.Ticket
	switchAck chan struct{}
	ackOnce   sync.Once
}

func (e *endpoint) singleFlowEnabled() bool {
	return e.cfg.role == "client" && strings.TrimSpace(e.cfg.realityServerName) != ""
}

func (e *endpoint) initSingleFlowClient() error {
	if !e.singleFlowEnabled() {
		return nil
	}
	if len(e.cfg.realityRouteKey) < 16 || e.cfg.username == "" || e.cfg.password == "" || strings.TrimSpace(e.cfg.ticketOut) == "" {
		return errors.New("single-flow client requires --reality-server-name, route key >=16 bytes, username/password and --ticket-out")
	}
	if e.cfg.bootstrapTimeout <= 0 {
		e.cfg.bootstrapTimeout = 12 * time.Second
	}
	e.single = &singleFlowClientState{
		phase:     singleFlowBootstrap,
		assembler: singleflow.NewOrderedAssembler(e.receiverNext()),
		inbound:   make(chan []byte, 128),
		switchAck: make(chan struct{}),
	}
	return nil
}

// runSingleFlowBootstrap runs after the raw SYN/SYNACK/ACK handshake while
// rawLoop and retransmitLoop are already active. TLS sees a normal ordered
// net.Conn backed by the same FakeTCP sequence space. No public TCP socket is
// dialed here.
func (e *endpoint) runSingleFlowBootstrap() error {
	if !e.singleFlowEnabled() {
		return nil
	}
	if e.single == nil {
		return errors.New("single-flow state not initialized")
	}
	app, carrier := net.Pipe()
	defer app.Close()
	defer carrier.Close()

	pumpErr := make(chan error, 2)
	go func() { pumpErr <- e.singleFlowCarrierWriter(carrier) }()
	go func() { pumpErr <- e.singleFlowCarrierReader(carrier) }()

	ctx, cancel := context.WithTimeout(context.Background(), e.cfg.bootstrapTimeout)
	defer cancel()
	ticket, tlsConn, err := realityfront.BootstrapClientSingleFlow(ctx, app, realityfront.SingleFlowClientConfig{
		ServerName: e.cfg.realityServerName,
		RouteKey: []byte(e.cfg.realityRouteKey),
		Username: e.cfg.username,
		Password: e.cfg.password,
		VerifyServer: e.cfg.verifyServer,
		Timeout: e.cfg.bootstrapTimeout,
	})
	if err != nil {
		return fmt.Errorf("single-flow Reality-like bootstrap: %w", err)
	}
	// Do not call tlsConn.Close(): that would emit close_notify into the shared
	// sequence space. The transport-level switch below is the phase boundary.
	_ = tlsConn
	if err := os.WriteFile(e.cfg.ticketOut, []byte(ticket.Hex()+"\n"), 0o600); err != nil {
		return fmt.Errorf("write single-flow ticket: %w", err)
	}

	e.single.mu.Lock()
	e.single.ticket = ticket
	e.single.phase = singleFlowAwaitSwitch
	e.single.mu.Unlock()

	// Closing the local pipe stops TLS byte pumps without producing a TLS alert
	// on the carrier. All auth reply bytes have already been read synchronously.
	_ = app.Close()
	_ = carrier.Close()

	if err := e.enqueueCarrierPayload(singleflow.SwitchRequest(ticket[:])); err != nil {
		return fmt.Errorf("send single-flow switch request: %w", err)
	}
	fmt.Fprintln(os.Stderr, "WBD_SINGLEFLOW_SWITCH_REQUEST_SENT")

	switchTimer := time.NewTimer(e.cfg.bootstrapTimeout)
	defer switchTimer.Stop()
	select {
	case <-e.single.switchAck:
		e.single.mu.Lock()
		e.single.phase = singleFlowDatagram
		e.single.assembler = nil
		e.single.mu.Unlock()
		fmt.Fprintln(os.Stderr, "WBD_SINGLEFLOW_DATAGRAM_READY public_flow=reused hol=bootstrap-only")
		return nil
	case err := <-pumpErr:
		if err != nil && !errors.Is(err, net.ErrClosed) {
			return fmt.Errorf("single-flow TLS carrier pump: %w", err)
		}
		// A pipe pump normally exits because we closed the local pipe after auth;
		// keep waiting for the raw SWITCH_ACK.
		select {
		case <-e.single.switchAck:
			e.single.mu.Lock()
			e.single.phase = singleFlowDatagram
			e.single.assembler = nil
			e.single.mu.Unlock()
			fmt.Fprintln(os.Stderr, "WBD_SINGLEFLOW_DATAGRAM_READY public_flow=reused hol=bootstrap-only")
			return nil
		case <-switchTimer.C:
			return errors.New("single-flow switch ACK timeout")
		}
	case <-switchTimer.C:
		return errors.New("single-flow switch ACK timeout")
	case <-e.stop:
		return os.ErrClosed
	}
}

// carrierWriter feeds contiguous peer bytes into the TLS side of net.Pipe.
func (e *endpoint) singleFlowCarrierWriter(carrier net.Conn) error {
	for {
		select {
		case <-e.stop:
			return nil
		case p := <-e.single.inbound:
			if len(p) == 0 {
				continue
			}
			if _, err := carrier.Write(p); err != nil {
				return err
			}
		}
	}
}

// carrierReader takes actual TLS record bytes written by crypto/tls and emits
// them as ordinary TCP-sized payload segments in the existing FakeTCP sequence
// space. This is the only ordered/HOL-sensitive phase and is bounded to setup.
func (e *endpoint) singleFlowCarrierReader(carrier net.Conn) error {
	buf := make([]byte, singleflow.BootstrapMaxPayload)
	for {
		n, err := carrier.Read(buf)
		if n > 0 {
			if werr := e.enqueueCarrierPayload(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			return err
		}
	}
}

func (e *endpoint) enqueueCarrierPayload(payload []byte) error {
	for len(payload) != 0 {
		n := len(payload)
		if n > singleflow.BootstrapMaxPayload {
			n = singleflow.BootstrapMaxPayload
		}
		e.senderMu.Lock()
		p := e.sender.Enqueue(payload[:n], time.Now())
		err := e.sendDataPending(p)
		e.senderMu.Unlock()
		if err != nil {
			return err
		}
		payload = payload[n:]
	}
	return nil
}

// handleDeliveredPayload is called after the existing first-arrival receiver
// has accepted one raw segment. During bootstrap it adds only an ephemeral
// ordered presentation for TLS; after SWITCH_ACK it immediately returns to the
// legacy datagram delivery path, preserving no-HOL steady-state semantics.
func (e *endpoint) handleDeliveredPayload(seg faketcp.Segment) error {
	if e.single == nil {
		peer := e.innerPeer()
		if peer != nil {
			_, _ = e.udp.WriteToUDP(seg.Payload, peer)
		}
		return nil
	}

	e.single.mu.RLock()
	phase := e.single.phase
	ticket := e.single.ticket
	assembler := e.single.assembler
	e.single.mu.RUnlock()

	switch phase {
	case singleFlowBootstrap:
		if assembler == nil {
			return errors.New("single-flow bootstrap assembler missing")
		}
		contiguous := assembler.Push(seg.Seq, seg.Payload)
		if len(contiguous) != 0 {
			select {
			case e.single.inbound <- contiguous:
			case <-e.stop:
				return os.ErrClosed
			}
		}
		return nil
	case singleFlowAwaitSwitch:
		if singleflow.IsSwitchAck(seg.Payload, ticket[:]) {
			e.single.ackOnce.Do(func() { close(e.single.switchAck) })
			fmt.Fprintln(os.Stderr, "WBD_SINGLEFLOW_SWITCH_ACK_RECEIVED")
		}
		return nil
	case singleFlowDatagram:
		peer := e.innerPeer()
		if peer != nil {
			_, _ = e.udp.WriteToUDP(seg.Payload, peer)
		}
		return nil
	default:
		return errors.New("single-flow invalid phase")
	}
}
