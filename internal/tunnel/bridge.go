package tunnel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
)

type Stats struct {
	TUNToNetworkPackets uint64 `json:"tun_to_network_packets"`
	TUNToNetworkBytes   uint64 `json:"tun_to_network_bytes"`
	NetworkToTUNPackets uint64 `json:"network_to_tun_packets"`
	NetworkToTUNBytes   uint64 `json:"network_to_tun_bytes"`
	DroppedPackets      uint64 `json:"dropped_packets"`
}

type counters struct {
	tunToNetworkPackets atomic.Uint64
	tunToNetworkBytes   atomic.Uint64
	networkToTUNPackets atomic.Uint64
	networkToTUNBytes   atomic.Uint64
	droppedPackets      atomic.Uint64
}

type Bridge struct {
	TUN       Endpoint
	Transport Endpoint
	MTU       int

	count counters
}

func (b *Bridge) Snapshot() Stats {
	return Stats{
		TUNToNetworkPackets: b.count.tunToNetworkPackets.Load(),
		TUNToNetworkBytes:   b.count.tunToNetworkBytes.Load(),
		NetworkToTUNPackets: b.count.networkToTUNPackets.Load(),
		NetworkToTUNBytes:   b.count.networkToTUNBytes.Load(),
		DroppedPackets:      b.count.droppedPackets.Load(),
	}
}

func (b *Bridge) Run(ctx context.Context) (Stats, error) {
	if b.TUN == nil || b.Transport == nil {
		return b.Snapshot(), errors.New("tunnel bridge endpoints are required")
	}
	if b.MTU < 576 || b.MTU > 9000 {
		return b.Snapshot(), fmt.Errorf("invalid MTU %d", b.MTU)
	}

	results := make(chan error, 2)
	go func() {
		results <- b.pump(b.TUN, b.Transport, true)
	}()
	go func() {
		results <- b.pump(b.Transport, b.TUN, false)
	}()

	var first error
	select {
	case <-ctx.Done():
		first = ctx.Err()
	case first = <-results:
	}

	_ = b.TUN.Close()
	_ = b.Transport.Close()

	select {
	case second := <-results:
		if first == nil {
			first = second
		}
	case <-ctx.Done():
	}

	if ctx.Err() != nil {
		return b.Snapshot(), nil
	}
	if errors.Is(first, io.EOF) {
		return b.Snapshot(), nil
	}
	return b.Snapshot(), first
}

func (b *Bridge) pump(src, dst Endpoint, outbound bool) error {
	buf := make([]byte, b.MTU+1)
	for {
		n, err := src.ReadPacket(buf)
		if err != nil {
			return err
		}
		if n <= 0 {
			b.count.droppedPackets.Add(1)
			continue
		}
		if n > b.MTU {
			b.count.droppedPackets.Add(1)
			continue
		}

		w, err := dst.WritePacket(buf[:n])
		if errors.Is(err, ErrPeerUnknown) {
			b.count.droppedPackets.Add(1)
			continue
		}
		if err != nil {
			return err
		}
		if w != n {
			return fmt.Errorf("%w: packet %d/%d", ErrShortWrite, w, n)
		}

		if outbound {
			b.count.tunToNetworkPackets.Add(1)
			b.count.tunToNetworkBytes.Add(uint64(n))
		} else {
			b.count.networkToTUNPackets.Add(1)
			b.count.networkToTUNBytes.Add(uint64(n))
		}
	}
}
