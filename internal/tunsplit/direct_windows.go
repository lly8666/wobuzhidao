//go:build windows

package tunsplit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/tunnel"
	"github.com/xjasonlyu/tun2socks/v2/core"
	"github.com/xjasonlyu/tun2socks/v2/core/adapter"
	"github.com/xjasonlyu/tun2socks/v2/core/device/iobased"
	"github.com/xjasonlyu/tun2socks/v2/dialer"
)

const (
	directPacketQueue = 4096
	directDialTimeout = 10 * time.Second
	directUDPTimeout  = 60 * time.Second
)

type DirectEngine struct {
	rw        *directPacketRW
	link      *iobased.Endpoint
	destroy   func()
	closeOnce sync.Once
}

func NewDirectEngine(interfaceIndex uint32, routeStatePath string, mtu int, output tunnel.Endpoint) (*DirectEngine, error) {
	if interfaceIndex == 0 {
		return nil, errors.New("direct split requires a physical interface index")
	}
	if mtu < 576 {
		return nil, fmt.Errorf("direct split MTU %d is invalid", mtu)
	}
	if output == nil {
		return nil, errors.New("direct split output endpoint is required")
	}
	rw := newDirectPacketRW(output)
	link, err := iobased.New(rw, uint32(mtu), 0)
	if err != nil {
		return nil, fmt.Errorf("create userspace direct link endpoint: %w", err)
	}
	handler := &directHandler{fallbackInterfaceIndex: interfaceIndex, routeStatePath: routeStatePath}
	s, err := core.CreateStack(&core.Config{LinkEndpoint: link, TransportHandler: handler})
	if err != nil {
		rw.Close()
		link.Close()
		return nil, fmt.Errorf("create userspace direct IP stack: %w", err)
	}
	return &DirectEngine{rw: rw, link: link, destroy: s.Destroy}, nil
}

func (e *DirectEngine) Inject(packet []byte) error {
	if e == nil || e.rw == nil {
		return errors.New("direct split engine is not initialized")
	}
	return e.rw.Inject(packet)
}

func (e *DirectEngine) Close() error {
	if e == nil {
		return nil
	}
	e.closeOnce.Do(func() {
		e.rw.Close()
		e.link.Close()
		if e.destroy != nil {
			e.destroy()
		}
	})
	return nil
}

type directPacketRW struct {
	out       tunnel.Endpoint
	in        chan []byte
	closed    chan struct{}
	closeOnce sync.Once
}

func newDirectPacketRW(out tunnel.Endpoint) *directPacketRW {
	return &directPacketRW{
		out:    out,
		in:     make(chan []byte, directPacketQueue),
		closed: make(chan struct{}),
	}
}

func (rw *directPacketRW) Inject(packet []byte) error {
	copyPacket := append([]byte(nil), packet...)
	select {
	case <-rw.closed:
		return io.EOF
	case rw.in <- copyPacket:
		return nil
	}
}

func (rw *directPacketRW) Read(p []byte) (int, error) {
	select {
	case <-rw.closed:
		return 0, io.EOF
	case packet := <-rw.in:
		if len(packet) > len(p) {
			return 0, io.ErrShortBuffer
		}
		copy(p, packet)
		return len(packet), nil
	}
}

func (rw *directPacketRW) Write(p []byte) (int, error) {
	select {
	case <-rw.closed:
		return 0, io.EOF
	default:
	}
	return rw.out.WritePacket(p)
}

func (rw *directPacketRW) Close() {
	rw.closeOnce.Do(func() { close(rw.closed) })
}

type directHandler struct {
	fallbackInterfaceIndex uint32
	routeStatePath         string
	d                      dialer.Dialer
}

type directRouteState struct {
	UnderlayRoutes []struct {
		InterfaceIndex uint32 `json:"InterfaceIndex"`
	} `json:"UnderlayRoutes"`
}

func (h *directHandler) interfaceIndex() int {
	if h.routeStatePath != "" {
		if raw, err := os.ReadFile(h.routeStatePath); err == nil {
			var state directRouteState
			if json.Unmarshal(raw, &state) == nil {
				for _, route := range state.UnderlayRoutes {
					if route.InterfaceIndex != 0 {
						return int(route.InterfaceIndex)
					}
				}
			}
		}
	}
	return int(h.fallbackInterfaceIndex)
}

func (h *directHandler) HandleTCP(origin adapter.TCPConn) {
	go h.handleTCP(origin)
}

func (h *directHandler) HandleUDP(origin adapter.UDPConn) {
	go h.handleUDP(origin)
}

func (h *directHandler) handleTCP(origin adapter.TCPConn) {
	defer origin.Close()
	id := origin.ID()
	dst, ok := netip.AddrFromSlice(id.LocalAddress.AsSlice())
	if !ok || !dst.Unmap().Is4() {
		return
	}
	target := net.JoinHostPort(dst.Unmap().String(), strconv.Itoa(int(id.LocalPort)))
	ifIndex := h.interfaceIndex()
	if ifIndex == 0 {
		fmt.Fprintf(os.Stderr, "WBD_TUN_DIRECT_TCP_DIAL_FAIL dst=%s error=%q\n", target, "physical interface unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), directDialTimeout)
	defer cancel()
	remote, err := h.d.DialContextWithOptions(ctx, "tcp4", target, &dialer.Options{InterfaceIndex: ifIndex})
	if err != nil {
		fmt.Fprintf(os.Stderr, "WBD_TUN_DIRECT_TCP_DIAL_FAIL dst=%s ifindex=%d error=%q\n", target, ifIndex, err)
		return
	}
	defer remote.Close()

	done := make(chan struct{}, 2)
	go func() {
		_, _ = io.Copy(remote, origin)
		if c, ok := remote.(interface{ CloseWrite() error }); ok {
			_ = c.CloseWrite()
		}
		done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(origin, remote)
		if c, ok := origin.(interface{ CloseWrite() error }); ok {
			_ = c.CloseWrite()
		}
		done <- struct{}{}
	}()
	<-done
	_ = origin.Close()
	_ = remote.Close()
	<-done
}

func (h *directHandler) handleUDP(origin adapter.UDPConn) {
	defer origin.Close()
	id := origin.ID()
	dst, ok := netip.AddrFromSlice(id.LocalAddress.AsSlice())
	if !ok || !dst.Unmap().Is4() {
		return
	}
	remote := &net.UDPAddr{IP: net.IP(dst.Unmap().AsSlice()), Port: int(id.LocalPort)}
	ifIndex := h.interfaceIndex()
	if ifIndex == 0 {
		fmt.Fprintf(os.Stderr, "WBD_TUN_DIRECT_UDP_OPEN_FAIL dst=%s error=%q\n", remote, "physical interface unavailable")
		return
	}
	pc, err := h.d.ListenPacketWithOptions("udp4", "0.0.0.0:0", &dialer.Options{InterfaceIndex: ifIndex})
	if err != nil {
		fmt.Fprintf(os.Stderr, "WBD_TUN_DIRECT_UDP_OPEN_FAIL dst=%s ifindex=%d error=%q\n", remote, ifIndex, err)
		return
	}
	defer pc.Close()

	done := make(chan struct{}, 2)
	go func() {
		buf := make([]byte, 65535)
		for {
			_ = origin.SetReadDeadline(time.Now().Add(directUDPTimeout))
			n, _, err := origin.ReadFrom(buf)
			if err != nil {
				break
			}
			if _, err := pc.WriteTo(buf[:n], remote); err != nil {
				break
			}
		}
		done <- struct{}{}
	}()
	go func() {
		buf := make([]byte, 65535)
		for {
			_ = pc.SetReadDeadline(time.Now().Add(directUDPTimeout))
			n, from, err := pc.ReadFrom(buf)
			if err != nil {
				break
			}
			if from == nil || from.String() != remote.String() {
				continue
			}
			if _, err := origin.WriteTo(buf[:n], nil); err != nil {
				break
			}
		}
		done <- struct{}{}
	}()
	<-done
	_ = origin.Close()
	_ = pc.Close()
	<-done
}
