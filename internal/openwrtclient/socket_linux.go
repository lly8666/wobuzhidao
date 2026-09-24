//go:build linux

package openwrtclient

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/platformflow"
)

type replySocket struct {
	conn     *net.UDPConn
	lastSeen time.Time
}

const (
	udpIngressBudgetRecords = 1024
	udpIngressWorkers       = 4
)

type UDPIngressDiagnostic struct {
	QueueCapacityRecords int           `json:"queue_capacity_records"`
	QueueCapacityBytes   int           `json:"queue_capacity_bytes"`
	QueueCurrent         int           `json:"queue_current"`
	QueueBytes           int           `json:"queue_bytes"`
	InFlightCurrent      int           `json:"inflight_current"`
	InFlightBytes        int           `json:"inflight_bytes"`
	TotalPeak            int           `json:"total_peak"`
	TotalBytesPeak       int           `json:"total_bytes_peak"`
	Enqueued             uint64        `json:"enqueued"`
	Dequeued             uint64        `json:"dequeued"`
	OverflowDrops        uint64        `json:"overflow_drops"`
	OverflowBytes        uint64        `json:"overflow_bytes"`
	OverflowAgeMax       time.Duration `json:"overflow_age_max"`
	GateErrors           uint64        `json:"gate_errors"`
	ForwardErrors        uint64        `json:"forward_errors"`
	CloseDrops           uint64        `json:"close_drops"`
	Workers              int           `json:"workers"`
	EvictionMaxScan      int           `json:"eviction_max_scan"`
}

type udpIngressItem struct {
	client   netip.AddrPort
	target   netip.AddrPort
	payload  []byte
	queuedAt time.Time
}

type udpIngressShard struct {
	entries []udpIngressItem
	head    int
	size    int
	bytes   int
	cond    *sync.Cond
}

type udpIngressQueue struct {
	mu              sync.Mutex
	shards          []udpIngressShard
	capacityRecords int
	queueSize       int
	queueBytes      int
	inFlight        int
	inFlightBytes   int
	closed          bool

	totalPeak       int
	totalBytesPeak  int
	enqueued        uint64
	dequeued        uint64
	overflowDrops   uint64
	overflowBytes   uint64
	overflowAgeMax  time.Duration
	gateErrors      uint64
	forwardErrors   uint64
	closeDrops      uint64
	evictionMaxScan int
}

func newUDPIngressQueue(records int) *udpIngressQueue {
	if records <= 0 {
		records = udpIngressBudgetRecords
	}
	q := &udpIngressQueue{
		capacityRecords: records,
		shards:          make([]udpIngressShard, udpIngressWorkers),
	}
	for i := range q.shards {
		q.shards[i].entries = make([]udpIngressItem, records)
		q.shards[i].cond = sync.NewCond(&q.mu)
	}
	return q
}

func (q *udpIngressQueue) capacityBytes() int {
	if q == nil {
		return 0
	}
	return q.capacityRecords * platformflow.MaxPayload
}

func (q *udpIngressQueue) shardFor(client netip.AddrPort) int {
	if q == nil || len(q.shards) == 0 {
		return 0
	}
	addr := client.Addr().Unmap()
	v := uint64(client.Port())
	if addr.Is4() {
		b := addr.As4()
		v ^= uint64(b[0])<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3])
	}
	return int(v % uint64(len(q.shards)))
}

func (q *udpIngressQueue) dropOldestQueuedLocked(now time.Time) bool {
	oldestShard := -1
	var oldest time.Time
	scanned := 0
	for i := range q.shards {
		scanned++
		sh := &q.shards[i]
		if sh.size == 0 {
			continue
		}
		item := sh.entries[sh.head]
		if oldestShard < 0 || item.queuedAt.Before(oldest) {
			oldestShard = i
			oldest = item.queuedAt
		}
	}
	if scanned > q.evictionMaxScan {
		q.evictionMaxScan = scanned
	}
	if oldestShard < 0 {
		return false
	}
	sh := &q.shards[oldestShard]
	dropped := sh.entries[sh.head]
	sh.entries[sh.head] = udpIngressItem{}
	sh.head = (sh.head + 1) % len(sh.entries)
	sh.size--
	sh.bytes -= len(dropped.payload)
	q.queueSize--
	q.queueBytes -= len(dropped.payload)
	q.overflowDrops++
	q.overflowBytes += uint64(len(dropped.payload))
	if age := now.Sub(dropped.queuedAt); age > q.overflowAgeMax {
		q.overflowAgeMax = age
	}
	return true
}

func (q *udpIngressQueue) enqueue(item udpIngressItem, now time.Time) bool {
	if q == nil || !item.client.IsValid() || !item.target.IsValid() || len(item.payload) > platformflow.MaxPayload {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	if q.queueSize+q.inFlight >= q.capacityRecords {
		if !q.dropOldestQueuedLocked(now) {
			q.overflowDrops++
			q.overflowBytes += uint64(len(item.payload))
			return true
		}
	}
	if q.queueBytes+q.inFlightBytes+len(item.payload) > q.capacityBytes() {
		q.overflowDrops++
		q.overflowBytes += uint64(len(item.payload))
		return true
	}
	idx := q.shardFor(item.client)
	sh := &q.shards[idx]
	if sh.size >= len(sh.entries) {
		q.overflowDrops++
		q.overflowBytes += uint64(len(item.payload))
		return true
	}
	tail := (sh.head + sh.size) % len(sh.entries)
	sh.entries[tail] = item
	sh.size++
	sh.bytes += len(item.payload)
	q.queueSize++
	q.queueBytes += len(item.payload)
	q.enqueued++
	total := q.queueSize + q.inFlight
	if total > q.totalPeak {
		q.totalPeak = total
	}
	totalBytes := q.queueBytes + q.inFlightBytes
	if totalBytes > q.totalBytesPeak {
		q.totalBytesPeak = totalBytes
	}
	sh.cond.Signal()
	return true
}

func (q *udpIngressQueue) popShard(idx int) (udpIngressItem, bool) {
	if q == nil || idx < 0 || idx >= len(q.shards) {
		return udpIngressItem{}, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	sh := &q.shards[idx]
	for sh.size == 0 && !q.closed {
		sh.cond.Wait()
	}
	if q.closed || sh.size == 0 {
		return udpIngressItem{}, false
	}
	item := sh.entries[sh.head]
	sh.entries[sh.head] = udpIngressItem{}
	sh.head = (sh.head + 1) % len(sh.entries)
	sh.size--
	sh.bytes -= len(item.payload)
	q.queueSize--
	q.queueBytes -= len(item.payload)
	q.inFlight++
	q.inFlightBytes += len(item.payload)
	q.dequeued++
	return item, true
}

func (q *udpIngressQueue) finish(item udpIngressItem, gateErr, forwardErr bool) {
	if q == nil {
		return
	}
	q.mu.Lock()
	if q.inFlight > 0 {
		q.inFlight--
	}
	if n := len(item.payload); n <= q.inFlightBytes {
		q.inFlightBytes -= n
	} else {
		q.inFlightBytes = 0
	}
	if gateErr {
		q.gateErrors++
	}
	if forwardErr {
		q.forwardErrors++
	}
	q.mu.Unlock()
}

func (q *udpIngressQueue) close() {
	if q == nil {
		return
	}
	q.mu.Lock()
	if !q.closed {
		q.closed = true
		q.closeDrops += uint64(q.queueSize)
		for i := range q.shards {
			sh := &q.shards[i]
			for sh.size > 0 {
				sh.entries[sh.head] = udpIngressItem{}
				sh.head = (sh.head + 1) % len(sh.entries)
				sh.size--
			}
			sh.bytes = 0
			sh.cond.Broadcast()
		}
		q.queueSize = 0
		q.queueBytes = 0
	}
	q.mu.Unlock()
}

func (q *udpIngressQueue) snapshot() UDPIngressDiagnostic {
	if q == nil {
		return UDPIngressDiagnostic{}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return UDPIngressDiagnostic{
		QueueCapacityRecords: q.capacityRecords,
		QueueCapacityBytes:   q.capacityBytes(),
		QueueCurrent:         q.queueSize,
		QueueBytes:           q.queueBytes,
		InFlightCurrent:      q.inFlight,
		InFlightBytes:        q.inFlightBytes,
		TotalPeak:            q.totalPeak,
		TotalBytesPeak:       q.totalBytesPeak,
		Enqueued:             q.enqueued,
		Dequeued:             q.dequeued,
		OverflowDrops:        q.overflowDrops,
		OverflowBytes:        q.overflowBytes,
		OverflowAgeMax:       q.overflowAgeMax,
		GateErrors:           q.gateErrors,
		ForwardErrors:        q.forwardErrors,
		CloseDrops:           q.closeDrops,
		Workers:              len(q.shards),
		EvictionMaxScan:      q.evictionMaxScan,
	}
}

type SocketAdapter struct {
	cfg    SocketConfig
	udp    *net.UDPConn
	tcp    *net.TCPListener
	client *platformflow.Client

	replyMu sync.Mutex
	replies map[netip.AddrPort]*replySocket

	udpIngress   *udpIngressQueue
	udpIngressWG sync.WaitGroup

	runMu   sync.Mutex
	running bool
	closeOnce sync.Once
	closeErr  error
}

func OpenSocketAdapter(cfg SocketConfig) (*SocketAdapter, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	addr := "0.0.0.0:" + strconv.Itoa(int(cfg.ListenPort))
	udp, err := listenTransparentUDP4(addr, true)
	if err != nil {
		return nil, err
	}
	tcp, err := listenTransparentTCP4(addr)
	if err != nil {
		_ = udp.Close()
		return nil, err
	}
	a := &SocketAdapter{
		cfg: cfg, udp: udp, tcp: tcp,
		replies: make(map[netip.AddrPort]*replySocket),
	}
	client, err := platformflow.NewClient(cfg.Channel, cfg.Client, a.replyUDP)
	if err != nil {
		_ = udp.Close()
		_ = tcp.Close()
		return nil, err
	}
	a.client = client
	a.udpIngress = newUDPIngressQueue(udpIngressBudgetRecords)
	a.udpIngressWG.Add(udpIngressWorkers)
	for i := 0; i < udpIngressWorkers; i++ {
		go a.udpIngressWorker(i)
	}
	return a, nil
}

func (a *SocketAdapter) Run(ctx context.Context) error {
	if a == nil {
		return ErrSocketClosed
	}
	if ctx == nil {
		return platformflow.ErrMalformed
	}
	a.runMu.Lock()
	if a.running {
		a.runMu.Unlock()
		return platformflow.ErrMalformed
	}
	a.running = true
	a.runMu.Unlock()
	defer func() {
		a.runMu.Lock()
		a.running = false
		a.runMu.Unlock()
	}()

	errCh := make(chan error, 2)
	go func() { errCh <- a.udpLoop() }()
	go func() { errCh <- a.tcpLoop() }()
	ticker := time.NewTicker(a.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = a.Close()
			return ctx.Err()
		case err := <-errCh:
			_ = a.Close()
			if errors.Is(err, net.ErrClosed) {
				return ErrSocketClosed
			}
			return err
		case now := <-ticker.C:
			a.client.Tick(now)
			a.expireReplies(now)
		}
	}
}

func (a *SocketAdapter) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		var errs []error
		if a.udp != nil {
			if err := a.udp.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				errs = append(errs, err)
			}
		}
		if a.tcp != nil {
			if err := a.tcp.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				errs = append(errs, err)
			}
		}
		if a.udpIngress != nil {
			a.udpIngress.close()
			a.udpIngressWG.Wait()
		}
		if a.client != nil {
			a.client.Close()
		}
		a.replyMu.Lock()
		replies := make([]*net.UDPConn, 0, len(a.replies))
		for peer, state := range a.replies {
			delete(a.replies, peer)
			replies = append(replies, state.conn)
		}
		a.replyMu.Unlock()
		for _, conn := range replies {
			if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				errs = append(errs, err)
			}
		}
		a.closeErr = errors.Join(errs...)
	})
	return a.closeErr
}

// DeliverFromOwner is the in-process reverse path from the leased TunnelOwner.
// OpenWrt socket mode owns only platform-service packets; ordinary leased IPv4
// is not silently accepted by this adapter.
func (a *SocketAdapter) DeliverFromOwner(packets [][]byte, now time.Time) error {
	if a == nil || a.client == nil {
		return ErrSocketClosed
	}
	for _, packet := range packets {
		handled, err := a.client.HandleServicePacket(packet, now)
		if err != nil {
			return err
		}
		if !handled {
			return platformflow.ErrUnsupported
		}
	}
	return nil
}

func (a *SocketAdapter) udpLoop() error {
	payload := make([]byte, platformflow.MaxPayload+1)
	oob := make([]byte, 256)
	for {
		n, oobn, flags, from, err := a.udp.ReadMsgUDP(payload, oob)
		if err != nil {
			return err
		}
		if flags&syscall.MSG_CTRUNC != 0 || n > platformflow.MaxPayload || from == nil {
			continue
		}
		target, err := udpOriginalDst4(oob[:oobn])
		if err != nil {
			continue
		}
		client := from.AddrPort()
		if !client.Addr().Unmap().Is4() {
			continue
		}
		now := time.Now()
		item := udpIngressItem{
			client: client, target: target,
			payload: append([]byte(nil), payload[:n]...),
			queuedAt: now,
		}
		if !a.udpIngress.enqueue(item, now) {
			return ErrSocketClosed
		}
	}
}

func (a *SocketAdapter) udpIngressWorker(shard int) {
	defer a.udpIngressWG.Done()
	for {
		item, ok := a.udpIngress.popShard(shard)
		if !ok {
			return
		}
		gateErr := false
		forwardErr := false
		if a.cfg.BeforeBusiness != nil {
			if err := a.cfg.BeforeBusiness(); err != nil {
				gateErr = true
			}
		}
		if !gateErr {
			if err := a.client.ForwardUDP(item.client, item.target, item.payload, time.Now()); err != nil {
				forwardErr = true
			}
		}
		a.udpIngress.finish(item, gateErr, forwardErr)
	}
}

func (a *SocketAdapter) UDPIngressDiagnostic() UDPIngressDiagnostic {
	if a == nil || a.udpIngress == nil {
		return UDPIngressDiagnostic{}
	}
	return a.udpIngress.snapshot()
}

func (a *SocketAdapter) tcpLoop() error {
	for {
		conn, err := a.tcp.AcceptTCP()
		if err != nil {
			return err
		}
		target, err := tcpOriginalDst4(conn)
		if err != nil {
			_ = conn.Close()
			continue
		}
		if a.cfg.BeforeBusiness != nil {
			if err := a.cfg.BeforeBusiness(); err != nil {
				_ = conn.Close()
				continue
			}
		}
		if _, err := a.client.AddTCP(conn, target, time.Now()); err != nil {
			_ = conn.Close()
		}
	}
}

func (a *SocketAdapter) replyUDP(peer, client netip.AddrPort, payload []byte) error {
	if !peer.Addr().Unmap().Is4() || !client.Addr().Unmap().Is4() {
		return ErrSocketUnsupported
	}
	now := time.Now()
	a.replyMu.Lock()
	state := a.replies[peer]
	if state == nil {
		if len(a.replies) >= a.cfg.MaxReplySockets {
			var oldestPeer netip.AddrPort
			var oldest *replySocket
			for candidate, current := range a.replies {
				if oldest == nil || current.lastSeen.Before(oldest.lastSeen) {
					oldestPeer, oldest = candidate, current
				}
			}
			if oldest != nil {
				delete(a.replies, oldestPeer)
				_ = oldest.conn.Close()
			}
		}
		conn, err := listenTransparentUDPSource4(peer)
		if err != nil {
			a.replyMu.Unlock()
			return err
		}
		state = &replySocket{conn: conn, lastSeen: now}
		a.replies[peer] = state
	} else {
		state.lastSeen = now
	}
	conn := state.conn
	a.replyMu.Unlock()

	if _, err := conn.WriteToUDPAddrPort(payload, client); err != nil {
		a.replyMu.Lock()
		if a.replies[peer] == state {
			delete(a.replies, peer)
		}
		a.replyMu.Unlock()
		_ = conn.Close()
		return err
	}
	return nil
}

func (a *SocketAdapter) expireReplies(now time.Time) {
	var stale []*net.UDPConn
	a.replyMu.Lock()
	for peer, state := range a.replies {
		if now.Before(state.lastSeen) || now.Sub(state.lastSeen) < a.cfg.Client.UDPIdle {
			continue
		}
		delete(a.replies, peer)
		stale = append(stale, state.conn)
	}
	a.replyMu.Unlock()
	for _, conn := range stale {
		_ = conn.Close()
	}
}

func listenTransparentUDP4(address string, originalDst bool) (*net.UDPConn, error) {
	lc := net.ListenConfig{Control: func(network, _ string, raw syscall.RawConn) error {
		var sockErr error
		if err := raw.Control(func(fd uintptr) {
			sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
			if sockErr == nil && originalDst {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1)
			}
		}); err != nil {
			return err
		}
		return sockErr
	}}
	pc, err := lc.ListenPacket(context.Background(), "udp4", address)
	if err != nil {
		return nil, err
	}
	udp, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, fmt.Errorf("openwrtclient: UDP listener type %T", pc)
	}
	return udp, nil
}

func listenTransparentUDPSource4(peer netip.AddrPort) (*net.UDPConn, error) {
	if !peer.IsValid() || peer.Port() == 0 || !peer.Addr().Unmap().Is4() {
		return nil, platformflow.ErrMalformed
	}
	return listenTransparentUDP4(peer.String(), false)
}

func listenTransparentTCP4(address string) (*net.TCPListener, error) {
	lc := net.ListenConfig{Control: func(_ string, _ string, raw syscall.RawConn) error {
		var sockErr error
		if err := raw.Control(func(fd uintptr) {
			sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
		}); err != nil {
			return err
		}
		return sockErr
	}}
	ln, err := lc.Listen(context.Background(), "tcp4", address)
	if err != nil {
		return nil, err
	}
	tcp, ok := ln.(*net.TCPListener)
	if !ok {
		_ = ln.Close()
		return nil, fmt.Errorf("openwrtclient: TCP listener type %T", ln)
	}
	return tcp, nil
}

func tcpOriginalDst4(conn *net.TCPConn) (netip.AddrPort, error) {
	if conn == nil {
		return netip.AddrPort{}, platformflow.ErrMalformed
	}
	local, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok || local == nil {
		return netip.AddrPort{}, platformflow.ErrMalformed
	}
	target := local.AddrPort()
	if !target.IsValid() || target.Port() == 0 || !target.Addr().Unmap().Is4() || target.Addr().IsUnspecified() {
		return netip.AddrPort{}, platformflow.ErrMalformed
	}
	return target, nil
}

func udpOriginalDst4(oob []byte) (netip.AddrPort, error) {
	messages, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return netip.AddrPort{}, err
	}
	for _, message := range messages {
		if message.Header.Level != syscall.SOL_IP || message.Header.Type != syscall.IP_ORIGDSTADDR {
			continue
		}
		if len(message.Data) < 8 {
			return netip.AddrPort{}, platformflow.ErrMalformed
		}
		port := binary.BigEndian.Uint16(message.Data[2:4])
		var raw [4]byte
		copy(raw[:], message.Data[4:8])
		out := netip.AddrPortFrom(netip.AddrFrom4(raw), port)
		if !out.IsValid() || out.Port() == 0 || out.Addr().IsUnspecified() {
			return netip.AddrPort{}, platformflow.ErrMalformed
		}
		return out, nil
	}
	return netip.AddrPort{}, platformflow.ErrMalformed
}
