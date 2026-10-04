//go:build linux

package platformproxy

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"syscall"
)

// ListenTransparentTCP binds the local TCP endpoint used by nft/iptables
// TPROXY. TPROXY preserves the packet destination rather than DNATing it, so an
// accepted connection's LocalAddr is the original destination requested by the
// intercepted client.
func ListenTransparentTCP(network, address string) (*net.TCPListener, error) {
	if network != "tcp4" && network != "tcp6" {
		return nil, fmt.Errorf("platformproxy: unsupported transparent TCP network %q", network)
	}
	lc := net.ListenConfig{Control: func(controlNetwork, _ string, raw syscall.RawConn) error {
		var sockErr error
		if err := raw.Control(func(fd uintptr) {
			if controlNetwork == "tcp4" {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
				return
			}
			sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IPV6, syscall.IPV6_V6ONLY, 1)
			if sockErr == nil {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IPV6, ipv6Transparent, 1)
			}
		}); err != nil {
			return err
		}
		return sockErr
	}}
	ln, err := lc.Listen(context.Background(), network, address)
	if err != nil {
		return nil, err
	}
	tcp, ok := ln.(*net.TCPListener)
	if !ok {
		_ = ln.Close()
		return nil, fmt.Errorf("platformproxy: transparent TCP listener is %T, want *net.TCPListener", ln)
	}
	return tcp, nil
}

func TCPOriginalDst(conn *net.TCPConn) (netip.AddrPort, error) {
	if conn == nil {
		return netip.AddrPort{}, fmt.Errorf("%w: nil transparent TCP connection", ErrMalformed)
	}
	local, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok || local == nil {
		return netip.AddrPort{}, fmt.Errorf("%w: invalid transparent TCP local address", ErrMalformed)
	}
	peer := local.AddrPort()
	if !validTCPRelayEndpoint(peer) {
		return netip.AddrPort{}, fmt.Errorf("%w: invalid original TCP destination", ErrMalformed)
	}
	return peer, nil
}
