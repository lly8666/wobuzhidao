package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/lly8666/wobuzhidao/internal/tunnel"
)

type sourceIPv4Endpoint struct {
	raw           tunnel.Endpoint
	expected      [4]byte
	expectedText  string
	mismatchOnce  sync.Once
	malformedOnce sync.Once
}

func newSourceIPv4Endpoint(raw tunnel.Endpoint, expected netip.Addr) (*sourceIPv4Endpoint, error) {
	if raw == nil {
		return nil, fmt.Errorf("source IPv4 filter endpoint is required")
	}
	expected = expected.Unmap()
	if !expected.Is4() {
		return nil, fmt.Errorf("expected source IPv4 must be IPv4")
	}
	return &sourceIPv4Endpoint{raw: raw, expected: expected.As4(), expectedText: expected.String()}, nil
}

func (e *sourceIPv4Endpoint) ReadPacket(p []byte) (int, error) {
	for {
		n, err := e.raw.ReadPacket(p)
		if err != nil {
			return n, err
		}
		if n < 20 || p[0]>>4 != 4 {
			e.malformedOnce.Do(func() {
				fmt.Fprintln(os.Stderr, "WBD_TUN_SOURCE_IPV4_DROP reason=non_ipv4_or_short fail_closed=1")
			})
			continue
		}
		ihl := int(p[0]&0x0f) * 4
		if ihl < 20 || ihl > n {
			e.malformedOnce.Do(func() {
				fmt.Fprintln(os.Stderr, "WBD_TUN_SOURCE_IPV4_DROP reason=invalid_ipv4_header fail_closed=1")
			})
			continue
		}
		var source [4]byte
		copy(source[:], p[12:16])
		if source != e.expected {
			actual := netip.AddrFrom4(source).String()
			e.mismatchOnce.Do(func() {
				fmt.Fprintf(os.Stderr, "WBD_TUN_SOURCE_IPV4_DROP expected=%s actual=%s fail_closed=1\n", e.expectedText, actual)
			})
			continue
		}
		return n, nil
	}
}

func (e *sourceIPv4Endpoint) WritePacket(p []byte) (int, error) { return e.raw.WritePacket(p) }
func (e *sourceIPv4Endpoint) Close() error                      { return e.raw.Close() }

func main() {
	var (
		mode               = flag.String("mode", "client", "client or server")
		ifname             = flag.String("ifname", "wbd0", "TUN interface name")
		mtu                = flag.Int("mtu", 1400, "maximum IP packet size")
		transport          = flag.String("transport", "127.0.0.1:4090", "client: local UDP transport target")
		local              = flag.String("local", "", "client: optional local UDP bind address")
		listen             = flag.String("listen", "127.0.0.1:4091", "server: UDP listen address for decoded transport")
		expectedSourceIPv4 = flag.String("expected-source-ipv4", "", "client: fail-closed source IPv4 lease for TUN outbound packets")
		runFor             = flag.Duration("run-for", 0, "optional qualification lifetime; 0 runs until signal")
	)
	flag.Parse()

	tunDev, err := tunnel.OpenTUN(*ifname)
	fatalIf(err)

	var tunEndpoint tunnel.Endpoint = tunDev
	var expectedSource netip.Addr
	if *expectedSourceIPv4 != "" {
		if *mode != "client" {
			fatalIf(fmt.Errorf("expected-source-ipv4 is client-only"))
		}
		expectedSource, err = netip.ParseAddr(*expectedSourceIPv4)
		fatalIf(err)
		filtered, err := newSourceIPv4Endpoint(tunDev, expectedSource)
		fatalIf(err)
		tunEndpoint = filtered
	}

	var raw tunnel.Endpoint
	switch *mode {
	case "client":
		remoteAddr, err := net.ResolveUDPAddr("udp", *transport)
		fatalIf(err)
		var localAddr *net.UDPAddr
		if *local != "" {
			localAddr, err = net.ResolveUDPAddr("udp", *local)
			fatalIf(err)
		}
		raw, err = tunnel.DialUDP(localAddr, remoteAddr)
		fatalIf(err)
	case "server":
		listenAddr, err := net.ResolveUDPAddr("udp", *listen)
		fatalIf(err)
		raw, err = tunnel.ListenLearnedUDP(listenAddr)
		fatalIf(err)
	default:
		fatalIf(fmt.Errorf("unknown mode %q", *mode))
	}

	transportEndpoint := &tunnel.FramedEndpoint{Raw: raw}
	bridge := &tunnel.Bridge{TUN: tunEndpoint, Transport: transportEndpoint, MTU: *mtu}

	if expectedSource.IsValid() {
		fmt.Fprintf(os.Stderr, "WBD_TUN_SOURCE_IPV4_FENCE expected=%s fail_closed=1\n", expectedSource.Unmap())
	}
	fmt.Fprintf(os.Stderr, "WBD_TUN_READY mode=%s ifname=%s mtu=%d\n", *mode, tunDev.Name(), *mtu)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if *runFor < 0 {
		fatalIf(fmt.Errorf("run-for must be non-negative"))
	}
	if *runFor > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, *runFor)
		defer cancel()
	}

	stats, err := bridge.Run(ctx)
	if err != nil {
		fatalIf(err)
	}
	enc := json.NewEncoder(os.Stdout)
	_ = enc.Encode(stats)
}

func fatalIf(err error) {
	if err == nil {
		return
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
