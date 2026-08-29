//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/dtlsworker"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

const halfOpenTimeout = 25 * time.Second

type config struct {
	listen      string
	dtlsShim    string
	linkTarget  string
	cert        string
	key         string
	maxSessions int
	recovery    string

	frontCert        string
	frontKey         string
	serverName       string
	routeKey         string
	username         string
	password         string
	ticketDir        string
	bootstrapTimeout time.Duration
	fallbackTarget   string
}

type sessionStage uint8

const (
	stageHandshake sessionStage = iota
	stageBootstrap
	stageFallback
	stageTransition
	stageData
)

type muxSession struct {
	flow  faketcp.ServerFlow
	assoc *faketcp.ServerAssociation

	mu          sync.RWMutex
	stage       sessionStage
	bootstrap   *faketcp.BootstrapStream
	ackNotify   chan struct{}
	relay       *net.UDPConn
	worker      *dtlsworker.Worker
	pendingData [][]byte
}

type muxServer struct {
	cfg config

	fd         int
	serverIP   [4]byte
	serverPort uint16
	table      *faketcp.ServerAssociationTable
	frontTLS   *tls.Config

	mu       sync.RWMutex
	sessions map[faketcp.ServerFlow]*muxSession

	sendMu  sync.Mutex
	sendBuf []byte
	ipID    uint32

	ctx    context.Context
	cancel context.CancelFunc
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "server" { usage(); os.Exit(2) }
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	var c config
	fs.StringVar(&c.listen, "listen", "", "public FakeTCP IPv4 address:port")
	fs.StringVar(&c.dtlsShim, "dtls-shim", "", "path to pinned wolfSSL wbd_dtls_shim")
	fs.StringVar(&c.linkTarget, "link-target", "127.0.0.1:47000", "shared DTLS plaintext WBD link server address")
	fs.StringVar(&c.cert, "cert", "", "DTLS server certificate chain")
	fs.StringVar(&c.key, "key", "", "DTLS server private key")
	fs.IntVar(&c.maxSessions, "max-sessions", 32, "maximum simultaneous raw/DTLS associations")
	fs.StringVar(&c.recovery, "shadow-recovery", "legacy", "legacy (default) or sack-rack experimental")
	fs.StringVar(&c.frontCert, "front-cert", "", "single-flow TLS bootstrap certificate")
	fs.StringVar(&c.frontKey, "front-key", "", "single-flow TLS bootstrap private key")
	fs.StringVar(&c.serverName, "server-name", "", "single-flow Reality-like SNI/server name")
	fs.StringVar(&c.routeKey, "route-key", "", "single-flow Reality-like classifier secret")
	fs.StringVar(&c.username, "username", "", "single-flow shared account username")
	fs.StringVar(&c.password, "password", "", "single-flow shared account password")
	fs.StringVar(&c.ticketDir, "ticket-dir", "", "single-flow one-time ticket directory")
	fs.DurationVar(&c.bootstrapTimeout, "bootstrap-timeout", 12*time.Second, "single-flow TLS/admission deadline")
	fs.StringVar(&c.fallbackTarget, "fallback-target", "", "ordinary TCP decoy target for unrecognized ClientHello")
	_ = fs.Parse(os.Args[2:])

	s, err := newMuxServer(c)
	if err != nil { fmt.Fprintln(os.Stderr, "wbd-faketcp-mux:", err); os.Exit(1) }
	defer s.Close()
	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	s.ctx, s.cancel = context.WithCancel(sigCtx)

	fmt.Printf("READY role=server-mux listen=%s max_sessions=%d recovery=%s link_target=%s single_flow_bootstrap=%t fallback=%t\n", c.listen, c.maxSessions, c.recovery, c.linkTarget, c.bootstrapEnabled(), c.fallbackTarget != "")
	if err := s.Run(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, os.ErrClosed) {
		fmt.Fprintln(os.Stderr, "wbd-faketcp-mux:", err); os.Exit(1)
	}
}

func (c config) bootstrapEnabled() bool {
	return strings.TrimSpace(c.serverName) != "" || strings.TrimSpace(c.routeKey) != "" || strings.TrimSpace(c.username) != "" || strings.TrimSpace(c.password) != "" || strings.TrimSpace(c.ticketDir) != "" || strings.TrimSpace(c.frontCert) != "" || strings.TrimSpace(c.frontKey) != ""
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wbd-faketcp-mux server --listen IP:PORT --dtls-shim PATH --link-target IP:PORT --cert CERT --key KEY [--max-sessions 32] [--shadow-recovery legacy|sack-rack]")
	fmt.Fprintln(os.Stderr, "  product single-flow mode additionally requires --front-cert --front-key --server-name --route-key --username --password --ticket-dir --fallback-target")
}

func parseRecovery(s string) (faketcp.RecoveryMode, error) {
	switch s { case "legacy": return faketcp.RecoveryLegacy, nil; case "sack-rack", "advanced": return faketcp.RecoverySACKRACK, nil; default: return faketcp.RecoveryLegacy, fmt.Errorf("unknown shadow recovery %q", s) }
}

func newMuxServer(c config) (*muxServer, error) {
	la, err := net.ResolveUDPAddr("udp4", c.listen)
	if err != nil || la == nil || la.Port <= 0 || la.Port > 65535 { return nil, errors.New("invalid --listen") }
	serverIP, ok := faketcp.IPv4(la.IP)
	if !ok || serverIP == ([4]byte{}) { return nil, errors.New("--listen must use a concrete IPv4 address") }
	if c.dtlsShim == "" || c.cert == "" || c.key == "" || c.maxSessions <= 0 { return nil, errors.New("--dtls-shim, --cert, --key and positive --max-sessions are required") }
	if _, err := parseRecovery(c.recovery); err != nil { return nil, err }
	if _, err := net.ResolveUDPAddr("udp4", c.linkTarget); err != nil { return nil, errors.New("invalid --link-target") }
	if c.bootstrapEnabled() {
		if c.frontCert == "" || c.frontKey == "" || c.serverName == "" || len(c.routeKey) < 16 || c.username == "" || c.password == "" || c.ticketDir == "" || c.bootstrapTimeout <= 0 || strings.TrimSpace(c.fallbackTarget) == "" {
			return nil, errors.New("single-flow bootstrap requires front cert/key, server-name, route-key >=16 bytes, username/password, ticket-dir, fallback-target and positive timeout")
		}
		if _, err := net.ResolveTCPAddr("tcp", c.fallbackTarget); err != nil { return nil, errors.New("invalid --fallback-target") }
	}
	table, err := faketcp.NewServerAssociationTable(c.maxSessions); if err != nil { return nil, err }
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP); if err != nil { return nil, err }
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil { _ = syscall.Close(fd); return nil, err }
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Addr: serverIP}); err != nil { _ = syscall.Close(fd); return nil, err }
	s := &muxServer{cfg: c, fd: fd, serverIP: serverIP, serverPort: uint16(la.Port), table: table, sessions: make(map[faketcp.ServerFlow]*muxSession), sendBuf: make([]byte, 65535)}
	if c.bootstrapEnabled() {
		cert, err := tls.LoadX509KeyPair(c.frontCert, c.frontKey); if err != nil { _ = syscall.Close(fd); return nil, err }
		s.frontTLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13}
	}
	return s, nil
}

func (s *muxServer) Run() error {
	errCh := make(chan error, 2)
	go func() { errCh <- s.rawLoop() }()
	go func() { errCh <- s.retransmitLoop() }()
	select { case <-s.ctx.Done(): return s.ctx.Err(); case err := <-errCh: return err }
}

func (s *muxServer) rawLoop() error {
	buf := make([]byte, 65535)
	for {
		var n int; var err error
		for { n, _, err = syscall.Recvfrom(s.fd, buf, 0); if err == syscall.EINTR { continue }; break }
		if err != nil { select { case <-s.ctx.Done(): return s.ctx.Err(); default: return err } }
		seg, err := faketcp.ParseIPv4TCP(buf[:n]); if err != nil || seg.DstIP != s.serverIP || seg.DstPort != s.serverPort { continue }
		flow := faketcp.ServerFlowFromSegment(seg); sess := s.getSession(flow)
		if sess == nil {
			if seg.Flags&faketcp.FlagSYN == 0 || seg.Flags&faketcp.FlagACK != 0 || len(seg.Payload) != 0 { continue }
			if !s.cfg.bootstrapEnabled() && !faketcp.IsWBDHandshakeSegment(seg) { continue }
			if err := s.acceptSYN(seg); err != nil && !errors.Is(err, faketcp.ErrMuxFull) && !errors.Is(err, faketcp.ErrAssociationExists) { fmt.Fprintln(os.Stderr, "wbd-faketcp-mux accept:", err) }
			continue
		}
		if seg.Flags&faketcp.FlagSYN != 0 && sess.assoc.State() == faketcp.ServerAssociationAwaitACK {
			if !s.cfg.bootstrapEnabled() && !faketcp.IsWBDHandshakeSegment(seg) { continue }
			_ = s.sendSYNACK(sess); continue
		}
		if sess.assoc.State() == faketcp.ServerAssociationAwaitACK {
			if err := sess.assoc.HandleHandshakeACK(seg); err != nil { continue }
			if s.cfg.bootstrapEnabled() { if err := s.startBootstrap(sess); err != nil { s.removeSessionMatch(flow, sess); continue } } else { if err := s.activateDTLS(sess); err != nil { s.removeSessionMatch(flow, sess); continue } }
			// final ACK may carry first TLS/DTLS bytes; fall through.
		}

		res, err := sess.assoc.HandleSegment(seg, time.Now()); if err != nil { continue }
		if seg.Flags&faketcp.FlagACK != 0 { sess.signalAck() }
		if res.FastRetransmit != nil { if err := s.sendPending(sess, res.FastRetransmit); err != nil { return err } }
		if res.AckNeeded { if err := s.sendACK(sess, res.Ack, res.SACK[:res.SACKN]); err != nil { return err } }
		if len(res.Deliver) != 0 { if err := sess.routeDeliver(res.DeliverSeq, res.Deliver); err != nil { s.removeSessionMatch(flow, sess) } }
	}
}

func (s *muxServer) acceptSYN(seg faketcp.Segment) error {
	mode, _ := parseRecovery(s.cfg.recovery)
	assoc, err := s.table.AddSYN(seg, randomSeq(), mode, time.Second); if err != nil { return err }
	flow := faketcp.ServerFlowFromSegment(seg)
	sess := &muxSession{flow: flow, assoc: assoc, stage: stageHandshake, ackNotify: make(chan struct{}, 1)}
	s.mu.Lock(); if _, exists := s.sessions[flow]; exists { s.mu.Unlock(); s.table.Remove(flow); return faketcp.ErrAssociationExists }; s.sessions[flow] = sess; s.mu.Unlock()
	if err := s.sendSYNACK(sess); err != nil { s.removeSessionMatch(flow, sess); return err }
	go func() {
		t := time.NewTimer(halfOpenTimeout); defer t.Stop()
		select { case <-s.ctx.Done(): return; case <-t.C: if sess.assoc.State() == faketcp.ServerAssociationAwaitACK { s.removeSessionMatch(flow, sess) } }
	}()
	return nil
}

func (s *muxServer) startBootstrap(sess *muxSession) error {
	local := &net.TCPAddr{IP: net.IPv4(sess.flow.ServerIP[0], sess.flow.ServerIP[1], sess.flow.ServerIP[2], sess.flow.ServerIP[3]), Port: int(sess.flow.ServerPort)}
	remote := &net.TCPAddr{IP: net.IPv4(sess.flow.ClientIP[0], sess.flow.ClientIP[1], sess.flow.ClientIP[2], sess.flow.ClientIP[3]), Port: int(sess.flow.ClientPort)}
	stream, err := faketcp.NewBootstrapStream(sess.assoc.ReceiverNext(), func(payload []byte) (uint32, error) {
		p, err := sess.assoc.Enqueue(payload, time.Now()); if err != nil { return 0, err }
		if err := s.sendPending(sess, p); err != nil { return 0, err }; return p.End, nil
	}, func(end uint32, deadline time.Time) error { return s.waitSessionAck(sess, end, deadline) }, local, remote)
	if err != nil { return err }
	sess.mu.Lock(); sess.stage = stageBootstrap; sess.bootstrap = stream; sess.mu.Unlock()
	go s.runBootstrap(sess, stream)
	return nil
}

func (s *muxServer) runBootstrap(sess *muxSession, stream *faketcp.BootstrapStream) {
	hello, err := realityfront.ReadSingleFlowHello(stream, s.cfg.serverName, []byte(s.cfg.routeKey), s.cfg.bootstrapTimeout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLE_FLOW_HELLO_FAIL remote=%s err=%q\n", stream.RemoteAddr(), err)
		s.removeSessionMatch(sess.flow, sess); return
	}
	if !hello.Recognized {
		sess.mu.Lock(); if sess.bootstrap == stream { sess.stage = stageFallback }; sess.mu.Unlock()
		fmt.Printf("WBD_SINGLE_FLOW_FALLBACK remote=%s sni=%s target=%s\n", stream.RemoteAddr(), hello.Info.ServerName, s.cfg.fallbackTarget)
		s.runFallback(sess, stream, hello.Raw)
		return
	}

	_, err = realityfront.BootstrapServerRecognizedSingleFlow(s.ctx, stream, hello.Raw, realityfront.SingleFlowServerConfig{
		ServerName: s.cfg.serverName, RouteKey: []byte(s.cfg.routeKey), ExpectedUsername: s.cfg.username,
		ExpectedPassword: s.cfg.password, TicketDir: s.cfg.ticketDir, TLSConfig: s.frontTLS, Timeout: s.cfg.bootstrapTimeout,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLE_FLOW_BOOTSTRAP_FAIL remote=%s err=%q\n", stream.RemoteAddr(), err)
		s.removeSessionMatch(sess.flow, sess); return
	}

	// TLS/auth bytes are ACK-gated. Enter transition before worker startup so
	// immediately arriving DTLS datagrams are queued, not fed back into TLS.
	sess.mu.Lock(); if sess.bootstrap == stream { sess.bootstrap = nil; sess.stage = stageTransition }; sess.mu.Unlock()
	_ = stream.Close()
	if err := s.activateDTLS(sess); err != nil {
		fmt.Fprintf(os.Stderr, "WBD_SINGLE_FLOW_DTLS_ACTIVATE_FAIL remote=%s err=%q\n", stream.RemoteAddr(), err)
		s.removeSessionMatch(sess.flow, sess); return
	}
	fmt.Printf("WBD_SINGLE_FLOW_BOOTSTRAP_READY remote=%s server_name=%s same_flow=1\n", stream.RemoteAddr(), s.cfg.serverName)
}

func (s *muxServer) runFallback(sess *muxSession, stream *faketcp.BootstrapStream, rawHello []byte) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	upstream, err := dialer.DialContext(s.ctx, "tcp", s.cfg.fallbackTarget)
	if err != nil { s.removeSessionMatch(sess.flow, sess); return }
	defer upstream.Close()
	if err := writeFull(upstream, rawHello); err != nil { s.removeSessionMatch(sess.flow, sess); return }

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(upstream, stream); done <- struct{}{} }()
	go func() { _, _ = io.Copy(stream, upstream); done <- struct{}{} }()
	select { case <-s.ctx.Done(): case <-done: }
	_ = upstream.Close()
	// Emit a TCP-shaped FIN before releasing raw state. The VPN product path
	// never enters fallback, so this does not alter steady-state no-HOL semantics.
	_ = s.sendRaw(sess.flow, sess.assoc.SenderNext(), sess.assoc.ReceiverNext(), faketcp.FlagFIN|faketcp.FlagACK, nil, nil)
	time.Sleep(50 * time.Millisecond)
	s.removeSessionMatch(sess.flow, sess)
}

func writeFull(w io.Writer, p []byte) error {
	for len(p) != 0 { n, err := w.Write(p); if err != nil { return err }; if n <= 0 { return io.ErrUnexpectedEOF }; p = p[n:] }
	return nil
}

func (s *muxServer) waitSessionAck(sess *muxSession, end uint32, deadline time.Time) error {
	for {
		if sess.assoc.SenderLastAck() == end { return nil }
		var timer *time.Timer; var timeout <-chan time.Time
		if !deadline.IsZero() { d := time.Until(deadline); if d <= 0 { return faketcp.ErrBootstrapTimeout }; timer = time.NewTimer(d); timeout = timer.C }
		select { case <-sess.ackNotify: case <-s.ctx.Done(): if timer != nil { timer.Stop() }; return s.ctx.Err(); case <-timeout: return faketcp.ErrBootstrapTimeout }
		if timer != nil { timer.Stop() }
	}
}

func (sess *muxSession) signalAck() { select { case sess.ackNotify <- struct{}{}: default: } }

func (sess *muxSession) routeDeliver(seq uint32, payload []byte) error {
	sess.mu.Lock()
	if sess.bootstrap != nil { stream := sess.bootstrap; sess.mu.Unlock(); stream.Feed(seq, payload); return nil }
	if sess.relay != nil && sess.stage == stageData { relay := sess.relay; sess.mu.Unlock(); _, err := relay.Write(payload); return err }
	if sess.stage == stageTransition {
		if len(sess.pendingData) >= 64 { sess.mu.Unlock(); return errors.New("single-flow transition queue full") }
		sess.pendingData = append(sess.pendingData, append([]byte(nil), payload...)); sess.mu.Unlock(); return nil
	}
	sess.mu.Unlock(); return nil
}

func (s *muxServer) activateDTLS(sess *muxSession) error {
	linkAddr, err := net.ResolveUDPAddr("udp4", s.cfg.linkTarget); if err != nil { return err }
	worker, err := dtlsworker.StartServer(s.ctx, dtlsworker.ServerSpec{ShimPath: s.cfg.dtlsShim, TargetIP: linkAddr.IP.String(), TargetPort: linkAddr.Port, CertPath: s.cfg.cert, KeyPath: s.cfg.key, Stdout: os.Stdout, Stderr: os.Stderr})
	if err != nil { return err }
	relay, err := net.DialUDP("udp4", nil, worker.Addr()); if err != nil { _ = worker.Stop(); return err }
	sess.mu.Lock()
	if sess.worker != nil || sess.relay != nil { sess.mu.Unlock(); _ = relay.Close(); _ = worker.Stop(); return errors.New("DTLS worker already active") }
	pending := sess.pendingData; sess.pendingData = nil; sess.worker = worker; sess.relay = relay; sess.stage = stageData; sess.mu.Unlock()
	for _, p := range pending { if _, err := relay.Write(p); err != nil { return err } }
	go s.relayLoop(sess)
	go func() { _ = worker.Wait(); s.removeSessionMatch(sess.flow, sess) }()
	return nil
}

func (s *muxServer) relayLoop(sess *muxSession) {
	buf := make([]byte, 65535)
	for {
		sess.mu.RLock(); relay := sess.relay; sess.mu.RUnlock(); if relay == nil { return }
		n, err := relay.Read(buf); if err != nil { return }
		p, err := sess.assoc.Enqueue(buf[:n], time.Now()); if err != nil { continue }
		if err := s.sendPending(sess, p); err != nil { return }
	}
}

func (s *muxServer) retransmitLoop() error {
	t := time.NewTicker(2 * time.Millisecond); defer t.Stop()
	for { select { case <-s.ctx.Done(): return s.ctx.Err(); case now := <-t.C: for _, sess := range s.snapshotSessions() { if p := sess.assoc.RetransmitDue(now); p != nil { if err := s.sendPending(sess, p); err != nil { return err } } } } }
}

func (s *muxServer) sendSYNACK(sess *muxSession) error { seq, ack, err := sess.assoc.SYNACK(); if err != nil { return err }; return s.sendRaw(sess.flow, seq, ack, faketcp.FlagSYN|faketcp.FlagACK, nil, nil) }
func (s *muxServer) sendACK(sess *muxSession, ack uint32, sacks []faketcp.SACKBlock) error { return s.sendRaw(sess.flow, sess.assoc.SenderNext(), ack, faketcp.FlagACK, sacks, nil) }
func (s *muxServer) sendPending(sess *muxSession, p *faketcp.Pending) error { if p == nil || len(p.Payload) == 0 { return nil }; return s.sendRaw(sess.flow, p.Seq, sess.assoc.ReceiverNext(), faketcp.FlagACK|faketcp.FlagPSH, nil, p.Payload) }

func (s *muxServer) sendRaw(flow faketcp.ServerFlow, seq, ack uint32, flags uint8, sacks []faketcp.SACKBlock, payload []byte) error {
	s.sendMu.Lock(); defer s.sendMu.Unlock(); id := uint16(atomic.AddUint32(&s.ipID, 1))
	pkt := faketcp.MarshalIPv4TCPSACKInto(s.sendBuf, flow.ServerIP, flow.ClientIP, flow.ServerPort, flow.ClientPort, seq, ack, flags, 65535, sacks, payload, id)
	return syscall.Sendto(s.fd, pkt, 0, &syscall.SockaddrInet4{Addr: flow.ClientIP})
}

func (s *muxServer) getSession(flow faketcp.ServerFlow) *muxSession { s.mu.RLock(); sess := s.sessions[flow]; s.mu.RUnlock(); return sess }
func (s *muxServer) snapshotSessions() []*muxSession { s.mu.RLock(); out := make([]*muxSession, 0, len(s.sessions)); for _, sess := range s.sessions { out = append(out, sess) }; s.mu.RUnlock(); return out }

func (s *muxServer) removeSessionMatch(flow faketcp.ServerFlow, expected *muxSession) {
	s.mu.Lock(); sess := s.sessions[flow]; if sess != expected { s.mu.Unlock(); return }; delete(s.sessions, flow); s.mu.Unlock(); if sess == nil { return }
	sess.mu.Lock(); stream := sess.bootstrap; sess.bootstrap = nil; relay := sess.relay; sess.relay = nil; worker := sess.worker; sess.worker = nil; sess.pendingData = nil; sess.mu.Unlock()
	if stream != nil { _ = stream.Close() }; if relay != nil { _ = relay.Close() }; if worker != nil { _ = worker.Stop() }; s.table.Remove(flow)
}

func (s *muxServer) Close() { if s.cancel != nil { s.cancel() }; for _, sess := range s.snapshotSessions() { s.removeSessionMatch(sess.flow, sess) }; if s.fd >= 0 { _ = syscall.Close(s.fd); s.fd = -1 } }

func randomSeq() uint32 { var b [4]byte; if _, err := rand.Read(b[:]); err == nil { return binary.BigEndian.Uint32(b[:]) }; return uint32(time.Now().UnixNano()) }
