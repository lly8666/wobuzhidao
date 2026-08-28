//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	pcapErrbufSize    = 256
	pcapDLTEthernet   = 1
	pcapReadTimeoutMS = 50
)

type pcapHeader struct {
	Sec    int32
	Usec   int32
	Caplen uint32
	Len    uint32
}

type npcapRawPacketIO struct {
	dll    *syscall.DLL
	handle uintptr

	pcapNextEx     *syscall.Proc
	pcapSendPacket *syscall.Proc
	pcapClose      *syscall.Proc
	pcapGetErr     *syscall.Proc

	sourceMAC [6]byte
	nextHopMAC [6]byte
	sourceIP  [4]byte
	remoteIP  [4]byte
	sourcePort uint16
	remotePort uint16
	rstOnce   sync.Once

	mu     sync.Mutex
	closed bool
	active sync.WaitGroup
}

func openRawPacketIO(c config, sourceIP [4]byte) (rawPacketIO, error) {
	if c.role != "client" {
		return nil, errors.New("Windows Npcap FakeTCP backend is client-only; run the public listener on Linux")
	}
	if c.packetDevice == "" || c.sourceMAC == "" || c.nextHopMAC == "" {
		return nil, errors.New("Windows FakeTCP client requires --packet-device --source-mac --next-hop-mac")
	}
	sourceMAC, err := parseEtherMAC(c.sourceMAC)
	if err != nil {
		return nil, fmt.Errorf("source MAC: %w", err)
	}
	nextHopMAC, err := parseEtherMAC(c.nextHopMAC)
	if err != nil {
		return nil, fmt.Errorf("next-hop MAC: %w", err)
	}
	sourceAddr, err := net.ResolveUDPAddr("udp4", c.source)
	if err != nil || sourceAddr == nil || sourceAddr.Port <= 0 || sourceAddr.Port > 65535 {
		return nil, fmt.Errorf("source flow: %w", err)
	}
	remoteAddr, err := net.ResolveUDPAddr("udp4", c.remote)
	if err != nil || remoteAddr == nil || remoteAddr.Port <= 0 || remoteAddr.Port > 65535 {
		return nil, fmt.Errorf("remote flow: %w", err)
	}
	remote4 := remoteAddr.IP.To4()
	if remote4 == nil {
		return nil, errors.New("Windows FakeTCP remote must be IPv4")
	}
	var remoteIP [4]byte
	copy(remoteIP[:], remote4)

	dll, err := loadNpcapDLL()
	if err != nil {
		return nil, err
	}
	fail := func(err error) (rawPacketIO, error) {
		_ = dll.Release()
		return nil, err
	}
	openLive, err := dll.FindProc("pcap_open_live")
	if err != nil {
		return fail(err)
	}
	nextEx, err := dll.FindProc("pcap_next_ex")
	if err != nil {
		return fail(err)
	}
	sendPacket, err := dll.FindProc("pcap_sendpacket")
	if err != nil {
		return fail(err)
	}
	closeProc, err := dll.FindProc("pcap_close")
	if err != nil {
		return fail(err)
	}
	getErr, err := dll.FindProc("pcap_geterr")
	if err != nil {
		return fail(err)
	}
	datalink, err := dll.FindProc("pcap_datalink")
	if err != nil {
		return fail(err)
	}

	device, err := syscall.BytePtrFromString(c.packetDevice)
	if err != nil {
		return fail(err)
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
		msg := cStringBytes(errbuf[:])
		if msg == "" {
			msg = fmt.Sprint(callErr)
		}
		return fail(fmt.Errorf("pcap_open_live %q: %s", c.packetDevice, msg))
	}
	linkType, _, _ := datalink.Call(handle)
	if int32(linkType) != pcapDLTEthernet {
		closeProc.Call(handle)
		return fail(fmt.Errorf("Npcap device %q datalink=%d, want Ethernet(%d)", c.packetDevice, int32(linkType), pcapDLTEthernet))
	}
	if setMinToCopy, err := dll.FindProc("pcap_setmintocopy"); err == nil {
		_, _, _ = setMinToCopy.Call(handle, 1)
	}
	// Npcap >=1.83 mode 0 restores the ordinary transmit path if a machine-wide
	// SendToRxAdapters override exists. Older versions simply lack this symbol.
	if setMode, err := dll.FindProc("pcap_setmode"); err == nil {
		_, _, _ = setMode.Call(handle, 0)
	}

	return &npcapRawPacketIO{
		dll: dll, handle: handle,
		pcapNextEx: nextEx, pcapSendPacket: sendPacket, pcapClose: closeProc, pcapGetErr: getErr,
		sourceMAC: sourceMAC, nextHopMAC: nextHopMAC,
		sourceIP: sourceIP, remoteIP: remoteIP,
		sourcePort: uint16(sourceAddr.Port), remotePort: uint16(remoteAddr.Port),
	}, nil
}

func loadNpcapDLL() (*syscall.DLL, error) {
	var candidates []string
	if root := os.Getenv("SystemRoot"); root != "" {
		candidates = append(candidates, filepath.Join(root, "System32", "Npcap", "wpcap.dll"))
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
	return nil, fmt.Errorf("load Npcap wpcap.dll: %w (install Npcap separately; WBD does not redistribute the Free Edition)", last)
}

func parseEtherMAC(raw string) ([6]byte, error) {
	var out [6]byte
	mac, err := net.ParseMAC(raw)
	if err != nil {
		return out, err
	}
	if len(mac) != 6 {
		return out, fmt.Errorf("want 6-byte Ethernet MAC, got %d bytes", len(mac))
	}
	copy(out[:], mac)
	return out, nil
}

func (r *npcapRawPacketIO) beginCall() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return false
	}
	r.active.Add(1)
	return true
}

func (r *npcapRawPacketIO) endCall() { r.active.Done() }

func (r *npcapRawPacketIO) isClosed() bool {
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	return closed
}

func (r *npcapRawPacketIO) ReadPacket(buf []byte) (int, error) {
	if !r.beginCall() {
		return 0, io.EOF
	}
	defer r.endCall()
	for {
		if r.isClosed() {
			return 0, io.EOF
		}
		var hdr *pcapHeader
		var data uintptr
		ret, _, _ := r.pcapNextEx.Call(
			r.handle,
			uintptr(unsafe.Pointer(&hdr)),
			uintptr(unsafe.Pointer(&data)),
		)
		switch int32(ret) {
		case 0:
			return 0, errRawTimeout
		case -1:
			return 0, fmt.Errorf("pcap_next_ex: %s", r.lastError())
		case -2:
			return 0, io.EOF
		case 1:
			if hdr == nil || data == 0 || hdr.Caplen == 0 {
				continue
			}
			frame := unsafe.Slice((*byte)(unsafe.Pointer(data)), int(hdr.Caplen))
			packet, ok := ethernetIPv4Payload(frame)
			if !ok {
				continue
			}
			if r.matchesKernelRST(packet) {
				r.rstOnce.Do(func() {
					fmt.Fprintf(os.Stderr,
						"WBD_FAKETCP_WINDOWS_KERNEL_RST_SEEN source_port=%d remote_port=%d\n",
						r.sourcePort, r.remotePort)
				})
			}
			if len(packet) > len(buf) {
				return 0, io.ErrShortBuffer
			}
			copy(buf, packet)
			return len(packet), nil
		default:
			return 0, fmt.Errorf("pcap_next_ex returned %d", int32(ret))
		}
	}
}

func (r *npcapRawPacketIO) matchesKernelRST(packet []byte) bool {
	if len(packet) < 40 || packet[0]>>4 != 4 || packet[9] != 6 {
		return false
	}
	ihl := int(packet[0]&0x0f) * 4
	if ihl < 20 || len(packet) < ihl+20 {
		return false
	}
	if packet[12] != r.sourceIP[0] || packet[13] != r.sourceIP[1] || packet[14] != r.sourceIP[2] || packet[15] != r.sourceIP[3] ||
		packet[16] != r.remoteIP[0] || packet[17] != r.remoteIP[1] || packet[18] != r.remoteIP[2] || packet[19] != r.remoteIP[3] {
		return false
	}
	if binary.BigEndian.Uint16(packet[ihl:ihl+2]) != r.sourcePort || binary.BigEndian.Uint16(packet[ihl+2:ihl+4]) != r.remotePort {
		return false
	}
	return packet[ihl+13]&0x04 != 0
}

func (r *npcapRawPacketIO) WritePacket(packet []byte, _ [4]byte) error {
	if len(packet) == 0 {
		return errors.New("Npcap send: empty packet")
	}
	if !r.beginCall() {
		return io.EOF
	}
	defer r.endCall()
	frame := make([]byte, 14+len(packet))
	copy(frame[0:6], r.nextHopMAC[:])
	copy(frame[6:12], r.sourceMAC[:])
	binary.BigEndian.PutUint16(frame[12:14], 0x0800)
	copy(frame[14:], packet)
	ret, _, _ := r.pcapSendPacket.Call(r.handle, uintptr(unsafe.Pointer(&frame[0])), uintptr(len(frame)))
	if int32(ret) != 0 {
		return fmt.Errorf("pcap_sendpacket: %s", r.lastError())
	}
	return nil
}

func (r *npcapRawPacketIO) SetReadTimeout(time.Duration) error { return nil }
func (r *npcapRawPacketIO) ClearReadTimeout() error            { return nil }

func (r *npcapRawPacketIO) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	r.mu.Unlock()
	// pcap_open_live was activated with a 50ms read timeout. Wait until every
	// pcap_next_ex/pcap_sendpacket call has returned before freeing the handle.
	r.active.Wait()
	if r.handle != 0 {
		r.pcapClose.Call(r.handle)
		r.handle = 0
	}
	if r.dll != nil {
		err := r.dll.Release()
		r.dll = nil
		return err
	}
	return nil
}

func (r *npcapRawPacketIO) lastError() string {
	ptr, _, _ := r.pcapGetErr.Call(r.handle)
	if ptr == 0 {
		return "unknown Npcap error"
	}
	b := unsafe.Slice((*byte)(unsafe.Pointer(ptr)), 1024)
	if s := cStringBytes(b); s != "" {
		return s
	}
	return "unknown Npcap error"
}

func ethernetIPv4Payload(frame []byte) ([]byte, bool) {
	if len(frame) < 14 {
		return nil, false
	}
	etherType := binary.BigEndian.Uint16(frame[12:14])
	offset := 14
	for etherType == 0x8100 || etherType == 0x88a8 {
		if len(frame) < offset+4 {
			return nil, false
		}
		etherType = binary.BigEndian.Uint16(frame[offset+2 : offset+4])
		offset += 4
	}
	if etherType != 0x0800 || len(frame) <= offset {
		return nil, false
	}
	return frame[offset:], true
}

func cStringBytes(buf []byte) string {
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

func notifySignals(ch chan<- os.Signal) {
	signal.Notify(ch, os.Interrupt)
}
