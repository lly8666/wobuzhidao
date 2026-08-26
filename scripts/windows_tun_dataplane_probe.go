//go:build windows

// windows_tun_dataplane_probe is a qualification peer for the Windows Wintun
// smoke gate. It is intentionally not a VPN transport: it proves that a real
// IPv4 ICMP packet can leave the Windows IP stack through Wintun/wbd-tun as a
// WBDP IP datagram and that a framed reply written back by the peer reaches the
// originating ping socket through Wintun.
package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"
)

const (
	headerLen = 8
	version1  = 1
	typeIP    = 1
)

var magic = [4]byte{'W', 'B', 'D', 'P'}

func main() {
	listen := flag.String("listen", "127.0.0.1:49999", "UDP listen address used by wbd-tun -transport")
	targetText := flag.String("target", "203.0.113.7", "IPv4 address that ping must target through Wintun")
	timeout := flag.Duration("timeout", 12*time.Second, "qualification receive timeout")
	flag.Parse()

	target, err := netip.ParseAddr(*targetText)
	fatalIf(err)
	if !target.Is4() {
		fatalIf(fmt.Errorf("target must be IPv4: %s", target))
	}
	addr, err := net.ResolveUDPAddr("udp4", *listen)
	fatalIf(err)
	conn, err := net.ListenUDP("udp4", addr)
	fatalIf(err)
	defer conn.Close()
	fatalIf(conn.SetReadDeadline(time.Now().Add(*timeout)))
	fmt.Printf("WBD_WINDOWS_TUN_DATAPLANE_PROBE_READY listen=%s target=%s\n", conn.LocalAddr(), target)

	buf := make([]byte, 65535)
	n, peer, err := conn.ReadFromUDP(buf)
	fatalIf(err)
	frame := append([]byte(nil), buf[:n]...)
	ip, err := parseFrame(frame)
	fatalIf(err)
	if len(ip) < 20 || ip[0]>>4 != 4 {
		fatalIf(fmt.Errorf("expected IPv4 packet, got %d bytes", len(ip)))
	}
	ihl := int(ip[0]&0x0f) * 4
	if ihl < 20 || ihl > len(ip) || int(binary.BigEndian.Uint16(ip[2:4])) != len(ip) {
		fatalIf(fmt.Errorf("malformed IPv4 header ihl=%d len=%d total=%d", ihl, len(ip), binary.BigEndian.Uint16(ip[2:4])))
	}
	if ip[9] != 1 || len(ip) < ihl+8 {
		fatalIf(fmt.Errorf("expected ICMP echo request protocol=%d len=%d ihl=%d", ip[9], len(ip), ihl))
	}
	dst := netip.AddrFrom4([4]byte{ip[16], ip[17], ip[18], ip[19]})
	src := netip.AddrFrom4([4]byte{ip[12], ip[13], ip[14], ip[15]})
	if dst != target {
		fatalIf(fmt.Errorf("unexpected IPv4 target %s want %s", dst, target))
	}
	if ip[ihl] != 8 || ip[ihl+1] != 0 {
		fatalIf(fmt.Errorf("expected ICMP echo request type/code, got %d/%d", ip[ihl], ip[ihl+1]))
	}

	replyIP := append([]byte(nil), ip...)
	copy(replyIP[12:16], ip[16:20])
	copy(replyIP[16:20], ip[12:16])
	replyIP[8] = 64
	replyIP[10], replyIP[11] = 0, 0
	binary.BigEndian.PutUint16(replyIP[10:12], checksum(replyIP[:ihl]))
	icmp := replyIP[ihl:]
	icmp[0] = 0
	icmp[1] = 0
	icmp[2], icmp[3] = 0, 0
	binary.BigEndian.PutUint16(icmp[2:4], checksum(icmp))

	reply := marshalFrame(replyIP)
	written, err := conn.WriteToUDP(reply, peer)
	fatalIf(err)
	if written != len(reply) {
		fatalIf(fmt.Errorf("short UDP reply %d/%d", written, len(reply)))
	}
	fmt.Printf("WBD_WINDOWS_TUN_DATAPLANE_PROBE_PASS source=%s target=%s ip_bytes=%d frame_bytes=%d\n", src, dst, len(replyIP), len(reply))
}

func parseFrame(frame []byte) ([]byte, error) {
	if len(frame) < headerLen || string(frame[:4]) != string(magic[:]) {
		return nil, fmt.Errorf("bad WBDP header")
	}
	if frame[4] != version1 || frame[5] != typeIP {
		return nil, fmt.Errorf("unexpected WBDP version/type %d/%d", frame[4], frame[5])
	}
	n := int(binary.BigEndian.Uint16(frame[6:8]))
	if n == 0 || len(frame) != headerLen+n {
		return nil, fmt.Errorf("bad WBDP payload length encoded=%d actual=%d", n, len(frame)-headerLen)
	}
	return frame[headerLen:], nil
}

func marshalFrame(ip []byte) []byte {
	frame := make([]byte, headerLen+len(ip))
	copy(frame[:4], magic[:])
	frame[4] = version1
	frame[5] = typeIP
	binary.BigEndian.PutUint16(frame[6:8], uint16(len(ip)))
	copy(frame[headerLen:], ip)
	return frame
}

func checksum(b []byte) uint16 {
	var sum uint32
	for len(b) >= 2 {
		sum += uint32(binary.BigEndian.Uint16(b[:2]))
		b = b[2:]
	}
	if len(b) == 1 {
		sum += uint32(b[0]) << 8
	}
	for sum>>16 != 0 {
		sum = (sum & 0xffff) + (sum >> 16)
	}
	return ^uint16(sum)
}

func fatalIf(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "WBD_WINDOWS_TUN_DATAPLANE_PROBE_FAIL", err)
	os.Exit(1)
}
