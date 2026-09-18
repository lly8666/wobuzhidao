//go:build windows

package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"strings"
	"time"
)

const (
	splitHeaderLen = 8
	splitVersion1  = 1
	splitTypeIP    = 1
)

var splitMagic = [4]byte{'W', 'B', 'D', 'P'}

type flowMatch struct {
	protocol byte
	dst      netip.Addr
}

func main() {
	mode := flag.String("mode", "client", "client or observer")
	tcpTarget := flag.String("tcp-target", "", "IPv4 TCP target host:port")
	udpTarget := flag.String("udp-target", "", "IPv4 UDP DNS target host:port")
	transport := flag.String("transport", "127.0.0.1:49998", "observer UDP transport listen address")
	expectation := flag.String("expectation", "expect", "observer: expect or forbid matching TCP/UDP frames")
	timeout := flag.Duration("timeout", 8*time.Second, "operation timeout")
	requireSuccess := flag.Bool("require-success", false, "client: require both TCP and UDP probes to receive responses")
	flag.Parse()

	tcpAddr, tcpIP, err := parseTarget(*tcpTarget)
	fatalIf(err)
	udpAddr, udpIP, err := parseTarget(*udpTarget)
	fatalIf(err)

	switch *mode {
	case "client":
		errTCP := probeTCP(tcpAddr, *timeout)
		errUDP := probeDNS(udpAddr, *timeout)
		fmt.Printf("WBD_WINDOWS_TUN_SPLIT_CLIENT tcp_ok=%d udp_ok=%d tcp_target=%s udp_target=%s\n", boolInt(errTCP == nil), boolInt(errUDP == nil), tcpAddr, udpAddr)
		if *requireSuccess {
			if errTCP != nil {
				fatalIf(fmt.Errorf("TCP direct probe: %w", errTCP))
			}
			if errUDP != nil {
				fatalIf(fmt.Errorf("UDP direct probe: %w", errUDP))
			}
		}
	case "observer":
		fatalIf(observeTransport(*transport, *expectation, []flowMatch{{protocol: 6, dst: tcpIP}, {protocol: 17, dst: udpIP}}, *timeout))
	default:
		fatalIf(fmt.Errorf("unknown mode %q", *mode))
	}
}

func parseTarget(value string) (string, netip.Addr, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", netip.Addr{}, errors.New("target is required")
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || port == "" {
		return "", netip.Addr{}, fmt.Errorf("target must be IPv4 host:port: %q", value)
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || !ip.Is4() {
		return "", netip.Addr{}, fmt.Errorf("target host must be IPv4: %q", host)
	}
	return net.JoinHostPort(ip.String(), port), ip, nil
}

func probeTCP(target string, timeout time.Duration) error {
	conn, err := net.DialTimeout("tcp4", target, timeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

func probeDNS(target string, timeout time.Duration) error {
	conn, err := net.DialTimeout("udp4", target, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	deadline := time.Now().Add(timeout)
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	query := dnsAQuery(0x5742, "example.com")
	if _, err := conn.Write(query); err != nil {
		return err
	}
	reply := make([]byte, 2048)
	n, err := conn.Read(reply)
	if err != nil {
		return err
	}
	if n < 12 || binary.BigEndian.Uint16(reply[:2]) != 0x5742 || reply[2]&0x80 == 0 {
		return fmt.Errorf("invalid DNS response bytes=%d", n)
	}
	return nil
}

func dnsAQuery(id uint16, name string) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint16(b[0:2], id)
	binary.BigEndian.PutUint16(b[2:4], 0x0100)
	binary.BigEndian.PutUint16(b[4:6], 1)
	for _, label := range strings.Split(name, ".") {
		b = append(b, byte(len(label)))
		b = append(b, label...)
	}
	b = append(b, 0)
	b = append(b, 0, 1, 0, 1)
	return b
}

func observeTransport(listen, expectation string, matches []flowMatch, timeout time.Duration) error {
	addr, err := net.ResolveUDPAddr("udp4", listen)
	if err != nil {
		return err
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	fmt.Printf("WBD_WINDOWS_TUN_SPLIT_OBSERVER_READY listen=%s expectation=%s\n", conn.LocalAddr(), expectation)

	seen := make(map[byte]bool)
	buf := make([]byte, 65535)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if err != nil {
			var ne net.Error
			if errors.As(err, &ne) && ne.Timeout() {
				if expectation == "forbid" {
					fmt.Println("WBD_WINDOWS_TUN_SPLIT_OBSERVER_PASS expectation=forbid tcp=0 udp=0")
					return nil
				}
				return fmt.Errorf("observer timeout tcp=%d udp=%d", boolInt(seen[6]), boolInt(seen[17]))
			}
			return err
		}
		ip, err := parseSplitFrame(buf[:n])
		if err != nil || len(ip) < 20 || ip[0]>>4 != 4 {
			continue
		}
		ihl := int(ip[0]&0x0f) * 4
		if ihl < 20 || ihl > len(ip) {
			continue
		}
		dst := netip.AddrFrom4([4]byte{ip[16], ip[17], ip[18], ip[19]})
		proto := ip[9]
		matched := false
		for _, want := range matches {
			if want.protocol == proto && want.dst == dst {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if expectation == "forbid" {
			return fmt.Errorf("direct flow leaked into proxy transport protocol=%d dst=%s", proto, dst)
		}
		if expectation != "expect" {
			return fmt.Errorf("unknown observer expectation %q", expectation)
		}
		seen[proto] = true
		if seen[6] && seen[17] {
			fmt.Println("WBD_WINDOWS_TUN_SPLIT_OBSERVER_PASS expectation=expect tcp=1 udp=1")
			return nil
		}
	}
}

func parseSplitFrame(frame []byte) ([]byte, error) {
	if len(frame) < splitHeaderLen || string(frame[:4]) != string(splitMagic[:]) {
		return nil, errors.New("bad WBDP header")
	}
	if frame[4] != splitVersion1 || frame[5] != splitTypeIP {
		return nil, errors.New("unexpected WBDP frame")
	}
	n := int(binary.BigEndian.Uint16(frame[6:8]))
	if n == 0 || len(frame) != splitHeaderLen+n {
		return nil, errors.New("bad WBDP length")
	}
	return frame[splitHeaderLen:], nil
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func fatalIf(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, "WBD_WINDOWS_TUN_SPLIT_PROBE_FAIL", err)
	os.Exit(1)
}
