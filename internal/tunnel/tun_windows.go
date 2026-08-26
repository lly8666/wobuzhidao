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

	wintunCreateAdapter       = wintunDLL.NewProc("WintunCreateAdapter")
	wintunOpenAdapter         = wintunDLL.NewProc("WintunOpenAdapter")
	wintunCloseAdapter        = wintunDLL.NewProc("WintunCloseAdapter")
	wintunStartSession        = wintunDLL.NewProc("WintunStartSession")
	wintunEndSession          = wintunDLL.NewProc("WintunEndSession")
	wintunGetReadWaitEvent    = wintunDLL.NewProc("WintunGetReadWaitEvent")
	wintunReceivePacket       = wintunDLL.NewProc("WintunReceivePacket")
	wintunReleaseReceivePacket = wintunDLL.NewProc("WintunReleaseReceivePacket")
	wintunAllocateSendPacket  = wintunDLL.NewProc("WintunAllocateSendPacket")
	wintunSendPacket          = wintunDLL.NewProc("WintunSendPacket")

	kernel32DLL       = syscall.NewLazyDLL("kernel32.dll")
	waitForSingleObject = kernel32DLL.NewProc("WaitForSingleObject")
)

type TUN struct {
	adapter uintptr
	session uintptr
	readEvt uintptr
	name    string

	closeOnce sync.Once
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

	return &TUN{adapter: adapter, session: session, readEvt: readEvt, name: name}, nil
}

func (t *TUN) Name() string { return t.name }

func (t *TUN) ReadPacket(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, io.ErrShortBuffer
	}
	for {
		var packetSize uint32
		packet, _, callErr := wintunReceivePacket.Call(
			t.session,
			uintptr(unsafe.Pointer(&packetSize)),
		)
		if packet != 0 {
			if int(packetSize) > len(p) {
				wintunReleaseReceivePacket.Call(t.session, packet)
				return 0, io.ErrShortBuffer
			}
			copy(p[:packetSize], unsafe.Slice((*byte)(unsafe.Pointer(packet)), int(packetSize)))
			wintunReleaseReceivePacket.Call(t.session, packet)
			return int(packetSize), nil
		}

		errno := wintunErrno(callErr)
		switch errno {
		case errorNoMoreItems:
			status, _, waitErr := waitForSingleObject.Call(t.readEvt, infinite)
			if status == waitObject0 {
				continue
			}
			if status == waitFailed {
				return 0, wintunCallError("WaitForSingleObject", waitErr, nil)
			}
			return 0, fmt.Errorf("WaitForSingleObject returned %#x", status)
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
	for {
		packet, _, callErr := wintunAllocateSendPacket.Call(t.session, uintptr(len(p)))
		if packet != 0 {
			copy(unsafe.Slice((*byte)(unsafe.Pointer(packet)), len(p)), p)
			wintunSendPacket.Call(t.session, packet)
			return len(p), nil
		}

		switch wintunErrno(callErr) {
		case errorBufferOverflow:
			// Wintun has no send-wait event. Back off instead of turning a
			// transient ring-full condition into a tunnel failure.
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
		if t.session != 0 {
			wintunEndSession.Call(t.session)
			t.session = 0
		}
		if t.adapter != 0 {
			wintunCloseAdapter.Call(t.adapter)
			t.adapter = 0
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
