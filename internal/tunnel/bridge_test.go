package tunnel

import (
	"context"
	"encoding/binary"
	"io"
	"sync"
	"testing"
	"time"
)

type packetEndpoint struct {
	in     chan []byte
	out    chan []byte
	closed chan struct{}
	once   sync.Once
}

func newPacketEndpoint() *packetEndpoint {
	return &packetEndpoint{
		in:     make(chan []byte, 8),
		out:    make(chan []byte, 8),
		closed: make(chan struct{}),
	}
}

func (e *packetEndpoint) ReadPacket(p []byte) (int, error) {
	select {
	case b := <-e.in:
		copy(p, b)
		return len(b), nil
	case <-e.closed:
		return 0, io.EOF
	}
}

func (e *packetEndpoint) WritePacket(p []byte) (int, error) {
	b := append([]byte(nil), p...)
	select {
	case e.out <- b:
		return len(p), nil
	case <-e.closed:
		return 0, io.EOF
	}
}

func (e *packetEndpoint) Close() error {
	e.once.Do(func() { close(e.closed) })
	return nil
}

func v4(payload string) []byte {
	p := make([]byte, 20+len(payload))
	p[0] = 0x45
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)))
	p[8] = 64
	p[9] = 17
	copy(p[20:], payload)
	return p
}

func TestFramedEndpointRoundTrip(t *testing.T) {
	raw := newPacketEndpoint()
	f := &FramedEndpoint{Raw: raw}
	packet := v4("abc")

	go func() {
		wire := <-raw.out
		raw.in <- wire
	}()

	if _, err := f.WritePacket(packet); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 1500)
	n, err := f.ReadPacket(buf)
	if err != nil {
		t.Fatal(err)
	}
	if string(buf[:n]) != string(packet) {
		t.Fatal("packet mismatch")
	}
}

func TestBridgeBidirectionalAndStats(t *testing.T) {
	tun := newPacketEndpoint()
	netep := newPacketEndpoint()
	b := &Bridge{TUN: tun, Transport: netep, MTU: 1400}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Stats, 1)
	go func() {
		stats, _ := b.Run(ctx)
		done <- stats
	}()

	a := v4("out")
	tun.in <- a
	select {
	case got := <-netep.out:
		if string(got) != string(a) {
			t.Fatal("outbound mismatch")
		}
	case <-time.After(time.Second):
		t.Fatal("outbound timeout")
	}

	z := v4("in")
	netep.in <- z
	select {
	case got := <-tun.out:
		if string(got) != string(z) {
			t.Fatal("inbound mismatch")
		}
	case <-time.After(time.Second):
		t.Fatal("inbound timeout")
	}

	cancel()
	stats := <-done
	if stats.TUNToNetworkPackets != 1 || stats.NetworkToTUNPackets != 1 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
}

func TestBridgeDropsOversize(t *testing.T) {
	tun := newPacketEndpoint()
	netep := newPacketEndpoint()
	b := &Bridge{TUN: tun, Transport: netep, MTU: 576}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan Stats, 1)
	go func() {
		stats, _ := b.Run(ctx)
		done <- stats
	}()

	tun.in <- make([]byte, 577)
	time.Sleep(20 * time.Millisecond)
	cancel()
	stats := <-done
	if stats.DroppedPackets == 0 {
		t.Fatalf("expected oversize drop: %+v", stats)
	}
}
