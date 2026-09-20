package windowsclient

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net/netip"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

var (
	ErrInvalidBinding  = errors.New("windowsclient: invalid Wintun logical tunnel binding")
	ErrInvalidIPv4     = errors.New("windowsclient: invalid IPv4 packet")
	ErrDestinationLeak = errors.New("windowsclient: inbound packet destination does not match tunnel lease")
	ErrShortTUNWrite   = errors.New("windowsclient: short Wintun write")
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

type RoutedOutbound struct {
	Normal []datapath.WireRecord
	Game   datapath.GameOutboundResult
	IsGame bool
}

// Router is the single-client Wintun boundary for one stable Logical Tunnel
// lease. It never creates BusinessFlows or lanes.
type Router struct {
	owner Owner
	tun   PacketWriter
	lease logicaltunnel.Lease
	addr  netip.Addr
}

func NewRouter(owner Owner, tun PacketWriter) (*Router, error) {
	if owner == nil || tun == nil {
		return nil, ErrInvalidBinding
	}
	lease, ok := owner.Lease()
	if !ok {
		return nil, ErrInvalidBinding
	}
	if err := lease.Validate(); err != nil {
		return nil, err
	}
	addr, err := lease.Config.LeaseIPv4()
	if err != nil {
		return nil, err
	}
	return &Router{owner: owner, tun: tun, lease: lease.Clone(), addr: addr.Unmap()}, nil
}

func (r *Router) Lease() logicaltunnel.Lease {
	if r == nil {
		return logicaltunnel.Lease{}
	}
	return r.lease.Clone()
}

// RouteFromTUN accepts only IPv4 sourced from the authenticated /32 lease.
// The leased client TunnelOwner repeats the same source fence before Lane state,
// keeping the platform boundary defense-in-depth.
func (r *Router) RouteFromTUN(packet []byte, now time.Time) (RoutedOutbound, error) {
	if r == nil {
		return RoutedOutbound{}, ErrInvalidBinding
	}
	if err := logicaltunnel.ValidateIPv4Source(packet, r.addr); err != nil {
		return RoutedOutbound{}, err
	}
	stats := r.owner.Stats()
	switch stats.DesiredLanes {
	case 1:
		records, err := r.owner.NormalOutbound(packet, now)
		return RoutedOutbound{Normal: records}, err
	case 2, 3, 4:
		game, err := r.owner.GameOutbound(packet, now)
		return RoutedOutbound{Game: game, IsGame: true}, err
	default:
		return RoutedOutbound{}, datapath.ErrLaneUnavailable
	}
}

// DeliverFromOwner writes server->client packets only when the IPv4 destination
// is the current lease. Non-IPv4 and cross-lease packets fail closed.
func (r *Router) DeliverFromOwner(packets [][]byte) error {
	if r == nil {
		return ErrInvalidBinding
	}
	for _, packet := range packets {
		_, dst, err := ipv4Endpoints(packet)
		if err != nil {
			return err
		}
		if dst != r.addr {
			return fmt.Errorf("%w: got=%s want=%s", ErrDestinationLeak, dst, r.addr)
		}
		owned := append([]byte(nil), packet...)
		n, err := r.tun.WritePacket(owned)
		if err != nil {
			return err
		}
		if n != len(owned) {
			return fmt.Errorf("%w: wrote=%d want=%d", ErrShortTUNWrite, n, len(owned))
		}
	}
	return nil
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
