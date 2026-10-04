//go:build linux

package platformproxy

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/netip"
	"syscall"
)

const (
	ipv6OrigDstAddr     = 0x4a
	ipv6RecvOrigDstAddr = 0x4a
	ipv6Transparent     = 0x4b
)

func ListenTransparentUDP(network, address string) (*net.UDPConn, error) {
	return listenTransparentUDP(network, address, true)
}

func ListenTransparentUDPSource(peer netip.AddrPort) (*net.UDPConn, error) {
	if !validUDPFlowEndpoint(peer) {
		return nil, fmt.Errorf("%w: invalid transparent UDP source", ErrMalformed)
	}
	network := "udp6"
	if peer.Addr().Unmap().Is4() {
		network = "udp4"
	}
	return listenTransparentUDP(network, peer.String(), false)
}

func listenTransparentUDP(network, address string, recvOrigDst bool) (*net.UDPConn, error) {
	if network != "udp4" && network != "udp6" {
		return nil, fmt.Errorf("platformproxy: unsupported transparent UDP network %q", network)
	}
	lc := net.ListenConfig{Control: func(controlNetwork, _ string, raw syscall.RawConn) error {
		var sockErr error
		if err := raw.Control(func(fd uintptr) {
			if controlNetwork == "udp4" {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
				if sockErr == nil && recvOrigDst {
					sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1)
				}
				return
			}
			sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IPV6, syscall.IPV6_V6ONLY, 1)
			if sockErr == nil {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IPV6, ipv6Transparent, 1)
			}
			if sockErr == nil && recvOrigDst {
				sockErr = syscall.SetsockoptInt(int(fd), syscall.SOL_IPV6, ipv6RecvOrigDstAddr, 1)
			}
		}); err != nil {
			return err
		}
		return sockErr
	}}
	pc, err := lc.ListenPacket(context.Background(), network, address)
	if err != nil {
		return nil, err
	}
	udp, ok := pc.(*net.UDPConn)
	if !ok {
		_ = pc.Close()
		return nil, fmt.Errorf("platformproxy: transparent listener is %T, want *net.UDPConn", pc)
	}
	return udp, nil
}

func UDPOriginalDst(oob []byte) (netip.AddrPort, error) {
	messages, err := syscall.ParseSocketControlMessage(oob)
	if err != nil {
		return netip.AddrPort{}, err
	}
	for _, message := range messages {
		switch {
		case message.Header.Level == syscall.SOL_IP && message.Header.Type == syscall.IP_ORIGDSTADDR:
			return decodeOrigDst4(message.Data)
		case message.Header.Level == syscall.SOL_IPV6 && message.Header.Type == ipv6OrigDstAddr:
			return decodeOrigDst6(message.Data)
		}
	}
	return netip.AddrPort{}, fmt.Errorf("%w: original UDP destination ancillary data missing", ErrMalformed)
}

func decodeOrigDst4(raw []byte) (netip.AddrPort, error) {
	if len(raw) < 8 {
		return netip.AddrPort{}, fmt.Errorf("%w: short IPv4 original destination", ErrMalformed)
	}
	port := binary.BigEndian.Uint16(raw[2:4])
	var addr [4]byte
	copy(addr[:], raw[4:8])
	out := netip.AddrPortFrom(netip.AddrFrom4(addr), port)
	if !validUDPFlowEndpoint(out) {
		return netip.AddrPort{}, fmt.Errorf("%w: invalid IPv4 original destination", ErrMalformed)
	}
	return out, nil
}

func decodeOrigDst6(raw []byte) (netip.AddrPort, error) {
	if len(raw) < 24 {
		return netip.AddrPort{}, fmt.Errorf("%w: short IPv6 original destination", ErrMalformed)
	}
	port := binary.BigEndian.Uint16(raw[2:4])
	var addr [16]byte
	copy(addr[:], raw[8:24])
	out := netip.AddrPortFrom(netip.AddrFrom16(addr), port)
	if !validUDPFlowEndpoint(out) {
		return netip.AddrPort{}, fmt.Errorf("%w: invalid IPv6 original destination", ErrMalformed)
	}
	return out, nil
}
