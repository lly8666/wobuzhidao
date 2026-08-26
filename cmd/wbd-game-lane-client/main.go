package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

type laneConn struct {
	id   uint8
	conn *net.UDPConn
	tx   uint64
	rx   uint64
}

type client struct {
	app   *net.UDPConn
	lanes []*laneConn
	enc   *gamelane.Encoder
	dec   *gamelane.Decoder

	peerMu sync.RWMutex
	peer   *net.UDPAddr
	decMu  sync.Mutex

	logicalTX uint64
	delivered uint64
	duplicate uint64
	stale     uint64
}

func main() {
	var listen, lanesRaw, sessionHex string
	var replayWindow int
	flag.StringVar(&listen, "listen", "", "local UDP address used by the platform client")
	flag.StringVar(&lanesRaw, "lanes", "", "comma-separated independent wbd-link-proxy UDP addresses, 1..4")
	flag.StringVar(&sessionHex, "session-id", "auto", "32 hex chars or auto")
	flag.IntVar(&replayWindow, "replay-window", 4096, "bounded first-arrival dedupe window")
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
	la, err := net.ResolveUDPAddr("udp4", listen)
	if err != nil { fatal(err) }
	app, err := net.ListenUDP("udp4", la)
	if err != nil { fatal(err) }
	defer app.Close()
	c := &client{app: app, enc: enc, dec: dec}
	for i, a := range laneAddrs {
		conn, err := net.DialUDP("udp4", nil, a)
		if err != nil { fatal(err) }
		_ = conn.SetReadBuffer(4 << 20)
		_ = conn.SetWriteBuffer(4 << 20)
		c.lanes = append(c.lanes, &laneConn{id: uint8(i + 1), conn: conn})
		defer conn.Close()
	}
	_ = app.SetReadBuffer(4 << 20)
	_ = app.SetWriteBuffer(4 << 20)

	errCh := make(chan error, len(c.lanes)+1)
	go func() { errCh <- c.appLoop() }()
	for i := range c.lanes {
		go func(index int) { errCh <- c.laneLoop(index) }(i)
	}

	fmt.Printf("WBD_GAME_LANE_CLIENT_READY listen=%s session_id=%x lanes=%d mode=race max_lanes=%d experimental=1\n",
		app.LocalAddr(), sid, len(c.lanes), gamelane.MaxLanes)
	for _, lane := range c.lanes {
		fmt.Printf("WBD_GAME_LANE_OUTER lane=%d local=%s proxy=%s association_required=independent\n", lane.id, lane.conn.LocalAddr(), lane.conn.RemoteAddr())
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	select {
	case <-sig:
	case err := <-errCh:
		if err != nil && !errors.Is(err, net.ErrClosed) { fatal(err) }
	}
	fmt.Printf("WBD_GAME_LANE_CLIENT_STATS logical_tx=%d delivered=%d duplicate=%d stale=%d\n",
		atomic.LoadUint64(&c.logicalTX), atomic.LoadUint64(&c.delivered), atomic.LoadUint64(&c.duplicate), atomic.LoadUint64(&c.stale))
	for _, lane := range c.lanes {
		fmt.Printf("WBD_GAME_LANE_CLIENT_LANE_STATS lane=%d tx=%d rx=%d\n", lane.id, atomic.LoadUint64(&lane.tx), atomic.LoadUint64(&lane.rx))
	}
}

func (c *client) appLoop() error {
	buf := make([]byte, 65535)
	laneIDs := make([]uint8, len(c.lanes))
	for i, lane := range c.lanes { laneIDs[i] = lane.id }
	for {
		n, from, err := c.app.ReadFromUDP(buf)
		if err != nil { return err }
		if n == 0 { continue }
		if !c.acceptPeer(from) { continue }
		_, copies, err := c.enc.WrapCopies(buf[:n], laneIDs)
		if err != nil { return err }
		atomic.AddUint64(&c.logicalTX, 1)
		for i, copy := range copies {
			lane := c.lanes[i]
			if copy.LaneID != lane.id { return errors.New("lane copy ordering invariant violated") }
			if _, err := lane.conn.Write(copy.Wire); err != nil { return err }
			atomic.AddUint64(&lane.tx, 1)
		}
	}
}

func (c *client) laneLoop(index int) error {
	buf := make([]byte, 65535)
	lane := c.lanes[index]
	for {
		n, err := lane.conn.Read(buf)
		if err != nil { return err }
		atomic.AddUint64(&lane.rx, 1)
		h, _, err := gamelane.Parse(buf[:n])
		if err != nil { return fmt.Errorf("lane %d parse: %w", lane.id, err) }
		if h.LaneID != lane.id {
			return fmt.Errorf("lane %d received envelope for lane %d", lane.id, h.LaneID)
		}
		c.decMu.Lock()
		result, derr := c.dec.Add(buf[:n])
		c.decMu.Unlock()
		if derr != nil {
			if errors.Is(derr, gamelane.ErrReplayTooOld) {
				atomic.AddUint64(&c.stale, 1)
				continue
			}
			return fmt.Errorf("lane %d decode: %w", lane.id, derr)
		}
		if result.Duplicate {
			atomic.AddUint64(&c.duplicate, 1)
			continue
		}
		if !result.Deliver { continue }
		peer := c.appPeer()
		if peer == nil { continue }
		if _, err := c.app.WriteToUDP(result.Payload, peer); err != nil { return err }
		atomic.AddUint64(&c.delivered, 1)
	}
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
	if c.peer == nil {
		c.peer = cloneUDPAddr(from)
		return true
	}
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
	if err != nil || len(b) != len(id) {
		return id, errors.New("-session-id must be exactly 32 hex chars or auto")
	}
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
