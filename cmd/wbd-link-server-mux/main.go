package main

import (
	"context"
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

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/fec"
	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/platformproxy"
	"github.com/lly8666/wobuzhidao/internal/session"
)

const (
	maxBlocks          = 64
	defaultIdleTimeout = 90 * time.Second
)

type serviceBackend string

const (
	backendPlatform serviceBackend = "platformproxy"
	backendRawIP    serviceBackend = "rawip"
	backendGame     serviceBackend = "game"
)

type config struct {
	listen        string
	service       string
	rawIPService  string
	ticketDir     string
	ticketTTL     time.Duration
	setupTimeout  time.Duration
	idleTimeout   time.Duration
	maxSessions   int
}

type startupSession interface {
	State() control.State
	Stats() control.LinkSessionStats
	HandleWire([]byte, uint64) ([]byte, error)
}

type peerSession struct {
	peer    *net.UDPAddr
	key     string
	startup startupSession
	created time.Time

	activityMu   sync.Mutex
	lastActivity time.Time

	account      string
	id           session.LiveID
	sid          string
	haveIdentity bool
	active       bool
	backend      serviceBackend
	service      *net.UDPConn

	linkRx      atomic.Uint64
	linkTx      atomic.Uint64
	drop        atomic.Uint64
	linkRxFirst sync.Once
	linkTxFirst sync.Once
}

func (p *peerSession) touch(now time.Time) {
	p.activityMu.Lock()
	p.lastActivity = now
	p.activityMu.Unlock()
}

func (p *peerSession) idleFor(now time.Time) time.Duration {
	p.activityMu.Lock()
	last := p.lastActivity
	p.activityMu.Unlock()
	if last.IsZero() || now.Before(last) { return 0 }
	return now.Sub(last)
}

type server struct {
	cfg              config
	conn             *net.UDPConn
	serviceAddr      *net.UDPAddr
	rawIPServiceAddr *net.UDPAddr
	plane            *session.DataPlane

	mu    sync.RWMutex
	peers map[string]*peerSession
}

func main() {
	var c config
	flag.StringVar(&c.listen, "listen", "", "shared UDP address receiving plaintext from all DTLS workers")
	flag.StringVar(&c.service, "service", "", "local UDP Game/platform service address")
	flag.StringVar(&c.rawIPService, "raw-ip-service", "", "optional local UDP raw-IP gateway service address")
	flag.StringVar(&c.ticketDir, "ticket-dir", "", "same-entry Reality one-time ticket directory")
	flag.DurationVar(&c.ticketTTL, "ticket-ttl", 60*time.Second, "maximum ticket age")
	flag.DurationVar(&c.setupTimeout, "setup-timeout", 10*time.Second, "ticket bind + LINK_INIT deadline per peer")
	flag.DurationVar(&c.idleTimeout, "idle-timeout", defaultIdleTimeout, "active session lease without data or PING activity")
	flag.IntVar(&c.maxSessions, "max-sessions", 32, "maximum simultaneous sessions")
	flag.Parse()

	s, err := newServer(c)
	if err != nil { fmt.Fprintln(os.Stderr, "WBD_LINK_SERVER_MUX_FAIL", err); os.Exit(1) }
	defer s.Close()
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	fmt.Printf("WBD_LINK_SERVER_MUX_READY listen=%s service=%s raw_ip_service=%s max_sessions=%d ticket_auth=1 logical_tunnel=1 game_backend=1 idle_timeout=%s\n", s.conn.LocalAddr(), c.service, c.rawIPService, c.maxSessions, s.cfg.idleTimeout)
	if err := s.Run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, os.ErrClosed) {
		fmt.Fprintln(os.Stderr, "WBD_LINK_SERVER_MUX_FAIL", err)
		os.Exit(1)
	}
}

func newServer(c config) (*server, error) {
	if c.idleTimeout <= 0 { c.idleTimeout = defaultIdleTimeout }
	if c.listen == "" || c.service == "" || c.ticketDir == "" || c.ticketTTL <= 0 || c.setupTimeout <= 0 || c.maxSessions <= 0 {
		return nil, errors.New("-listen, -service, -ticket-dir and positive ttl/timeout/max-sessions are required")
	}
	listenAddr, err := net.ResolveUDPAddr("udp4", c.listen)
	if err != nil { return nil, err }
	serviceAddr, err := net.ResolveUDPAddr("udp4", c.service)
	if err != nil { return nil, err }
	var rawIPServiceAddr *net.UDPAddr
	if c.rawIPService != "" {
		rawIPServiceAddr, err = net.ResolveUDPAddr("udp4", c.rawIPService)
		if err != nil { return nil, err }
	}
	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil { return nil, err }
	_ = conn.SetReadBuffer(4 << 20)
	_ = conn.SetWriteBuffer(4 << 20)
	plane, err := session.NewDataPlane(c.maxSessions, maxBlocks)
	if err != nil { _ = conn.Close(); return nil, err }
	return &server{cfg: c, conn: conn, serviceAddr: serviceAddr, rawIPServiceAddr: rawIPServiceAddr, plane: plane, peers: make(map[string]*peerSession)}, nil
}

func (s *server) Addr() *net.UDPAddr {
	if s == nil || s.conn == nil { return nil }
	a, _ := s.conn.LocalAddr().(*net.UDPAddr)
	return cloneUDPAddr(a)
}

func (s *server) Run(ctx context.Context) error {
	buf := make([]byte, 65535)
	for {
		if err := s.conn.SetReadDeadline(time.Now().Add(2 * time.Millisecond)); err != nil { return err }
		n, from, err := s.conn.ReadFromUDP(buf)
		now := time.Now()
		if err != nil {
			if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
				select { case <-ctx.Done(): return ctx.Err(); default: return err }
			}
		} else if err := s.handleDatagram(from, buf[:n], now); err != nil {
			fmt.Fprintf(os.Stderr, "WBD_LINK_SERVER_MUX_DROP peer=%s err=%v\n", from, err)
			s.removePeer(from.String(), true)
		}
		s.flushDue(now)
		s.expirePeers(now)
		select { case <-ctx.Done(): return ctx.Err(); default: }
	}
}

func (s *server) handleDatagram(from *net.UDPAddr, packet []byte, now time.Time) error {
	key := from.String()
	ps := s.getPeer(key)
	if ps == nil {
		typ, ok := controlFrameType(packet)
		if !ok || typ != control.TypeDemoBind { return nil }
		var err error
		ps, err = s.newPeer(from, now)
		if err != nil { return err }
	}

	if !ps.active || isStartupControl(packet) || isLifecycleControl(packet) {
		if err := s.handleControl(ps, packet, now); err != nil { ps.drop.Add(1); return err }
		ps.touch(now)
		return nil
	}
	_, packets, err := s.plane.Inbound(ps.key, packet)
	if err != nil {
		if errors.Is(err, fec.ErrDecoderFull) { ps.drop.Add(1); return nil }
		ps.drop.Add(1)
		return err
	}
	ps.touch(now)
	for _, p := range packets {
		if err := s.ensureService(ps, p); err != nil { ps.drop.Add(1); return err }
		if ps.backend == backendRawIP {
			if err := validatePeerRawIPSource(ps, p); err != nil {
				ps.drop.Add(1)
				fmt.Fprintf(os.Stderr, "WBD_LINK_RAW_IP_SPOOF_DROP tunnel_id_prefix=%s err=%v\n", printableSID(ps), err)
				continue
			}
		}
		ps.linkRx.Add(1)
		ps.linkRxFirst.Do(func() { fmt.Printf("WBD_LINK_RX_FIRST tunnel_id_prefix=%s bytes=%d backend=%s\n", ps.sid, len(p), ps.backend) })
		if _, err := ps.service.Write(p); err != nil { ps.drop.Add(1); return err }
	}
	return nil
}

func (s *server) newPeer(peer *net.UDPAddr, now time.Time) (*peerSession, error) {
	key := peer.String()
	s.mu.Lock()
	if existing := s.peers[key]; existing != nil { s.mu.Unlock(); return existing, nil }
	if len(s.peers) >= s.cfg.maxSessions { s.mu.Unlock(); return nil, session.ErrRegistryFull }
	ps := &peerSession{peer: cloneUDPAddr(peer), key: key, created: now, lastActivity: now}
	verify := func(bind [control.DemoWitnessLen]byte) error { return s.consumeLogicalTunnelTicket(ps, bind) }
	startup, err := control.NewDemoTicketReliableLinkServerSession(1, 1, control.CurrentLinkPolicy(), verify)
	if err != nil { s.mu.Unlock(); return nil, err }
	ps.startup = startup
	s.peers[key] = ps
	s.mu.Unlock()
	return ps, nil
}

func (s *server) handleControl(ps *peerSession, packet []byte, now time.Time) error {
	reply, err := ps.startup.HandleWire(packet, uint64(now.UnixNano()))
	if err != nil { return err }
	if len(reply) != 0 {
		if _, err := s.conn.WriteToUDP(reply, ps.peer); err != nil { return err }
	}
	if ps.startup.State() == control.StateFailed { return errors.New("session state failed; reconnect required") }
	if ps.startup.State() == control.StateClosed {
		reason := ps.startup.Stats().CloseReason
		fmt.Printf("WBD_LINK_MUX_SESSION_CLOSE tunnel_id_prefix=%s reason=%d\n", printableSID(ps), reason)
		s.removePeer(ps.key, true)
		return nil
	}
	if ps.startup.State() != control.StateEstablished || ps.active { return nil }
	if !ps.haveIdentity || ps.account == "" || ps.id == (session.LiveID{}) || ps.sid == "" { return errors.New("established lane lacks Logical Tunnel ticket binding") }
	if _, ok := peerTunnelBinding(ps); !ok { return errors.New("established lane lacks Logical Tunnel binding") }
	if err := s.plane.Reserve(ps.account, ps.id, ps.key, now); err != nil { return err }
	cfg := ps.startup.Stats().Config
	if err := s.plane.Activate(ps.id, cfg); err != nil { s.plane.Remove(ps.id); return err }
	ps.active = true
	ps.touch(now)
	fmt.Printf("WBD_LINK_MUX_SESSION_READY tunnel_id_prefix=%s fec_mode=%d fec=%d:%d mtu=%d lanes=%d backend=pending\n", ps.sid, cfg.FECMode, cfg.DataShards, cfg.ParityShards, cfg.MTU, cfg.LaneCount)
	return nil
}

func (s *server) ensureService(ps *peerSession, packet []byte) error {
	if ps.service != nil { return nil }
	backend, err := classifyServicePayload(packet)
	if err != nil { return err }
	var addr *net.UDPAddr
	switch backend {
	case backendGame, backendPlatform:
		addr = s.serviceAddr
	case backendRawIP:
		addr = s.rawIPServiceAddr
		if addr == nil { return errors.New("raw-IP application datagram received but -raw-ip-service is not configured") }
	default:
		return fmt.Errorf("unsupported backend %q", backend)
	}
	service, err := net.DialUDP("udp4", nil, addr)
	if err != nil { return err }
	// Raw-IP and Game product backends both require server-authenticated Logical
	// Tunnel metadata before payload. For Game this metadata is consumed by the
	// private race server and forwarded once to the shared-TUN gateway.
	if backend == backendRawIP || backend == backendGame {
		meta, err := marshalPeerTunnelMeta(ps)
		if err != nil { _ = service.Close(); return err }
		if _, err := service.Write(meta); err != nil {
			_ = service.Close()
			return fmt.Errorf("write authenticated Logical Tunnel metadata: %w", err)
		}
	}
	ps.backend = backend
	ps.service = service
	go s.serviceLoop(ps, service)
	local := "unknown"
	if service.LocalAddr() != nil { local = service.LocalAddr().String() }
	fmt.Printf("WBD_LINK_MUX_BACKEND_READY tunnel_id_prefix=%s backend=%s service_local=%s\n", ps.sid, backend, local)
	return nil
}

func classifyServicePayload(packet []byte) (serviceBackend, error) {
	if _, err := dataplane.UnmarshalIP(packet); err == nil { return backendRawIP, nil }
	if _, _, err := gamelane.Parse(packet); err == nil { return backendGame, nil }
	if _, err := platformproxy.Unmarshal(packet); err == nil { return backendPlatform, nil }
	return "", errors.New("application datagram is neither Game envelope, M6A raw-IP nor platformproxy frame")
}

func (s *server) serviceLoop(ps *peerSession, service *net.UDPConn) {
	buf := make([]byte, 65535)
	for {
		n, err := service.Read(buf)
		if err != nil { return }
		if isRawIPBackendMeta(buf[:n]) { continue }
		now := time.Now()
		ps.linkTx.Add(1)
		ps.linkTxFirst.Do(func() { fmt.Printf("WBD_LINK_TX_FIRST tunnel_id_prefix=%s bytes=%d backend=%s\n", ps.sid, n, ps.backend) })
		peerKey, wire, err := s.plane.Outbound(ps.id, buf[:n], now)
		if err != nil || peerKey != ps.key {
			ps.drop.Add(1)
			if err != nil { fmt.Fprintf(os.Stderr, "WBD_LINK_SERVER_MUX_SERVICE_DROP tunnel_id_prefix=%s backend=%s err=%v\n", ps.sid, ps.backend, err) }
			return
		}
		if err := sendWire(s.conn, ps.peer, wire); err != nil { ps.drop.Add(1); return }
		ps.touch(now)
	}
}

func (s *server) flushDue(now time.Time) {
	for _, ps := range s.snapshotPeers() {
		if !ps.active { continue }
		peerKey, wire, err := s.plane.FlushDue(ps.id, now)
		if err != nil || peerKey != ps.key { continue }
		_ = sendWire(s.conn, ps.peer, wire)
	}
}

func (s *server) expirePeers(now time.Time) {
	for _, ps := range s.snapshotPeers() {
		if !ps.active {
			if now.Sub(ps.created) > s.cfg.setupTimeout { s.removePeer(ps.key, false) }
			continue
		}
		if ps.idleFor(now) < s.cfg.idleTimeout { continue }
		if wire, err := control.MarshalLink(control.Close{Reason: control.CloseIdleTimeout, Detail: "session idle lease expired"}); err == nil { _, _ = s.conn.WriteToUDP(wire, ps.peer) }
		fmt.Printf("WBD_LINK_MUX_SESSION_CLOSE tunnel_id_prefix=%s reason=%d\n", ps.sid, control.CloseIdleTimeout)
		s.removePeer(ps.key, true)
	}
}

func (s *server) getPeer(key string) *peerSession {
	s.mu.RLock(); ps := s.peers[key]; s.mu.RUnlock(); return ps
}

func (s *server) snapshotPeers() []*peerSession {
	s.mu.RLock()
	out := make([]*peerSession, 0, len(s.peers))
	for _, ps := range s.peers { out = append(out, ps) }
	s.mu.RUnlock()
	return out
}

func (s *server) removePeer(key string, flush bool) {
	s.mu.Lock(); ps := s.peers[key]; if ps != nil { delete(s.peers, key) }; s.mu.Unlock()
	if ps == nil { return }
	if ps.active {
		if flush { if peerKey, wire, err := s.plane.Flush(ps.id); err == nil && peerKey == ps.key { _ = sendWire(s.conn, ps.peer, wire) } }
		s.plane.Remove(ps.id)
	}
	if ps.service != nil { _ = ps.service.Close() }
	forgetPeerTunnel(ps)
	if ps.sid != "" { fmt.Printf("WBD_LINK_SESSION_COUNTERS tunnel_id_prefix=%s tx=%d rx=%d drop=%d\n", ps.sid, ps.linkTx.Load(), ps.linkRx.Load(), ps.drop.Load()) }
}

func (s *server) Close() {
	for _, ps := range s.snapshotPeers() { s.removePeer(ps.key, true) }
	if s.conn != nil { _ = s.conn.Close() }
}

func printableSID(ps *peerSession) string {
	if ps == nil || ps.sid == "" { return "pending" }
	return ps.sid
}

func controlFrameType(packet []byte) (control.Type, bool) {
	if len(packet) < control.HeaderLen || string(packet[:4]) != string(control.Magic[:]) || packet[4] != control.FrameVersion1 { return 0, false }
	return control.Type(packet[5]), true
}

func isStartupControl(packet []byte) bool {
	typ, ok := controlFrameType(packet)
	if !ok { return false }
	switch typ {
	case control.TypeDemoBind, control.TypeDemoBindOK, control.TypeLinkInit, control.TypeLinkAccept, control.TypeError, control.TypeAuth, control.TypeAuthOK:
		return true
	default:
		return false
	}
}

func isLifecycleControl(packet []byte) bool {
	typ, ok := controlFrameType(packet)
	if !ok { return false }
	switch typ {
	case control.TypePing, control.TypePong, control.TypeClose:
		return true
	default:
		return false
	}
}

func sendWire(conn *net.UDPConn, dst *net.UDPAddr, wire [][]byte) error {
	for _, packet := range wire { if _, err := conn.WriteToUDP(packet, dst); err != nil { return err } }
	return nil
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil { return nil }
	return &net.UDPAddr{IP: append(net.IP(nil), a.IP...), Port: a.Port, Zone: a.Zone}
}
