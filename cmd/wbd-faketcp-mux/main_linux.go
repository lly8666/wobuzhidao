//go:build linux

package main

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/dtlsworker"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

type config struct {
	listen      string
	dtlsShim    string
	linkTarget  string
	cert        string
	key         string
	maxSessions int
	recovery    string
}

type muxSession struct {
	flow   faketcp.ServerFlow
	assoc  *faketcp.ServerAssociation
	relay  *net.UDPConn
	worker *dtlsworker.Worker
}

type muxServer struct {
	cfg config

	fd         int
	serverIP   [4]byte
	serverPort uint16
	table      *faketcp.ServerAssociationTable

	mu       sync.RWMutex
	sessions map[faketcp.ServerFlow]*muxSession

	sendMu  sync.Mutex
	sendBuf []byte
	ipID    uint32

	ctx    context.Context
	cancel context.CancelFunc
}

func main() {
	if len(os.Args) < 2 || os.Args[1] != "server" {
		usage()
		os.Exit(2)
	}
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	var c config
	fs.StringVar(&c.listen, "listen", "", "public FakeTCP IPv4 address:port")
	fs.StringVar(&c.dtlsShim, "dtls-shim", "", "path to pinned wolfSSL wbd_dtls_shim")
	fs.StringVar(&c.linkTarget, "link-target", "127.0.0.1:47000", "shared DTLS plaintext WBD link server address")
	fs.StringVar(&c.cert, "cert", "", "DTLS server certificate chain")
	fs.StringVar(&c.key, "key", "", "DTLS server private key")
	fs.IntVar(&c.maxSessions, "max-sessions", 32, "maximum simultaneous raw/DTLS associations")
	fs.StringVar(&c.recovery, "shadow-recovery", "legacy", "legacy (default) or sack-rack experimental")
	_ = fs.Parse(os.Args[2:])

	s, err := newMuxServer(c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "wbd-faketcp-mux:", err)
		os.Exit(1)
	}
	defer s.Close()

	sigCtx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	s.ctx, s.cancel = context.WithCancel(sigCtx)

	fmt.Printf("READY role=server-mux listen=%s max_sessions=%d recovery=%s link_target=%s\n", c.listen, c.maxSessions, c.recovery, c.linkTarget)
	if err := s.Run(); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, os.ErrClosed) {
		fmt.Fprintln(os.Stderr, "wbd-faketcp-mux:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: wbd-faketcp-mux server --listen IP:PORT --dtls-shim PATH --link-target IP:PORT --cert CERT --key KEY [--max-sessions 32] [--shadow-recovery legacy|sack-rack]")
}

func parseRecovery(s string) (faketcp.RecoveryMode, error) {
	switch s {
	case "legacy":
		return faketcp.RecoveryLegacy, nil
	case "sack-rack", "advanced":
		return faketcp.RecoverySACKRACK, nil
	default:
		return faketcp.RecoveryLegacy, fmt.Errorf("unknown shadow recovery %q", s)
	}
}

func newMuxServer(c config) (*muxServer, error) {
	la, err := net.ResolveUDPAddr("udp4", c.listen)
	if err != nil || la == nil || la.Port <= 0 || la.Port > 65535 {
		return nil, errors.New("invalid --listen")
	}
	serverIP, ok := faketcp.IPv4(la.IP)
	if !ok || serverIP == ([4]byte{}) {
		return nil, errors.New("--listen must use a concrete IPv4 address")
	}
	if c.dtlsShim == "" || c.cert == "" || c.key == "" || c.maxSessions <= 0 {
		return nil, errors.New("--dtls-shim, --cert, --key and positive --max-sessions are required")
	}
	if _, err := parseRecovery(c.recovery); err != nil {
		return nil, err
	}
	if _, err := net.ResolveUDPAddr("udp4", c.linkTarget); err != nil {
		return nil, errors.New("invalid --link-target")
	}
	table, err := faketcp.NewServerAssociationTable(c.maxSessions)
	if err != nil {
		return nil, err
	}
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
	if err != nil {
		return nil, err
	}
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Addr: serverIP}); err != nil {
		_ = syscall.Close(fd)
		return nil, err
	}
	return &muxServer{
		cfg: c, fd: fd, serverIP: serverIP, serverPort: uint16(la.Port), table: table,
		sessions: make(map[faketcp.ServerFlow]*muxSession), sendBuf: make([]byte, 65535),
	}, nil
}

func (s *muxServer) Run() error {
	errCh := make(chan error, 2)
	go func() { errCh <- s.rawLoop() }()
	go func() { errCh <- s.retransmitLoop() }()
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case err := <-errCh:
		return err
	}
}

func (s *muxServer) rawLoop() error {
	buf := make([]byte, 65535)
	for {
		n, _, err := syscall.Recvfrom(s.fd, buf, 0)
		if err != nil {
			select {
			case <-s.ctx.Done():
				return s.ctx.Err()
			default:
				return err
			}
		}
		seg, err := faketcp.ParseIPv4TCP(buf[:n])
		if err != nil || seg.DstIP != s.serverIP || seg.DstPort != s.serverPort {
			continue
		}
		flow := faketcp.ServerFlowFromSegment(seg)
		sess := s.getSession(flow)
		if sess == nil {
			if seg.Flags&faketcp.FlagSYN == 0 || seg.Flags&faketcp.FlagACK != 0 || len(seg.Payload) != 0 || !faketcp.IsWBDHandshakeSegment(seg) {
				continue
			}
			if err := s.acceptSYN(seg); err != nil {
				if !errors.Is(err, faketcp.ErrMuxFull) && !errors.Is(err, faketcp.ErrAssociationExists) {
					fmt.Fprintln(os.Stderr, "wbd-faketcp-mux accept:", err)
				}
			}
			continue
		}

		if seg.Flags&faketcp.FlagSYN != 0 && sess.assoc.State() == faketcp.ServerAssociationAwaitACK {
			if !faketcp.IsWBDHandshakeSegment(seg) {
				continue
			}
			_ = s.sendSYNACK(sess)
			continue
		}
		if sess.assoc.State() == faketcp.ServerAssociationAwaitACK {
			if err := sess.assoc.HandleHandshakeACK(seg); err != nil {
				continue
			}
			continue
		}
		res, err := sess.assoc.HandleSegment(seg, time.Now())
		if err != nil {
			continue
		}
		if res.FastRetransmit != nil {
			if err := s.sendPending(sess, res.FastRetransmit); err != nil {
				return err
			}
		}
		if res.AckNeeded {
			if err := s.sendACK(sess, res.Ack, res.SACK[:res.SACKN]); err != nil {
				return err
			}
		}
		if len(res.Deliver) != 0 {
			if _, err := sess.relay.Write(res.Deliver); err != nil {
				s.removeSession(flow)
			}
		}
	}
}

func (s *muxServer) acceptSYN(seg faketcp.Segment) error {
	mode, _ := parseRecovery(s.cfg.recovery)
	assoc, err := s.table.AddSYN(seg, randomSeq(), mode, time.Second)
	if err != nil {
		return err
	}
	flow := faketcp.ServerFlowFromSegment(seg)
	linkAddr, err := net.ResolveUDPAddr("udp4", s.cfg.linkTarget)
	if err != nil {
		s.table.Remove(flow)
		return err
	}
	worker, err := dtlsworker.StartServer(s.ctx, dtlsworker.ServerSpec{
		ShimPath: s.cfg.dtlsShim, TargetIP: linkAddr.IP.String(), TargetPort: linkAddr.Port,
		CertPath: s.cfg.cert, KeyPath: s.cfg.key, Stdout: os.Stdout, Stderr: os.Stderr,
	})
	if err != nil {
		s.table.Remove(flow)
		return err
	}
	relay, err := net.DialUDP("udp4", nil, worker.Addr())
	if err != nil {
		_ = worker.Stop()
		s.table.Remove(flow)
		return err
	}
	sess := &muxSession{flow: flow, assoc: assoc, relay: relay, worker: worker}
	s.mu.Lock()
	if _, exists := s.sessions[flow]; exists {
		s.mu.Unlock()
		_ = relay.Close()
		_ = worker.Stop()
		s.table.Remove(flow)
		return faketcp.ErrAssociationExists
	}
	s.sessions[flow] = sess
	s.mu.Unlock()

	go s.relayLoop(sess)
	go func() {
		_ = worker.Wait()
		s.removeSession(flow)
	}()
	if err := s.sendSYNACK(sess); err != nil {
		s.removeSession(flow)
		return err
	}
	return nil
}

func (s *muxServer) relayLoop(sess *muxSession) {
	buf := make([]byte, 65535)
	for {
		n, err := sess.relay.Read(buf)
		if err != nil {
			return
		}
		p, err := sess.assoc.Enqueue(buf[:n], time.Now())
		if err != nil {
			continue
		}
		if err := s.sendPending(sess, p); err != nil {
			return
		}
	}
}

func (s *muxServer) retransmitLoop() error {
	t := time.NewTicker(2 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return s.ctx.Err()
		case now := <-t.C:
			for _, sess := range s.snapshotSessions() {
				if p := sess.assoc.RetransmitDue(now); p != nil {
					if err := s.sendPending(sess, p); err != nil {
						return err
					}
				}
			}
		}
	}
}

func (s *muxServer) sendSYNACK(sess *muxSession) error {
	seq, ack, err := sess.assoc.SYNACK()
	if err != nil {
		return err
	}
	return s.sendRaw(sess.flow, seq, ack, faketcp.FlagSYN|faketcp.FlagACK, nil, nil)
}

func (s *muxServer) sendACK(sess *muxSession, ack uint32, sacks []faketcp.SACKBlock) error {
	return s.sendRaw(sess.flow, sess.assoc.SenderNext(), ack, faketcp.FlagACK, sacks, nil)
}

func (s *muxServer) sendPending(sess *muxSession, p *faketcp.Pending) error {
	if p == nil || len(p.Payload) == 0 {
		return nil
	}
	return s.sendRaw(sess.flow, p.Seq, sess.assoc.ReceiverNext(), faketcp.FlagACK|faketcp.FlagPSH, nil, p.Payload)
}

func (s *muxServer) sendRaw(flow faketcp.ServerFlow, seq, ack uint32, flags uint8, sacks []faketcp.SACKBlock, payload []byte) error {
	s.sendMu.Lock()
	defer s.sendMu.Unlock()
	id := uint16(atomic.AddUint32(&s.ipID, 1))
	pkt := faketcp.MarshalIPv4TCPSACKInto(
		s.sendBuf, flow.ServerIP, flow.ClientIP, flow.ServerPort, flow.ClientPort,
		seq, ack, flags, 65535, sacks, payload, id,
	)
	return syscall.Sendto(s.fd, pkt, 0, &syscall.SockaddrInet4{Addr: flow.ClientIP})
}

func (s *muxServer) getSession(flow faketcp.ServerFlow) *muxSession {
	s.mu.RLock()
	sess := s.sessions[flow]
	s.mu.RUnlock()
	return sess
}

func (s *muxServer) snapshotSessions() []*muxSession {
	s.mu.RLock()
	out := make([]*muxSession, 0, len(s.sessions))
	for _, sess := range s.sessions {
		out = append(out, sess)
	}
	s.mu.RUnlock()
	return out
}

func (s *muxServer) removeSession(flow faketcp.ServerFlow) {
	s.mu.Lock()
	sess := s.sessions[flow]
	if sess != nil {
		delete(s.sessions, flow)
	}
	s.mu.Unlock()
	if sess == nil {
		return
	}
	_ = sess.relay.Close()
	_ = sess.worker.Stop()
	s.table.Remove(flow)
}

func (s *muxServer) Close() {
	if s.cancel != nil {
		s.cancel()
	}
	for _, sess := range s.snapshotSessions() {
		s.removeSession(sess.flow)
	}
	if s.fd >= 0 {
		_ = syscall.Close(s.fd)
		s.fd = -1
	}
}

func randomSeq() uint32 {
	var b [4]byte
	if _, err := rand.Read(b[:]); err == nil {
		return binary.BigEndian.Uint32(b[:])
	}
	return uint32(time.Now().UnixNano())
}
