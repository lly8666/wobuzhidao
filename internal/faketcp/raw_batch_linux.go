//go:build linux

package faketcp

import (
	"errors"
	"runtime"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

const rawBatchSize = 8

// Native mmsghdr layout follows the architecture-specific unix.Msghdr.
// Go's struct tail alignment supplies the native padding after the uint32.
type rawMessage struct {
	Header unix.Msghdr
	Length uint32
}

type rawReceiveBatch struct {
	frames [rawBatchSize][]byte
	iov [rawBatchSize]unix.Iovec
	from [rawBatchSize]unix.RawSockaddrLinklayer
	msg [rawBatchSize]rawMessage
	next, count int
}

type RawIODiagnostic struct {
	Enabled bool `json:"enabled"`
	ReceiveCalls uint64 `json:"receive_calls"`
	ReceiveMessages uint64 `json:"receive_messages"`
	ReceiveMulti uint64 `json:"receive_multi"`
	ReceiveFallbacks uint64 `json:"receive_fallbacks"`
}

type rawIOCounters struct {
	enabled atomic.Bool
	rxCalls atomic.Uint64
	rxMessages atomic.Uint64
	rxMulti atomic.Uint64
	rxFallbacks atomic.Uint64
}

func (e *RawIPv4Endpoint) SetIODiagnostics(enabled bool) { e.ioStats.enabled.Store(enabled) }

func (e *RawIPv4Endpoint) IODiagnostic() RawIODiagnostic {
	return RawIODiagnostic{Enabled: e.ioStats.enabled.Load(), ReceiveCalls: e.ioStats.rxCalls.Load(),
		ReceiveMessages: e.ioStats.rxMessages.Load(), ReceiveMulti: e.ioStats.rxMulti.Load(),
		ReceiveFallbacks: e.ioStats.rxFallbacks.Load()}
}

// readRawFrame is called only under recvMu. WAITFORONE blocks for the first
// packet, then consumes only packets already available, never waiting to fill
// eight slots. Each ReadSegment still returns an independent owned packet.
func (e *RawIPv4Endpoint) readRawFrame() ([]byte, bool, error) {
	if !e.recvBatchDisabled {
		if e.recvBatch == nil {
			b := new(rawReceiveBatch)
			b.frames[0] = e.recvBuf
			for i := 1; i < rawBatchSize; i++ { b.frames[i] = make([]byte, len(e.recvBuf)) }
			e.recvBatch = b
		}
		b := e.recvBatch
		if b.next == b.count {
			for i := range b.msg {
				b.from[i] = unix.RawSockaddrLinklayer{}
				b.iov[i].Base = &b.frames[i][0]
				b.iov[i].SetLen(len(b.frames[i]))
				b.msg[i] = rawMessage{Header: unix.Msghdr{
					Name: (*byte)(unsafe.Pointer(&b.from[i])), Namelen: unix.SizeofSockaddrLinklayer,
					Iov: &b.iov[i], Iovlen: 1,
				}}
			}
			n, _, errno := unix.Syscall6(unix.SYS_RECVMMSG, uintptr(e.recvFD),
				uintptr(unsafe.Pointer(&b.msg[0])), rawBatchSize, unix.MSG_WAITFORONE, 0, 0)
			runtime.KeepAlive(b)
			if e.ioStats.enabled.Load() { e.ioStats.rxCalls.Add(1) }
			if errno != 0 {
				if errors.Is(errno, unix.ENOSYS) || errors.Is(errno, unix.EINVAL) {
					e.recvBatchDisabled = true
					if e.ioStats.enabled.Load() { e.ioStats.rxFallbacks.Add(1) }
					return e.readRawFrame()
				}
				return nil, false, errno
			}
			if n == 0 { return nil, false, syscall.EAGAIN }
			b.next, b.count = 0, int(n)
			if e.ioStats.enabled.Load() {
				e.ioStats.rxMessages.Add(uint64(n))
				if n > 1 { e.ioStats.rxMulti.Add(1) }
			}
		}
		i := b.next
		b.next++
		if b.msg[i].Header.Flags&unix.MSG_TRUNC != 0 || int(b.msg[i].Length) > len(b.frames[i]) {
			return nil, false, nil // discard a truncated frame; never parse a prefix
		}
		outgoing := b.from[i].Family == unix.AF_PACKET && b.from[i].Pkttype == rawPacketOutgoing
		return b.frames[i][:int(b.msg[i].Length)], outgoing, nil
	}
	n, from, err := syscall.Recvfrom(e.recvFD, e.recvBuf, 0)
	if e.ioStats.enabled.Load() {
		e.ioStats.rxCalls.Add(1)
		if err == nil { e.ioStats.rxMessages.Add(1) }
	}
	if err != nil { return nil, false, err }
	ll, ok := from.(*syscall.SockaddrLinklayer)
	return e.recvBuf[:n], ok && ll.Pkttype == rawPacketOutgoing, nil
}
