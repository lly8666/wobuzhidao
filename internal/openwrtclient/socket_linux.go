//go:build linux

package openwrtclient

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/platformflow"
)

type replySocket struct {
	conn     *net.UDPConn
	lastSeen time.Time
}

type SocketAdapter struct {
	cfg    SocketConfig
	udp    *net.UDPConn
	tcp    *net.TCPListener
	client *platformflow.Client

	replyMu sync.Mutex
	replies map[netip.AddrPort]*replySocket

	runMu   sync.Mutex
	running bool
	closeOnce sync.Once
	closeErr  error
}

func OpenSocketAdapter(cfg SocketConfig) (*SocketAdapter, error) {
	if err := cfg.normalize(); err != nil {
		return nil, err
	}
	addr := "0.0.0.0:" + strconv.Itoa(int(cfg.ListenPort))
	udp, err := listenTransparentUDP4(addr, true)
	if err != nil {
		return nil, err
	}
	tcp, err := listenTransparentTCP4(addr)
	if err != nil {
		_ = udp.Close()
		return nil, err
	}
	a := &SocketAdapter{
		cfg: cfg, udp: udp, tcp: tcp,
		replies: make(map[netip.AddrPort]*replySocket),
	}
	client, err := platformflow.NewClient(cfg.Channel, cfg.Client, a.replyUDP)
	if err != nil {
		_ = udp.Close()
		_ = tcp.Close()
		return nil, err
	}
	a.client = client
	return a, nil
}

func (a *SocketAdapter) Run(ctx context.Context) error {
	if a == nil {
		return ErrSocketClosed
	}
	if ctx == nil {
		return platformflow.ErrMalformed
	}
	a.runMu.Lock()
	if a.running {
		a.runMu.Unlock()
		return platformflow.ErrMalformed
	}
	a.running = true
	a.runMu.Unlock()
	defer func() {
		a.runMu.Lock()
		a.running = false
		a.runMu.Unlock()
	}()

	errCh := make(chan error, 2)
	go func() { errCh <- a.udpLoop() }()
	go func() { errCh <- a.tcpLoop() }()
	ticker := time.NewTicker(a.cfg.TickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = a.Close()
			return ctx.Err()
		case err := <-errCh:
			_ = a.Close()
			if errors.Is(err, net.ErrClosed) {
				return ErrSocketClosed
			}
			return err
		case now := <-ticker.C:
			a.client.Tick(now)
			a.expireReplies(now)
		}
	}
}

func (a *SocketAdapter) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		var errs []error
		if a.udp != nil {
			if err := a.udp.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				errs = append(errs, err)
			}
		}
		if a.tcp != nil {
			if err := a.tcp.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				errs = append(errs, err)
			}
		}
		if a.client != nil {
			a.client.Close()
		}
		a.replyMu.Lock()
		replies := make([]*net.UDPConn, 0, len(a.replies))
		for peer, state := range a.replies {
			delete(a.replies, peer)
			replies = append(replies, state.conn)
		}
		a.replyMu.Unlock()
		for _, conn := range replies {
			if err := conn.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
				errs = append(errs, err)
			}
		}
		a.closeErr = errors.Join(errs...)
	})
	return a.closeErr
}

// DeliverFromOwner is the in-process reverse path from the leased TunnelOwner.
// OpenWrt socket mode owns only platform-service packets; ordinary leased IPv4
// is not silently accepted by this adapter.
func (a *SocketAdapter) DeliverFromOwner(packets [][]byte, now time.Time) error {
	if a == nil || a.client == nil {
		return ErrSocketClosed
	}
	for _, packet := range packets {
		handled, err := a.client.HandleServicePacket(packet, now)
		if err != nil {
			return err
		}
		if !handled {
			return platformflow.ErrUnsupported
		}
	}
	return nil
}

func (a *SocketAdapter) udpLoop() error {
	payload := make([]byte, platformflow.MaxPayload+1)
	oob := make([]byte, 256)
	for {
		n, oobn, flags, from, err := a.udp.ReadMsgUDP(payload, oob)
		if err != nil {
			return err
		}
		if flags&syscall.MSG_CTRUNC != 0 || n > platformflow.MaxPayload || from == nil {
			continue
		}
		target, err := udpOriginalDst4(oob[:oobn])
		if err != nil {
			continue
		}
		client := from.AddrPort()
		if !client.Addr().Unmap().Is4() {
			continue
		}
		if a.cfg.BeforeBusiness != nil {
			if err := a.cfg.BeforeBusiness(); err != nil {
				continue
			}
		}
		if err := a.client.ForwardUDP(client, target, append([]byte(nil), payload[:n]...), time.Now()); err != nil {
			continue
		}
	}
}

func (a *SocketAdapter) tcpLoop() error {
	for {
		conn, err := a.tcp.AcceptTCP()
		if err != nil {
			return err
		}
		target, err := tcpOriginalDst4(conn)
		if err != nil {
			_ = conn.Close()
			continue
		}
		if a.cfg.BeforeBusiness != nil {
			if err := a.cfg.BeforeBusiness(); err != nil {
				_ = conn.Close()
				continue
			}
		}
		if _, err := a.client.AddTCP(conn, target, time.Now()); err != nil {
			_ = conn.Close()
		}
	}
}

func (a *SocketAdapter) replyUDP(peer, client netip.AddrPort, payload []byte) error {
	if !peer.Addr().Unmap().Is4() || !client.Addr().Unmap().Is4() {
		return ErrSocketUnsupported
	}
	now := time.Now()
	a.replyMu.Lock()
	state := a.replies[peer]
	if state == nil {
		if len(a.replies) >= a.cfg.MaxReplySockets {
			var oldestPeer netip.AddrPort
			var oldest *replySocket
			for candidate, current := range a.replies {
				if oldest == nil || current.lastSeen.Before(oldest.lastSeen) {
					oldestPeer, oldest = candidate, current
				}
			}
			if oldest != nil {
				delete(a.replies, oldestPeer)
				_ = oldest.conn.Close()
			}
		}
		conn, err := listenTransparentUDPSource4(peer)
		if err != nil {
			a.replyMu.Unlock()
			return err
		}
		state = &replySocket{conn: conn, lastSeen: now}
		a.replies[peer] = state
	} else {
		state.lastSeen = now
	}
	conn := state.conn
	a.replyMu.Unlock()

	if _, err := conn.WriteToUDPAddrPort(payload, client); err != nil {
		a.replyMu.Lock()
		if a.replies[peer] == state {
			delete(a.replies, peer)
		}
		a.replyMu.Unlock()
		_ = conn.Close()
		return err
	}
	return nil
}

func (a *SocketAdapter) expireReplies(now time.Time) {
	var stale []*net.UDPConn
	a.replyMu.Lock()
	for peer, state := range a.replies {
		if now.Before(state.lastSeen) || now.Sub(state.lastSeen) < a.cfg.Client.UDPIdle {
			continue
		}
		delete(a.replies, peer)
		stale = append(stale, state.conn)
	}
	a.replyMu.Unlock()
	for _, conn := range stale {
		_ = conn.Close()
	}
}

func listenTransparentUDP4(address string, originalDst bool) (*net.UDPConn, error) {
	lc := net.ListenConfig{Control: func(network, _ string, raw syscall.RawConn) error {
		var sockErr error
		if err := raw.Control(func(fd uintptr) {
			sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
			if sockErr == nil && originalDst {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1)
			}
		}); err != nil {
			return err
		}
		return sockErr
	}}
	pc, err := lc.ListenPacket(context.Background(), "udp4", address)
	if err != nil {
		return nil, err
	}
	udp, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, fmt.Errorf("openwrtclient: UDP listener type %T", pc)
	}
	return udp, nil
}

func listenTransparentUDPSource4(peer netip.AddrPort) (*net.UDPConn, error) {
	if !peer.IsValid() || peer.Port() == 0 || !peer.Addr().Unmap().Is4() {
		return nil, platformflow.ErrMalformed
	}
	return listenTransparentUDP4(peer.String(), false)
}

func listenTransparentTCP4(address string) (*net.TCPListener, error) {
	lc := net.ListenConfig{Control: func(_ string, _ string, raw syscall.RawConn) error {
		var sockErr error
		if err := raw.Control(func(fd uintptr) {
			sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
		}); err != nil {
			return err
		}
		return sockErr
	}}
	ln, err := lc.Listen(context.Background(), "tcp4", address)
	if err != nil {
		return nil, err
	}
	tcp, ok := ln.(*net.TCPListener)
	if !ok {
		_ = ln.Close()
		return nil, fmt.Errorf("openwrtclient: TCP listener type %T", ln)
	}
	return tcp, nil
}

func tcpOriginalDst4(conn *net.TCPConn) (netip.AddrPort, error) {
	if conn == nil {
		return netip.AddrPort{}, platformflow.ErrMalformed
	}
	local, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok || local == nil {
		return netip.AddrPort{}, platformflow.ErrMalformed
	}
	target := local.AddrPort()
	if !target.IsValid() || target.Port() == 0 || !target.Addr().Unmap().Is4() || target.Addr().IsUnspecified() {
		return netip.AddrPort{}, platformflow.ErrMalformed
	}
	return target, nil
}

func udpOriginalDst4(oob []byte) (netip.AddrPort, error) {
	messages, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return netip.AddrPort{}, err
	}
	for _, message := range messages {
		if message.Header.Level != syscall.SOL_IP || message.Header.Type != syscall.IP_ORIGDSTADDR {
			continue
		}
		if len(message.Data) < 8 {
			return netip.AddrPort{}, platformflow.ErrMalformed
		}
		port := binary.BigEndian.Uint16(message.Data[2:4])
		var raw [4]byte
		copy(raw[:], message.Data[4:8])
		out := netip.AddrPortFrom(netip.AddrFrom4(raw), port)
		if !out.IsValid() || out.Port() == 0 || out.Addr().IsUnspecified() {
			return netip.AddrPort{}, platformflow.ErrMalformed
		}
		return out, nil
	}
	return netip.AddrPort{}, platformflow.ErrMalformed
}
