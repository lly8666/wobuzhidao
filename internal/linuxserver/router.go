package linuxserver

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

var (
	ErrInvalidLeasePool = errors.New("linuxserver: invalid shared-TUN lease pool")
	ErrRouterCapacity   = errors.New("linuxserver: shared-TUN router capacity reached")
	ErrLeaseInUse       = errors.New("linuxserver: leased IPv4 already registered")
	ErrTunnelInUse      = errors.New("linuxserver: tunnel id already registered with another lease")
	ErrNoLeaseRoute     = errors.New("linuxserver: no live Logical Tunnel for IPv4 destination")
	ErrStaleBinding     = errors.New("linuxserver: stale shared-TUN binding")
	ErrShortTUNWrite    = errors.New("linuxserver: short shared-TUN write")
	ErrInvalidIPv4      = errors.New("linuxserver: invalid IPv4 packet")
)

type Owner interface {
	Lease() (logicaltunnel.Lease, bool)
	Stats() datapath.TunnelOwnerStats
	NormalOutbound([]byte, time.Time) ([]datapath.WireRecord, error)
	GameOutbound([]byte, time.Time) (datapath.GameOutboundResult, error)
}

type PacketWriter interface {
	WritePacket([]byte) (int, error)
}

type BindingToken struct {
	TunnelID logicaltunnel.TunnelID
	serial   uint64
}

type routeBinding struct {
	lease  logicaltunnel.Lease
	owner  Owner
	serial uint64
}

type SharedTUNRouter struct {
	mu sync.Mutex

	pool netip.Prefix
	max  int
	tun  PacketWriter

	nextSerial uint64
	byLease    map[netip.Addr]*routeBinding
	byTunnel   map[logicaltunnel.TunnelID]*routeBinding
}

func NewSharedTUNRouter(pool netip.Prefix, maxTunnels int, tun PacketWriter) (*SharedTUNRouter, error) {
	if !pool.IsValid() || !pool.Addr().Is4() || pool.Bits() > 30 {
		return nil, ErrInvalidLeasePool
	}
	if maxTunnels <= 0 || tun == nil {
		return nil, ErrRouterCapacity
	}
	pool = pool.Masked()
	return &SharedTUNRouter{
		pool:       pool,
		max:        maxTunnels,
		tun:        tun,
		nextSerial: 1,
		byLease:    make(map[netip.Addr]*routeBinding, maxTunnels),
		byTunnel:   make(map[logicaltunnel.TunnelID]*routeBinding, maxTunnels),
	}, nil
}

func (r *SharedTUNRouter) LeasePool() netip.Prefix {
	if r == nil {
		return netip.Prefix{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.pool
}

// Register binds one stable Logical Tunnel lease to its current owner. Rebinding
// the exact same TunnelID+/32 to a replacement runtime owner is allowed and
// returns a fresh token; stale tokens can no longer write into the shared TUN.
func (r *SharedTUNRouter) Register(owner Owner) (BindingToken, error) {
	if r == nil || owner == nil {
		return BindingToken{}, logicaltunnel.ErrInvalidIdentity
	}
	lease, ok := owner.Lease()
	if !ok {
		return BindingToken{}, logicaltunnel.ErrInvalidIdentity
	}
	if err := lease.Validate(); err != nil {
		return BindingToken{}, err
	}
	addr, err := lease.Config.LeaseIPv4()
	if err != nil {
		return BindingToken{}, err
	}
	addr = addr.Unmap()
	if !r.pool.Contains(addr) {
		return BindingToken{}, fmt.Errorf("%w: lease=%s pool=%s", ErrInvalidLeasePool, addr, r.pool)
	}
	tunnelID := lease.Config.TunnelID

	r.mu.Lock()
	defer r.mu.Unlock()

	byLease := r.byLease[addr]
	byTunnel := r.byTunnel[tunnelID]
	if byLease != nil || byTunnel != nil {
		if byLease == nil || byTunnel == nil || byLease != byTunnel {
			if byLease != nil && byLease.lease.Config.TunnelID != tunnelID {
				return BindingToken{}, fmt.Errorf("%w: lease=%s", ErrLeaseInUse, addr)
			}
			return BindingToken{}, fmt.Errorf("%w: tunnel=%s", ErrTunnelInUse, tunnelID.String())
		}
		oldAddr, oldErr := byLease.lease.Config.LeaseIPv4()
		if oldErr != nil || oldAddr.Unmap() != addr {
			return BindingToken{}, fmt.Errorf("%w: tunnel=%s", ErrTunnelInUse, tunnelID.String())
		}
		byLease.owner = owner
		byLease.serial = r.takeSerialLocked()
		return BindingToken{TunnelID: tunnelID, serial: byLease.serial}, nil
	}
	if len(r.byTunnel) >= r.max {
		return BindingToken{}, ErrRouterCapacity
	}
	binding := &routeBinding{
		lease:  lease.Clone(),
		owner:  owner,
		serial: r.takeSerialLocked(),
	}
	r.byLease[addr] = binding
	r.byTunnel[tunnelID] = binding
	return BindingToken{TunnelID: tunnelID, serial: binding.serial}, nil
}

func (r *SharedTUNRouter) takeSerialLocked() uint64 {
	serial := r.nextSerial
	r.nextSerial++
	if serial == 0 || r.nextSerial == 0 {
		// Zero is reserved for an invalid token. Saturation is practically
		// unreachable, but keep stale-token fencing fail-closed.
		r.nextSerial = 1
	}
	return serial
}

func (r *SharedTUNRouter) Unregister(token BindingToken) bool {
	if r == nil || token.TunnelID == (logicaltunnel.TunnelID{}) || token.serial == 0 {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	binding := r.byTunnel[token.TunnelID]
	if binding == nil || binding.serial != token.serial {
		return false
	}
	addr, err := binding.lease.Config.LeaseIPv4()
	if err == nil {
		delete(r.byLease, addr.Unmap())
	}
	delete(r.byTunnel, token.TunnelID)
	return true
}

type RoutedOutbound struct {
	TunnelID logicaltunnel.TunnelID
	Lease    netip.Addr

	Normal []datapath.WireRecord
	Game   datapath.GameOutboundResult
	IsGame bool
}

// RouteFromTUN demultiplexes one return IPv4 packet by destination lease, then
// sends it through the already-existing TunnelOwner. It never creates a lane or
// a BusinessFlow.
func (r *SharedTUNRouter) RouteFromTUN(packet []byte, now time.Time) (RoutedOutbound, error) {
	_, dst, err := ipv4Endpoints(packet)
	if err != nil {
		return RoutedOutbound{}, err
	}

	r.mu.Lock()
	binding := r.byLease[dst]
	if binding == nil {
		r.mu.Unlock()
		return RoutedOutbound{}, fmt.Errorf("%w: dst=%s", ErrNoLeaseRoute, dst)
	}
	owner := binding.owner
	lease := binding.lease.Clone()
	r.mu.Unlock()

	stats := owner.Stats()
	out := RoutedOutbound{
		TunnelID: lease.Config.TunnelID,
		Lease:    dst,
	}
	switch stats.DesiredLanes {
	case 1:
		out.Normal, err = owner.NormalOutbound(packet, now)
		return out, err
	case 2, 3, 4:
		out.Game, err = owner.GameOutbound(packet, now)
		out.IsGame = true
		return out, err
	default:
		return RoutedOutbound{}, datapath.ErrLaneUnavailable
	}
}

// DeliverFromOwner writes already-decoded client->Internet packets into the
// shared TUN. The current binding token is required so a retired/rebound runtime
// cannot write after ownership moved. Source==lease is checked again at the
// platform boundary as defense in depth; the TunnelOwner server ingress fence
// remains authoritative earlier in the path.
func (r *SharedTUNRouter) DeliverFromOwner(token BindingToken, packets [][]byte) error {
	if r == nil || token.serial == 0 {
		return ErrStaleBinding
	}
	r.mu.Lock()
	binding := r.byTunnel[token.TunnelID]
	if binding == nil || binding.serial != token.serial {
		r.mu.Unlock()
		return ErrStaleBinding
	}
	lease := binding.lease.Clone()
	tun := r.tun
	r.mu.Unlock()

	addr, err := lease.Config.LeaseIPv4()
	if err != nil {
		return err
	}
	for _, packet := range packets {
		if err := logicaltunnel.ValidateIPv4Source(packet, addr); err != nil {
			return err
		}
		owned := append([]byte(nil), packet...)
		n, err := tun.WritePacket(owned)
		if err != nil {
			return err
		}
		if n != len(owned) {
			return fmt.Errorf("%w: wrote=%d want=%d", ErrShortTUNWrite, n, len(owned))
		}
	}
	return nil
}

func (r *SharedTUNRouter) ActiveTunnels() int {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.byTunnel)
}

func ipv4Endpoints(packet []byte) (netip.Addr, netip.Addr, error) {
	if len(packet) < 20 || len(packet) > logicaltunnel.MaxLeasedIPv4PacketLen || packet[0]>>4 != 4 {
		return netip.Addr{}, netip.Addr{}, ErrInvalidIPv4
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || ihl > len(packet) {
		return netip.Addr{}, netip.Addr{}, ErrInvalidIPv4
	}
	total := int(binary.BigEndian.Uint16(packet[2:4]))
	if total != len(packet) {
		return netip.Addr{}, netip.Addr{}, ErrInvalidIPv4
	}
	var srcRaw, dstRaw [4]byte
	copy(srcRaw[:], packet[12:16])
	copy(dstRaw[:], packet[16:20])
	return netip.AddrFrom4(srcRaw), netip.AddrFrom4(dstRaw), nil
}
