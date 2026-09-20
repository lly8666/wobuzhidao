//go:build windows

package faketcp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

const (
	pcapErrbufSize         = 256
	pcapDLTEthernet        = 1
	pcapReadTimeoutMS      = 50
	npcapModeSendToRxClear = 0x0200
	pcapNetmaskUnknown     = 0xffffffff
	maxNpcapCapturedFrame  = 65535 + 64
)

type pcapHeader struct {
	Sec    int32
	Usec   int32
	Caplen uint32
	Len    uint32
}

type pcapBPFProgram struct {
	Length uint32
	Insns  uintptr
}

type NpcapEndpoint struct {
	cfg    NpcapConfig
	dll    *syscall.DLL
	handle uintptr

	nextEx     *syscall.Proc
	sendPacket *syscall.Proc
	closeProc  *syscall.Proc
	getErr     *syscall.Proc
	breakLoop  *syscall.Proc

	gate   *npcapCallGate
	sendMu sync.Mutex
	ipID   uint16
}

func OpenNpcapEndpoint(cfg NpcapConfig) (*NpcapEndpoint, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	filter, err := cfg.Flow.BPFFilter()
	if err != nil {
		return nil, err
	}
	dll, err := loadNpcapDLL()
	if err != nil {
		return nil, err
	}
	release := true
	defer func() {
		if release {
			_ = dll.Release()
		}
	}()

	openLive, err := dll.FindProc("pcap_open_live")
	if err != nil {
		return nil, err
	}
	nextEx, err := dll.FindProc("pcap_next_ex")
	if err != nil {
		return nil, err
	}
	sendPacket, err := dll.FindProc("pcap_sendpacket")
	if err != nil {
		return nil, err
	}
	closeProc, err := dll.FindProc("pcap_close")
	if err != nil {
		return nil, err
	}
	getErr, err := dll.FindProc("pcap_geterr")
	if err != nil {
		return nil, err
	}
	datalink, err := dll.FindProc("pcap_datalink")
	if err != nil {
		return nil, err
	}
	setMode, err := dll.FindProc("pcap_setmode")
	if err != nil {
		return nil, fmt.Errorf("Npcap pcap_setmode export missing: %w", err)
	}
	compileFilter, err := dll.FindProc("pcap_compile")
	if err != nil {
		return nil, err
	}
	setFilter, err := dll.FindProc("pcap_setfilter")
	if err != nil {
		return nil, err
	}
	freeCode, err := dll.FindProc("pcap_freecode")
	if err != nil {
		return nil, err
	}
	breakLoop, _ := dll.FindProc("pcap_breakloop")

	device, err := syscall.BytePtrFromString(cfg.Device)
	if err != nil {
		return nil, err
	}
	var errbuf [pcapErrbufSize]byte
	handle, _, callErr := openLive.Call(
		uintptr(unsafe.Pointer(device)),
		65535,
		0,
		pcapReadTimeoutMS,
		uintptr(unsafe.Pointer(&errbuf[0])),
	)
	if handle == 0 {
		msg := cString(errbuf[:])
		if msg == "" {
			msg = fmt.Sprint(callErr)
		}
		return nil, fmt.Errorf("pcap_open_live %q: %s", cfg.Device, msg)
	}
	closeHandle := true
	defer func() {
		if closeHandle {
			closeProc.Call(handle)
		}
	}()

	linkType, _, _ := datalink.Call(handle)
	if int32(linkType) != pcapDLTEthernet {
		return nil, fmt.Errorf(
			"Npcap device %q datalink=%d, want Ethernet(%d)",
			cfg.Device, int32(linkType), pcapDLTEthernet,
		)
	}
	if setMinToCopy, err := dll.FindProc("pcap_setmintocopy"); err == nil {
		_, _, _ = setMinToCopy.Call(handle, 1)
	}
	if ret, _, _ := setMode.Call(handle, npcapModeSendToRxClear); int32(ret) != 0 {
		return nil, fmt.Errorf(
			"pcap_setmode MODE_SENDTORX_CLEAR: %s",
			pcapError(getErr, handle),
		)
	}

	filterCString, err := syscall.BytePtrFromString(filter)
	if err != nil {
		return nil, err
	}
	var program pcapBPFProgram
	if ret, _, _ := compileFilter.Call(
		handle,
		uintptr(unsafe.Pointer(&program)),
		uintptr(unsafe.Pointer(filterCString)),
		1,
		pcapNetmaskUnknown,
	); int32(ret) != 0 {
		return nil, fmt.Errorf(
			"pcap_compile %q: %s",
			filter, pcapError(getErr, handle),
		)
	}
	defer freeCode.Call(uintptr(unsafe.Pointer(&program)))
	if ret, _, _ := setFilter.Call(
		handle,
		uintptr(unsafe.Pointer(&program)),
	); int32(ret) != 0 {
		return nil, fmt.Errorf(
			"pcap_setfilter %q: %s",
			filter, pcapError(getErr, handle),
		)
	}

	release = false
	closeHandle = false
	return &NpcapEndpoint{
		cfg:        cfg,
		dll:        dll,
		handle:     handle,
		nextEx:     nextEx,
		sendPacket: sendPacket,
		closeProc:  closeProc,
		getErr:     getErr,
		breakLoop:  breakLoop,
		gate:       newNpcapCallGate(cfg.Generation),
		ipID:       1,
	}, nil
}

func (e *NpcapEndpoint) ReadSegment(generation uint64) (Segment, []byte, error) {
	if e == nil || e.gate == nil {
		return Segment{}, nil, ErrNpcapClosed
	}
	if err := e.gate.begin(generation); err != nil {
		return Segment{}, nil, err
	}
	defer e.gate.end()

	for {
		if e.gate.isClosed() {
			return Segment{}, nil, ErrNpcapClosed
		}
		var hdr *pcapHeader
		var data uintptr
		ret, _, _ := e.nextEx.Call(
			e.handle,
			uintptr(unsafe.Pointer(&hdr)),
			uintptr(unsafe.Pointer(&data)),
		)
		switch int32(ret) {
		case 0:
			continue
		case -1:
			return Segment{}, nil, fmt.Errorf(
				"pcap_next_ex: %s",
				pcapError(e.getErr, e.handle),
			)
		case -2:
			if e.gate.isClosed() {
				return Segment{}, nil, ErrNpcapClosed
			}
			return Segment{}, nil, io.EOF
		case 1:
			if hdr == nil || data == 0 ||
				hdr.Caplen == 0 || hdr.Caplen > maxNpcapCapturedFrame {
				continue
			}
			frame := unsafe.Slice(
				(*byte)(unsafe.Pointer(data)),
				int(hdr.Caplen),
			)
			seg, packet, ok, err := decodeNpcapInbound(frame, e.cfg.Flow)
			if err != nil {
				return Segment{}, nil, err
			}
			if !ok {
				continue
			}
			return seg, packet, nil
		default:
			return Segment{}, nil, fmt.Errorf(
				"pcap_next_ex returned %d",
				int32(ret),
			)
		}
	}
}

func (e *NpcapEndpoint) WriteSegment(generation uint64, seg Segment) ([]byte, error) {
	if e == nil || e.gate == nil {
		return nil, ErrNpcapClosed
	}
	if err := e.gate.begin(generation); err != nil {
		return nil, err
	}
	defer e.gate.end()

	e.sendMu.Lock()
	defer e.sendMu.Unlock()
	packet, frame, err := encodeNpcapOutbound(seg, e.cfg, e.ipID)
	if err != nil {
		return nil, err
	}
	e.ipID++
	ret, _, _ := e.sendPacket.Call(
		e.handle,
		uintptr(unsafe.Pointer(&frame[0])),
		uintptr(len(frame)),
	)
	if int32(ret) != 0 {
		return nil, fmt.Errorf(
			"pcap_sendpacket: %s",
			pcapError(e.getErr, e.handle),
		)
	}
	return packet, nil
}

func (e *NpcapEndpoint) Close() error {
	if e == nil || e.gate == nil {
		return nil
	}
	if !e.gate.close() {
		return nil
	}
	if e.breakLoop != nil && e.handle != 0 {
		e.breakLoop.Call(e.handle)
	}
	e.gate.wait()

	if e.handle != 0 {
		e.closeProc.Call(e.handle)
		e.handle = 0
	}
	if e.dll != nil {
		err := e.dll.Release()
		e.dll = nil
		return err
	}
	return nil
}

func loadNpcapDLL() (*syscall.DLL, error) {
	var candidates []string
	if root := os.Getenv("SystemRoot"); root != "" {
		candidates = append(
			candidates,
			filepath.Join(root, "System32", "Npcap", "wpcap.dll"),
		)
	}
	candidates = append(candidates, "wpcap.dll")
	var last error
	for _, path := range candidates {
		dll, err := syscall.LoadDLL(path)
		if err == nil {
			return dll, nil
		}
		last = err
	}
	return nil, fmt.Errorf("load Npcap wpcap.dll: %w", last)
}

func pcapError(getErr *syscall.Proc, handle uintptr) string {
	if getErr == nil || handle == 0 {
		return "unknown Npcap error"
	}
	ptr, _, _ := getErr.Call(handle)
	if ptr == 0 {
		return "unknown Npcap error"
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), 1024)
	if s := cString(buf); s != "" {
		return s
	}
	return "unknown Npcap error"
}

func cString(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}
