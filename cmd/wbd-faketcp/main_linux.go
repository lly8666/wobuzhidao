//go:build linux

package main

import (
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"
)

type linuxRawPacketIO struct {
	fd int
}

func openRawPacketIO(_ config, srcIP [4]byte) (rawPacketIO, error) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_TCP)
	if err != nil {
		return nil, err
	}
	r := &linuxRawPacketIO{fd: fd}
	if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {
		_ = r.Close()
		return nil, err
	}
	if err := syscall.Bind(fd, &syscall.SockaddrInet4{Addr: srcIP}); err != nil {
		_ = r.Close()
		return nil, err
	}
	return r, nil
}

func (r *linuxRawPacketIO) ReadPacket(buf []byte) (int, error) {
	n, _, err := syscall.Recvfrom(r.fd, buf, 0)
	if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
		return 0, errRawTimeout
	}
	return n, err
}

func (r *linuxRawPacketIO) WritePacket(packet []byte, dst [4]byte) error {
	return syscall.Sendto(r.fd, packet, 0, &syscall.SockaddrInet4{Addr: dst})
}

func (r *linuxRawPacketIO) SetReadTimeout(d time.Duration) error {
	tv := syscall.NsecToTimeval(d.Nanoseconds())
	return syscall.SetsockoptTimeval(r.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &tv)
}

func (r *linuxRawPacketIO) ClearReadTimeout() error {
	return syscall.SetsockoptTimeval(r.fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{})
}

func (r *linuxRawPacketIO) Close() error {
	if r.fd < 0 {
		return nil
	}
	err := syscall.Close(r.fd)
	r.fd = -1
	return err
}

func notifySignals(ch chan<- os.Signal) {
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
}
