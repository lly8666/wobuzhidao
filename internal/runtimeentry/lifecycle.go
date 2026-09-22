package runtimeentry

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/linuxserver"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/runtimeowner"
)

var (
	ErrLifecycleBusy      = errors.New("runtimeentry: lifecycle transition already pending")
	ErrLifecycleDormant   = errors.New("runtimeentry: logical tunnel is dormant")
	ErrLifecycleLaneState = errors.New("runtimeentry: invalid lifecycle lane state")
	ErrSegmentMuxClosed   = errors.New("runtimeentry: segment mux closed")
	ErrSegmentMuxFlow     = errors.New("runtimeentry: segment mux flow already registered")
)

const (
	rotatingSourcePortSpan uint64 = 1024
	defaultReplacementGrace = 3 * time.Second
)

// RotatingSourcePort allocates a bounded reusable client port window for lane
// incarnations. At most one transition is orchestrated at a time, so cycling a
// 1024-port window cannot collide with the currently active/retiring set.
func RotatingSourcePort(base uint16, incarnation uint64) (uint16, error) {
	if base == 0 || incarnation == 0 || uint64(base)+rotatingSourcePortSpan-1 > 65535 {
		return 0, ErrEndpointConfig
	}
	return base + uint16((incarnation-1)%rotatingSourcePortSpan), nil
}

// SegmentMux lets multiple client FakeTCP associations share one broad Linux
// raw endpoint. It dispatches exact inbound four-tuples and never creates a
// localhost carrier or second network protocol.
type SegmentMux struct {
	base SegmentIO

	mu     sync.Mutex
	routes map[faketcp.ClientFlow]*segmentMuxRoute
	closed bool
	errCh  chan error
	once   sync.Once
}

type segmentMuxRoute struct {
	in   chan faketcp.Segment
	done chan struct{}
	once sync.Once
}

func NewSegmentMux(base SegmentIO) (*SegmentMux, error) {
	if err := base.validate(); err != nil {
		return nil, err
	}
	m := &SegmentMux{
		base: base,
		routes: make(map[faketcp.ClientFlow]*segmentMuxRoute),
		errCh: make(chan error, 1),
	}
	go m.readLoop()
	return m, nil
}

func (m *SegmentMux) Errors() <-chan error {
	if m == nil {
		return nil
	}
	return m.errCh
}

func (m *SegmentMux) Open(flow faketcp.ClientFlow) (SegmentIO, error) {
	if m == nil {
		return SegmentIO{}, ErrSegmentMuxClosed
	}
	if err := flow.Validate(); err != nil {
		return SegmentIO{}, err
	}
	route := &segmentMuxRoute{
		in: make(chan faketcp.Segment, 256),
		done: make(chan struct{}),
	}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return SegmentIO{}, ErrSegmentMuxClosed
	}
	if m.routes[flow] != nil {
		m.mu.Unlock()
		return SegmentIO{}, ErrSegmentMuxFlow
	}
	m.routes[flow] = route
	m.mu.Unlock()

	return SegmentIO{
		Read: func() (faketcp.Segment, error) {
			select {
			case <-route.done:
				return faketcp.Segment{}, io.EOF
			case seg := <-route.in:
				return seg, nil
			}
		},
		Emit: func(seg faketcp.Segment) error {
			if !clientFlowMatchesOutbound(flow, seg) {
				return ErrEndpointConfig
			}
			return m.base.Emit(seg)
		},
		Close: func() error {
			m.mu.Lock()
			if m.routes[flow] == route {
				delete(m.routes, flow)
			}
			m.mu.Unlock()
			route.once.Do(func() { close(route.done) })
			return nil
		},
	}, nil
}

func (m *SegmentMux) readLoop() {
	for {
		seg, err := m.base.Read()
		if err != nil {
			m.mu.Lock()
			closed := m.closed
			m.mu.Unlock()
			if !closed {
				select {
				case m.errCh <- err:
				default:
				}
			}
			return
		}
		flow := faketcp.ClientFlow{
			LocalIP: seg.DstIP, PeerIP: seg.SrcIP,
			LocalPort: seg.DstPort, PeerPort: seg.SrcPort,
		}
		m.mu.Lock()
		route := m.routes[flow]
		m.mu.Unlock()
		if route == nil {
			continue
		}
		copySeg := seg
		copySeg.Payload = append([]byte(nil), seg.Payload...)
		select {
		case <-route.done:
		case route.in <- copySeg:
		}
	}
}

func (m *SegmentMux) Close() error {
	if m == nil {
		return nil
	}
	var out error
	m.once.Do(func() {
		m.mu.Lock()
		m.closed = true
		routes := make([]*segmentMuxRoute, 0, len(m.routes))
		for flow, route := range m.routes {
			delete(m.routes, flow)
			routes = append(routes, route)
		}
		m.mu.Unlock()
		for _, route := range routes {
			route.once.Do(func() { close(route.done) })
		}
		out = m.base.close()
	})
	return out
}

func clientFlowMatchesOutbound(flow faketcp.ClientFlow, seg faketcp.Segment) bool {
	return seg.SrcIP == flow.LocalIP && seg.DstIP == flow.PeerIP &&
		seg.SrcPort == flow.LocalPort && seg.DstPort == flow.PeerPort
}

type ClientLaneOpener func(laneID uint8, incarnation uint64) (SegmentIO, faketcp.ClientFlow, error)

type TunnelClientConfig struct {
	TLSStartupPadding bool
	OpenLane ClientLaneOpener

	Lease        logicaltunnel.Lease
	DesiredLanes int
	MaxFlows     int
	Admission    realityfront.ClientAdmissionConfig
	Lane         datapath.ClientLaneParams
	Deliver      runtimeowner.PacketSink

	InitialRTO   time.Duration
	TickInterval time.Duration
	DormantAfter time.Duration
	RotateMin        time.Duration
	RotateMax        time.Duration
	ReplacementGrace time.Duration
}

type clientLifecycleLane struct {
	id       uint8
	ref      logicaltunnel.LaneRef
	io       SegmentIO
	assoc    *faketcp.ClientAssociation
	attached bool
	retiring bool
	cancel   context.CancelFunc
	once     sync.Once
}

func (l *clientLifecycleLane) close() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		if l.cancel != nil {
			l.cancel()
		}
		if l.assoc != nil {
			l.assoc.Close()
		}
		_ = l.io.close()
	})
}

type clientRetiring struct {
	oldRef       logicaltunnel.LaneRef
	freshRef     logicaltunnel.LaneRef
	old          *clientLifecycleLane
	promotedAt   time.Time
	closeStarted time.Time
}

type TunnelClient struct {
	cfg TunnelClientConfig

	owner *datapath.TunnelOwner
	rt    *runtimeowner.Runtime

	opMu sync.Mutex
	mu   sync.Mutex

	lanes     map[uint8]*clientLifecycleLane
	retiring  map[logicaltunnel.LaneRef]*clientRetiring
	nextInc   uint64
	dormant   bool
	closed    bool
	lastPayload  time.Time
	nextRotation time.Time
	actionPending bool

	runCtx context.Context
	cancel context.CancelFunc
	errCh  chan error
	once   sync.Once
}

func DialTunnelClient(ctx context.Context, cfg TunnelClientConfig) (*TunnelClient, error) {
	if ctx == nil || cfg.OpenLane == nil {
		return nil, ErrEndpointConfig
	}
	if err := cfg.Lease.Validate(); err != nil {
		return nil, err
	}
	if err := logicaltunnel.ValidateProductTransportLaneCount(cfg.DesiredLanes); err != nil {
		return nil, err
	}
	if cfg.MaxFlows <= 0 {
		cfg.MaxFlows = 4096
	}
	if cfg.InitialRTO <= 0 {
		cfg.InitialRTO = runtimeowner.DefaultRepairRTO
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 100 * time.Millisecond
	}
	if cfg.DormantAfter < 0 || cfg.RotateMin < 0 || cfg.RotateMax < 0 || cfg.ReplacementGrace < 0 ||
		(cfg.RotateMin == 0) != (cfg.RotateMax == 0) ||
		(cfg.RotateMin > 0 && cfg.RotateMax < cfg.RotateMin) {
		return nil, ErrLifecycleLaneState
	}
	if cfg.ReplacementGrace == 0 {
		cfg.ReplacementGrace = defaultReplacementGrace
	}
	if len(cfg.Admission.TunnelID) == 0 {
		cfg.Admission.TunnelID = cfg.Lease.Config.TunnelID.Bytes()
	}
	requested, err := logicaltunnel.TunnelIDFromBytes(cfg.Admission.TunnelID)
	if err != nil || requested != cfg.Lease.Config.TunnelID {
		return nil, ErrLeaseMismatch
	}

	owner, err := datapath.NewLeasedTunnelOwner(cfg.Lease, cfg.DesiredLanes, cfg.MaxFlows)
	if err != nil {
		return nil, err
	}
	if err := owner.ConfigurePadding(datapath.TLSStartupPaddingPolicy(cfg.TLSStartupPadding)); err != nil {
		owner.Close()
		return nil, err
	}
	rt, err := runtimeowner.New(owner, cfg.Deliver)
	if err != nil {
		owner.Close()
		return nil, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	c := &TunnelClient{
		cfg: cfg, owner: owner, rt: rt,
		lanes: make(map[uint8]*clientLifecycleLane, cfg.DesiredLanes),
		retiring: make(map[logicaltunnel.LaneRef]*clientRetiring),
		runCtx: runCtx, cancel: cancel, errCh: make(chan error, 16),
		lastPayload: time.Now(),
	}
	c.opMu.Lock()
	for id := 1; id <= cfg.DesiredLanes; id++ {
		if _, err := c.connectLaneLocked(ctx, uint8(id), logicaltunnel.LaneRef{}); err != nil {
			c.opMu.Unlock()
			c.Close()
			return nil, err
		}
	}
	c.scheduleNextRotationLocked(time.Now())
	c.opMu.Unlock()
	go c.lifecycleLoop()
	return c, nil
}

func (c *TunnelClient) Owner() *datapath.TunnelOwner {
	if c == nil {
		return nil
	}
	return c.owner
}

func (c *TunnelClient) Errors() <-chan error {
	if c == nil {
		return nil
	}
	return c.errCh
}

func (c *TunnelClient) IsDormant() bool {
	if c == nil {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.dormant
}

func (c *TunnelClient) PrepareBusiness(ctx context.Context) error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.mu.Lock()
	dormant := c.dormant
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return ErrClientRuntimeStopped
	}
	if dormant {
		if err := c.Wake(ctx); err != nil {
			return err
		}
	}
	c.mu.Lock()
	c.lastPayload = time.Now()
	c.mu.Unlock()
	return nil
}

func (c *TunnelClient) SendPacket(ctx context.Context, packet []byte, now time.Time) error {
	if err := c.PrepareBusiness(ctx); err != nil {
		return err
	}
	stats := c.owner.Stats()
	switch stats.DesiredLanes {
	case 1:
		records, err := c.owner.NormalOutbound(packet, now)
		if err != nil {
			return err
		}
		return c.rt.SendNormal(records, now)
	case 2, 3, 4:
		out, err := c.owner.GameOutbound(packet, now)
		if err != nil {
			return err
		}
		return c.rt.SendGame(out, now)
	default:
		return datapath.ErrLaneUnavailable
	}
}

func (c *TunnelClient) SendNormal(records []datapath.WireRecord, now time.Time) error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	c.mu.Lock()
	c.lastPayload = time.Now()
	c.mu.Unlock()
	return c.rt.SendNormal(records, now)
}

func (c *TunnelClient) SendGame(out datapath.GameOutboundResult, now time.Time) error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	c.mu.Lock()
	c.lastPayload = time.Now()
	c.mu.Unlock()
	return c.rt.SendGame(out, now)
}

func (c *TunnelClient) PlatformWireSink(out platformflow.Outbound) error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	c.mu.Lock()
	c.lastPayload = time.Now()
	c.mu.Unlock()
	return c.rt.PlatformWireSink(out)
}

func (c *TunnelClient) RotateOldest(ctx context.Context) error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClientRuntimeStopped
	}
	if c.dormant {
		c.mu.Unlock()
		return ErrLifecycleDormant
	}
	if len(c.retiring) != 0 {
		c.mu.Unlock()
		return ErrLifecycleBusy
	}
	var old *clientLifecycleLane
	for _, lane := range c.lanes {
		if old == nil || lane.ref.Generation < old.ref.Generation {
			old = lane
		}
	}
	c.mu.Unlock()
	if old == nil {
		return ErrLifecycleLaneState
	}
	_, err := c.connectLaneLocked(ctx, old.id, old.ref)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.scheduleNextRotationLocked(time.Now())
	c.mu.Unlock()
	return nil
}

func (c *TunnelClient) Dormant() error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClientRuntimeStopped
	}
	if c.dormant {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	if _, err := c.rt.Dormant(); err != nil {
		return err
	}

	c.mu.Lock()
	lanes := make([]*clientLifecycleLane, 0, len(c.lanes)+len(c.retiring))
	seen := make(map[*clientLifecycleLane]struct{})
	for _, lane := range c.lanes {
		if _, ok := seen[lane]; !ok {
			seen[lane] = struct{}{}
			lanes = append(lanes, lane)
		}
	}
	for _, retiring := range c.retiring {
		if _, ok := seen[retiring.old]; !ok {
			seen[retiring.old] = struct{}{}
			lanes = append(lanes, retiring.old)
		}
	}
	clear(c.lanes)
	clear(c.retiring)
	c.dormant = true
	c.nextRotation = time.Time{}
	c.mu.Unlock()
	for _, lane := range lanes {
		lane.close()
	}
	return nil
}

func (c *TunnelClient) Wake(ctx context.Context) error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	if ctx == nil {
		ctx = context.Background()
	}
	c.opMu.Lock()
	defer c.opMu.Unlock()

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return ErrClientRuntimeStopped
	}
	if !c.dormant {
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()

	attached := make([]uint8, 0, c.cfg.DesiredLanes)
	for id := 1; id <= c.cfg.DesiredLanes; id++ {
		if _, err := c.connectLaneLocked(ctx, uint8(id), logicaltunnel.LaneRef{}); err != nil {
			_, _ = c.rt.Dormant()
			c.mu.Lock()
			lanes := make([]*clientLifecycleLane, 0, len(c.lanes))
			for _, lane := range c.lanes {
				lanes = append(lanes, lane)
			}
			clear(c.lanes)
			c.mu.Unlock()
			for _, lane := range lanes {
				lane.close()
			}
			return fmt.Errorf("runtimeentry: wake lane %v after %v: %w", id, attached, err)
		}
		attached = append(attached, uint8(id))
	}
	c.mu.Lock()
	c.dormant = false
	c.lastPayload = time.Now()
	c.scheduleNextRotationLocked(time.Now())
	c.mu.Unlock()
	return nil
}

func (c *TunnelClient) connectLaneLocked(ctx context.Context, laneID uint8, replacing logicaltunnel.LaneRef) (logicaltunnel.LaneRef, error) {
	c.mu.Lock()
	c.nextInc++
	incarnation := c.nextInc
	c.mu.Unlock()

	ioCfg, flow, err := c.cfg.OpenLane(laneID, incarnation)
	if err != nil {
		return logicaltunnel.LaneRef{}, err
	}
	if err := ioCfg.validate(); err != nil {
		_ = ioCfg.close()
		return logicaltunnel.LaneRef{}, err
	}
	isn, err := randomISN()
	if err != nil {
		_ = ioCfg.close()
		return logicaltunnel.LaneRef{}, err
	}
	assoc, err := faketcp.NewClientAssociation(flow, isn, c.cfg.InitialRTO, ioCfg.Emit)
	if err != nil {
		_ = ioCfg.close()
		return logicaltunnel.LaneRef{}, err
	}
	runCtx, cancel := context.WithCancel(c.runCtx)
	laneState := &clientLifecycleLane{id: laneID, io: ioCfg, assoc: assoc, cancel: cancel}
	go c.clientLaneReadLoop(runCtx, laneState)
	go c.clientLaneBootstrapTick(runCtx, laneState)

	if err := assoc.Start(time.Now()); err != nil {
		laneState.close()
		return logicaltunnel.LaneRef{}, err
	}
	if err := assoc.WaitEstablished(ctx); err != nil {
		laneState.close()
		return logicaltunnel.LaneRef{}, err
	}
	admission := c.cfg.Admission
	admission.LaneID = laneID
	session, err := realityfront.EstablishClient(ctx, assoc.BootstrapConn(), admission)
	if err != nil {
		laneState.close()
		return logicaltunnel.LaneRef{}, err
	}
	gotTunnel, err := logicaltunnel.TunnelIDFromBytes(session.Negotiated.TunnelID)
	if err != nil || gotTunnel != c.cfg.Lease.Config.TunnelID {
		laneState.close()
		return logicaltunnel.LaneRef{}, ErrLeaseMismatch
	}
	handoff, err := assoc.Detach()
	if err != nil {
		laneState.close()
		return logicaltunnel.LaneRef{}, err
	}
	laneParams := c.cfg.Lane
	laneParams.PeerMSS = handoff.Peer.MSS
	laneParams.PeerMSSSet = handoff.Peer.AdvertisedMSS
	transport := runtimeowner.TransportConfig{
		LocalIP: flow.LocalIP, PeerIP: flow.PeerIP,
		LocalPort: flow.LocalPort, PeerPort: flow.PeerPort,
		SendNext: handoff.SendNext, ReceiveNext: handoff.ReceiveNext,
		AdvertisedWindow: handoff.AdvertisedWindow, AdvertisedWindowSet: true,
		WindowScale: handoff.WindowScale, WindowScaleSet: handoff.WindowScaleSet,
		InitialRTO: runtimeowner.DefaultRepairRTO,
		RepairHorizon: runtimeowner.DefaultRepairHorizon,
		SACKPermitted: handoff.Peer.SACKPermitted,
		Emit: ioCfg.Emit,
	}

	var snapshot datapath.TunnelLaneSnapshot
	if replacing == (logicaltunnel.LaneRef{}) {
		snapshot, err = c.rt.AttachClientAdmission(laneID, session, laneParams, transport)
	} else {
		cfg, cfgErr := datapath.ClientLaneConfigFromAdmission(session, laneParams)
		if cfgErr != nil {
			laneState.close()
			return logicaltunnel.LaneRef{}, cfgErr
		}
		lane, laneErr := datapath.NewLane(cfg)
		if laneErr != nil {
			laneState.close()
			return logicaltunnel.LaneRef{}, laneErr
		}
		if err = c.rt.BeginSameIDReplacement(replacing, lane, transport); err != nil {
			lane.Close()
		} else {
			snapshot, err = c.rt.PromoteSameIDReplacement(replacing)
			if err != nil {
				_ = c.rt.FailSameIDReplacement(replacing)
			}
		}
	}
	if err != nil {
		laneState.close()
		return logicaltunnel.LaneRef{}, err
	}

	c.mu.Lock()
	laneState.ref = snapshot.Ref
	laneState.attached = true
	if c.closed {
		c.mu.Unlock()
		laneState.close()
		return logicaltunnel.LaneRef{}, ErrClientRuntimeStopped
	}
	if replacing == (logicaltunnel.LaneRef{}) {
		c.lanes[laneID] = laneState
	} else {
		old := c.lanes[laneID]
		if old == nil || old.ref != replacing {
			c.mu.Unlock()
			laneState.close()
			return logicaltunnel.LaneRef{}, ErrLifecycleLaneState
		}
		old.retiring = true
		c.lanes[laneID] = laneState
		c.retiring[snapshot.Ref] = &clientRetiring{
			oldRef: replacing, freshRef: snapshot.Ref, old: old, promotedAt: time.Now(),
		}
	}
	c.mu.Unlock()
	return snapshot.Ref, nil
}

func (c *TunnelClient) clientLaneReadLoop(ctx context.Context, lane *clientLifecycleLane) {
	for {
		seg, err := lane.io.Read()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			c.report(err)
			return
		}
		c.mu.Lock()
		closed := c.closed
		attached := lane.attached
		ref := lane.ref
		retiring := lane.retiring
		c.mu.Unlock()
		if closed {
			return
		}
		if attached {
			err = c.rt.HandleSegment(ref, seg, time.Now())
		} else {
			err = lane.assoc.HandleSegment(seg, time.Now())
		}
		if err == nil || errors.Is(err, faketcp.ErrClientDetached) {
			continue
		}
		if retiring && errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) {
			continue
		}
		c.report(err)
		return
	}
}

func (c *TunnelClient) clientLaneBootstrapTick(ctx context.Context, lane *clientLifecycleLane) {
	ticker := time.NewTicker(c.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.mu.Lock()
			attached := lane.attached
			closed := c.closed
			c.mu.Unlock()
			if closed || attached {
				return
			}
			if _, err := lane.assoc.EmitRetransmitDue(now); err != nil {
				c.report(err)
			}
		}
	}
}

func (c *TunnelClient) lifecycleLoop() {
	ticker := time.NewTicker(c.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.runCtx.Done():
			return
		case now := <-ticker.C:
			if err := c.rt.Tick(now); err != nil {
				c.report(err)
			}
			c.retireQualified(now)
			c.maybeScheduleLifecycle(now)
		}
	}
}

func (c *TunnelClient) retireQualified(now time.Time) {
	c.mu.Lock()
	pending := make([]*clientRetiring, 0, len(c.retiring))
	for _, item := range c.retiring {
		pending = append(pending, item)
	}
	c.mu.Unlock()
	for _, item := range pending {
		stats, ok := c.rt.TransportStats(item.freshRef)
		if !ok {
			continue
		}
		qualified := stats.Acked != 0 || stats.Received != 0
		graceExpired := !now.Before(item.promotedAt) && now.Sub(item.promotedAt) >= c.cfg.ReplacementGrace

		c.mu.Lock()
		closeStarted := item.closeStarted
		c.mu.Unlock()
		if closeStarted.IsZero() {
			if !qualified && !graceExpired {
				continue
			}
			if err := c.rt.CloseWrite(item.oldRef, now); err != nil &&
				!errors.Is(err, runtimeowner.ErrRuntimeClosed) &&
				!errors.Is(err, runtimeowner.ErrTransportPeerReset) {
				c.report(err)
			}
			c.mu.Lock()
			if current := c.retiring[item.freshRef]; current == item && item.closeStarted.IsZero() {
				item.closeStarted = now
			}
			c.mu.Unlock()
			continue
		}

		oldStats, oldOK := c.rt.TransportStats(item.oldRef)
		closeComplete := oldOK && oldStats.LocalFINAcked && oldStats.PeerFIN
		closeBudget := c.cfg.ReplacementGrace
		if rtoBudget := 2 * c.cfg.InitialRTO; rtoBudget > 0 && rtoBudget < closeBudget {
			closeBudget = rtoBudget
		}
		closeExpired := !now.Before(closeStarted) && now.Sub(closeStarted) >= closeBudget
		if !closeComplete && !closeExpired {
			continue
		}
		if err := c.rt.RetireIncarnation(item.oldRef); err != nil &&
			!errors.Is(err, datapath.ErrLaneUnavailable) {
			c.report(err)
			continue
		}
		c.mu.Lock()
		if current := c.retiring[item.freshRef]; current == item {
			delete(c.retiring, item.freshRef)
		}
		c.mu.Unlock()
		item.old.close()
	}
}

func (c *TunnelClient) maybeScheduleLifecycle(now time.Time) {
	c.mu.Lock()
	if c.closed || c.actionPending {
		c.mu.Unlock()
		return
	}
	if c.cfg.DormantAfter > 0 && !c.dormant && !now.Before(c.lastPayload) &&
		now.Sub(c.lastPayload) >= c.cfg.DormantAfter {
		c.actionPending = true
		c.mu.Unlock()
		go func() {
			err := c.Dormant()
			c.mu.Lock()
			c.actionPending = false
			c.mu.Unlock()
			if err != nil {
				c.report(err)
			}
		}()
		return
	}
	if c.cfg.RotateMin > 0 && !c.dormant && !c.nextRotation.IsZero() &&
		!now.Before(c.nextRotation) && len(c.retiring) == 0 {
		c.actionPending = true
		c.mu.Unlock()
		go func() {
			timeout := c.cfg.Admission.TLS.Timeout
			if timeout <= 0 {
				timeout = 15 * time.Second
			}
			ctx, cancel := context.WithTimeout(c.runCtx, timeout)
			err := c.RotateOldest(ctx)
			cancel()
			c.mu.Lock()
			c.actionPending = false
			if err != nil {
				c.nextRotation = time.Now().Add(c.cfg.TickInterval)
			}
			c.mu.Unlock()
			if err != nil && !errors.Is(err, context.Canceled) {
				c.report(err)
			}
		}()
		return
	}
	c.mu.Unlock()
}

func (c *TunnelClient) scheduleNextRotationLocked(now time.Time) {
	if c.cfg.RotateMin <= 0 {
		c.nextRotation = time.Time{}
		return
	}
	d, err := randomDuration(c.cfg.RotateMin, c.cfg.RotateMax)
	if err != nil {
		d = c.cfg.RotateMin
	}
	c.nextRotation = now.Add(d)
}

func (c *TunnelClient) report(err error) {
	if err == nil {
		return
	}
	select {
	case c.errCh <- err:
	default:
	}
}

func (c *TunnelClient) Close() error {
	if c == nil {
		return nil
	}
	var out error
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		lanes := make([]*clientLifecycleLane, 0, len(c.lanes)+len(c.retiring))
		seen := make(map[*clientLifecycleLane]struct{})
		for _, lane := range c.lanes {
			if _, ok := seen[lane]; !ok {
				seen[lane] = struct{}{}
				lanes = append(lanes, lane)
			}
		}
		for _, retiring := range c.retiring {
			if _, ok := seen[retiring.old]; !ok {
				seen[retiring.old] = struct{}{}
				lanes = append(lanes, retiring.old)
			}
		}
		clear(c.lanes)
		clear(c.retiring)
		c.mu.Unlock()
		if c.cancel != nil {
			c.cancel()
		}
		c.rt.Close()
		for _, lane := range lanes {
			lane.close()
		}
	})
	return out
}

func randomDuration(minimum, maximum time.Duration) (time.Duration, error) {
	if minimum <= 0 || maximum < minimum {
		return 0, ErrLifecycleLaneState
	}
	if maximum == minimum {
		return minimum, nil
	}
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, err
	}
	span := uint64(maximum - minimum)
	return minimum + time.Duration(binary.BigEndian.Uint64(raw[:])%(span+1)), nil
}

type LifecycleServerConfig struct {
	ServerConfig
	DesiredLanes     int
	DormantAfter     time.Duration
	ReplacementGrace time.Duration
}

type serverLifecycleLane struct {
	flow         faketcp.ServerFlow
	assoc        *faketcp.ServerAssociation
	ref          logicaltunnel.LaneRef
	qualified    bool
	retiring     bool
	replaces     *serverLifecycleLane
	promotedAt   time.Time
	closeStarted time.Time
	group        *serverLifecycleTunnel
}

type serverLifecycleTunnel struct {
	id        logicaltunnel.TunnelID
	leaseAddr netip.Addr
	owner     *datapath.TunnelOwner
	rt        *runtimeowner.Runtime
	token     linuxserver.BindingToken
	service   *platformflow.Server
	lanes     map[uint8]*serverLifecycleLane
	retiring  map[logicaltunnel.LaneRef]*serverLifecycleLane
	dormant   bool
	lastPayload time.Time
}

type LifecycleServer struct {
	cfg LifecycleServerConfig

	table *faketcp.ServerAssociationTable

	admitMu sync.Mutex
	mu sync.Mutex
	started map[faketcp.ServerFlow]bool
	pending map[faketcp.ServerFlow][]faketcp.Segment
	byFlow map[faketcp.ServerFlow]*serverLifecycleLane
	byTunnel map[logicaltunnel.TunnelID]*serverLifecycleTunnel
	byLease map[netip.Addr]*serverLifecycleTunnel

	once sync.Once
}

func NewLifecycleServer(cfg LifecycleServerConfig) (*LifecycleServer, error) {
	if err := cfg.IO.validate(); err != nil {
		return nil, err
	}
	if cfg.ListenPort == 0 || cfg.Router == nil || cfg.LookupLease == nil {
		return nil, ErrEndpointConfig
	}
	if err := logicaltunnel.ValidateProductTransportLaneCount(cfg.DesiredLanes); err != nil {
		return nil, err
	}
	if cfg.DormantAfter < 0 || cfg.ReplacementGrace < 0 {
		return nil, ErrLifecycleLaneState
	}
	if cfg.ReplacementGrace == 0 {
		cfg.ReplacementGrace = defaultReplacementGrace
	}
	if cfg.MaxAssociations <= 0 {
		cfg.MaxAssociations = 4096
	}
	if cfg.MaxFlows <= 0 {
		cfg.MaxFlows = 4096
	}
	if cfg.InitialRTO <= 0 {
		cfg.InitialRTO = runtimeowner.DefaultRepairRTO
	}
	if cfg.TickInterval <= 0 {
		cfg.TickInterval = 100 * time.Millisecond
	}
	if cfg.Service.UDPIdle <= 0 && cfg.Service.MaxUDPFlows <= 0 {
		cfg.Service = platformflow.DefaultServerConfig()
	}
	table, err := faketcp.NewServerAssociationTable(cfg.MaxAssociations, cfg.IO.Emit)
	if err != nil {
		return nil, err
	}
	return &LifecycleServer{
		cfg: cfg, table: table,
		started: make(map[faketcp.ServerFlow]bool),
		pending: make(map[faketcp.ServerFlow][]faketcp.Segment),
		byFlow: make(map[faketcp.ServerFlow]*serverLifecycleLane),
		byTunnel: make(map[logicaltunnel.TunnelID]*serverLifecycleTunnel),
		byLease: make(map[netip.Addr]*serverLifecycleTunnel),
	}, nil
}

func (s *LifecycleServer) Run(ctx context.Context) error {
	if s == nil || ctx == nil {
		return ErrEndpointConfig
	}
	defer s.Close()
	readCh := make(chan segmentRead, 1)
	go func() {
		for {
			seg, err := s.cfg.IO.Read()
			select {
			case readCh <- segmentRead{seg: seg, err: err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()
	ticker := time.NewTicker(s.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case read := <-readCh:
			if read.err != nil {
				if errors.Is(read.err, io.EOF) {
					return nil
				}
				return read.err
			}
			if err := s.handleSegment(ctx, read.seg, time.Now()); err != nil {
				return err
			}
		case now := <-ticker.C:
			if err := s.tick(now); err != nil {
				return err
			}
		}
	}
}

func (s *LifecycleServer) handleSegment(ctx context.Context, seg faketcp.Segment, now time.Time) error {
	if seg.DstPort != s.cfg.ListenPort {
		return nil
	}
	if faketcp.IsInitialSYN(seg) {
		isn, err := randomISN()
		if err != nil {
			return err
		}
		assoc, err := s.table.AddSYN(seg, isn, s.cfg.InitialRTO)
		if err != nil {
			if errors.Is(err, faketcp.ErrAssociationExists) {
				return nil
			}
			return err
		}
		synack, err := assoc.SYNACKSegment()
		if err != nil {
			return err
		}
		return s.cfg.IO.Emit(synack)
	}
	assoc, ok := s.table.GetSegment(seg)
	if !ok {
		return nil
	}
	flow := assoc.Flow()

	s.mu.Lock()
	lane := s.byFlow[flow]
	admitting := s.started[flow]
	s.mu.Unlock()
	if lane == nil {
		if assoc.State() == faketcp.ServerAssociationClosed {
			return nil
		}
		if state, exists := assoc.TransitionState(); exists && state == faketcp.TransitionDetached {
			if !admitting {
				return nil
			}
			s.mu.Lock()
			if !s.started[flow] || s.byFlow[flow] != nil {
				s.mu.Unlock()
				return nil
			}
			queue := s.pending[flow]
			if len(queue) >= faketcp.MaxBootstrapPendingChunks {
				s.mu.Unlock()
				return ErrSteadyQueueFull
			}
			copySeg := seg
			copySeg.Payload = append([]byte(nil), seg.Payload...)
			s.pending[flow] = append(queue, copySeg)
			s.mu.Unlock()
			return nil
		}
	}

	if lane != nil {
		qualified, err := lane.group.rt.HandleServerSegmentQualified(lane.ref, assoc, seg, now)
		if err != nil {
			if lane.retiring && errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) {
				return nil
			}
			if errors.Is(err, runtimeowner.ErrTransportMissing) ||
				errors.Is(err, runtimeowner.ErrRuntimeClosed) ||
				errors.Is(err, runtimeowner.ErrTransportPeerReset) {
				s.mu.Lock()
				current := s.byFlow[flow]
				dormant := lane.group.dormant
				s.mu.Unlock()
				if current != lane || dormant {
					return nil
				}
			}
			return err
		}
		if qualified {
			s.markLaneQualified(lane, now)
		}
		return nil
	}

	result, err := assoc.HandleSegment(seg, now)
	if err != nil {
		return err
	}
	if result.AckNeeded {
		if err := s.cfg.IO.Emit(assoc.ACKSegment(result.Ack)); err != nil {
			return err
		}
	}
	if assoc.State() == faketcp.ServerAssociationEstablished {
		s.mu.Lock()
		if !s.started[flow] {
			s.started[flow] = true
			go s.admit(ctx, assoc)
		}
		s.mu.Unlock()
	}
	return nil
}

func (s *LifecycleServer) admit(ctx context.Context, assoc *faketcp.ServerAssociation) {
	flow := assoc.Flow()
	admission := s.cfg.Admission
	priorValidator := admission.ValidateRequest
	admissionLocked := false
	admission.ValidateRequest = func(req realityfront.AdmissionRequest) error {
		s.admitMu.Lock()
		admissionLocked = true
		if priorValidator != nil {
			if err := priorValidator(req); err != nil {
				s.admitMu.Unlock()
				admissionLocked = false
				return err
			}
		}
		if err := s.validateAdmissionRequest(req); err != nil {
			s.admitMu.Unlock()
			admissionLocked = false
			return err
		}
		return nil
	}
	result, err := realityfront.HandleServerAssociation(ctx, assoc, admission, s.cfg.Fallback)
	if admissionLocked {
		defer s.admitMu.Unlock()
	}
	if err != nil || result.Admission == nil {
		s.dropAdmission(flow)
		return
	}
	tunnelID, err := logicaltunnel.TunnelIDFromBytes(result.Admission.Negotiated.TunnelID)
	if err != nil {
		s.dropAdmission(flow)
		return
	}
	lease, err := s.cfg.LookupLease(tunnelID)
	if err != nil || lease.Config.TunnelID != tunnelID {
		s.dropAdmission(flow)
		return
	}
	leaseAddr, err := lease.Config.LeaseIPv4()
	if err != nil {
		s.dropAdmission(flow)
		return
	}
	leaseAddr = leaseAddr.Unmap()

	group, err := s.ensureTunnel(tunnelID, lease, leaseAddr)
	if err != nil {
		s.dropAdmission(flow)
		return
	}
	if s.cfg.DormantAfter > 0 {
		now := time.Now()
		s.mu.Lock()
		idle := !group.dormant && !group.lastPayload.IsZero() &&
			!now.Before(group.lastPayload) && now.Sub(group.lastPayload) >= s.cfg.DormantAfter
		s.mu.Unlock()
		if idle {
			if err := s.dormantGroup(group); err != nil {
				s.dropAdmission(flow)
				return
			}
		}
	}

	laneID := result.Admission.Negotiated.LaneID
	if !logicaltunnel.ValidProductLaneID(laneID) || int(laneID) > s.cfg.DesiredLanes {
		s.dropAdmission(flow)
		return
	}
	s.mu.Lock()
	replacing := group.lanes[laneID]
	if replacing != nil {
		if len(group.lanes) != s.cfg.DesiredLanes || len(group.retiring) != 0 || !s.groupReadyLocked(group) {
			s.mu.Unlock()
			s.dropAdmission(flow)
			return
		}
	} else if len(group.retiring) != 0 {
		s.mu.Unlock()
		s.dropAdmission(flow)
		return
	}
	s.mu.Unlock()

	now := time.Now()
	var snapshot datapath.TunnelLaneSnapshot
	if replacing == nil {
		snapshot, err = group.rt.AttachServerAdmission(
			laneID, result.Admission, assoc, s.cfg.Lane, s.cfg.IO.Emit, now,
		)
	} else {
		var lane *datapath.Lane
		lane, err = s.serverCandidateLane(result.Admission, assoc, lease)
		if err == nil {
			cfg := s.serverTransportConfig(result.Admission, assoc)
			err = group.rt.BeginSameIDReplacement(replacing.ref, lane, cfg)
			if err == nil {
				snapshot, err = group.rt.PromoteSameIDReplacement(replacing.ref)
				if err != nil {
					_ = group.rt.FailSameIDReplacement(replacing.ref)
				}
			} else {
				lane.Close()
			}
		}
	}
	if err != nil {
		s.dropAdmission(flow)
		return
	}

	steadyQualified := len(result.Admission.EarlyRecords) != 0
	fresh := &serverLifecycleLane{
		flow: flow, assoc: assoc, ref: snapshot.Ref,
		qualified: replacing != nil || steadyQualified,
		group: group, replaces: replacing, promotedAt: now,
	}
	if replacing != nil {
		replacing.retiring = true
	}
	s.mu.Lock()
	pending := s.pending[flow]
	delete(s.pending, flow)
	group.lanes[laneID] = fresh
	if group.dormant {
		group.lastPayload = now
	}
	group.dormant = false
	if replacing != nil {
		group.retiring[replacing.ref] = replacing
	}
	s.byFlow[flow] = fresh
	delete(s.started, flow)
	s.mu.Unlock()

	if replacing != nil {
		for _, early := range result.Admission.EarlyRecords {
			seg := faketcp.Segment{
				SrcIP: flow.ClientIP, DstIP: flow.ServerIP,
				SrcPort: flow.ClientPort, DstPort: flow.ServerPort,
				Seq: early.Seq, Ack: assoc.SenderNext(),
				Flags: faketcp.FlagACK | faketcp.FlagPSH, Window: 65535,
				Payload: append([]byte(nil), early.Payload...),
			}
			if err := group.rt.HandleSegment(snapshot.Ref, seg, now); err != nil {
				s.reportTunnelError(group, err)
				return
			}
		}
	}
	for _, seg := range pending {
		qualified, err := group.rt.HandleServerSegmentQualified(snapshot.Ref, assoc, seg, time.Now())
		if err != nil {
			s.reportTunnelError(group, err)
			return
		}
		if qualified {
			fresh.qualified = true
			steadyQualified = true
		}
	}
	if steadyQualified {
		s.markLaneQualified(fresh, time.Now())
	}
}

func (s *LifecycleServer) validateAdmissionRequest(req realityfront.AdmissionRequest) error {
	tunnelID, err := logicaltunnel.TunnelIDFromBytes(req.TunnelID)
	if err != nil {
		return ErrLeaseMismatch
	}
	if !logicaltunnel.ValidProductLaneID(req.LaneID) || int(req.LaneID) > s.cfg.DesiredLanes {
		return ErrLifecycleLaneState
	}
	lease, err := s.cfg.LookupLease(tunnelID)
	if err != nil || lease.Config.TunnelID != tunnelID {
		return ErrLeaseMismatch
	}
	leaseAddr, err := lease.Config.LeaseIPv4()
	if err != nil {
		return ErrLeaseMismatch
	}
	leaseAddr = leaseAddr.Unmap()

	s.mu.Lock()
	defer s.mu.Unlock()
	group := s.byTunnel[tunnelID]
	if group == nil {
		return nil
	}
	if group.leaseAddr != leaseAddr {
		return ErrLeaseMismatch
	}
	current := group.lanes[req.LaneID]
	if current == nil {
		if len(group.retiring) != 0 {
			return ErrLifecycleBusy
		}
		return nil
	}
	if len(group.lanes) != s.cfg.DesiredLanes || len(group.retiring) != 0 || !s.groupReadyLocked(group) {
		return ErrLifecycleBusy
	}
	return nil
}

func (s *LifecycleServer) deliverTunnelPackets(group *serverLifecycleTunnel, packets [][]byte, now time.Time) error {
	if group == nil {
		return logicaltunnel.ErrUnknownTunnel
	}
	s.mu.Lock()
	if group.dormant {
		s.mu.Unlock()
		return nil
	}
	group.lastPayload = now
	token := group.token
	s.mu.Unlock()

	err := s.cfg.Router.DeliverFromOwnerAt(token, packets, now)
	if err == nil {
		return nil
	}

	// DORMANT marks the group before runtime transports are detached. A packet
	// already in the owner->service delivery path can therefore race with flow
	// retirement and observe a platformflow "unknown flow" error. Once the
	// group is intentionally dormant that late old-generation business error
	// is local to the retiring lane and must not terminate LifecycleServer.Run.
	s.mu.Lock()
	dormant := group.dormant
	s.mu.Unlock()
	if dormant {
		return nil
	}
	return err
}

func (s *LifecycleServer) ensureTunnel(id logicaltunnel.TunnelID, lease logicaltunnel.Lease, leaseAddr netip.Addr) (*serverLifecycleTunnel, error) {
	s.mu.Lock()
	if existing := s.byTunnel[id]; existing != nil {
		if existing.leaseAddr != leaseAddr {
			s.mu.Unlock()
			return nil, ErrLeaseMismatch
		}
		s.mu.Unlock()
		return existing, nil
	}
	s.mu.Unlock()

	owner, err := datapath.NewLeasedTunnelOwner(lease, s.cfg.DesiredLanes, s.cfg.MaxFlows)
	if err != nil {
		return nil, err
	}
	if err := owner.ConfigurePadding(datapath.TLSStartupPaddingPolicy(s.cfg.TLSStartupPadding)); err != nil {
		owner.Close()
		return nil, err
	}
	group := &serverLifecycleTunnel{
		id: id, leaseAddr: leaseAddr, owner: owner,
		lanes: make(map[uint8]*serverLifecycleLane, s.cfg.DesiredLanes),
		retiring: make(map[logicaltunnel.LaneRef]*serverLifecycleLane),
		lastPayload: time.Now(),
	}
	rt, err := runtimeowner.New(owner, func(packets [][]byte, now time.Time) error {
		return s.deliverTunnelPackets(group, packets, now)
	})
	if err != nil {
		owner.Close()
		return nil, err
	}
	group.rt = rt
	token, err := s.cfg.Router.Register(owner)
	if err != nil {
		rt.Close()
		return nil, err
	}
	group.token = token
	channel, err := platformflow.NewTunnelChannel(owner, func(out platformflow.Outbound) error {
		s.mu.Lock()
		group.lastPayload = time.Now()
		s.mu.Unlock()
		return rt.PlatformWireSink(out)
	})
	if err != nil {
		s.cfg.Router.Unregister(token)
		rt.Close()
		return nil, err
	}
	service, err := platformflow.NewServer(channel, s.cfg.Service)
	if err != nil {
		s.cfg.Router.Unregister(token)
		rt.Close()
		return nil, err
	}
	group.service = service
	if err := s.cfg.Router.SetServiceHandler(token, service); err != nil {
		service.Close()
		s.cfg.Router.Unregister(token)
		rt.Close()
		return nil, err
	}

	s.mu.Lock()
	if existing := s.byTunnel[id]; existing != nil {
		s.mu.Unlock()
		service.Close()
		s.cfg.Router.Unregister(token)
		rt.Close()
		return existing, nil
	}
	s.byTunnel[id] = group
	s.byLease[leaseAddr] = group
	s.mu.Unlock()
	return group, nil
}

func (s *LifecycleServer) serverCandidateLane(session *realityfront.ServerAdmissionSession, assoc *faketcp.ServerAssociation, lease logicaltunnel.Lease) (*datapath.Lane, error) {
	cfg, err := datapath.ServerLaneConfigFromLeasedAdmission(session, assoc, lease, s.cfg.Lane)
	if err != nil {
		return nil, err
	}
	return datapath.NewLane(cfg)
}

func (s *LifecycleServer) serverTransportConfig(session *realityfront.ServerAdmissionSession, assoc *faketcp.ServerAssociation) runtimeowner.TransportConfig {
	flow := assoc.Flow()
	peer := assoc.PeerTCPProfile()
	window, scale, scaleSet := assoc.SteadyWindowProfile()
	return runtimeowner.TransportConfig{
		LocalIP: flow.ServerIP, PeerIP: flow.ClientIP,
		LocalPort: flow.ServerPort, PeerPort: flow.ClientPort,
		SendNext: assoc.SenderNext(), ReceiveNext: session.Boundary,
		AdvertisedWindow: window, AdvertisedWindowSet: true,
		WindowScale: scale, WindowScaleSet: scaleSet,
		InitialRTO: runtimeowner.DefaultRepairRTO,
		RepairHorizon: runtimeowner.DefaultRepairHorizon,
		SACKPermitted: peer.SACKPermitted,
		Emit: s.cfg.IO.Emit,
	}
}

func (s *LifecycleServer) markLaneQualified(lane *serverLifecycleLane, now time.Time) {
	s.mu.Lock()
	current := lane.group.lanes[lane.ref.ID]
	if current != lane {
		s.mu.Unlock()
		return
	}
	lane.qualified = true
	lane.group.lastPayload = now
	s.mu.Unlock()
	s.retireServerReplacement(lane)
}

func (s *LifecycleServer) retireServerReplacement(lane *serverLifecycleLane) {
	if lane == nil || lane.group == nil {
		return
	}
	s.mu.Lock()
	replacing := lane.replaces
	var closeStarted time.Time
	if replacing != nil {
		closeStarted = replacing.closeStarted
	}
	s.mu.Unlock()
	if replacing == nil {
		return
	}
	now := time.Now()
	if closeStarted.IsZero() {
		if err := lane.group.rt.CloseWrite(replacing.ref, now); err != nil &&
			!errors.Is(err, runtimeowner.ErrRuntimeClosed) &&
			!errors.Is(err, runtimeowner.ErrTransportPeerReset) {
			return
		}
		s.mu.Lock()
		if lane.replaces == replacing && replacing.closeStarted.IsZero() {
			replacing.closeStarted = now
		}
		s.mu.Unlock()
		return
	}

	stats, ok := lane.group.rt.TransportStats(replacing.ref)
	closeComplete := ok && stats.LocalFINAcked && stats.PeerFIN
	closeBudget := s.cfg.ReplacementGrace
	if rtoBudget := 2 * s.cfg.InitialRTO; rtoBudget > 0 && rtoBudget < closeBudget {
		closeBudget = rtoBudget
	}
	closeExpired := !now.Before(closeStarted) && now.Sub(closeStarted) >= closeBudget
	if !closeComplete && !closeExpired {
		return
	}
	if err := lane.group.rt.RetireIncarnation(replacing.ref); err != nil &&
		!errors.Is(err, datapath.ErrLaneUnavailable) {
		return
	}
	s.mu.Lock()
	if current := lane.group.retiring[replacing.ref]; current == replacing {
		delete(lane.group.retiring, replacing.ref)
	}
	if lane.replaces == replacing {
		lane.replaces = nil
	}
	delete(s.byFlow, replacing.flow)
	s.mu.Unlock()
	s.table.Remove(replacing.flow)
}

func (s *LifecycleServer) RoutePacket(packet []byte, now time.Time) error {
	dst, err := packetDestination4(packet)
	if err != nil {
		return err
	}
	s.mu.Lock()
	group := s.byLease[dst]
	ready := s.groupReadyLocked(group)
	s.mu.Unlock()
	if group == nil {
		return linuxserver.ErrNoLeaseRoute
	}
	if !ready {
		s.refreshQualifiedFromTransport(group)
		s.mu.Lock()
		ready = s.groupReadyLocked(group)
		s.mu.Unlock()
	}
	if !ready {
		return ErrTunnelNotQualified
	}
	out, err := s.cfg.Router.RouteFromTUN(packet, now)
	if err != nil {
		return err
	}
	if out.IsGame {
		err = group.rt.SendGame(out.Game, now)
	} else {
		err = group.rt.SendNormal(out.Normal, now)
	}
	if err == nil {
		s.mu.Lock()
		group.lastPayload = now
		s.mu.Unlock()
	}
	return err
}

func (s *LifecycleServer) refreshQualifiedFromTransport(group *serverLifecycleTunnel) {
	if group == nil {
		return
	}
	s.mu.Lock()
	lanes := make([]*serverLifecycleLane, 0, len(group.lanes))
	for _, lane := range group.lanes {
		if !lane.qualified {
			lanes = append(lanes, lane)
		}
	}
	s.mu.Unlock()
	for _, lane := range lanes {
		stats, ok := group.rt.TransportStats(lane.ref)
		if !ok || stats.Received == 0 {
			continue
		}
		s.mu.Lock()
		if current := group.lanes[lane.ref.ID]; current == lane {
			lane.qualified = true
		}
		s.mu.Unlock()
	}
}

func (s *LifecycleServer) groupReadyLocked(group *serverLifecycleTunnel) bool {
	if group == nil || group.dormant || len(group.lanes) != s.cfg.DesiredLanes {
		return false
	}
	for _, lane := range group.lanes {
		if !lane.qualified {
			return false
		}
	}
	return true
}

func (s *LifecycleServer) tick(now time.Time) error {
	if err := s.table.EmitRetransmitDue(now); err != nil {
		return err
	}
	s.table.Sweep(now)
	s.mu.Lock()
	groups := make([]*serverLifecycleTunnel, 0, len(s.byTunnel))
	for _, group := range s.byTunnel {
		groups = append(groups, group)
	}
	s.mu.Unlock()
	var errs []error
	for _, group := range groups {
		if err := group.rt.Tick(now); err != nil {
			errs = append(errs, err)
		}
		group.service.Tick(now)
		s.mu.Lock()
		replacements := make([]*serverLifecycleLane, 0, len(group.retiring))
		for _, lane := range group.lanes {
			if lane.replaces == nil {
				continue
			}
			closeStarted := lane.replaces.closeStarted
			graceExpired := !now.Before(lane.promotedAt) &&
				now.Sub(lane.promotedAt) >= s.cfg.ReplacementGrace
			if !closeStarted.IsZero() || graceExpired {
				replacements = append(replacements, lane)
			}
		}
		s.mu.Unlock()
		for _, lane := range replacements {
			s.retireServerReplacement(lane)
		}
		if s.cfg.DormantAfter > 0 {
			s.mu.Lock()
			active := !group.dormant && len(group.lanes) != 0
			last := group.lastPayload
			s.mu.Unlock()
			if active && !last.IsZero() && !now.Before(last) && now.Sub(last) >= s.cfg.DormantAfter {
				if err := s.DormantTunnel(group.id); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}
	return errors.Join(errs...)
}

func (s *LifecycleServer) DormantTunnel(id logicaltunnel.TunnelID) error {
	s.admitMu.Lock()
	defer s.admitMu.Unlock()
	s.mu.Lock()
	group := s.byTunnel[id]
	s.mu.Unlock()
	if group == nil {
		return logicaltunnel.ErrUnknownTunnel
	}
	return s.dormantGroup(group)
}

func (s *LifecycleServer) dormantGroup(group *serverLifecycleTunnel) error {
	if group == nil {
		return logicaltunnel.ErrUnknownTunnel
	}
	s.mu.Lock()
	if group.dormant {
		s.mu.Unlock()
		return nil
	}
	group.dormant = true
	lanes := make([]*serverLifecycleLane, 0, len(group.lanes)+len(group.retiring))
	seen := make(map[*serverLifecycleLane]struct{})
	for _, lane := range group.lanes {
		if _, ok := seen[lane]; !ok {
			seen[lane] = struct{}{}
			lanes = append(lanes, lane)
		}
	}
	for _, lane := range group.retiring {
		if _, ok := seen[lane]; !ok {
			seen[lane] = struct{}{}
			lanes = append(lanes, lane)
		}
	}
	s.mu.Unlock()

	if _, err := group.rt.Dormant(); err != nil {
		s.mu.Lock()
		group.dormant = false
		s.mu.Unlock()
		return err
	}

	s.mu.Lock()
	for _, lane := range lanes {
		delete(s.byFlow, lane.flow)
	}
	clear(group.lanes)
	clear(group.retiring)
	group.lastPayload = time.Time{}
	s.mu.Unlock()
	for _, lane := range lanes {
		s.table.Remove(lane.flow)
	}
	return nil
}

func (s *LifecycleServer) TunnelQualified(id logicaltunnel.TunnelID) bool {
	if s == nil {
		return false
	}
	s.mu.Lock()
	group := s.byTunnel[id]
	ready := s.groupReadyLocked(group)
	s.mu.Unlock()
	if group == nil || ready {
		return ready
	}
	s.refreshQualifiedFromTransport(group)
	s.mu.Lock()
	ready = s.groupReadyLocked(group)
	s.mu.Unlock()
	return ready
}

func (s *LifecycleServer) TunnelStats(id logicaltunnel.TunnelID) (datapath.TunnelOwnerStats, bool) {
	s.mu.Lock()
	group := s.byTunnel[id]
	s.mu.Unlock()
	if group == nil {
		return datapath.TunnelOwnerStats{}, false
	}
	return group.owner.Stats(), true
}

func (s *LifecycleServer) dropAdmission(flow faketcp.ServerFlow) {
	s.table.Remove(flow)
	s.mu.Lock()
	delete(s.started, flow)
	delete(s.pending, flow)
	s.mu.Unlock()
}

func (s *LifecycleServer) reportTunnelError(_ *serverLifecycleTunnel, _ error) {
	// Admission runs off the server read loop. The active association will fail
	// closed on its next routed segment; no background error is promoted into a
	// process-wide crash from this helper.
}

func (s *LifecycleServer) Close() error {
	if s == nil {
		return nil
	}
	var out error
	s.once.Do(func() {
		s.mu.Lock()
		groups := make([]*serverLifecycleTunnel, 0, len(s.byTunnel))
		for _, group := range s.byTunnel {
			groups = append(groups, group)
		}
		clear(s.byFlow)
		clear(s.byTunnel)
		clear(s.byLease)
		clear(s.pending)
		s.mu.Unlock()
		for _, group := range groups {
			group.service.Close()
			s.cfg.Router.Unregister(group.token)
			group.rt.Close()
		}
		s.table.Close()
		out = s.cfg.IO.close()
	})
	return out
}
