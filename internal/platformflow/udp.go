package platformflow

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"
)

const (
	DefaultUDPIdleTimeout = 60 * time.Second
	DefaultMaxUDPFlows    = 4096

	// UDP upstream mappings may have up to DefaultMaxUDPFlows sockets, so a
	// large queue per mapping is not a valid bound. Keep one shared budget for
	// all mapping replies owned by a UDPServer. The byte ceiling is derived
	// from the record ceiling and MaxPayload, so one O(1) oldest eviction is
	// always enough to admit one newest valid datagram.
	udpServerSendBudgetRecords = 1024
	udpServerSendBudgetBytes   = udpServerSendBudgetRecords * MaxPayload
	udpServerSendWorkers       = 4
)

type UDPReply func(peer, client netip.AddrPort, payload []byte) error

type UDPServerDiagnostic struct {
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
	StaleDrops           uint64        `json:"stale_drops"`
	SendErrors           uint64        `json:"send_errors"`
	CloseDrops           uint64        `json:"close_drops"`
	Workers              int           `json:"workers"`
	EvictionMaxScan      int           `json:"eviction_max_scan"`
}

type udpServerQueuedDatagram struct {
	state    *udpServerState
	peer     netip.AddrPort
	payload  []byte
	queuedAt time.Time
}

type udpServerSendShard struct {
	entries []udpServerQueuedDatagram
	head    int
	size    int
	bytes   int
	cond    *sync.Cond
}

type udpServerSendQueue struct {
	mu              sync.Mutex
	shards          []udpServerSendShard
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
	staleDrops      uint64
	sendErrors      uint64
	closeDrops      uint64
	evictionMaxScan int
}

func newUDPServerSendQueue(records int) *udpServerSendQueue {
	if records <= 0 {
		records = udpServerSendBudgetRecords
	}
	q := &udpServerSendQueue{
		capacityRecords: records,
		shards: make([]udpServerSendShard, udpServerSendWorkers),
	}
	for i := range q.shards {
		// Each ring can represent the entire global record budget, but actual
		// payload ownership is still constrained by capacityRecords globally.
		q.shards[i].entries = make([]udpServerQueuedDatagram, records)
		q.shards[i].cond = sync.NewCond(&q.mu)
	}
	return q
}

func (q *udpServerSendQueue) capacityBytes() int {
	if q == nil {
		return 0
	}
	return q.capacityRecords * MaxPayload
}

func (q *udpServerSendQueue) shardFor(flowID uint64) int {
	if q == nil || len(q.shards) == 0 {
		return 0
	}
	return int(flowID % uint64(len(q.shards)))
}

func (q *udpServerSendQueue) dropOldestQueuedLocked(now time.Time) bool {
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
	sh.entries[sh.head] = udpServerQueuedDatagram{}
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

func (q *udpServerSendQueue) enqueue(item udpServerQueuedDatagram, now time.Time) bool {
	if q == nil || item.state == nil || len(item.payload) > MaxPayload {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}

	// Global budget includes queued plus currently sending records. At record
	// capacity, inspect exactly the four fixed shard heads and evict one oldest
	// queued datagram. This remains O(1) independent of flow count/backlog.
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

	idx := q.shardFor(item.state.id)
	sh := &q.shards[idx]
	if sh.size >= len(sh.entries) {
		// Global capacity guarantees this should only be reachable for malformed
		// internal accounting; fail bounded rather than allocate.
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

func (q *udpServerSendQueue) popShard(idx int) (udpServerQueuedDatagram, bool) {
	if q == nil || idx < 0 || idx >= len(q.shards) {
		return udpServerQueuedDatagram{}, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	sh := &q.shards[idx]
	for sh.size == 0 && !q.closed {
		sh.cond.Wait()
	}
	if q.closed || sh.size == 0 {
		return udpServerQueuedDatagram{}, false
	}
	item := sh.entries[sh.head]
	sh.entries[sh.head] = udpServerQueuedDatagram{}
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

func (q *udpServerSendQueue) finish(item udpServerQueuedDatagram, stale, sendErr bool) {
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
	if stale {
		q.staleDrops++
	}
	if sendErr {
		q.sendErrors++
	}
	q.mu.Unlock()
}

func (q *udpServerSendQueue) close() {
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
				sh.entries[sh.head] = udpServerQueuedDatagram{}
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

func (q *udpServerSendQueue) snapshot() UDPServerDiagnostic {
	if q == nil {
		return UDPServerDiagnostic{}
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	return UDPServerDiagnostic{
		QueueCapacityRecords: q.capacityRecords,
		QueueCapacityBytes: q.capacityBytes(),
		QueueCurrent: q.queueSize,
		QueueBytes: q.queueBytes,
		InFlightCurrent: q.inFlight,
		InFlightBytes: q.inFlightBytes,
		TotalPeak: q.totalPeak,
		TotalBytesPeak: q.totalBytesPeak,
		Enqueued: q.enqueued,
		Dequeued: q.dequeued,
		OverflowDrops: q.overflowDrops,
		OverflowBytes: q.overflowBytes,
		OverflowAgeMax: q.overflowAgeMax,
		StaleDrops: q.staleDrops,
		SendErrors: q.sendErrors,
		CloseDrops: q.closeDrops,
		Workers: len(q.shards),
		EvictionMaxScan: q.evictionMaxScan,
	}
}

type udpClientState struct {
	id       uint64
	client   netip.AddrPort
	ipv4     bool
	lastSeen time.Time
	tunnel   *TunnelFlow
}

type UDPClient struct {
	channel     *TunnelChannel
	reply       UDPReply
	idleTimeout time.Duration
	maxFlows    int

	mu    sync.Mutex
	next  uint64
	byKey map[netip.AddrPort]*udpClientState
	byID  map[uint64]*udpClientState
}

func NewUDPClient(channel *TunnelChannel, idle time.Duration, maxFlows int, reply UDPReply) (*UDPClient, error) {
	if channel == nil || reply == nil {
		return nil, ErrMalformed
	}
	if idle <= 0 {
		idle = DefaultUDPIdleTimeout
	}
	if maxFlows <= 0 {
		maxFlows = DefaultMaxUDPFlows
	}
	return &UDPClient{
		channel: channel, reply: reply, idleTimeout: idle, maxFlows: maxFlows,
		byKey: make(map[netip.AddrPort]*udpClientState),
		byID:  make(map[uint64]*udpClientState),
	}, nil
}

func (c *UDPClient) Forward(client, peer netip.AddrPort, payload []byte, now time.Time) error {
	if !validEndpoint(client) || !validEndpoint(peer) || client.Addr().Unmap().Is4() != peer.Addr().Unmap().Is4() {
		return fmt.Errorf("%w: invalid UDP endpoints", ErrMalformed)
	}
	if len(payload) > MaxPayload {
		return fmt.Errorf("%w: UDP payload=%d", ErrLimit, len(payload))
	}
	c.mu.Lock()
	state := c.byKey[client]
	if state != nil {
		if state.ipv4 != peer.Addr().Unmap().Is4() {
			c.mu.Unlock()
			return fmt.Errorf("%w: UDP family changed", ErrMalformed)
		}
		state.lastSeen = now
		id := state.id
		tunnel := state.tunnel
		c.mu.Unlock()
		return tunnel.Send(Frame{Kind: KindUDPDatagram, FlowID: id, Peer: peer, Payload: append([]byte(nil), payload...)}, now)
	}
	if len(c.byID) >= c.maxFlows {
		c.mu.Unlock()
		return ErrLimit
	}
	id, err := c.allocateIDLocked()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	tunnel, err := c.channel.OpenFlow()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	state = &udpClientState{id: id, client: client, ipv4: client.Addr().Unmap().Is4(), lastSeen: now, tunnel: tunnel}
	c.byKey[client] = state
	c.byID[id] = state
	c.mu.Unlock()
	if err := tunnel.Send(Frame{Kind: KindUDPDatagram, FlowID: id, Peer: peer, Payload: append([]byte(nil), payload...)}, now); err != nil {
		c.remove(state)
		return err
	}
	return nil
}

func (c *UDPClient) Handle(frame Frame, now time.Time) error {
	if frame.Kind != KindUDPDatagram || frame.FlowID == 0 || !validEndpoint(frame.Peer) {
		return ErrMalformed
	}
	c.mu.Lock()
	state := c.byID[frame.FlowID]
	if state == nil {
		c.mu.Unlock()
		return fmt.Errorf("%w: unknown UDP flow", ErrMalformed)
	}
	if state.ipv4 != frame.Peer.Addr().Unmap().Is4() {
		c.mu.Unlock()
		return fmt.Errorf("%w: UDP reverse family changed", ErrMalformed)
	}
	state.lastSeen = now
	client := state.client
	c.mu.Unlock()
	return c.reply(frame.Peer, client, append([]byte(nil), frame.Payload...))
}

func (c *UDPClient) Tick(now time.Time) {
	var stale []*udpClientState
	c.mu.Lock()
	for key, state := range c.byKey {
		if now.Before(state.lastSeen) || now.Sub(state.lastSeen) < c.idleTimeout {
			continue
		}
		delete(c.byKey, key)
		delete(c.byID, state.id)
		stale = append(stale, state)
	}
	c.mu.Unlock()
	for _, state := range stale {
		_ = state.tunnel.Close()
	}
}

func (c *UDPClient) Close() {
	c.mu.Lock()
	states := make([]*udpClientState, 0, len(c.byID))
	for key, state := range c.byKey {
		delete(c.byKey, key)
		delete(c.byID, state.id)
		states = append(states, state)
	}
	c.mu.Unlock()
	for _, state := range states {
		_ = state.tunnel.Close()
	}
}

func (c *UDPClient) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.byID)
}

func (c *UDPClient) allocateIDLocked() (uint64, error) {
	for attempts := 0; attempts <= len(c.byID); attempts++ {
		c.next++
		if c.next == 0 {
			c.next++
		}
		if c.byID[c.next] == nil {
			return c.next, nil
		}
	}
	return 0, ErrLimit
}

func (c *UDPClient) remove(want *udpClientState) {
	c.mu.Lock()
	if c.byID[want.id] == want {
		delete(c.byID, want.id)
		delete(c.byKey, want.client)
	}
	c.mu.Unlock()
	_ = want.tunnel.Close()
}

type udpServerState struct {
	id       uint64
	ipv4     bool
	lastSeen time.Time
	upstream *net.UDPConn
	tunnel   *TunnelFlow
	closeOnce sync.Once
}

func (s *udpServerState) close() {
	s.closeOnce.Do(func() {
		_ = s.upstream.Close()
		_ = s.tunnel.Close()
	})
}

type UDPServer struct {
	channel     *TunnelChannel
	idleTimeout time.Duration
	maxFlows    int

	mu    sync.Mutex
	flows map[uint64]*udpServerState
	listen func(bool) (*net.UDPConn, error)

	sendQueue *udpServerSendQueue
	sendWG    sync.WaitGroup
	closeOnce sync.Once
}

func NewUDPServer(channel *TunnelChannel, idle time.Duration, maxFlows int) (*UDPServer, error) {
	if channel == nil {
		return nil, ErrMalformed
	}
	if idle <= 0 {
		idle = DefaultUDPIdleTimeout
	}
	if maxFlows <= 0 {
		maxFlows = DefaultMaxUDPFlows
	}
	s := &UDPServer{
		channel: channel, idleTimeout: idle, maxFlows: maxFlows,
		flows: make(map[uint64]*udpServerState),
		listen: listenUDPMapping,
		sendQueue: newUDPServerSendQueue(udpServerSendBudgetRecords),
	}
	s.sendWG.Add(udpServerSendWorkers)
	for i := 0; i < udpServerSendWorkers; i++ {
		go s.sendWorker(i)
	}
	return s, nil
}

func (s *UDPServer) Handle(frame Frame, now time.Time) error {
	if frame.Kind != KindUDPDatagram || frame.FlowID == 0 || !validEndpoint(frame.Peer) {
		return ErrMalformed
	}
	ipv4 := frame.Peer.Addr().Unmap().Is4()
	s.mu.Lock()
	state := s.flows[frame.FlowID]
	if state != nil {
		if state.ipv4 != ipv4 {
			s.mu.Unlock()
			return fmt.Errorf("%w: UDP mapping family changed", ErrMalformed)
		}
		state.lastSeen = now
		s.mu.Unlock()
		_, err := state.upstream.WriteToUDPAddrPort(frame.Payload, frame.Peer)
		return err
	}
	if len(s.flows) >= s.maxFlows {
		s.mu.Unlock()
		return ErrLimit
	}
	upstream, err := s.listen(ipv4)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	tunnel, err := s.channel.OpenFlow()
	if err != nil {
		s.mu.Unlock()
		_ = upstream.Close()
		return err
	}
	state = &udpServerState{id: frame.FlowID, ipv4: ipv4, lastSeen: now, upstream: upstream, tunnel: tunnel}
	s.flows[frame.FlowID] = state
	s.mu.Unlock()
	go s.readUpstream(state)
	if _, err := upstream.WriteToUDPAddrPort(frame.Payload, frame.Peer); err != nil {
		s.remove(state)
		return err
	}
	return nil
}

func (s *UDPServer) readUpstream(state *udpServerState) {
	buf := make([]byte, MaxPayload)
	for {
		n, peer, err := state.upstream.ReadFromUDPAddrPort(buf)
		if err != nil {
			s.remove(state)
			return
		}
		now := time.Now()
		s.mu.Lock()
		if s.flows[state.id] != state {
			s.mu.Unlock()
			return
		}
		state.lastSeen = now
		s.mu.Unlock()
		item := udpServerQueuedDatagram{
			state: state, peer: peer,
			payload: append([]byte(nil), buf[:n]...),
			queuedAt: now,
		}
		if !s.sendQueue.enqueue(item, now) {
			return
		}
	}
}

func (s *UDPServer) sendWorker(shard int) {
	defer s.sendWG.Done()
	for {
		item, ok := s.sendQueue.popShard(shard)
		if !ok {
			return
		}
		state := item.state
		s.mu.Lock()
		active := s.flows[state.id] == state
		s.mu.Unlock()
		if !active {
			s.sendQueue.finish(item, true, false)
			continue
		}
		err := state.tunnel.Send(Frame{
			Kind: KindUDPDatagram, FlowID: state.id, Peer: item.peer,
			Payload: item.payload,
		}, time.Now())
		s.sendQueue.finish(item, false, err != nil)
		if err != nil {
			s.remove(state)
		}
	}
}

func (s *UDPServer) Diagnostic() UDPServerDiagnostic {
	if s == nil {
		return UDPServerDiagnostic{}
	}
	return s.sendQueue.snapshot()
}

func (s *UDPServer) Tick(now time.Time) {
	var stale []*udpServerState
	s.mu.Lock()
	for id, state := range s.flows {
		if now.Before(state.lastSeen) || now.Sub(state.lastSeen) < s.idleTimeout {
			continue
		}
		delete(s.flows, id)
		stale = append(stale, state)
	}
	s.mu.Unlock()
	for _, state := range stale {
		state.close()
	}
}

func (s *UDPServer) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		s.sendQueue.close()
		s.mu.Lock()
		states := make([]*udpServerState, 0, len(s.flows))
		for id, state := range s.flows {
			delete(s.flows, id)
			states = append(states, state)
		}
		s.mu.Unlock()
		for _, state := range states {
			state.close()
		}
		s.sendWG.Wait()
	})
}

func (s *UDPServer) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.flows)
}

func (s *UDPServer) remove(want *udpServerState) {
	s.mu.Lock()
	if s.flows[want.id] == want {
		delete(s.flows, want.id)
	}
	s.mu.Unlock()
	want.close()
}

func listenUDPMapping(ipv4 bool) (*net.UDPConn, error) {
	if ipv4 {
		return net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero})
	}
	return net.ListenUDP("udp6", &net.UDPAddr{IP: net.IPv6unspecified})
}

func validEndpoint(v netip.AddrPort) bool {
	return v.IsValid() && v.Port() != 0 && !v.Addr().IsUnspecified()
}

