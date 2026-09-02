package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

type laneConn struct {
	id   uint8
	addr string
	conn *net.UDPConn
	tx   uint64
	rx   uint64
}

type laneRaceGroup struct {
	primary *laneConn
	overlap *laneConn
}

type client struct {
	app     *net.UDPConn
	control *net.UDPConn
	enc     *gamelane.Encoder
	dec     *gamelane.Decoder
	pacer   *gamelane.InnerPacer

	lanesMu sync.RWMutex
	lanes   map[uint8]*laneConn
	overlap map[uint8]*laneConn

	peerMu sync.RWMutex
	peer   *net.UDPAddr
	decMu  sync.Mutex

	activity payloadActivityTracker

	logicalTX   uint64
	delivered   uint64
	duplicate   uint64
	stale       uint64
	laneFail    uint64
	dormantDrop uint64
}

func main() {
	var listen, lanesRaw, sessionHex, control string
	var replayWindow int
	var innerRateMbps float64
	flag.StringVar(&listen, "listen", "", "local UDP address used by the platform client")
	flag.StringVar(&lanesRaw, "lanes", "", "comma-separated initial independent wbd-link-proxy UDP addresses, 1..4")
	flag.StringVar(&control, "control", "", "optional IPv4 loopback UDP control address for atomic dynamic lane membership")
	flag.StringVar(&sessionHex, "session-id", "auto", "32 hex chars or auto")
	flag.IntVar(&replayWindow, "replay-window", 4096, "bounded first-arrival dedupe window")
	flag.Float64Var(&innerRateMbps, "inner-rate-mbps", 0, "logical inner Mbps ceiling from the external link-speed/FEC/lane controller; 0 disables pacing")
	flag.Parse()
	laneAddrs, err := parseLaneAddrs(lanesRaw)
	if err != nil { fatal(err) }
	if listen == "" { fatal(errors.New("-listen is required")) }
	sid, err := parseOrRandomSessionID(sessionHex)
	if err != nil { fatal(err) }
	enc, err := gamelane.NewEncoder(sid, 1)
	if err != nil { fatal(err) }
	dec, err := gamelane.NewDecoder(sid, replayWindow)
	if err != nil { fatal(err) }
	pacer, err := gamelane.NewInnerPacer(innerRateMbps)
	if err != nil { fatal(err) }
	la, err := net.ResolveUDPAddr("udp4", listen)
	if err != nil { fatal(err) }
	app, err := net.ListenUDP("udp4", la)
	if err != nil { fatal(err) }
	defer app.Close()
	c := &client{app: app, enc: enc, dec: dec, pacer: pacer, lanes: make(map[uint8]*laneConn, gamelane.MaxLanes), overlap: make(map[uint8]*laneConn, 1)}

	initial := make([]gamelane.LaneTarget, 0, len(laneAddrs))
	for i, addr := range laneAddrs {
		initial = append(initial, gamelane.LaneTarget{ID: uint8(i + 1), Address: addr.String()})
	}
	if _, err := c.setLaneTargets(initial); err != nil { fatal(err) }
	defer c.closeAllLanes()

	if strings.TrimSpace(control) != "" {
		ca, err := net.ResolveUDPAddr("udp4", control)
		if err != nil { fatal(err) }
		if ca.IP == nil || !ca.IP.IsLoopback() || ca.Port == 0 { fatal(errors.New("-control must be an IPv4 loopback address:port")) }
		ctrl, err := net.ListenUDP("udp4", ca)
		if err != nil { fatal(err) }
		c.control = ctrl
		defer ctrl.Close()
	}
	_ = app.SetReadBuffer(4 << 20)
	_ = app.SetWriteBuffer(4 << 20)

	errCh := make(chan error, 2)
	go func() { errCh <- c.appLoop() }()
	if c.control != nil { go func() { errCh <- c.controlLoop() }() }

	controlAddr := "off"
	if c.control != nil { controlAddr = c.control.LocalAddr().String() }
	fmt.Printf("WBD_GAME_LANE_CLIENT_READY listen=%s session_id=%x lanes=%d mode=race max_lanes=%d inner_ceiling_mbps=%.6f dynamic_membership=1 control=%s experimental=1\n",
		app.LocalAddr(), sid, len(c.activeIDs()), gamelane.MaxLanes, pacer.Mbps(), controlAddr)
	for _, lane := range c.activeLanes() {
		fmt.Printf("WBD_GAME_LANE_OUTER lane=%d local=%s proxy=%s association_required=independent\n", lane.id, lane.conn.LocalAddr(), lane.conn.RemoteAddr())
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case err := <-errCh:
		if err != nil && !errors.Is(err, net.ErrClosed) { fatal(err) }
	}
	fmt.Printf("WBD_GAME_LANE_CLIENT_STATS logical_tx=%d delivered=%d duplicate=%d stale=%d lane_fail=%d dormant_drop=%d\n",
		atomic.LoadUint64(&c.logicalTX), atomic.LoadUint64(&c.delivered), atomic.LoadUint64(&c.duplicate), atomic.LoadUint64(&c.stale), atomic.LoadUint64(&c.laneFail), atomic.LoadUint64(&c.dormantDrop))
	for _, lane := range c.activeLanes() {
		fmt.Printf("WBD_GAME_LANE_CLIENT_LANE_STATS lane=%d tx=%d rx=%d\n", lane.id, atomic.LoadUint64(&lane.tx), atomic.LoadUint64(&lane.rx))
	}
}

func (c *client) appLoop() error {
	buf := make([]byte, 65535)
	for {
		n, from, err := c.app.ReadFromUDP(buf)
		if err != nil { return err }
		if n == 0 { continue }
		if !c.acceptPeer(from) { continue }
		c.activity.mark(time.Now())
		groups := c.activeRaceGroups()
		if len(groups) == 0 { atomic.AddUint64(&c.dormantDrop, 1); continue }
		if wait := c.pacer.Reserve(n, time.Now()); wait > 0 { time.Sleep(wait) }
		laneIDs := make([]uint8, len(groups))
		for i, group := range groups { laneIDs[i] = group.primary.id }
		_, copies, err := c.enc.WrapCopies(buf[:n], laneIDs)
		if err != nil { return err }
		atomic.AddUint64(&c.logicalTX, 1)
		for i, copy := range copies {
			group := groups[i]
			if copy.LaneID != group.primary.id { return errors.New("lane copy ordering invariant violated") }
			targets := []*laneConn{group.primary}
			if group.overlap != nil { targets = append(targets, group.overlap) }
			for _, lane := range targets {
				if _, err := lane.conn.Write(copy.Wire); err != nil { c.failLane(lane, fmt.Errorf("write: %w", err)); continue }
				atomic.AddUint64(&lane.tx, 1)
			}
		}
	}
}

func (c *client) laneLoop(lane *laneConn) {
	buf := make([]byte, 65535)
	for {
		n, err := lane.conn.Read(buf)
		if err != nil { if !errors.Is(err, net.ErrClosed) { c.failLane(lane, fmt.Errorf("read: %w", err)) }; return }
		atomic.AddUint64(&lane.rx, 1)
		h, _, err := gamelane.Parse(buf[:n])
		if err != nil { c.failLane(lane, fmt.Errorf("parse: %w", err)); return }
		if h.LaneID != lane.id { c.failLane(lane, fmt.Errorf("received envelope for lane %d", h.LaneID)); return }
		c.decMu.Lock(); result, derr := c.dec.Add(buf[:n]); c.decMu.Unlock()
		if derr != nil {
			if errors.Is(derr, gamelane.ErrReplayTooOld) { atomic.AddUint64(&c.stale, 1); continue }
			c.failLane(lane, fmt.Errorf("decode: %w", derr)); return
		}
		if result.Duplicate { atomic.AddUint64(&c.duplicate, 1); continue }
		if !result.Deliver { continue }
		peer := c.appPeer(); if peer == nil { continue }
		if _, err := c.app.WriteToUDP(result.Payload, peer); err != nil { return }
		atomic.AddUint64(&c.delivered, 1)
	}
}

func (c *client) controlLoop() error {
	buf := make([]byte, 4096)
	for {
		n, peer, err := c.control.ReadFromUDP(buf)
		if err != nil { return err }
		reply := c.handleControlRequest(buf[:n])
		wire, _ := json.Marshal(reply)
		if _, err := c.control.WriteToUDP(wire, peer); err != nil { return err }
	}
}

func (c *client) setLaneTargets(targets []gamelane.LaneTarget) ([]uint8, error) {
	cmd := gamelane.LaneSetCommand{Op: gamelane.LaneControlSet, Lanes: targets}
	if err := cmd.Validate(); err != nil { return nil, err }
	targets = gamelane.CanonicalLaneTargets(targets)

	c.lanesMu.Lock()
	if c.lanes == nil { c.lanes = make(map[uint8]*laneConn, gamelane.MaxLanes) }
	if c.overlap == nil { c.overlap = make(map[uint8]*laneConn, 1) }
	grouped := make(map[uint8][]gamelane.LaneTarget, gamelane.MaxLanes)
	ids := make([]uint8, 0, gamelane.MaxLanes)
	for _, target := range targets {
		if len(grouped[target.ID]) == 0 { ids = append(ids, target.ID) }
		grouped[target.ID] = append(grouped[target.ID], target)
	}
	next := make(map[uint8]*laneConn, len(ids))
	nextOverlap := make(map[uint8]*laneConn, 1)
	created := make([]*laneConn, 0, len(targets))
	resolve := func(target gamelane.LaneTarget) (*laneConn, error) {
		if existing := c.lanes[target.ID]; existing != nil && existing.addr == target.Address { return existing, nil }
		if existing := c.overlap[target.ID]; existing != nil && existing.addr == target.Address { return existing, nil }
		ra, err := net.ResolveUDPAddr("udp4", target.Address)
		if err != nil { return nil, err }
		conn, err := net.DialUDP("udp4", nil, ra)
		if err != nil { return nil, err }
		_ = conn.SetReadBuffer(4 << 20); _ = conn.SetWriteBuffer(4 << 20)
		lane := &laneConn{id: target.ID, addr: target.Address, conn: conn}
		created = append(created, lane)
		return lane, nil
	}
	fail := func(err error) ([]uint8, error) {
		for _, lane := range created { _ = lane.conn.Close() }
		c.lanesMu.Unlock()
		return nil, err
	}
	for _, id := range ids {
		wanted := grouped[id]
		primaryIndex := 0
		if current := c.lanes[id]; current != nil {
			for i, target := range wanted {
				if target.Address == current.addr { primaryIndex = i; break }
			}
		}
		primary, err := resolve(wanted[primaryIndex])
		if err != nil { return fail(err) }
		next[id] = primary
		if len(wanted) == 2 {
			other := 1 - primaryIndex
			candidate, err := resolve(wanted[other])
			if err != nil { return fail(err) }
			nextOverlap[id] = candidate
		}
	}
	kept := make(map[*laneConn]bool, len(next)+len(nextOverlap))
	for _, lane := range next { kept[lane] = true }
	for _, lane := range nextOverlap { kept[lane] = true }
	removed := make([]*laneConn, 0, len(c.lanes)+len(c.overlap))
	for _, lane := range c.lanes { if !kept[lane] { removed = append(removed, lane) } }
	for _, lane := range c.overlap { if !kept[lane] { removed = append(removed, lane) } }
	c.lanes = next
	c.overlap = nextOverlap
	c.lanesMu.Unlock()

	for _, lane := range removed {
		c.announceLaneLeave(lane)
		_ = lane.conn.Close()
	}
	for _, lane := range created { go c.laneLoop(lane) }
	idsOut := c.activeIDs(); c.logLaneState(idsOut, "control"); return idsOut, nil
}

func (c *client) announceLaneLeave(lane *laneConn) {
	if lane == nil || lane.conn == nil || c.enc == nil { return }
	wire, err := gamelane.MarshalLaneLeave(c.enc.SessionID(), lane.id)
	if err != nil { fmt.Fprintf(os.Stderr,"WBD_GAME_LANE_CLIENT_LEAVE_FAIL lane=%d err=%v\n",lane.id,err); return }
	if _, err := lane.conn.Write(wire); err != nil { fmt.Fprintf(os.Stderr,"WBD_GAME_LANE_CLIENT_LEAVE_FAIL lane=%d proxy=%s err=%v\n",lane.id,lane.addr,err); return }
	fmt.Printf("WBD_GAME_LANE_CLIENT_LEAVE lane=%d proxy=%s\n",lane.id,lane.addr)
}

func (c *client) failLane(lane *laneConn, err error) {
	removed := false
	c.lanesMu.Lock()
	if current := c.lanes[lane.id]; current == lane {
		delete(c.lanes, lane.id)
		if standby := c.overlap[lane.id]; standby != nil {
			c.lanes[lane.id] = standby
			delete(c.overlap, lane.id)
		}
		removed = true
	} else if standby := c.overlap[lane.id]; standby == lane {
		delete(c.overlap, lane.id)
		removed = true
	}
	c.lanesMu.Unlock()
	if !removed { return }
	_ = lane.conn.Close(); atomic.AddUint64(&c.laneFail, 1)
	fmt.Fprintf(os.Stderr, "WBD_GAME_LANE_CLIENT_LANE_FAIL lane=%d proxy=%s err=%v\n", lane.id, lane.addr, err)
	c.logLaneState(c.activeIDs(), "lane_fail")
}

func (c *client) activeRaceGroups() []laneRaceGroup {
	c.lanesMu.RLock()
	out := make([]laneRaceGroup, 0, len(c.lanes))
	for id, lane := range c.lanes { out = append(out, laneRaceGroup{primary:lane, overlap:c.overlap[id]}) }
	c.lanesMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].primary.id < out[j].primary.id })
	return out
}

func (c *client) activeLanes() []*laneConn {
	groups := c.activeRaceGroups()
	out := make([]*laneConn, len(groups))
	for i, group := range groups { out[i] = group.primary }
	return out
}

func (c *client) activeIDs() []uint8 {
	lanes := c.activeLanes(); out := make([]uint8, len(lanes)); for i, lane := range lanes { out[i] = lane.id }; return out
}

func (c *client) logLaneState(ids []uint8, reason string) {
	parts := make([]string, len(ids)); for i, id := range ids { parts[i] = fmt.Sprintf("%d", id) }
	dormant := 0; if len(ids) == 0 { dormant = 1 }
	fmt.Printf("WBD_GAME_LANE_CLIENT_STATE active=%s count=%d dormant=%d reason=%s\n", strings.Join(parts, ","), len(ids), dormant, reason)
}

func (c *client) closeAllLanes() {
	c.lanesMu.Lock()
	lanes := make([]*laneConn, 0, len(c.lanes)+len(c.overlap))
	for _, lane := range c.lanes { lanes = append(lanes, lane) }
	for _, lane := range c.overlap { lanes = append(lanes, lane) }
	c.lanes = make(map[uint8]*laneConn)
	c.overlap = make(map[uint8]*laneConn)
	c.lanesMu.Unlock()
	for _, lane := range lanes { _ = lane.conn.Close() }
}

func parseLaneAddrs(raw string) ([]*net.UDPAddr, error) {
	parts := strings.Split(raw, ",")
	if strings.TrimSpace(raw) == "" || len(parts) == 0 || len(parts) > gamelane.MaxLanes { return nil, fmt.Errorf("-lanes requires 1..%d addresses", gamelane.MaxLanes) }
	out := make([]*net.UDPAddr, 0, len(parts)); seen := map[string]bool{}
	for _, part := range parts { a, err := net.ResolveUDPAddr("udp4", strings.TrimSpace(part)); if err != nil { return nil, err }; key := a.String(); if seen[key] { return nil, fmt.Errorf("duplicate lane proxy %s", key) }; seen[key] = true; out = append(out, a) }
	return out, nil
}

func (c *client) acceptPeer(from *net.UDPAddr) bool {
	c.peerMu.Lock(); defer c.peerMu.Unlock(); if c.peer == nil { c.peer = cloneUDPAddr(from); return true }; return c.peer.Port == from.Port && c.peer.IP.Equal(from.IP)
}

func (c *client) appPeer() *net.UDPAddr { c.peerMu.RLock(); defer c.peerMu.RUnlock(); return cloneUDPAddr(c.peer) }

func parseOrRandomSessionID(raw string) (gamelane.SessionID, error) {
	var id gamelane.SessionID
	if raw == "" || raw == "auto" { if _, err := rand.Read(id[:]); err != nil { return id, err }; if id == (gamelane.SessionID{}) { id[0] = 1 }; return id, nil }
	b, err := hex.DecodeString(raw); if err != nil || len(b) != len(id) { return id, errors.New("-session-id must be exactly 32 hex chars or auto") }
	copy(id[:], b); if id == (gamelane.SessionID{}) { return id, errors.New("zero session id is invalid") }; return id, nil
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr { if a == nil { return nil }; return &net.UDPAddr{IP: append(net.IP(nil), a.IP...), Port: a.Port, Zone: a.Zone} }

func fatal(err error) { fmt.Fprintln(os.Stderr, "WBD_GAME_LANE_CLIENT_FAIL", err); os.Exit(1) }
