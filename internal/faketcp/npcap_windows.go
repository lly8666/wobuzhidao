//go:build windows

package faketcp

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
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

	nextEx                                  *syscall.Proc
	sendPacket                              *syscall.Proc
	closeProc                               *syscall.Proc
	getErr                                  *syscall.Proc
	breakLoop                               *syscall.Proc
	queueAlloc, queueTransmit, queueDestroy *syscall.Proc
	sendQueue                               uintptr // native scratch, sendMu-owned; no deferred packets.
	batchDisabled                           bool

	gate      *npcapCallGate
	sendMu    sync.Mutex
	ipID      uint16
	statsProc *syscall.Proc
	diag      npcapIODiagnostics
	// Only ReadSegment's reader touches these clocks and the driver stats call.
	statsAt, readAt time.Time
}

// Windows pcap_stat has six 32-bit unsigned fields (not native Go ints).
type npcapDriverStats struct{ Received, Dropped, InterfaceDropped, Captured, Sent, NetDropped uint32 }

func (e *NpcapEndpoint) SetIODiagnostics(enabled bool) { e.diag.enabled.Store(enabled) }
func (e *NpcapEndpoint) IODiagnostic() NpcapIODiagnostic {
	out := e.diag.snapshot(e.cfg.Generation, e.statsProc != nil)
	out.BatchSupported = e.queueTransmit != nil
	return out
}

func (e *NpcapEndpoint) sampleDriverStats(now time.Time) {
	if now.Before(e.statsAt) {
		return
	}
	e.statsAt = now.Add(time.Second)
	if e.statsProc == nil {
		return
	}
	var stats npcapDriverStats
	ret, _, _ := e.statsProc.Call(e.handle, uintptr(unsafe.Pointer(&stats)))
	if int32(ret) != 0 {
		e.diag.statsErrors.Add(1)
		return
	}
	e.diag.driverReceived.Store(uint64(stats.Received))
	e.diag.driverDropped.Store(uint64(stats.Dropped))
	e.diag.interfaceDropped.Store(uint64(stats.InterfaceDropped))
	e.diag.statsSamples.Add(1)
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
	statsProc, _ := dll.FindProc("pcap_stats")
	queueAlloc, _ := dll.FindProc("pcap_sendqueue_alloc")
	queueTransmit, _ := dll.FindProc("pcap_sendqueue_transmit")
	queueDestroy, _ := dll.FindProc("pcap_sendqueue_destroy")
	if queueAlloc == nil || queueTransmit == nil || queueDestroy == nil {
		queueAlloc, queueTransmit, queueDestroy = nil, nil, nil
	}

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
		statsProc:  statsProc,
		queueAlloc: queueAlloc, queueTransmit: queueTransmit, queueDestroy: queueDestroy,
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
		if e.diag.enabled.Load() {
			e.sampleDriverStats(time.Now())
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
			if e.diag.enabled.Load() {
				now := time.Now()
				var gap uint64
				if !e.readAt.IsZero() {
					gap = uint64(now.Sub(e.readAt))
				}
				e.readAt = now
				e.diag.observeRead(uint64(len(packet)), gap)
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

	e.lockSend()
	defer e.sendMu.Unlock()
	packet, frame, err := encodeNpcapOutbound(seg, e.cfg, e.ipID)
	if err != nil {
		return nil, err
	}
	e.ipID++
	if err := e.writeFrameLocked(frame); err != nil {
		return nil, err
	}
	if e.diag.enabled.Load() {
		e.diag.writePackets.Add(1)
		e.diag.writeBytes.Add(uint64(len(packet)))
	}
	return packet, nil
}

func (e *NpcapEndpoint) lockSend() {
	if !e.diag.enabled.Load() {
		e.sendMu.Lock()
		return
	}
	started := time.Now()
	e.sendMu.Lock()
	e.diag.observeSendWait(uint64(time.Since(started)))
}

func (e *NpcapEndpoint) writeFrameLocked(frame []byte) error {
	observe := e.diag.enabled.Load()
	var started time.Time
	if observe {
		started = time.Now()
	}
	ret, _, _ := e.sendPacket.Call(e.handle, uintptr(unsafe.Pointer(&frame[0])), uintptr(len(frame)))
	if observe {
		e.diag.observeWriteCall(uint64(time.Since(started)), 1)
	}
	if int32(ret) != 0 {
		return fmt.Errorf("pcap_sendpacket: %s", pcapError(e.getErr, e.handle))
	}
	return nil
}

// Native Windows pcap_send_queue uses two uint32 lengths followed by a pointer.
// The DLL allocator owns its memory and frees it only after all gated calls end.
type npcapSendQueue struct {
	MaxLen, Len uint32
	Buffer      uintptr
}

func (e *NpcapEndpoint) WriteSegments(generation uint64, segments []Segment) (int, error) {
	if e == nil || e.gate == nil {
		return 0, ErrNpcapClosed
	}
	if err := e.gate.begin(generation); err != nil {
		return 0, err
	}
	defer e.gate.end()
	e.lockSend()
	defer e.sendMu.Unlock()
	var scratch []byte
	var transmit func([]byte) (uint32, error)
	if len(segments) > 1 && e.queueTransmit != nil && !e.batchDisabled {
		if e.sendQueue == 0 {
			e.sendQueue, _, _ = e.queueAlloc.Call(npcapSendQueueBytes)
			if e.sendQueue == 0 {
				e.batchDisabled = true
			}
		}
		if e.sendQueue != 0 {
			q := (*npcapSendQueue)(unsafe.Pointer(e.sendQueue))
			if q.MaxLen != npcapSendQueueBytes || q.Buffer == 0 {
				return 0, errNpcapBatchReceipt
			}
			scratch = unsafe.Slice((*byte)(unsafe.Pointer(q.Buffer)), npcapSendQueueBytes)
			transmit = func(b []byte) (uint32, error) {
				q.Len = uint32(len(b))
				observe := e.diag.enabled.Load()
				var started time.Time
				if observe {
					started = time.Now()
				}
				ret, _, _ := e.queueTransmit.Call(e.handle, e.sendQueue, 0)
				if observe {
					packets := 0
					for offset := 0; offset < len(b); packets++ {
						offset += npcapQueueHeaderBytes + int(binary.LittleEndian.Uint32(b[offset+8:offset+12]))
					}
					e.diag.observeWriteCall(uint64(time.Since(started)), packets)
				}
				q.Len = 0
				// Never retry a partial queue through pcap_sendpacket: prefix
				// may already be on wire. Runtime retires only unsent backups.
				if uint32(ret) != uint32(len(b)) {
					return uint32(ret), fmt.Errorf("pcap_sendqueue_transmit short %d/%d: %s", uint32(ret), len(b), pcapError(e.getErr, e.handle))
				}
				return uint32(ret), nil
			}
		}
	}
	var written func(int, int)
	if e.diag.enabled.Load() {
		written = func(packets, bytes int) {
			e.diag.writePackets.Add(uint64(packets))
			e.diag.writeBytes.Add(uint64(bytes))
		}
	}
	return writeNpcapReadySegments(segments, e.cfg, &e.ipID, scratch, e.writeFrameLocked, transmit, written)
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
	if e.sendQueue != 0 {
		e.queueDestroy.Call(e.sendQueue)
		e.sendQueue = 0
	}

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
