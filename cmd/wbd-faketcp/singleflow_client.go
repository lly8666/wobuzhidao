package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	singleFlowDatagram
)

type singleFlowClientState struct {
	mu        sync.RWMutex
	phase     singleFlowPhase
	assembler *singleflow.OrderedAssembler
	inbound   chan []byte
	ticket    realityfront.Ticket
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

	go func() { _ = e.singleFlowCarrierWriter(carrier) }()
	go func() { _ = e.singleFlowCarrierReader(carrier) }()

	ctx, cancel := context.WithTimeout(context.Background(), e.cfg.bootstrapTimeout)
	defer cancel()
	ticket, tlsConn, err := realityfront.BootstrapClientSingleFlow(ctx, app, realityfront.SingleFlowClientConfig{
		ServerName:   e.cfg.realityServerName,
		RouteKey:     []byte(e.cfg.realityRouteKey),
		Username:     e.cfg.username,
		Password:     e.cfg.password,
		VerifyServer: e.cfg.verifyServer,
		Timeout:      e.cfg.bootstrapTimeout,
	})
	if err != nil {
		return fmt.Errorf("single-flow Reality-like bootstrap: %w", err)
	}
	// Do not call tlsConn.Close(): that would emit close_notify into the shared
	// sequence space. The encrypted in-TLS switch below is the phase boundary.
	if err := os.WriteFile(e.cfg.ticketOut, []byte(ticket.Hex()+"\n"), 0o600); err != nil {
		return fmt.Errorf("write single-flow ticket: %w", err)
	}

	// Application-level TLS/auth completion is not yet a transport barrier. Wait
	// until every client->server bootstrap byte has left the FakeTCP sender's
	// pending set before appending the final encrypted switch request record.
	// This HOL wait exists only during bounded bootstrap.
	if err := e.waitSingleFlowSenderDrained(e.cfg.bootstrapTimeout); err != nil {
		return fmt.Errorf("drain single-flow client bootstrap: %w", err)
	}
	fmt.Fprintln(os.Stderr, "WBD_SINGLEFLOW_BOOTSTRAP_TX_DRAINED")

	e.single.mu.Lock()
	e.single.ticket = ticket
	e.single.mu.Unlock()

	// Keep the transition control inside TLS 1.3 application data. A public
	// observer sees a normal encrypted TLS record, never a plaintext WBSF marker.
	deadline := time.Now().Add(e.cfg.bootstrapTimeout)
	_ = tlsConn.SetDeadline(deadline)
	if _, err := tlsConn.Write(singleflow.SwitchRequest(ticket[:])); err != nil {
		return fmt.Errorf("send encrypted single-flow switch request: %w", err)
	}
	fmt.Fprintln(os.Stderr, "WBD_SINGLEFLOW_TLS_SWITCH_REQUEST_SENT")

	ack := make([]byte, singleflow.SwitchFrameLen)
	if _, err := io.ReadFull(tlsConn, ack); err != nil {
		return fmt.Errorf("read encrypted single-flow switch ACK: %w", err)
	}
	if !singleflow.IsSwitchAck(ack, ticket[:]) {
		return singleflow.ErrBadSwitchFrame
	}
	_ = tlsConn.SetDeadline(time.Time{})
	fmt.Fprintln(os.Stderr, "WBD_SINGLEFLOW_TLS_SWITCH_ACK_RECEIVED")

	// The server switches to datagram mode immediately after queuing the TLS ACK
	// record. Therefore successful decryption of that ACK is the client authority
	// to discard ordered bootstrap state and start DTLS on the same public flow.
	e.single.mu.Lock()
	e.single.phase = singleFlowDatagram
	e.single.assembler = nil
	e.single.mu.Unlock()
	_ = app.Close()
	_ = carrier.Close()
	fmt.Fprintln(os.Stderr, "WBD_SINGLEFLOW_DATAGRAM_READY public_flow=reused hol=bootstrap-only")
	return nil
}

func (e *endpoint) waitSingleFlowSenderDrained(timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		e.senderMu.Lock()
		pending := e.sender.Pending()
		e.senderMu.Unlock()
		if pending == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("timeout with %d pending segments", pending)
		}
		select {
		case <-ticker.C:
		case <-e.stop:
			return os.ErrClosed
		}
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
// ordered presentation for TLS; after the encrypted switch ACK it immediately
// returns to the legacy datagram delivery path, preserving no-HOL steady-state
// semantics.
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
