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
	ErrEndpointConfig       = errors.New("runtimeentry: invalid segment endpoint")
	ErrLeaseMismatch        = errors.New("runtimeentry: protected admission tunnel does not match configured lease")
	ErrTunnelActive         = errors.New("runtimeentry: logical tunnel already has an active runtime")
	ErrTunnelNotQualified   = errors.New("runtimeentry: server egress blocked until first client steady record")
	ErrSteadyQueueFull      = errors.New("runtimeentry: pre-attach steady queue full")
	ErrClientRuntimeStopped = errors.New("runtimeentry: client runtime stopped")
)

type SegmentIO struct {
	Read  func() (faketcp.Segment, error)
	Emit  faketcp.SegmentEmitter
	Close func() error
}

func (io SegmentIO) validate() error {
	if io.Read == nil || io.Emit == nil {
		return ErrEndpointConfig
	}
	return nil
}

func (io SegmentIO) close() error {
	if io.Close == nil {
		return nil
	}
	return io.Close()
}

type ClientConfig struct {
	IO         SegmentIO
	Flow       faketcp.ClientFlow
	ClientISN  uint32
	InitialRTO time.Duration

	Lease     logicaltunnel.Lease
	MaxFlows  int
	Admission realityfront.ClientAdmissionConfig
	Lane      datapath.ClientLaneParams
	Deliver   runtimeowner.PacketSink

	TickInterval time.Duration
}

type Client struct {
	mu sync.Mutex

	io    SegmentIO
	assoc *faketcp.ClientAssociation
	owner *datapath.TunnelOwner
	rt    *runtimeowner.Runtime
	ref   logicaltunnel.LaneRef

	attached bool
	closed   bool

	cancel context.CancelFunc
	errCh  chan error
	once   sync.Once
}

func DialClient(ctx context.Context, cfg ClientConfig) (*Client, error) {
	if ctx == nil {
		return nil, ErrEndpointConfig
	}
	if err := cfg.IO.validate(); err != nil {
		return nil, err
	}
	if err := cfg.Flow.Validate(); err != nil {
		return nil, err
	}
	if err := cfg.Lease.Validate(); err != nil {
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
	if len(cfg.Admission.TunnelID) == 0 {
		cfg.Admission.TunnelID = cfg.Lease.Config.TunnelID.Bytes()
	}
	requested, err := logicaltunnel.TunnelIDFromBytes(cfg.Admission.TunnelID)
	if err != nil || requested != cfg.Lease.Config.TunnelID {
		return nil, ErrLeaseMismatch
	}
	if cfg.ClientISN == 0 {
		cfg.ClientISN, err = randomISN()
		if err != nil {
			return nil, err
		}
	}

	owner, err := datapath.NewLeasedTunnelOwner(cfg.Lease, 1, cfg.MaxFlows)
	if err != nil {
		return nil, err
	}
	rt, err := runtimeowner.New(owner, cfg.Deliver)
	if err != nil {
		owner.Close()
		return nil, err
	}
	assoc, err := faketcp.NewClientAssociation(cfg.Flow, cfg.ClientISN, cfg.InitialRTO, cfg.IO.Emit)
	if err != nil {
		rt.Close()
		return nil, err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	c := &Client{
		io: cfg.IO, assoc: assoc, owner: owner, rt: rt,
		cancel: cancel, errCh: make(chan error, 1),
	}
	go c.readLoop(runCtx)
	go c.tickLoop(runCtx, cfg.TickInterval)

	if err := assoc.Start(time.Now()); err != nil {
		c.Close()
		return nil, err
	}
	if err := assoc.WaitEstablished(ctx); err != nil {
		c.Close()
		return nil, err
	}
	conn := assoc.BootstrapConn()
	if conn == nil {
		c.Close()
		return nil, ErrClientRuntimeStopped
	}
	session, err := realityfront.EstablishClient(ctx, conn, cfg.Admission)
	if err != nil {
		c.Close()
		return nil, err
	}
	gotTunnel, err := logicaltunnel.TunnelIDFromBytes(session.Negotiated.TunnelID)
	if err != nil || gotTunnel != cfg.Lease.Config.TunnelID || session.Negotiated.LaneID != 1 {
		c.Close()
		return nil, ErrLeaseMismatch
	}
	handoff, err := assoc.Detach()
	if err != nil {
		c.Close()
		return nil, err
	}

	laneParams := cfg.Lane
	laneParams.PeerMSS = handoff.Peer.MSS
	laneParams.PeerMSSSet = handoff.Peer.AdvertisedMSS
	snapshot, err := rt.AttachClientAdmission(1, session, laneParams, runtimeowner.TransportConfig{
		LocalIP: cfg.Flow.LocalIP, PeerIP: cfg.Flow.PeerIP,
		LocalPort: cfg.Flow.LocalPort, PeerPort: cfg.Flow.PeerPort,
		SendNext: handoff.SendNext, ReceiveNext: handoff.ReceiveNext,
		InitialRTO: runtimeowner.DefaultRepairRTO,
		RepairHorizon: runtimeowner.DefaultRepairHorizon,
		SACKPermitted: handoff.Peer.SACKPermitted,
		Emit: cfg.IO.Emit,
	})
	if err != nil {
		c.Close()
		return nil, err
	}
	c.mu.Lock()
	c.ref = snapshot.Ref
	c.attached = true
	c.mu.Unlock()
	return c, nil
}

func (c *Client) Owner() *datapath.TunnelOwner {
	if c == nil {
		return nil
	}
	return c.owner
}

func (c *Client) Ref() logicaltunnel.LaneRef {
	if c == nil {
		return logicaltunnel.LaneRef{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ref
}

func (c *Client) Errors() <-chan error {
	if c == nil {
		return nil
	}
	return c.errCh
}

func (c *Client) SendPacket(packet []byte, now time.Time) error {
	if c == nil {
		return ErrClientRuntimeStopped
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

func (c *Client) SendNormal(records []datapath.WireRecord, now time.Time) error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	return c.rt.SendNormal(records, now)
}

func (c *Client) SendGame(out datapath.GameOutboundResult, now time.Time) error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	return c.rt.SendGame(out, now)
}

func (c *Client) PlatformWireSink(out platformflow.Outbound) error {
	if c == nil {
		return ErrClientRuntimeStopped
	}
	return c.rt.PlatformWireSink(out)
}

func (c *Client) readLoop(ctx context.Context) {
	for {
		seg, err := c.io.Read()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			default:
			}
			c.report(err)
			c.assoc.Close()
			return
		}
		c.mu.Lock()
		attached := c.attached
		ref := c.ref
		closed := c.closed
		c.mu.Unlock()
		if closed {
			return
		}
		if attached {
			err = c.rt.HandleSegment(ref, seg, time.Now())
		} else {
			err = c.assoc.HandleSegment(seg, time.Now())
		}
		if err != nil {
			if errors.Is(err, faketcp.ErrClientDetached) {
				continue
			}
			c.report(err)
			c.assoc.Close()
			return
		}
	}
}

func (c *Client) tickLoop(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			c.mu.Lock()
			attached := c.attached
			closed := c.closed
			c.mu.Unlock()
			if closed {
				return
			}
			var err error
			if attached {
				err = c.rt.Tick(now)
			} else {
				_, err = c.assoc.EmitRetransmitDue(now)
			}
			if err != nil {
				c.report(err)
			}
		}
	}
}

func (c *Client) report(err error) {
	if err == nil {
		return
	}
	select {
	case c.errCh <- err:
	default:
	}
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	c.once.Do(func() {
		c.mu.Lock()
		c.closed = true
		c.mu.Unlock()
		if c.cancel != nil {
			c.cancel()
		}
		c.assoc.Close()
		c.rt.Close()
		closeErr = c.io.close()
	})
	return closeErr
}

type LeaseLookup func(logicaltunnel.TunnelID) (logicaltunnel.Lease, error)

type ServerConfig struct {
	IO             SegmentIO
	ListenPort     uint16
	MaxAssociations int
	InitialRTO     time.Duration
	TickInterval   time.Duration
	MaxFlows       int

	Admission realityfront.ServerAdmissionConfig
	Fallback  realityfront.FallbackConfig
	LookupLease LeaseLookup
	Lane      datapath.ServerLaneParams

	Router  *linuxserver.SharedTUNRouter
	Service platformflow.ServerConfig
}

type serverTunnel struct {
	flow      faketcp.ServerFlow
	leaseAddr netip.Addr
	assoc     *faketcp.ServerAssociation
	owner     *datapath.TunnelOwner
	rt        *runtimeowner.Runtime
	ref       logicaltunnel.LaneRef
	token     linuxserver.BindingToken
	service   *platformflow.Server
	qualified bool
}

type Server struct {
	mu sync.Mutex

	cfg   ServerConfig
	table *faketcp.ServerAssociationTable

	started map[faketcp.ServerFlow]bool
	pending map[faketcp.ServerFlow][]faketcp.Segment
	byFlow  map[faketcp.ServerFlow]*serverTunnel
	byTunnel map[logicaltunnel.TunnelID]*serverTunnel
	byLease map[netip.Addr]*serverTunnel

	closed bool
	once sync.Once
}

func NewServer(cfg ServerConfig) (*Server, error) {
	if err := cfg.IO.validate(); err != nil {
		return nil, err
	}
	if cfg.ListenPort == 0 || cfg.Router == nil || cfg.LookupLease == nil {
		return nil, ErrEndpointConfig
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
	return &Server{
		cfg: cfg, table: table,
		started: make(map[faketcp.ServerFlow]bool),
		pending: make(map[faketcp.ServerFlow][]faketcp.Segment),
		byFlow: make(map[faketcp.ServerFlow]*serverTunnel),
		byTunnel: make(map[logicaltunnel.TunnelID]*serverTunnel),
		byLease: make(map[netip.Addr]*serverTunnel),
	}, nil
}

type segmentRead struct {
	seg faketcp.Segment
	err error
}

func (s *Server) Run(ctx context.Context) error {
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

func (s *Server) handleSegment(ctx context.Context, seg faketcp.Segment, now time.Time) error {
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
	tunnel := s.byFlow[flow]
	if tunnel == nil {
		if state, exists := assoc.TransitionState(); exists && state == faketcp.TransitionDetached {
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
	s.mu.Unlock()

	if tunnel != nil {
		qualified, err := tunnel.rt.HandleServerSegmentQualified(tunnel.ref, assoc, seg, now)
		if err != nil {
			return err
		}
		if qualified {
			s.mu.Lock()
			if current := s.byFlow[flow]; current == tunnel {
				current.qualified = true
			}
			s.mu.Unlock()
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

func (s *Server) admit(ctx context.Context, assoc *faketcp.ServerAssociation) {
	flow := assoc.Flow()
	result, err := realityfront.HandleServerAssociation(ctx, assoc, s.cfg.Admission, s.cfg.Fallback)
	if err != nil || result.Admission == nil {
		s.table.Remove(flow)
		s.mu.Lock()
		delete(s.started, flow)
		delete(s.pending, flow)
		s.mu.Unlock()
		return
	}
	tunnelID, err := logicaltunnel.TunnelIDFromBytes(result.Admission.Negotiated.TunnelID)
	if err != nil || result.Admission.Negotiated.LaneID != 1 {
		s.table.Remove(flow)
		return
	}
	lease, err := s.cfg.LookupLease(tunnelID)
	if err != nil || lease.Config.TunnelID != tunnelID {
		s.table.Remove(flow)
		return
	}
	leaseAddr, err := lease.Config.LeaseIPv4()
	if err != nil {
		s.table.Remove(flow)
		return
	}
	leaseAddr = leaseAddr.Unmap()

	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, s.cfg.MaxFlows)
	if err != nil {
		s.table.Remove(flow)
		return
	}
	var token linuxserver.BindingToken
	rt, err := runtimeowner.New(owner, func(packets [][]byte, now time.Time) error {
		return s.cfg.Router.DeliverFromOwnerAt(token, packets, now)
	})
	if err != nil {
		owner.Close()
		s.table.Remove(flow)
		return
	}
	token, err = s.cfg.Router.Register(owner)
	if err != nil {
		rt.Close()
		s.table.Remove(flow)
		return
	}
	channel, err := platformflow.NewTunnelChannel(owner, rt.PlatformWireSink)
	if err != nil {
		s.cfg.Router.Unregister(token)
		rt.Close()
		s.table.Remove(flow)
		return
	}
	service, err := platformflow.NewServer(channel, s.cfg.Service)
	if err != nil {
		s.cfg.Router.Unregister(token)
		rt.Close()
		s.table.Remove(flow)
		return
	}
	if err := s.cfg.Router.SetServiceHandler(token, service); err != nil {
		service.Close()
		s.cfg.Router.Unregister(token)
		rt.Close()
		s.table.Remove(flow)
		return
	}
	snapshot, err := rt.AttachServerAdmission(1, result.Admission, assoc, s.cfg.Lane, s.cfg.IO.Emit, time.Now())
	if err != nil {
		service.Close()
		s.cfg.Router.Unregister(token)
		rt.Close()
		s.table.Remove(flow)
		return
	}
	tunnel := &serverTunnel{
		flow: flow, leaseAddr: leaseAddr, assoc: assoc,
		owner: owner, rt: rt, ref: snapshot.Ref, token: token, service: service,
		qualified: len(result.Admission.EarlyRecords) != 0,
	}

	s.mu.Lock()
	if existing := s.byTunnel[tunnelID]; existing != nil {
		s.mu.Unlock()
		service.Close()
		s.cfg.Router.Unregister(token)
		rt.Close()
		s.table.Remove(flow)
		return
	}
	pending := s.pending[flow]
	delete(s.pending, flow)
	s.byFlow[flow] = tunnel
	s.byTunnel[tunnelID] = tunnel
	s.byLease[leaseAddr] = tunnel
	s.mu.Unlock()

	for _, seg := range pending {
		qualified, err := rt.HandleServerSegmentQualified(snapshot.Ref, assoc, seg, time.Now())
		if err != nil {
			return
		}
		if qualified {
			s.mu.Lock()
			if current := s.byFlow[flow]; current == tunnel {
				current.qualified = true
			}
			s.mu.Unlock()
		}
	}
}

func (s *Server) RoutePacket(packet []byte, now time.Time) error {
	dst, err := packetDestination4(packet)
	if err != nil {
		return err
	}
	s.mu.Lock()
	tunnel := s.byLease[dst]
	qualified := tunnel != nil && tunnel.qualified
	s.mu.Unlock()
	if tunnel == nil {
		return linuxserver.ErrNoLeaseRoute
	}
	if !qualified {
		if stats, ok := tunnel.rt.TransportStats(tunnel.ref); ok && stats.Received != 0 {
			s.mu.Lock()
			if current := s.byFlow[tunnel.flow]; current == tunnel {
				current.qualified = true
				qualified = true
			}
			s.mu.Unlock()
		}
	}
	if !qualified {
		return ErrTunnelNotQualified
	}

	out, err := s.cfg.Router.RouteFromTUN(packet, now)
	if err != nil {
		return err
	}
	if out.IsGame {
		return tunnel.rt.SendGame(out.Game, now)
	}
	return tunnel.rt.SendNormal(out.Normal, now)
}

func (s *Server) tick(now time.Time) error {
	if err := s.table.EmitRetransmitDue(now); err != nil {
		return err
	}
	s.table.Sweep(now)
	s.mu.Lock()
	tunnels := make([]*serverTunnel, 0, len(s.byTunnel))
	for _, tunnel := range s.byTunnel {
		tunnels = append(tunnels, tunnel)
	}
	s.mu.Unlock()
	var errs []error
	for _, tunnel := range tunnels {
		if err := tunnel.rt.Tick(now); err != nil {
			errs = append(errs, err)
		}
		tunnel.service.Tick(now)
	}
	return errors.Join(errs...)
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	var closeErr error
	s.once.Do(func() {
		s.mu.Lock()
		s.closed = true
		tunnels := make([]*serverTunnel, 0, len(s.byTunnel))
		for _, tunnel := range s.byTunnel {
			tunnels = append(tunnels, tunnel)
		}
		clear(s.byFlow)
		clear(s.byTunnel)
		clear(s.byLease)
		clear(s.pending)
		s.mu.Unlock()

		for _, tunnel := range tunnels {
			tunnel.service.Close()
			s.cfg.Router.Unregister(tunnel.token)
			tunnel.rt.Close()
		}
		s.table.Close()
		closeErr = s.cfg.IO.close()
	})
	return closeErr
}

func packetDestination4(packet []byte) (netip.Addr, error) {
	if len(packet) < 20 || packet[0]>>4 != 4 {
		return netip.Addr{}, linuxserver.ErrInvalidIPv4
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || ihl > len(packet) {
		return netip.Addr{}, linuxserver.ErrInvalidIPv4
	}
	total := int(binary.BigEndian.Uint16(packet[2:4]))
	if total != len(packet) {
		return netip.Addr{}, linuxserver.ErrInvalidIPv4
	}
	var raw [4]byte
	copy(raw[:], packet[16:20])
	return netip.AddrFrom4(raw), nil
}

func randomISN() (uint32, error) {
	var raw [4]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return 0, fmt.Errorf("runtimeentry: random ISN: %w", err)
	}
	return binary.BigEndian.Uint32(raw[:]), nil
}
