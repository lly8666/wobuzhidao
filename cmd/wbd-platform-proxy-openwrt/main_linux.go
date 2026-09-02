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
	tcp4     *net.TCPListener
	tcp6     *net.TCPListener
	wbd      *net.UDPConn
	udpFlows *platformproxy.UDPClientFlowTable
	tcp      *platformproxy.TCPClient
	udpIdle  time.Duration

	replyMu sync.Mutex
	replies map[netip.AddrPort]*replySocket
}

func main() {
	var port int
	var wbdAddress string
	var udpIdle time.Duration
	var tcpIdle time.Duration
	var ipv6 bool
	flag.IntVar(&port, "port", 12345, "TPROXY TCP/UDP capture port (must match scripts/openwrt_tproxy.sh --port)")
	flag.StringVar(&wbdAddress, "wbd", "", "local wbd-link-proxy UDP application address")
	flag.DurationVar(&udpIdle, "udp-idle", 60*time.Second, "idle timeout for one full-cone UDP mapping")
	flag.DurationVar(&tcpIdle, "tcp-idle", 90*time.Second, "idle timeout for one transparent TCP flow")
	flag.BoolVar(&ipv6, "ipv6", true, "also bind IPv6 TPROXY sockets")
	flag.Parse()

	if port <= 0 || port > 65535 || wbdAddress == "" || udpIdle <= 0 || tcpIdle <= 0 {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_OPENWRT_FAIL valid -port, -wbd and positive -udp-idle/-tcp-idle are required")
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
	tcp4, err := platformproxy.ListenTransparentTCP("tcp4", fmt.Sprintf("0.0.0.0:%d", port))
	if err != nil {
		fail(err)
	}
	defer tcp4.Close()

	var capture6 *net.UDPConn
	var tcp6 *net.TCPListener
	if ipv6 {
		capture6, err = platformproxy.ListenTransparentUDP("udp6", fmt.Sprintf("[::]:%d", port))
		if err != nil {
			fail(err)
		}
		defer capture6.Close()
		tcp6, err = platformproxy.ListenTransparentTCP("tcp6", fmt.Sprintf("[::]:%d", port))
		if err != nil {
			fail(err)
		}
		defer tcp6.Close()
	}

	sendFrame := func(frame platformproxy.Frame) error {
		wire, err := platformproxy.Marshal(frame)
		if err != nil {
			return err
		}
		_, err = wbd.Write(wire)
		return err
	}
	tcpCfg := platformproxy.DefaultTCPClientConfig()
	tcpCfg.IdleTimeout = tcpIdle
	tcpClient, err := platformproxy.NewTCPClient(sendFrame, tcpCfg)
	if err != nil {
		fail(err)
	}

	a := &adapter{
		capture4: capture4,
		capture6: capture6,
		tcp4:     tcp4,
		tcp6:     tcp6,
		wbd:      wbd,
		udpFlows: platformproxy.NewUDPClientFlowTable(udpIdle),
		tcp:      tcpClient,
		udpIdle:  udpIdle,
		replies:  make(map[netip.AddrPort]*replySocket),
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	fmt.Printf("WBD_PLATFORM_PROXY_OPENWRT_READY port=%d wbd=%s udp=1 udp_fullcone=1 tcp=1 ipv6=%t transparent=1\n", port, wbdAddr, ipv6)
	if err := a.run(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_OPENWRT_FAIL", err)
	os.Exit(1)
}

func (a *adapter) run(ctx context.Context) error {
	errCh := make(chan error, 6)
	go func() { errCh <- a.captureLoop(a.capture4) }()
	go func() { errCh <- a.tcpAcceptLoop(a.tcp4) }()
	if a.capture6 != nil {
		go func() { errCh <- a.captureLoop(a.capture6) }()
	}
	if a.tcp6 != nil {
		go func() { errCh <- a.tcpAcceptLoop(a.tcp6) }()
	}
	go func() { errCh <- a.reverseLoop() }()

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	defer a.closeReplies()
	defer a.tcp.Close()
	for {
		select {
		case <-ctx.Done():
			a.closeListeners()
			_ = a.wbd.Close()
			return ctx.Err()
		case err := <-errCh:
			return err
		case now := <-ticker.C:
			a.udpFlows.Expire(now)
			a.expireReplies(now)
			a.tcp.Tick(now)
		}
	}
}

func (a *adapter) closeListeners() {
	_ = a.capture4.Close()
	_ = a.tcp4.Close()
	if a.capture6 != nil {
		_ = a.capture6.Close()
	}
	if a.tcp6 != nil {
		_ = a.tcp6.Close()
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
		flow, err := a.udpFlows.Forward(client, peer, time.Now())
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

func (a *adapter) tcpAcceptLoop(listener *net.TCPListener) error {
	for {
		conn, err := listener.AcceptTCP()
		if err != nil {
			return err
		}
		target, err := platformproxy.TCPOriginalDst(conn)
		if err != nil {
			_ = conn.Close()
			continue
		}
		if _, err := a.tcp.Add(conn, target, time.Now()); err != nil {
			_ = conn.Close()
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
		if err != nil {
			continue
		}
		now := time.Now()
		switch frame.Kind {
		case platformproxy.KindUDPDatagram:
			if err := a.handleUDPReverse(frame, now); err != nil {
				continue
			}
		case platformproxy.KindTCPAck, platformproxy.KindTCPData, platformproxy.KindTCPClose:
			if err := a.tcp.HandleFrame(frame, now); err != nil {
				continue
			}
		}
	}
}

func (a *adapter) handleUDPReverse(frame platformproxy.Frame, now time.Time) error {
	flow, err := a.udpFlows.Reverse(frame.FlowID, frame.Peer, now)
	if err != nil {
		return err
	}
	reply, err := a.replySocket(frame.Peer, now)
	if err != nil {
		return err
	}
	if _, err := reply.WriteToUDPAddrPort(frame.Payload, flow.Client); err != nil {
		a.dropReply(frame.Peer)
		return err
	}
	return nil
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
		if now.Before(current.lastSeen) || now.Sub(current.lastSeen) < a.udpIdle {
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
