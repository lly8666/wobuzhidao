//go:build linux

package faketcp

import (
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"syscall"
	"testing"
	"time"

	"golang.org/x/net/bpf"
	"golang.org/x/sys/unix"
)

func rawFilterFrame(port uint16, flags uint8) []byte {
	ip := MarshalSegment(Segment{SrcIP: [4]byte{127, 0, 0, 2}, DstIP: [4]byte{127, 0, 0, 1}, SrcPort: 44444, DstPort: port, Seq: 4242, Flags: flags, Window: 1234}, 1, PacketPersonaLegacy)
	frame := make([]byte, 14+len(ip))
	frame[12], frame[13] = 8, 0
	copy(frame[14:], ip)
	return frame
}

func TestRawPortFilterMatchesOnlyDestinationWithoutPersonaGate(t *testing.T) {
	program := rawReceivePortFilter([4]byte{127, 0, 0, 1}, 24343)
	raw := make([]bpf.RawInstruction, len(program))
	for i, p := range program {
		raw[i] = bpf.RawInstruction{Op: p.Code, Jt: p.Jt, Jf: p.Jf, K: p.K}
	}
	decoded, ok := bpf.Disassemble(raw)
	if !ok {
		t.Fatal("filter instructions not fully decoded")
	}
	vm, err := bpf.NewVM(decoded)
	if err != nil {
		t.Fatal(err)
	}
	for _, flags := range []uint8{FlagSYN, FlagSYN | 0xc0, FlagACK, FlagPSH | FlagACK, FlagFIN | FlagACK, FlagRST} {
		frame := rawFilterFrame(24343, flags)
		got, err := vm.Run(frame)
		if err != nil || got == 0 {
			t.Fatalf("flags%x rejected: got%d err%v", flags, got, err)
		}
	}
	base := rawFilterFrame(24343, FlagACK)
	ipOptions := append([]byte(nil), base[:34]...)
	ipOptions = append(ipOptions, 1, 1, 1, 1)
	ipOptions = append(ipOptions, base[34:]...)
	ipOptions[14] = 0x46
	binary.BigEndian.PutUint16(ipOptions[16:18], uint16(len(ipOptions)-14))
	got, err := vm.Run(ipOptions)
	if err != nil || got == 0 {
		t.Fatalf("variable IPv4 IHL rejected: %d %v", got, err)
	}
	for _, kind := range []string{"port", "destination", "protocol", "fragment-offset", "short", "ethernet"} {
		frame := append([]byte(nil), base...)
		switch kind {
		case "port":
			frame = rawFilterFrame(22, FlagACK)
		case "destination":
			frame[33] = 3
		case "protocol":
			frame[23] = 17
		case "fragment-offset":
			frame[21] = 1
		case "short":
			frame = frame[:18]
		case "ethernet":
			frame[12] = 0x86
			frame[13] = 0xdd
		}
		got, err := vm.Run(frame)
		if err != nil || got != 0 {
			t.Fatalf("unrelated %s admitted: %d %v", kind, got, err)
		}
	}
}

// Explicit isolated Actions gate, not a benchmark. Exercise real AF_PACKET
// ingress, SO_ATTACH_FILTER, default all-port compatibility and owned cleanup.
func TestRawPortFilterKernelIngress(t *testing.T) {
	if os.Getenv("WBD_RAW_PORT_FILTER_KERNEL") != "1" {
		t.Skip("requires explicit isolated Actions kernel fixture")
	}
	if os.Getenv("GITHUB_ACTIONS") != "true" {
		t.Fatal("kernel qualification belongs in Actions")
	}
	local := [4]byte{127, 0, 0, 1}
	filtered, err := OpenRawIPv4EndpointForPort("lo", local, PacketPersonaLegacy, 24343)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { filtered.Close() })
	all, err := OpenRawIPv4Endpoint("lo", local, PacketPersonaLegacy)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { all.Close() })
	if d := filtered.IODiagnostic(); d.ReceivePort != 24343 || !d.KernelPortFilter {
		t.Fatalf("filter not installed: %+v", d)
	}
	if err = syscall.SetNonblock(filtered.recvFD, true); err != nil {
		t.Fatal(err)
	}
	if err = syscall.SetNonblock(all.recvFD, true); err != nil {
		t.Fatal(err)
	}
	sender, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_RAW, syscall.IPPROTO_RAW)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { syscall.Close(sender) })
	if err = syscall.SetsockoptInt(sender, syscall.IPPROTO_IP, syscall.IP_HDRINCL, 1); err != nil {
		t.Fatal(err)
	}
	for _, port := range []uint16{24343, 24344} {
		frame := rawFilterFrame(port, FlagACK|FlagPSH)
		if err = syscall.Sendto(sender, frame[14:], 0, &syscall.SockaddrInet4{Addr: local}); err != nil {
			t.Fatal(err)
		}
	}
	collect := func(endpoint *RawIPv4Endpoint) map[uint16][]byte {
		result := map[uint16][]byte{}
		buf := make([]byte, 65536+64)
		deadline := time.Now().Add(200 * time.Millisecond)
		for time.Now().Before(deadline) {
			n, from, err := syscall.Recvfrom(endpoint.recvFD, buf, 0)
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				time.Sleep(time.Millisecond)
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			if ll, ok := from.(*syscall.SockaddrLinklayer); ok && ll.Pkttype == rawPacketOutgoing {
				continue
			}
			ip := rawExtractIPv4(buf[:n])
			seg, err := ParseIPv4TCP(ip)
			if err == nil && seg.Seq == 4242 && seg.SrcPort == 44444 {
				result[seg.DstPort] = append([]byte(nil), ip...)
			}
		}
		return result
	}
	kept, unfiltered := collect(filtered), collect(all)
	if len(kept) != 1 || kept[24343] == nil || !bytes.Equal(kept[24343], unfiltered[24343]) || unfiltered[24344] == nil {
		t.Fatalf("wrong ingress filtering/bytes: filtered%v default%v", kept, unfiltered)
	}
}
