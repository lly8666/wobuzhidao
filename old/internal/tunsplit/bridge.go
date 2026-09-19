package tunsplit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/tunnel"
)

type Stats struct {
	TUNToNetworkPackets uint64 `json:"tun_to_network_packets"`
	TUNToNetworkBytes   uint64 `json:"tun_to_network_bytes"`
	NetworkToTUNPackets uint64 `json:"network_to_tun_packets"`
	NetworkToTUNBytes   uint64 `json:"network_to_tun_bytes"`
	DroppedPackets      uint64 `json:"dropped_packets"`
	ProxyPackets        uint64 `json:"proxy_packets"`
	DirectPackets       uint64 `json:"direct_packets"`
}

type splitCounters struct {
	tunToNetworkPackets atomic.Uint64
	tunToNetworkBytes   atomic.Uint64
	networkToTUNPackets atomic.Uint64
	networkToTUNBytes   atomic.Uint64
	droppedPackets      atomic.Uint64
	proxyPackets        atomic.Uint64
	directPackets       atomic.Uint64
}

type SerialEndpoint struct {
	Raw tunnel.Endpoint
	mu  sync.Mutex
}

func NewSerialEndpoint(raw tunnel.Endpoint) *SerialEndpoint {
	return &SerialEndpoint{Raw: raw}
}

func (e *SerialEndpoint) ReadPacket(p []byte) (int, error) { return e.Raw.ReadPacket(p) }
func (e *SerialEndpoint) WritePacket(p []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Raw.WritePacket(p)
}
func (e *SerialEndpoint) Close() error { return e.Raw.Close() }

type Bridge struct {
	TUN        tunnel.Endpoint
	Proxy      tunnel.Endpoint
	Classifier *Classifier
	Direct     *DirectEngine
	MTU        int

	count splitCounters
}

func (b *Bridge) Snapshot() Stats {
	return Stats{
		TUNToNetworkPackets: b.count.tunToNetworkPackets.Load(),
		TUNToNetworkBytes:   b.count.tunToNetworkBytes.Load(),
		NetworkToTUNPackets: b.count.networkToTUNPackets.Load(),
		NetworkToTUNBytes:   b.count.networkToTUNBytes.Load(),
		DroppedPackets:      b.count.droppedPackets.Load(),
		ProxyPackets:        b.count.proxyPackets.Load(),
		DirectPackets:       b.count.directPackets.Load(),
	}
}

func (b *Bridge) Run(ctx context.Context) (Stats, error) {
	if b.TUN == nil || b.Proxy == nil || b.Classifier == nil {
		return b.Snapshot(), errors.New("split bridge endpoints and classifier are required")
	}
	if b.MTU < 576 || b.MTU > dataplane.MaxPacketLen {
		return b.Snapshot(), fmt.Errorf("invalid MTU %d", b.MTU)
	}
	results := make(chan error, 2)
	go func() { results <- b.pumpOutbound() }()
	go func() { results <- b.pumpProxyInbound() }()

	var first error
	select {
	case <-ctx.Done():
		first = ctx.Err()
	case first = <-results:
	}
	if b.Direct != nil {
		_ = b.Direct.Close()
	}
	_ = b.TUN.Close()
	_ = b.Proxy.Close()

	select {
	case second := <-results:
		if first == nil {
			first = second
		}
	case <-ctx.Done():
	}
	if ctx.Err() != nil || errors.Is(first, io.EOF) {
		return b.Snapshot(), nil
	}
	return b.Snapshot(), first
}

func (b *Bridge) pumpOutbound() error {
	buf := make([]byte, dataplane.MaxPacketLen)
	for {
		n, err := b.TUN.ReadPacket(buf)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(buf) {
			b.count.droppedPackets.Add(1)
			continue
		}
		packet := buf[:n]
		if b.shouldDirect(packet) {
			if b.Direct == nil {
				return errors.New("packet classified direct without a direct engine")
			}
			if err := b.Direct.Inject(packet); err != nil {
				return err
			}
			b.count.directPackets.Add(1)
		} else {
			w, err := b.Proxy.WritePacket(packet)
			if errors.Is(err, tunnel.ErrPeerUnknown) {
				b.count.droppedPackets.Add(1)
				continue
			}
			if err != nil {
				return err
			}
			if w != n {
				return fmt.Errorf("%w: packet %d/%d", tunnel.ErrShortWrite, w, n)
			}
			b.count.proxyPackets.Add(1)
		}
		b.count.tunToNetworkPackets.Add(1)
		b.count.tunToNetworkBytes.Add(uint64(n))
	}
}

func (b *Bridge) pumpProxyInbound() error {
	buf := make([]byte, dataplane.MaxPacketLen)
	for {
		n, err := b.Proxy.ReadPacket(buf)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(buf) {
			b.count.droppedPackets.Add(1)
			continue
		}
		w, err := b.TUN.WritePacket(buf[:n])
		if err != nil {
			return err
		}
		if w != n {
			return fmt.Errorf("%w: packet %d/%d", tunnel.ErrShortWrite, w, n)
		}
		b.count.networkToTUNPackets.Add(1)
		b.count.networkToTUNBytes.Add(uint64(n))
	}
}

func (b *Bridge) shouldDirect(packet []byte) bool {
	if b.Classifier.ProxyPacket(packet) {
		return false
	}
	// TCP and UDP have a mature userspace direct path. Keep ICMP and any
	// unfamiliar L4 protocol on the existing WBD packet tunnel rather than
	// pretending a raw packet NAT path is safe on Windows.
	if len(packet) < 20 || packet[0]>>4 != 4 {
		return false
	}
	switch packet[9] {
	case 6, 17:
		return true
	default:
		return false
	}
}
