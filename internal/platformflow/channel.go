package platformflow

import (
	"errors"
	"net/netip"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
)

type Outbound struct {
	Normal []datapath.WireRecord
	Game   datapath.GameOutboundResult
	IsGame bool
}

type WireSink func(Outbound) error

type TunnelChannel struct {
	owner     *datapath.TunnelOwner
	leaseAddr [4]byte
	sink      WireSink
}

func NewTunnelChannel(owner *datapath.TunnelOwner, sink WireSink) (*TunnelChannel, error) {
	if owner == nil || sink == nil {
		return nil, ErrMalformed
	}
	lease, ok := owner.Lease()
	if !ok {
		return nil, ErrMalformed
	}
	addr, err := lease.Config.LeaseIPv4()
	if err != nil {
		return nil, err
	}
	return &TunnelChannel{owner: owner, leaseAddr: addr.Unmap().As4(), sink: sink}, nil
}

func (c *TunnelChannel) LeaseAddr() netip.Addr {
	if c == nil {
		return netip.Addr{}
	}
	return netip.AddrFrom4(c.leaseAddr)
}

func (c *TunnelChannel) Decode(packet []byte) (Frame, bool, error) {
	if c == nil {
		return Frame{}, false, ErrClosed
	}
	return ParsePacket(packet, c.LeaseAddr())
}

func (c *TunnelChannel) OpenFlow() (*TunnelFlow, error) {
	if c == nil || c.owner == nil {
		return nil, ErrClosed
	}
	business, err := c.owner.OpenFlow()
	if err != nil {
		return nil, err
	}
	return &TunnelFlow{channel: c, business: business}, nil
}

type TunnelFlow struct {
	mu       sync.Mutex
	channel  *TunnelChannel
	business *datapath.BusinessFlow
	closed   bool
}

func (f *TunnelFlow) Send(frame Frame, now time.Time) error {
	if f == nil {
		return ErrClosed
	}
	f.mu.Lock()
	if f.closed || f.channel == nil || f.business == nil {
		f.mu.Unlock()
		return ErrClosed
	}
	channel := f.channel
	business := f.business
	f.mu.Unlock()

	packet, err := MarshalPacket(channel.LeaseAddr(), frame)
	if err != nil {
		return err
	}
	stats := channel.owner.Stats()
	switch stats.DesiredLanes {
	case 1:
		records, err := business.Outbound(packet, now)
		if err != nil {
			return err
		}
		if len(records) == 0 {
			return ErrNoWireOutput
		}
		return channel.sink(Outbound{Normal: records})
	case 2, 3, 4:
		out, err := channel.owner.GameOutbound(packet, now)
		if err != nil {
			return err
		}
		if len(out.Lanes) == 0 {
			if len(out.Failures) == 0 {
				return ErrNoWireOutput
			}
			errs := make([]error, 0, len(out.Failures)+1)
			errs = append(errs, ErrNoWireOutput)
			for _, failure := range out.Failures {
				errs = append(errs, failure.Err)
			}
			return errors.Join(errs...)
		}
		return channel.sink(Outbound{Game: out, IsGame: true})
	default:
		return datapath.ErrLaneUnavailable
	}
}

func (f *TunnelFlow) Close() error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return nil
	}
	f.closed = true
	business := f.business
	f.business = nil
	f.mu.Unlock()
	if business == nil {
		return nil
	}
	err := business.Close()
	if errors.Is(err, datapath.ErrTunnelOwnerClosed) || errors.Is(err, datapath.ErrFlowClosed) {
		return nil
	}
	return err
}
