//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"golang.org/x/sys/windows"
)

const (
	fakeTCPSupervisorEventPrefix = `Local\WBDFakeTCPShutdown-`
	fakeTCPRetireResetCopies     = 3
	fakeTCPRetireResetSpacing    = 20 * time.Millisecond
)

type fakeTCPRetireResetSpec struct {
	packetDevice string
	sourceMAC    [6]byte
	nextHopMAC   [6]byte
	sourceIP     [4]byte
	remoteIP     [4]byte
	sourcePort   uint16
	remotePort   uint16
}

// The ordinary Windows process stop is TerminateProcess and cannot run Go
// defers. Arm a dedicated retirement event instead. The Windows runtime signals
// it only after the LINK CloseNormal ACK and DTLS teardown for a replaced lane.
// A successful event sends an exact-flow FakeTCP RST before process exit so the
// Linux mux can remove the old association immediately instead of waiting for
// no_client_rx idle GC.
func init() {
	if len(os.Args) < 2 || os.Args[1] != "client" {
		return
	}
	name, err := windows.UTF16PtrFromString(fakeTCPSupervisorEventPrefix + strconv.Itoa(os.Getpid()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_FAKETCP_RETIRE_EVENT_FAIL", err)
		return
	}
	h, err := windows.CreateEvent(nil, 0, 0, name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_FAKETCP_RETIRE_EVENT_FAIL", err)
		return
	}
	go func() {
		defer windows.CloseHandle(h)
		if _, err := windows.WaitForSingleObject(h, windows.INFINITE); err != nil {
			fmt.Fprintln(os.Stderr, "WBD_FAKETCP_RETIRE_EVENT_FAIL", err)
			os.Exit(3)
		}
		spec, err := fakeTCPRetireResetSpecFromArgs(os.Args[2:])
		if err != nil {
			fmt.Fprintln(os.Stderr, "WBD_FAKETCP_RETIRE_RESET_FAIL", err)
			os.Exit(3)
		}
		if err := sendFakeTCPRetireReset(spec); err != nil {
			fmt.Fprintln(os.Stderr, "WBD_FAKETCP_RETIRE_RESET_FAIL", err)
			os.Exit(3)
		}
		fmt.Printf("WBD_FAKETCP_RETIRE_RESET_TX source_port=%d remote_port=%d copies=%d\n", spec.sourcePort, spec.remotePort, fakeTCPRetireResetCopies)
		os.Exit(0)
	}()
}

func fakeTCPRetireResetSpecFromArgs(args []string) (fakeTCPRetireResetSpec, error) {
	var spec fakeTCPRetireResetSpec
	device, ok := fakeTCPRetireArg(args, "--packet-device")
	if !ok || strings.TrimSpace(device) == "" {
		return spec, errors.New("retirement reset missing --packet-device")
	}
	sourceMACRaw, ok := fakeTCPRetireArg(args, "--source-mac")
	if !ok {
		return spec, errors.New("retirement reset missing --source-mac")
	}
	nextHopMACRaw, ok := fakeTCPRetireArg(args, "--next-hop-mac")
	if !ok {
		return spec, errors.New("retirement reset missing --next-hop-mac")
	}
	sourceRaw, ok := fakeTCPRetireArg(args, "--source")
	if !ok {
		return spec, errors.New("retirement reset missing --source")
	}
	remoteRaw, ok := fakeTCPRetireArg(args, "--remote")
	if !ok {
		return spec, errors.New("retirement reset missing --remote")
	}

	var err error
	spec.packetDevice = device
	spec.sourceMAC, err = parseEtherMAC(sourceMACRaw)
	if err != nil {
		return spec, fmt.Errorf("retirement source MAC: %w", err)
	}
	spec.nextHopMAC, err = parseEtherMAC(nextHopMACRaw)
	if err != nil {
		return spec, fmt.Errorf("retirement next-hop MAC: %w", err)
	}
	sourceAddr, err := net.ResolveUDPAddr("udp4", sourceRaw)
	if err != nil || sourceAddr == nil || sourceAddr.Port <= 0 || sourceAddr.Port > 65535 {
		return spec, fmt.Errorf("retirement source flow: %w", err)
	}
	remoteAddr, err := net.ResolveUDPAddr("udp4", remoteRaw)
	if err != nil || remoteAddr == nil || remoteAddr.Port <= 0 || remoteAddr.Port > 65535 {
		return spec, fmt.Errorf("retirement remote flow: %w", err)
	}
	source4 := sourceAddr.IP.To4()
	remote4 := remoteAddr.IP.To4()
	if source4 == nil || remote4 == nil {
		return spec, errors.New("retirement reset requires IPv4 source and remote")
	}
	copy(spec.sourceIP[:], source4)
	copy(spec.remoteIP[:], remote4)
	spec.sourcePort = uint16(sourceAddr.Port)
	spec.remotePort = uint16(remoteAddr.Port)
	return spec, nil
}

func fakeTCPRetireArg(args []string, name string) (string, bool) {
	prefix := name + "="
	for i := 0; i < len(args); i++ {
		if args[i] == name {
			if i+1 >= len(args) {
				return "", false
			}
			return args[i+1], true
		}
		if strings.HasPrefix(args[i], prefix) {
			return strings.TrimPrefix(args[i], prefix), true
		}
	}
	return "", false
}

func fakeTCPRetireResetPacket(spec fakeTCPRetireResetSpec, ipID uint16) []byte {
	buf := make([]byte, 64)
	return faketcp.MarshalIPv4TCPSACKInto(
		buf,
		spec.sourceIP,
		spec.remoteIP,
		spec.sourcePort,
		spec.remotePort,
		0,
		0,
		faketcp.FlagRST,
		0,
		nil,
		nil,
		ipID,
	)
}

func sendFakeTCPRetireReset(spec fakeTCPRetireResetSpec) error {
	dll, err := loadNpcapDLL()
	if err != nil {
		return err
	}
	defer dll.Release()
	openLive, err := dll.FindProc("pcap_open_live")
	if err != nil {
		return err
	}
	sendPacket, err := dll.FindProc("pcap_sendpacket")
	if err != nil {
		return err
	}
	closeProc, err := dll.FindProc("pcap_close")
	if err != nil {
		return err
	}
	getErr, err := dll.FindProc("pcap_geterr")
	if err != nil {
		return err
	}
	datalink, err := dll.FindProc("pcap_datalink")
	if err != nil {
		return err
	}
	setMode, err := dll.FindProc("pcap_setmode")
	if err != nil {
		return fmt.Errorf("Npcap pcap_setmode export missing during retirement: %w", err)
	}
	device, err := syscall.BytePtrFromString(spec.packetDevice)
	if err != nil {
		return err
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
		return fmt.Errorf("retirement pcap_open_live %q: %s", spec.packetDevice, msg)
	}
	defer closeProc.Call(handle)
	linkType, _, _ := datalink.Call(handle)
	if int32(linkType) != pcapDLTEthernet {
		return fmt.Errorf("retirement Npcap datalink=%d, want Ethernet(%d)", int32(linkType), pcapDLTEthernet)
	}
	if ret, _, _ := setMode.Call(handle, npcapModeSendToRxClear); int32(ret) != 0 {
		return fmt.Errorf("retirement pcap_setmode MODE_SENDTORX_CLEAR: %s", pcapHandleError(getErr, handle))
	}

	packet := fakeTCPRetireResetPacket(spec, uint16(time.Now().UnixNano()))
	frame := make([]byte, 14+len(packet))
	copy(frame[0:6], spec.nextHopMAC[:])
	copy(frame[6:12], spec.sourceMAC[:])
	binary.BigEndian.PutUint16(frame[12:14], 0x0800)
	copy(frame[14:], packet)
	for i := 0; i < fakeTCPRetireResetCopies; i++ {
		ret, _, _ := sendPacket.Call(handle, uintptr(unsafe.Pointer(&frame[0])), uintptr(len(frame)))
		if int32(ret) != 0 {
			return fmt.Errorf("retirement pcap_sendpacket: %s", pcapHandleError(getErr, handle))
		}
		if i+1 < fakeTCPRetireResetCopies {
			time.Sleep(fakeTCPRetireResetSpacing)
		}
	}
	return nil
}
