//go:build linux

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/platformproxy"
)

type replySocket struct {
	conn     *net.UDPConn
	lastSeen time.Time
}

type adapter struct {
	capture4 *net.UDPConn
	capture6 *net.UDPConn
	wbd      *net.UDPConn
	flows    *platformproxy.UDPClientFlowTable
	idle     time.Duration

	replyMu sync.Mutex
	replies map[netip.AddrPort]*replySocket
}

func main() {
	var port int
	var wbdAddress string
	var idle time.Duration
	var ipv6 bool
	flag.IntVar(&port, "port", 12345, "TPROXY UDP capture port (must match scripts/openwrt_tproxy.sh --port)")
	flag.StringVar(&wbdAddress, "wbd", "", "local wbd-link-proxy UDP application address")
	flag.DurationVar(&idle, "udp-idle", 60*time.Second, "idle timeout for intercepted UDP flow state")
	flag.BoolVar(&ipv6, "ipv6", true, "also bind the IPv6 TPROXY UDP socket")
	flag.Parse()

	if port <= 0 || port > 65535 || wbdAddress == "" || idle <= 0 {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_OPENWRT_FAIL valid -port, -wbd and positive -udp-idle are required")
		os.Exit(2)
	}
	wbdAddr, err := net.ResolveUDPAddr("udp4", wbdAddress)
	if err != nil {
		fail(err)
	}
	wbd, err := net.DialUDP("udp4", nil, wbdAddr)
	if err != nil {
		fail(err)
	}
	defer wbd.Close()

	capture4, err := platformproxy.ListenTransparentUDP("udp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		fail(err)
	}
	defer capture4.Close()
	var capture6 *net.UDPConn
	if ipv6 {
		capture6, err = platformproxy.ListenTransparentUDP("udp6", fmt.Sprintf("[::]:%d", port))
		if err != nil {
			fail(err)
		}
		defer capture6.Close()
	}

	a := &adapter{
		capture4: capture4,
		capture6: capture6,
		wbd:      wbd,
		flows:    platformproxy.NewUDPClientFlowTable(idle),
		idle:     idle,
		replies:  make(map[netip.AddrPort]*replySocket),
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	fmt.Printf("WBD_PLATFORM_PROXY_OPENWRT_READY port=%d wbd=%s udp=1 tcp=0 ipv6=%t transparent=1\n", port, wbdAddr, ipv6)
	if err := a.run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_OPENWRT_FAIL", err)
	os.Exit(1)
}

func (a *adapter) run(ctx context.Context) error {
	errCh := make(chan error, 3)
	go func() { errCh <- a.captureLoop(a.capture4) }()
	if a.capture6 != nil {
		go func() { errCh <- a.captureLoop(a.capture6) }()
	}
	go func() { errCh <- a.reverseLoop() }()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer a.closeReplies()
	for {
		select {
		case <-ctx.Done():
			_ = a.capture4.Close()
			if a.capture6 != nil {
				_ = a.capture6.Close()
			}
			_ = a.wbd.Close()
			return ctx.Err()
		case err := <-errCh:
			return err
		case now := <-ticker.C:
			a.flows.Expire(now)
			a.expireReplies(now)
		}
	}
}

func (a *adapter) captureLoop(conn *net.UDPConn) error {
	payload := make([]byte, 65535)
	oob := make([]byte, 256)
	for {
		n, oobn, flags, from, err := conn.ReadMsgUDP(payload, oob)
		if err != nil {
			return err
		}
		if flags&syscall.MSG_CTRUNC != 0 || n > platformproxy.MaxPayload {
			continue
		}
		peer, err := platformproxy.UDPOriginalDst(oob[:oobn])
		if err != nil {
			continue
		}
		client := from.AddrPort()
		flow, err := a.flows.Forward(client, peer, time.Now())
		if err != nil {
			continue
		}
		frame, err := platformproxy.Marshal(platformproxy.Frame{
			Kind: platformproxy.KindUDPDatagram, FlowID: flow.FlowID,
			Peer: peer, Payload: payload[:n],
		})
		if err != nil {
			continue
		}
		if _, err := a.wbd.Write(frame); err != nil {
			return err
		}
	}
}

func (a *adapter) reverseLoop() error {
	buf := make([]byte, 65535)
	for {
		n, err := a.wbd.Read(buf)
		if err != nil {
			return err
		}
		frame, err := platformproxy.Unmarshal(buf[:n])
		if err != nil || frame.Kind != platformproxy.KindUDPDatagram {
			continue
		}
		now := time.Now()
		flow, err := a.flows.Reverse(frame.FlowID, frame.Peer, now)
		if err != nil {
			continue
		}
		reply, err := a.replySocket(frame.Peer, now)
		if err != nil {
			continue
		}
		if _, err := reply.WriteToUDPAddrPort(frame.Payload, flow.Client); err != nil {
			a.dropReply(frame.Peer)
		}
	}
}

func (a *adapter) replySocket(peer netip.AddrPort, now time.Time) (*net.UDPConn, error) {
	a.replyMu.Lock()
	defer a.replyMu.Unlock()
	if current := a.replies[peer]; current != nil {
		current.lastSeen = now
		return current.conn, nil
	}
	conn, err := platformproxy.ListenTransparentUDPSource(peer)
	if err != nil {
		return nil, err
	}
	a.replies[peer] = &replySocket{conn: conn, lastSeen: now}
	return conn, nil
}

func (a *adapter) dropReply(peer netip.AddrPort) {
	a.replyMu.Lock()
	current := a.replies[peer]
	delete(a.replies, peer)
	a.replyMu.Unlock()
	if current != nil {
		_ = current.conn.Close()
	}
}

func (a *adapter) expireReplies(now time.Time) {
	var stale []*net.UDPConn
	a.replyMu.Lock()
	for peer, current := range a.replies {
		if now.Before(current.lastSeen) || now.Sub(current.lastSeen) < a.idle {
			continue
		}
		delete(a.replies, peer)
		stale = append(stale, current.conn)
	}
	a.replyMu.Unlock()
	for _, conn := range stale {
		_ = conn.Close()
	}
}

func (a *adapter) closeReplies() {
	a.replyMu.Lock()
	conns := make([]*net.UDPConn, 0, len(a.replies))
	for peer, current := range a.replies {
		delete(a.replies, peer)
		conns = append(conns, current.conn)
	}
	a.replyMu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}
