//go:build linux

package faketcp

import (
	"errors"
	"io"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
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
	frames      [rawBatchSize][]byte
	iov         [rawBatchSize]unix.Iovec
	from        [rawBatchSize]unix.RawSockaddrLinklayer
	msg         [rawBatchSize]rawMessage
	next, count int
}

type RawIODiagnostic struct {
	Enabled          bool                     `json:"enabled"`
	ReceivePort      uint16                   `json:"receive_port"`
	KernelPortFilter bool                     `json:"kernel_port_filter"`
	ReceiveBuffer    RawReceiveBufferStatus   `json:"receive_buffer"`
	ReceiveCalls     uint64                   `json:"receive_calls"`
	ReceiveMessages  uint64                   `json:"receive_messages"`
	ReceiveMulti     uint64                   `json:"receive_multi"`
	ReceiveFallbacks uint64                   `json:"receive_fallbacks"`
	SendCalls        uint64                   `json:"send_calls"`
	SendMessages     uint64                   `json:"send_messages"`
	SendMulti        uint64                   `json:"send_multi"`
	SendFallbacks    uint64                   `json:"send_fallbacks"`
	WriteTiming      RawWriteTimingDiagnostic `json:"write_timing"`
}

type rawIOCounters struct {
	enabled     atomic.Bool
	rxCalls     atomic.Uint64
	rxMessages  atomic.Uint64
	rxMulti     atomic.Uint64
	rxFallbacks atomic.Uint64
	txCalls     atomic.Uint64
	txMessages  atomic.Uint64
	txMulti     atomic.Uint64
	txFallbacks atomic.Uint64
}

func (e *RawIPv4Endpoint) SetIODiagnostics(enabled bool) { e.ioStats.enabled.Store(enabled) }

func (e *RawIPv4Endpoint) IODiagnostic() RawIODiagnostic {
	return RawIODiagnostic{Enabled: e.ioStats.enabled.Load(), ReceivePort: e.receivePort, KernelPortFilter: e.kernelPortFilter, ReceiveBuffer: e.receiveBuffer, ReceiveCalls: e.ioStats.rxCalls.Load(),
		ReceiveMessages: e.ioStats.rxMessages.Load(), ReceiveMulti: e.ioStats.rxMulti.Load(),
		ReceiveFallbacks: e.ioStats.rxFallbacks.Load(), SendCalls: e.ioStats.txCalls.Load(),
		SendMessages: e.ioStats.txMessages.Load(), SendMulti: e.ioStats.txMulti.Load(), SendFallbacks: e.ioStats.txFallbacks.Load(), WriteTiming: e.writeTiming.snapshot()}
}

// WriteSegments injects already-generated packets synchronously in <=8-packet
// chunks. sendmmsg partial success is an exact prefix: retry only the remainder.
// ENOSYS switches to Sendto; an actually-sent packet is never sent twice here.
func (e *RawIPv4Endpoint) WriteSegments(segments []Segment) (int, error) {
	if e == nil {
		return 0, errors.New("faketcp: nil raw IPv4 endpoint")
	}
	total := 0
	for total < len(segments) {
		end := total + rawBatchSize
		if end > len(segments) {
			end = len(segments)
		}
		n, err := e.writeReadySegments(segments[total:end])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}

func (e *RawIPv4Endpoint) writeReadySegments(segments []Segment) (int, error) {
	observeWrite := e.writeTiming.enabled.Load()
	var waitStarted, holdStarted, marshalStarted time.Time
	if observeWrite {
		waitStarted = time.Now()
	}
	e.mu.Lock()
	if observeWrite {
		holdStarted = time.Now()
		e.writeTiming.lockWait.observe(holdStarted.Sub(waitStarted))
		marshalStarted = time.Now()
	}
	defer func() {
		if observeWrite {
			e.writeTiming.lockHold.observe(time.Since(holdStarted))
		}
		e.mu.Unlock()
	}()
	var packets [rawBatchSize][]byte
	var iov [rawBatchSize]unix.Iovec
	var from [rawBatchSize]unix.RawSockaddrInet4
	var msg [rawBatchSize]rawMessage
	for i, seg := range segments {
		packets[i] = MarshalSegment(seg, e.ipID, e.persona)
		e.ipID++
		iov[i].Base = &packets[i][0]
		iov[i].SetLen(len(packets[i]))
		from[i] = unix.RawSockaddrInet4{Family: unix.AF_INET, Port: rawHTONS(seg.DstPort), Addr: seg.DstIP}
		msg[i] = rawMessage{Header: unix.Msghdr{Name: (*byte)(unsafe.Pointer(&from[i])),
			Namelen: unix.SizeofSockaddrInet4, Iov: &iov[i], Iovlen: 1}}
	}
	if observeWrite {
		e.writeTiming.marshal.observe(time.Since(marshalStarted))
	}
	sent := 0
	for sent < len(segments) {
		if e.sendBatchDisabled {
			var syscallStarted time.Time
			if observeWrite {
				syscallStarted = time.Now()
			}
			err := syscall.Sendto(e.sendFD, packets[sent], 0, &syscall.SockaddrInet4{
				Port: int(segments[sent].DstPort), Addr: segments[sent].DstIP})
			if observeWrite {
				e.writeTiming.syscall.observe(time.Since(syscallStarted))
			}
			if e.ioStats.enabled.Load() {
				e.ioStats.txCalls.Add(1)
			}
			if rawIOInterrupted(err) {
				continue
			}
			if err != nil {
				return sent, err
			}
			sent++
			if e.ioStats.enabled.Load() {
				e.ioStats.txMessages.Add(1)
			}
			continue
		}
		var syscallStarted time.Time
		if observeWrite {
			syscallStarted = time.Now()
		}
		n, _, errno := unix.Syscall6(unix.SYS_SENDMMSG, uintptr(e.sendFD),
			uintptr(unsafe.Pointer(&msg[sent])), uintptr(len(segments)-sent), 0, 0, 0)
		if observeWrite {
			e.writeTiming.syscall.observe(time.Since(syscallStarted))
		}
		runtime.KeepAlive(packets)
		runtime.KeepAlive(iov)
		runtime.KeepAlive(from)
		runtime.KeepAlive(msg)
		if e.ioStats.enabled.Load() {
			e.ioStats.txCalls.Add(1)
		}
		if rawIOInterrupted(errno) {
			continue
		}
		if errno != 0 {
			if errors.Is(errno, unix.ENOSYS) {
				e.sendBatchDisabled = true
				if e.ioStats.enabled.Load() {
					e.ioStats.txFallbacks.Add(1)
				}
				continue
			}
			return sent, errno
		}
		if n == 0 {
			return sent, io.ErrShortWrite
		}
		sent += int(n)
		if e.ioStats.enabled.Load() {
			e.ioStats.txMessages.Add(uint64(n))
			if n > 1 {
				e.ioStats.txMulti.Add(1)
			}
		}
	}
	return sent, nil
}

// readRawFrame is called only under recvMu. WAITFORONE blocks for the first
// packet, then consumes only packets already available, never waiting to fill
// eight slots. Each ReadSegment still returns an independent owned packet.
func (e *RawIPv4Endpoint) readRawFrame() ([]byte, bool, error) {
	if !e.recvBatchDisabled {
		if e.recvBatch == nil {
			b := new(rawReceiveBatch)
			b.frames[0] = e.recvBuf
			for i := 1; i < rawBatchSize; i++ {
				b.frames[i] = make([]byte, len(e.recvBuf))
			}
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
			if e.ioStats.enabled.Load() {
				e.ioStats.rxCalls.Add(1)
			}
			if errno != 0 {
				if errors.Is(errno, unix.ENOSYS) || errors.Is(errno, unix.EINVAL) {
					e.recvBatchDisabled = true
					if e.ioStats.enabled.Load() {
						e.ioStats.rxFallbacks.Add(1)
					}
					return e.readRawFrame()
				}
				return nil, false, errno
			}
			if n == 0 {
				return nil, false, syscall.EAGAIN
			}
			b.next, b.count = 0, int(n)
			if e.ioStats.enabled.Load() {
				e.ioStats.rxMessages.Add(uint64(n))
				if n > 1 {
					e.ioStats.rxMulti.Add(1)
				}
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
		if err == nil {
			e.ioStats.rxMessages.Add(1)
		}
	}
	if err != nil {
		return nil, false, err
	}
	ll, ok := from.(*syscall.SockaddrLinklayer)
	return e.recvBuf[:n], ok && ll.Pkttype == rawPacketOutgoing, nil
}
