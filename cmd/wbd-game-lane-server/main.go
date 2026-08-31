package main

import (
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/rawipbackend"
)

type gameSession struct {
	id      gamelane.SessionID
	meta    rawipbackend.TunnelMeta
	dec     *gamelane.Decoder
	enc     *gamelane.Encoder
	service *net.UDPConn

	mu       sync.Mutex
	lanes    map[uint8]*net.UDPAddr
	peerLane map[string]uint8
	last     time.Time
	closed   bool
	inFirst  uint64
	inDup    uint64
	outLogic uint64
	outLane  uint64
}

type server struct {
	conn         *net.UDPConn
	serviceAddr  *net.UDPAddr
	replayWindow int
	maxSessions  int
	maxLanes     int
	idle         time.Duration

	mu          sync.Mutex
	sessions    map[gamelane.SessionID]*gameSession
	peerSession map[string]gamelane.SessionID
	peerMeta    map[string]rawipbackend.TunnelMeta
}

func main() {
	var listen, service string
	var maxSessions, replayWindow, maxLanes int
	var idle time.Duration
	flag.StringVar(&listen, "listen", "", "private UDP address receiving authenticated Game lane traffic from wbd-link-server-mux")
	flag.StringVar(&service, "service", "", "downstream shared-TUN/raw-IP service address")
	flag.IntVar(&maxSessions, "max-sessions", 32, "maximum logical game sessions")
	flag.IntVar(&maxLanes, "max-lanes", gamelane.MaxLanes, "maximum independent WBD associations per Logical Tunnel, 1..4")
	flag.IntVar(&replayWindow, "replay-window", 4096, "bounded first-arrival dedupe window")
	flag.DurationVar(&idle, "idle", 90*time.Second, "logical game session transport idle timeout")
	flag.Parse()
	if listen == "" || service == "" || maxSessions <= 0 || maxLanes < 1 || maxLanes > gamelane.MaxLanes || replayWindow < 64 || idle <= 0 {
		fatal(errors.New("-listen, -service and valid positive max-sessions/max-lanes/replay-window/idle are required"))
	}
	la, err := net.ResolveUDPAddr("udp4", listen)
	if err != nil { fatal(err) }
	sa, err := net.ResolveUDPAddr("udp4", service)
	if err != nil { fatal(err) }
	conn, err := net.ListenUDP("udp4", la)
	if err != nil { fatal(err) }
	s := &server{
		conn: conn, serviceAddr: sa, replayWindow: replayWindow, maxSessions: maxSessions, maxLanes: maxLanes, idle: idle,
		sessions: make(map[gamelane.SessionID]*gameSession), peerSession: make(map[string]gamelane.SessionID), peerMeta: make(map[string]rawipbackend.TunnelMeta),
	}
	defer s.Close()
	_ = conn.SetReadBuffer(4 << 20)
	_ = conn.SetWriteBuffer(4 << 20)

	fmt.Printf("WBD_GAME_LANE_SERVER_READY listen=%s service=%s max_sessions=%d max_lanes=%d mode=race product=1 authenticated_tunnel_meta=1\n", conn.LocalAddr(), sa, maxSessions, maxLanes)
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	errCh := make(chan error, 1)
	go func() { errCh <- s.Run() }()
	select {
	case <-sig:
	case err := <-errCh:
		if err != nil && !errors.Is(err, net.ErrClosed) { fatal(err) }
	}
}

func (s *server) Run() error {
	buf := make([]byte, 65535)
	for {
		if err := s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil { return err }
		n, peer, err := s.conn.ReadFromUDP(buf)
		now := time.Now()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				s.expire(now)
				continue
			}
			return err
		}
		if n == 0 { continue }
		if err := s.handle(peer, buf[:n], now); err != nil {
			fmt.Fprintf(os.Stderr, "WBD_GAME_LANE_SERVER_DROP peer=%s err=%v\n", peer, err)
		}
		s.expire(now)
	}
}

func (s *server) handle(peer *net.UDPAddr, wire []byte, now time.Time) error {
	if meta, ok := rawipbackend.UnmarshalTunnelMeta(wire); ok {
		return s.registerPeerMeta(peer, meta, now)
	}
	h, _, err := gamelane.Parse(wire)
	if err != nil { return err }
	meta, ok := s.metadataForPeer(peer)
	if !ok { return errors.New("Game lane requires authenticated Logical Tunnel metadata before payload") }
	if hex.EncodeToString(h.SessionID[:]) != string(meta.TunnelID) {
		return errors.New("Game SessionID does not match authenticated Logical Tunnel ID")
	}
	gs, err := s.bindLane(h.SessionID, h.LaneID, peer, meta, now)
	if err != nil { return err }

	gs.mu.Lock()
	if gs.closed {
		gs.mu.Unlock()
		return net.ErrClosed
	}
	if bound := gs.peerLane[peer.String()]; bound != h.LaneID {
		gs.mu.Unlock()
		return fmt.Errorf("peer %s lane changed from %d to %d", peer, bound, h.LaneID)
	}
	result, err := gs.dec.Add(wire)
	gs.last = now
	if err == nil {
		if result.Duplicate { gs.inDup++ }
		if result.Deliver { gs.inFirst++ }
	}
	gs.mu.Unlock()
	if err != nil {
		if errors.Is(err, gamelane.ErrReplayTooOld) { return nil }
		return err
	}
	if !result.Deliver { return nil }
	_, err = gs.service.Write(result.Payload)
	return err
}

func (s *server) registerPeerMeta(peer *net.UDPAddr, meta rawipbackend.TunnelMeta, now time.Time) error {
	if peer == nil || !meta.Address4.Is4() { return errors.New("invalid authenticated Logical Tunnel metadata") }
	key := peer.String()
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.peerMeta[key]; ok {
		if existing.TunnelID != meta.TunnelID || existing.Address4 != meta.Address4 {
			return errors.New("authenticated Logical Tunnel metadata changed for active lane peer")
		}
		if gs := s.sessions[s.peerSession[key]]; gs != nil {
			gs.mu.Lock(); gs.last = now; gs.mu.Unlock()
		}
		return nil
	}
	s.peerMeta[key] = meta
	fmt.Printf("WBD_GAME_LANE_TUNNEL_META_READY tunnel_id_prefix=%s address4=%s association_peer=%s\n", tunnelIDPrefix(meta), meta.Address4, key)
	return nil
}

func tunnelIDPrefix(meta rawipbackend.TunnelMeta) string {
	s := string(meta.TunnelID)
	if len(s) > 8 { return s[:8] }
	return s
}

func (s *server) metadataForPeer(peer *net.UDPAddr) (rawipbackend.TunnelMeta, bool) {
	if peer == nil { return rawipbackend.TunnelMeta{}, false }
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, ok := s.peerMeta[peer.String()]
	return meta, ok
}

func (s *server) bindLane(id gamelane.SessionID, laneID uint8, peer *net.UDPAddr, meta rawipbackend.TunnelMeta, now time.Time) (*gameSession, error) {
	key := peer.String()
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.peerSession[key]; ok && existing != id {
		return nil, errors.New("authenticated service peer changed logical game session")
	}
	gs := s.sessions[id]
	if gs == nil {
		if len(s.sessions) >= s.maxSessions { return nil, errors.New("logical game session limit reached") }
		dec, err := gamelane.NewDecoder(id, s.replayWindow)
		if err != nil { return nil, err }
		enc, err := gamelane.NewEncoder(id, 1)
		if err != nil { return nil, err }
		service, err := net.DialUDP("udp4", nil, s.serviceAddr)
		if err != nil { return nil, err }
		_ = service.SetReadBuffer(4 << 20)
		_ = service.SetWriteBuffer(4 << 20)
		metaWire, err := rawipbackend.MarshalTunnelMeta(meta.TunnelID, meta.Address4)
		if err != nil { _ = service.Close(); return nil, err }
		if _, err := service.Write(metaWire); err != nil { _ = service.Close(); return nil, fmt.Errorf("register Game Logical Tunnel downstream: %w", err) }
		gs = &gameSession{id: id, meta: meta, dec: dec, enc: enc, service: service, lanes: make(map[uint8]*net.UDPAddr, s.maxLanes), peerLane: make(map[string]uint8, s.maxLanes), last: now}
		s.sessions[id] = gs
		go s.serviceLoop(gs)
		fmt.Printf("WBD_GAME_LANE_SESSION_OPEN tunnel_id_prefix=%s address4=%s downstream_peer=%s\n", tunnelIDPrefix(meta), meta.Address4, service.LocalAddr())
	} else if gs.meta.TunnelID != meta.TunnelID || gs.meta.Address4 != meta.Address4 {
		return nil, errors.New("Game lane metadata does not match existing Logical Tunnel")
	}
	gs.mu.Lock()
	defer gs.mu.Unlock()
	if oldLane, ok := gs.peerLane[key]; ok {
		if oldLane != laneID { return nil, errors.New("one WBD association cannot impersonate another lane id") }
		gs.last = now
		return gs, nil
	}
	if oldPeer := gs.lanes[laneID]; oldPeer != nil && oldPeer.String() != key {
		return nil, errors.New("lane id already bound to another WBD association")
	}
	if len(gs.lanes) >= s.maxLanes {
		return nil, errors.New("logical game session lane limit reached")
	}
	gs.lanes[laneID] = cloneUDPAddr(peer)
	gs.peerLane[key] = laneID
	s.peerSession[key] = id
	gs.last = now
	fmt.Printf("WBD_GAME_LANE_BIND tunnel_id_prefix=%s lane=%d association_peer=%s lanes=%d\n", tunnelIDPrefix(meta), laneID, key, len(gs.lanes))
	return gs, nil
}

func (s *server) serviceLoop(gs *gameSession) {
	buf := make([]byte, 65535)
	for {
		n, err := gs.service.Read(buf)
		if err != nil { return }
		if _, ok := rawipbackend.UnmarshalTunnelMeta(buf[:n]); ok { continue }
		gs.mu.Lock()
		if gs.closed { gs.mu.Unlock(); return }
		laneIDs := make([]uint8, 0, len(gs.lanes))
		peers := make(map[uint8]*net.UDPAddr, len(gs.lanes))
		for id, peer := range gs.lanes {
			laneIDs = append(laneIDs, id)
			peers[id] = cloneUDPAddr(peer)
		}
		sort.Slice(laneIDs, func(i, j int) bool { return laneIDs[i] < laneIDs[j] })
		_, copies, err := gs.enc.WrapCopies(buf[:n], laneIDs)
		if err != nil { gs.mu.Unlock(); return }
		gs.outLogic++
		gs.last = time.Now()
		gs.mu.Unlock()
		for _, copy := range copies {
			peer := peers[copy.LaneID]
			if peer == nil { continue }
			if _, err := s.conn.WriteToUDP(copy.Wire, peer); err != nil { return }
			gs.mu.Lock(); gs.outLane++; gs.mu.Unlock()
		}
	}
}

func (s *server) expire(now time.Time) {
	var expired []gamelane.SessionID
	s.mu.Lock()
	for id, gs := range s.sessions {
		gs.mu.Lock()
		last := gs.last
		gs.mu.Unlock()
		if now.Sub(last) >= s.idle { expired = append(expired, id) }
	}
	s.mu.Unlock()
	for _, id := range expired { s.remove(id, "idle") }
}

func (s *server) remove(id gamelane.SessionID, reason string) {
	s.mu.Lock()
	gs := s.sessions[id]
	if gs == nil { s.mu.Unlock(); return }
	delete(s.sessions, id)
	gs.mu.Lock()
	for key := range gs.peerLane {
		delete(s.peerSession, key)
		delete(s.peerMeta, key)
	}
	gs.closed = true
	inFirst, inDup, outLogic, outLane := gs.inFirst, gs.inDup, gs.outLogic, gs.outLane
	gs.mu.Unlock()
	s.mu.Unlock()
	_ = gs.service.Close()
	fmt.Printf("WBD_GAME_LANE_SESSION_CLOSE tunnel_id_prefix=%s reason=%s in_first=%d in_dup=%d out_logical=%d out_lane=%d\n",
		tunnelIDPrefix(gs.meta), reason, inFirst, inDup, outLogic, outLane)
}

func (s *server) Close() {
	s.mu.Lock()
	ids := make([]gamelane.SessionID, 0, len(s.sessions))
	for id := range s.sessions { ids = append(ids, id) }
	s.mu.Unlock()
	for _, id := range ids { s.remove(id, "server_close") }
	if s.conn != nil { _ = s.conn.Close() }
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil { return nil }
	return &net.UDPAddr{IP: append(net.IP(nil), a.IP...), Port: a.Port, Zone: a.Zone}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "WBD_GAME_LANE_SERVER_FAIL", err)
	os.Exit(1)
}
