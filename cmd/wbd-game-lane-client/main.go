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

type client struct {
	app     *net.UDPConn
	control *net.UDPConn
	enc     *gamelane.Encoder
	dec     *gamelane.Decoder
	pacer   *gamelane.InnerPacer

	lanesMu sync.RWMutex
	lanes   map[uint8]*laneConn

	peerMu sync.RWMutex
	peer   *net.UDPAddr
	decMu  sync.Mutex

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
	c := &client{app: app, enc: enc, dec: dec, pacer: pacer, lanes: make(map[uint8]*laneConn, gamelane.MaxLanes)}

	initial := make([]gamelane.LaneTarget, 0, len(laneAddrs))
	for i, addr := range laneAddrs {
		initial = append(initial, gamelane.LaneTarget{ID: uint8(i + 1), Address: addr.String()})
	}
	if _, err := c.setLaneTargets(initial); err != nil { fatal(err) }
	defer c.closeAllLanes()

	if strings.TrimSpace(control) != "" {
		ca, err := net.ResolveUDPAddr("udp4", control)
		if err != nil { fatal(err) }
		if ca.IP == nil || !ca.IP.IsLoopback() || ca.Port == 0 {
			fatal(errors.New("-control must be an IPv4 loopback address:port"))
		}
		ctrl, err := net.ListenUDP("udp4", ca)
		if err != nil { fatal(err) }
		c.control = ctrl
		defer ctrl.Close()
	}
	_ = app.SetReadBuffer(4 << 20)
	_ = app.SetWriteBuffer(4 << 20)

	errCh := make(chan error, 2)
	go func() { errCh <- c.appLoop() }()
	if c.control != nil {
		go func() { errCh <- c.controlLoop() }()
	}

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
		lanes := c.activeLanes()
		if len(lanes) == 0 {
			atomic.AddUint64(&c.dormantDrop, 1)
			continue
		}
		if wait := c.pacer.Reserve(n, time.Now()); wait > 0 { time.Sleep(wait) }
		laneIDs := make([]uint8, len(lanes))
		for i, lane := range lanes { laneIDs[i] = lane.id }
		_, copies, err := c.enc.WrapCopies(buf[:n], laneIDs)
		if err != nil { return err }
		atomic.AddUint64(&c.logicalTX, 1)
		for i, copy := range copies {
			lane := lanes[i]
			if copy.LaneID != lane.id { return errors.New("lane copy ordering invariant violated") }
			if _, err := lane.conn.Write(copy.Wire); err != nil {
				c.failLane(lane, fmt.Errorf("write: %w", err))
				continue
			}
			atomic.AddUint64(&lane.tx, 1)
		}
	}
}

func (c *client) laneLoop(lane *laneConn) {
	buf := make([]byte, 65535)
	for {
		n, err := lane.conn.Read(buf)
		if err != nil {
			if !errors.Is(err, net.ErrClosed) { c.failLane(lane, fmt.Errorf("read: %w", err)) }
			return
		}
		atomic.AddUint64(&lane.rx, 1)
		h, _, err := gamelane.Parse(buf[:n])
		if err != nil { c.failLane(lane, fmt.Errorf("parse: %w", err)); return }
		if h.LaneID != lane.id { c.failLane(lane, fmt.Errorf("received envelope for lane %d", h.LaneID)); return }
		c.decMu.Lock()
		result, derr := c.dec.Add(buf[:n])
		c.decMu.Unlock()
		if derr != nil {
			if errors.Is(derr, gamelane.ErrReplayTooOld) {
				atomic.AddUint64(&c.stale, 1)
				continue
			}
			c.failLane(lane, fmt.Errorf("decode: %w", derr))
			return
		}
		if result.Duplicate { atomic.AddUint64(&c.duplicate, 1); continue }
		if !result.Deliver { continue }
		peer := c.appPeer()
		if peer == nil { continue }
		if _, err := c.app.WriteToUDP(result.Payload, peer); err != nil { return }
		atomic.AddUint64(&c.delivered, 1)
	}
}

func (c *client) controlLoop() error {
	buf := make([]byte, 4096)
	for {
		n, peer, err := c.control.ReadFromUDP(buf)
		if err != nil { return err }
		cmd, parseErr := gamelane.ParseLaneSetCommand(buf[:n])
		reply := gamelane.LaneControlReply{}
		if parseErr != nil {
			reply.Error = parseErr.Error()
		} else {
			active, applyErr := c.setLaneTargets(cmd.Lanes)
			if applyErr != nil { reply.Error = applyErr.Error() } else { reply.OK = true; reply.Active = active }
		}
		wire, _ := json.Marshal(reply)
		if _, err := c.control.WriteToUDP(wire, peer); err != nil { return err }
	}
}

// setLaneTargets is atomic from the application's point of view. All new local
// UDP lane sockets are created before the active map is swapped. If any target
// cannot be prepared, the old healthy membership is left untouched. This is the
// Game-side make-before-break barrier; the runtime is responsible for invoking
// it only after candidate FakeTCP + Reality + DTLS + LINK health succeeds.
func (c *client) setLaneTargets(targets []gamelane.LaneTarget) ([]uint8, error) {
	cmd := gamelane.LaneSetCommand{Op: gamelane.LaneControlSet, Lanes: targets}
	if err := cmd.Validate(); err != nil { return nil, err }
	targets = gamelane.CanonicalLaneTargets(targets)

	c.lanesMu.Lock()
	next := make(map[uint8]*laneConn, len(targets))
	created := make([]*laneConn, 0, len(targets))
	for _, target := range targets {
		if existing := c.lanes[target.ID]; existing != nil && existing.addr == target.Address {
			next[target.ID] = existing
			continue
		}
		ra, err := net.ResolveUDPAddr("udp4", target.Address)
		if err != nil {
			for _, lane := range created { _ = lane.conn.Close() }
			c.lanesMu.Unlock()
			return nil, err
		}
		conn, err := net.DialUDP("udp4", nil, ra)
		if err != nil {
			for _, lane := range created { _ = lane.conn.Close() }
			c.lanesMu.Unlock()
			return nil, err
		}
		_ = conn.SetReadBuffer(4 << 20)
		_ = conn.SetWriteBuffer(4 << 20)
		lane := &laneConn{id: target.ID, addr: target.Address, conn: conn}
		next[target.ID] = lane
		created = append(created, lane)
	}
	removed := make([]*laneConn, 0, len(c.lanes))
	for id, lane := range c.lanes {
		if next[id] != lane { removed = append(removed, lane) }
	}
	c.lanes = next
	c.lanesMu.Unlock()

	for _, lane := range removed { _ = lane.conn.Close() }
	for _, lane := range created { go c.laneLoop(lane) }
	ids := c.activeIDs()
	c.logLaneState(ids, "control")
	return ids, nil
}

func (c *client) failLane(lane *laneConn, err error) {
	removed := false
	c.lanesMu.Lock()
	if current := c.lanes[lane.id]; current == lane {
		delete(c.lanes, lane.id)
		removed = true
	}
	c.lanesMu.Unlock()
	if !removed { return }
	_ = lane.conn.Close()
	atomic.AddUint64(&c.laneFail, 1)
	fmt.Fprintf(os.Stderr, "WBD_GAME_LANE_CLIENT_LANE_FAIL lane=%d proxy=%s err=%v\n", lane.id, lane.addr, err)
	c.logLaneState(c.activeIDs(), "lane_fail")
}

func (c *client) activeLanes() []*laneConn {
	c.lanesMu.RLock()
	out := make([]*laneConn, 0, len(c.lanes))
	for _, lane := range c.lanes { out = append(out, lane) }
	c.lanesMu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].id < out[j].id })
	return out
}

func (c *client) activeIDs() []uint8 {
	lanes := c.activeLanes()
	out := make([]uint8, len(lanes))
	for i, lane := range lanes { out[i] = lane.id }
	return out
}

func (c *client) logLaneState(ids []uint8, reason string) {
	parts := make([]string, len(ids))
	for i, id := range ids { parts[i] = fmt.Sprintf("%d", id) }
	dormant := 0
	if len(ids) == 0 { dormant = 1 }
	fmt.Printf("WBD_GAME_LANE_CLIENT_STATE active=%s count=%d dormant=%d reason=%s\n", strings.Join(parts, ","), len(ids), dormant, reason)
}

func (c *client) closeAllLanes() {
	c.lanesMu.Lock()
	lanes := c.lanes
	c.lanes = make(map[uint8]*laneConn)
	c.lanesMu.Unlock()
	for _, lane := range lanes { _ = lane.conn.Close() }
}

func parseLaneAddrs(raw string) ([]*net.UDPAddr, error) {
	parts := strings.Split(raw, ",")
	if strings.TrimSpace(raw) == "" || len(parts) == 0 || len(parts) > gamelane.MaxLanes {
		return nil, fmt.Errorf("-lanes requires 1..%d addresses", gamelane.MaxLanes)
	}
	out := make([]*net.UDPAddr, 0, len(parts))
	seen := map[string]bool{}
	for _, part := range parts {
		a, err := net.ResolveUDPAddr("udp4", strings.TrimSpace(part))
		if err != nil { return nil, err }
		key := a.String()
		if seen[key] { return nil, fmt.Errorf("duplicate lane proxy %s", key) }
		seen[key] = true
		out = append(out, a)
	}
	return out, nil
}

func (c *client) acceptPeer(from *net.UDPAddr) bool {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()
	if c.peer == nil { c.peer = cloneUDPAddr(from); return true }
	return c.peer.Port == from.Port && c.peer.IP.Equal(from.IP)
}

func (c *client) appPeer() *net.UDPAddr {
	c.peerMu.RLock()
	defer c.peerMu.RUnlock()
	return cloneUDPAddr(c.peer)
}

func parseOrRandomSessionID(raw string) (gamelane.SessionID, error) {
	var id gamelane.SessionID
	if raw == "" || raw == "auto" {
		if _, err := rand.Read(id[:]); err != nil { return id, err }
		if id == (gamelane.SessionID{}) { id[0] = 1 }
		return id, nil
	}
	b, err := hex.DecodeString(raw)
	if err != nil || len(b) != len(id) { return id, errors.New("-session-id must be exactly 32 hex chars or auto") }
	copy(id[:], b)
	if id == (gamelane.SessionID{}) { return id, errors.New("zero session id is invalid") }
	return id, nil
}

func cloneUDPAddr(a *net.UDPAddr) *net.UDPAddr {
	if a == nil { return nil }
	return &net.UDPAddr{IP: append(net.IP(nil), a.IP...), Port: a.Port, Zone: a.Zone}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "WBD_GAME_LANE_CLIENT_FAIL", err)
	os.Exit(1)
}
