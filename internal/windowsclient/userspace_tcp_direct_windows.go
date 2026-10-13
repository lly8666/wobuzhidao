//go:build windows

// The userspace TCP leg replaces per-CN system routes, not the WBD transport.
// It is intentionally a distinct, physically bound TCP connection: unlike the
// abandoned packet-injection design, it does not pretend to preserve one
// kernel TCP session across the Wintun and physical interfaces.
package windowsclient

import (
    "context"
    "encoding/binary"
    "errors"
    "fmt"
    "io"
    "net"
    "net/netip"
    "strconv"
    "sync"
    "time"

    "github.com/xjasonlyu/tun2socks/v2/core"
    "github.com/xjasonlyu/tun2socks/v2/core/adapter"
    "github.com/xjasonlyu/tun2socks/v2/core/device/iobased"
    "github.com/xjasonlyu/tun2socks/v2/dialer"
)

const (
    n4TCPPacketQueue = 1024
    n4TCPDialTimeout = 10 * time.Second
)

var (
    ErrN4TCPQueueFull = errors.New("windowsclient: bounded userspace TCP input queue full")
    ErrN4TCPPacket = errors.New("windowsclient: invalid or unsupported direct TCP IPv4 packet")
)

// UserspaceTCPDirect consumes selected inner TCP IPv4 packets with a gVisor
// stack and opens a second Windows TCP socket explicitly bound to the selected
// physical interface. No PowerShell, WFP driver, or route mutation occurs.
// Callers must install a qualified owned network plan BEFORE enabling this.
type UserspaceTCPDirect struct {
    iface uint32
    generation uint64
    rw *n4TCPPacketRW
    link *iobased.Endpoint
    destroy func()
    closeOnce sync.Once
}

// NewUserspaceTCPDirect is per-TUN generation; after an underlay/NIC change,
// close and recreate it with a fresh generation and physical interface index.
func NewUserspaceTCPDirect(iface uint32, generation uint64, mtu int, output PacketWriter) (*UserspaceTCPDirect, error) {
    if iface == 0 || generation == 0 || mtu < 576 || mtu > 9000 || output == nil {
        return nil, ErrInvalidBinding
    }
    rw := &n4TCPPacketRW{
        out: output,
        packets: make(chan []byte, n4TCPPacketQueue),
        done: make(chan struct{}),
    }
    link, err := iobased.New(rw, uint32(mtu), 0)
    if err != nil {
        rw.Close()
        return nil, fmt.Errorf("userspace TCP link: %w", err)
    }
    stack, err := core.CreateStack(&core.Config{
        LinkEndpoint: link,
        TransportHandler: &n4TCPTransport{iface: iface},
    })
    if err != nil {
        rw.Close()
        link.Close()
        return nil, fmt.Errorf("userspace TCP stack: %w", err)
    }
    return &UserspaceTCPDirect{
        iface: iface, generation: generation,
        rw: rw, link: link, destroy: stack.Destroy,
    }, nil
}

// ForwardDirectIPv4 never interprets UDP as TCP. Inner IPv4 fragments are
// handed to the gVisor stack; the policy boundary must already have associated
// later fragments with the same generation and Direct decision.
func (d *UserspaceTCPDirect) ForwardDirectIPv4(packet []byte, iface uint32, generation uint64) error {
    if d == nil || d.rw == nil || iface == 0 || iface != d.iface || generation != d.generation {
        return ErrTUNFlowScope
    }
    if len(packet) < 20 || packet[0]>>4 != 4 || packet[9] != 6 {
        return ErrN4TCPPacket
    }
    ihl := int(packet[0]&15)*4
    if ihl < 20 || ihl > len(packet) || int(binary.BigEndian.Uint16(packet[2:4])) != len(packet) {
        return ErrN4TCPPacket
    }
    return d.rw.Inject(packet)
}

func (d *UserspaceTCPDirect) Close() error {
    if d == nil { return nil }
    d.closeOnce.Do(func() {
        d.rw.Close()
        d.link.Close()
        if d.destroy != nil { d.destroy() }
    })
    return nil
}

type n4TCPPacketRW struct {
    out PacketWriter
    packets chan []byte
    done chan struct{}
    once sync.Once
}

// The bounded queue makes enqueue O(1) and prevents a stalled direct TCP
// peer from blocking unrelated protected input behind a channel send.
func (rw *n4TCPPacketRW) Inject(packet []byte) error {
    if rw == nil || len(packet) == 0 { return ErrN4TCPPacket }
    owned := append([]byte(nil), packet...)
    select {
    case <-rw.done:
        return io.EOF
    default:
    }
    select {
    case <-rw.done:
        return io.EOF
    case rw.packets <- owned:
        return nil
    default:
        return ErrN4TCPQueueFull
    }
}
func (rw *n4TCPPacketRW) Read(b []byte) (int, error) {
    if rw == nil { return 0, io.EOF }
    select {
    case <-rw.done:
        return 0, io.EOF
    case packet := <-rw.packets:
        if len(packet) > len(b) { return 0, io.ErrShortBuffer }
        return copy(b, packet), nil
    }
}
func (rw *n4TCPPacketRW) Write(b []byte) (int, error) {
    if rw == nil { return 0, io.EOF }
    select { case <-rw.done: return 0, io.EOF; default: }
    return rw.out.WritePacket(b)
}
func (rw *n4TCPPacketRW) Close() {
    if rw != nil { rw.once.Do(func(){close(rw.done)}) }
}

// The handler is intentionally restricted to TCP; UDP remains owned by the
// independently scoped/qualified NativeUDPDirect backend.
type n4TCPTransport struct {
    iface uint32
    dialer dialer.Dialer
}
func (t *n4TCPTransport) HandleTCP(c adapter.TCPConn) {
    go t.handleTCP(c)
}
func (t *n4TCPTransport) HandleUDP(c adapter.UDPConn) {
    _ = c.Close()
}
func (t *n4TCPTransport) handleTCP(inner adapter.TCPConn) {
    defer inner.Close()
    id := inner.ID()
    targetAddr, valid := netip.AddrFromSlice(id.LocalAddress.AsSlice())
    if !valid || !targetAddr.Unmap().Is4() || t.iface == 0 { return }
    target := net.JoinHostPort(targetAddr.Unmap().String(), strconv.Itoa(int(id.LocalPort)))
    ctx, cancel := context.WithTimeout(context.Background(), n4TCPDialTimeout)
    outer, err := t.dialer.DialContextWithOptions(ctx, "tcp4", target, &dialer.Options{InterfaceIndex: int(t.iface)})
    cancel()
    if err != nil { return }
    defer outer.Close()

    result := make(chan struct{}, 2)
    go func() {
        _, _ = io.Copy(outer, inner)
        if half, ok := outer.(interface{ CloseWrite() error }); ok { _ = half.CloseWrite() }
        result <- struct{}{}
    }()
    go func() {
        _, _ = io.Copy(inner, outer)
        if half, ok := inner.(interface{ CloseWrite() error }); ok { _ = half.CloseWrite() }
        result <- struct{}{}
    }()
    <-result
    _ = inner.Close()
    _ = outer.Close()
    <-result
}

// N4NativeDirect demultiplexes by L4 protocol without mixing the WBD tunnel
// ciphertext with direct traffic. Both owners are created for one generation.
type N4NativeDirect struct {
    tcp *UserspaceTCPDirect
    udp *NativeUDPDirect
}
func NewN4NativeDirect(tcp *UserspaceTCPDirect, udp *NativeUDPDirect) (*N4NativeDirect, error) {
    if tcp == nil || udp == nil { return nil, ErrInvalidBinding }
    return &N4NativeDirect{tcp:tcp,udp:udp},nil
}
func (d *N4NativeDirect) ForwardDirectIPv4(p []byte, iface uint32, generation uint64) error {
    if d == nil || len(p)<20 { return ErrN4TCPPacket }
    switch p[9] {
    case 6: return d.tcp.ForwardDirectIPv4(p,iface,generation)
    case 17: return d.udp.ForwardDirectIPv4(p,iface,generation)
    default: return ErrN4TCPPacket
    }
}
