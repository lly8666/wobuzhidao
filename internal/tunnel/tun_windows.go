//go:build windows

package tunnel

import (
	"fmt"
	"io"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	wintunRingCapacity = 0x400000 // 4 MiB, power-of-two within the Wintun API limits.
	wintunMaxIPPacket  = 0xffff

	errorHandleEOF      syscall.Errno = 38
	errorBufferOverflow syscall.Errno = 111
	errorNoMoreItems    syscall.Errno = 259

	waitObject0 = 0
	waitFailed  = 0xffffffff
	infinite    = 0xffffffff
)

var (
	wintunDLL = syscall.NewLazyDLL("wintun.dll")

	wintunCreateAdapter        = wintunDLL.NewProc("WintunCreateAdapter")
	wintunOpenAdapter          = wintunDLL.NewProc("WintunOpenAdapter")
	wintunCloseAdapter         = wintunDLL.NewProc("WintunCloseAdapter")
	wintunStartSession         = wintunDLL.NewProc("WintunStartSession")
	wintunEndSession           = wintunDLL.NewProc("WintunEndSession")
	wintunGetReadWaitEvent     = wintunDLL.NewProc("WintunGetReadWaitEvent")
	wintunReceivePacket        = wintunDLL.NewProc("WintunReceivePacket")
	wintunReleaseReceivePacket = wintunDLL.NewProc("WintunReleaseReceivePacket")
	wintunAllocateSendPacket   = wintunDLL.NewProc("WintunAllocateSendPacket")
	wintunSendPacket           = wintunDLL.NewProc("WintunSendPacket")

	kernel32DLL            = syscall.NewLazyDLL("kernel32.dll")
	createEventW           = kernel32DLL.NewProc("CreateEventW")
	setEvent               = kernel32DLL.NewProc("SetEvent")
	closeHandle            = kernel32DLL.NewProc("CloseHandle")
	waitForMultipleObjects = kernel32DLL.NewProc("WaitForMultipleObjects")
)

type TUN struct {
	adapter  uintptr
	session  uintptr
	readEvt  uintptr
	closeEvt uintptr
	name     string

	stateMu         sync.Mutex
	closed          bool
	active          sync.WaitGroup
	closeOnce       sync.Once
	nonIPv4DropOnce sync.Once
}

func OpenTUN(name string) (*TUN, error) {
	if name == "" {
		name = "WBD"
	}
	name16, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, fmt.Errorf("Wintun adapter name: %w", err)
	}
	type16, err := syscall.UTF16PtrFromString("WBD")
	if err != nil {
		return nil, fmt.Errorf("Wintun tunnel type: %w", err)
	}

	adapter, _, openErr := wintunOpenAdapter.Call(uintptr(unsafe.Pointer(name16)))
	if adapter == 0 {
		adapter, _, err = wintunCreateAdapter.Call(
			uintptr(unsafe.Pointer(name16)),
			uintptr(unsafe.Pointer(type16)),
			0,
		)
		if adapter == 0 {
			return nil, wintunCallError("WintunCreateAdapter", err, openErr)
		}
	}

	session, _, callErr := wintunStartSession.Call(adapter, wintunRingCapacity)
	if session == 0 {
		wintunCloseAdapter.Call(adapter)
		return nil, wintunCallError("WintunStartSession", callErr, nil)
	}
	readEvt, _, callErr := wintunGetReadWaitEvent.Call(session)
	if readEvt == 0 {
		wintunEndSession.Call(session)
		wintunCloseAdapter.Call(adapter)
		return nil, wintunCallError("WintunGetReadWaitEvent", callErr, nil)
	}
	closeEvt, _, callErr := createEventW.Call(0, 1, 0, 0) // manual-reset, initially non-signaled
	if closeEvt == 0 {
		wintunEndSession.Call(session)
		wintunCloseAdapter.Call(adapter)
		return nil, wintunCallError("CreateEventW", callErr, nil)
	}

	return &TUN{
		adapter:  adapter,
		session:  session,
		readEvt:  readEvt,
		closeEvt: closeEvt,
		name:     name,
	}, nil
}

func (t *TUN) Name() string { return t.name }

func (t *TUN) beginCall() bool {
	t.stateMu.Lock()
	defer t.stateMu.Unlock()
	if t.closed {
		return false
	}
	t.active.Add(1)
	return true
}

func (t *TUN) endCall() {
	t.active.Done()
}

func (t *TUN) isClosed() bool {
	t.stateMu.Lock()
	closed := t.closed
	t.stateMu.Unlock()
	return closed
}

func isIPv4Packet(packet []byte) bool {
	if len(packet) < 20 || packet[0]>>4 != 4 {
		return false
	}
	ihl := int(packet[0]&0x0f) * 4
	return ihl >= 20 && ihl <= len(packet)
}

func (t *TUN) ReadPacket(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, io.ErrShortBuffer
	}
	if !t.beginCall() {
		return 0, io.EOF
	}
	defer t.endCall()

	for {
		if t.isClosed() {
			return 0, io.EOF
		}

		var packetSize uint32
		packet, _, callErr := wintunReceivePacket.Call(
			t.session,
			uintptr(unsafe.Pointer(&packetSize)),
		)
		if packet != 0 {
			packetBytes := unsafe.Slice((*byte)(unsafe.Pointer(packet)), int(packetSize))
			// The current Windows product path is IPv4-only. Wintun is an L3
			// adapter and may surface IPv6 (notably link-local control traffic).
			// Fail closed here so non-IPv4 can never enter the Game/raw-IP wire.
			if !isIPv4Packet(packetBytes) {
				wintunReleaseReceivePacket.Call(t.session, packet)
				t.nonIPv4DropOnce.Do(func() {
					fmt.Println("WBD_TUN_WINDOWS_NON_IPV4_DROP fail_closed=1")
				})
				continue
			}
			if int(packetSize) > len(p) {
				wintunReleaseReceivePacket.Call(t.session, packet)
				return 0, io.ErrShortBuffer
			}
			copy(p[:packetSize], packetBytes)
			wintunReleaseReceivePacket.Call(t.session, packet)
			return int(packetSize), nil
		}

		errno := wintunErrno(callErr)
		switch errno {
		case errorNoMoreItems:
			handles := [2]uintptr{t.readEvt, t.closeEvt}
			status, _, waitErr := waitForMultipleObjects.Call(
				uintptr(len(handles)),
				uintptr(unsafe.Pointer(&handles[0])),
				0,
				infinite,
			)
			switch status {
			case waitObject0:
				continue
			case waitObject0 + 1:
				return 0, io.EOF
			case waitFailed:
				return 0, wintunCallError("WaitForMultipleObjects", waitErr, nil)
			default:
				return 0, fmt.Errorf("WaitForMultipleObjects returned %#x", status)
			}
		case errorHandleEOF:
			return 0, io.EOF
		default:
			return 0, wintunCallError("WintunReceivePacket", callErr, nil)
		}
	}
}

func (t *TUN) WritePacket(p []byte) (int, error) {
	if len(p) == 0 || len(p) > wintunMaxIPPacket {
		return 0, fmt.Errorf("%w: packet size %d", ErrMTU, len(p))
	}
	if !t.beginCall() {
		return 0, io.EOF
	}
	defer t.endCall()

	for {
		if t.isClosed() {
			return 0, io.EOF
		}

		packet, _, callErr := wintunAllocateSendPacket.Call(t.session, uintptr(len(p)))
		if packet != 0 {
			copy(unsafe.Slice((*byte)(unsafe.Pointer(packet)), len(p)), p)
			wintunSendPacket.Call(t.session, packet)
			return len(p), nil
		}

		switch wintunErrno(callErr) {
		case errorBufferOverflow:
			// Wintun has no send-wait event. Back off instead of turning a
			// transient ring-full condition into a tunnel failure. Close sets
			// the state flag, so a draining writer cannot stall teardown forever.
			runtime.Gosched()
			time.Sleep(time.Millisecond)
			continue
		case errorHandleEOF:
			return 0, io.EOF
		default:
			return 0, wintunCallError("WintunAllocateSendPacket", callErr, nil)
		}
	}
}

func (t *TUN) Close() error {
	t.closeOnce.Do(func() {
		// First prevent new Wintun calls and wake any reader blocked on the
		// Wintun read event. The session remains alive until all calls that
		// entered before this point have returned.
		t.stateMu.Lock()
		t.closed = true
		if t.closeEvt != 0 {
			setEvent.Call(t.closeEvt)
		}
		t.stateMu.Unlock()

		t.active.Wait()

		if t.session != 0 {
			wintunEndSession.Call(t.session)
			t.session = 0
		}
		if t.adapter != 0 {
			wintunCloseAdapter.Call(t.adapter)
			t.adapter = 0
		}
		if t.closeEvt != 0 {
			closeHandle.Call(t.closeEvt)
			t.closeEvt = 0
		}
	})
	return nil
}

func wintunErrno(err error) syscall.Errno {
	if errno, ok := err.(syscall.Errno); ok {
		return errno
	}
	return 0
}

func wintunCallError(op string, primary, fallback error) error {
	if errno := wintunErrno(primary); errno != 0 {
		return fmt.Errorf("%s: %w", op, errno)
	}
	if errno := wintunErrno(fallback); errno != 0 {
		return fmt.Errorf("%s: %w", op, errno)
	}
	return fmt.Errorf("%s failed", op)
}
