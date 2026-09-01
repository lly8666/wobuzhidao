package main

import (
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/lly8666/wobuzhidao/internal/platformproxy"
)

func main() {
	var listen, wbd, target string
	flag.StringVar(&listen, "listen", "", "local UDP address receiving raw benchmark datagrams")
	flag.StringVar(&wbd, "wbd", "", "local wbd-link-proxy UDP address")
	flag.StringVar(&target, "target", "", "server-side UDP benchmark target carried in platformproxy frames")
	flag.Parse()
	if listen == "" || wbd == "" || target == "" {
		fmt.Fprintln(os.Stderr, "WBD_BENCH_PLATFORM_ADAPTER_FAIL -listen, -wbd and -target are required")
		os.Exit(2)
	}
	listenAddr, err := net.ResolveUDPAddr("udp4", listen)
	if err != nil {
		fail(err)
	}
	wbdAddr, err := netip.ParseAddrPort(wbd)
	if err != nil || !wbdAddr.Addr().Unmap().Is4() {
		fail(fmt.Errorf("invalid -wbd %q", wbd))
	}
	targetAddr, err := netip.ParseAddrPort(target)
	if err != nil || !targetAddr.Addr().Unmap().Is4() {
		fail(fmt.Errorf("invalid -target %q", target))
	}
	conn, err := net.ListenUDP("udp4", listenAddr)
	if err != nil {
		fail(err)
	}
	defer conn.Close()
	_ = conn.SetReadBuffer(4 << 20)
	_ = conn.SetWriteBuffer(4 << 20)

	flowForPeer := make(map[netip.AddrPort]uint64)
	peerForFlow := make(map[uint64]netip.AddrPort)
	var nextFlow uint64 = 1
	buf := make([]byte, 65535)
	fmt.Printf("WBD_BENCH_PLATFORM_ADAPTER_READY listen=%s wbd=%s target=%s\n", conn.LocalAddr(), wbdAddr, targetAddr)
	for {
		n, peer, err := conn.ReadFromUDPAddrPort(buf)
		if err != nil {
			fail(err)
		}
		if peer == wbdAddr {
			frame, err := platformproxy.Unmarshal(buf[:n])
			if err != nil || frame.Kind != platformproxy.KindUDPDatagram {
				continue
			}
			client, ok := peerForFlow[frame.FlowID]
			if !ok {
				continue
			}
			if _, err := conn.WriteToUDPAddrPort(frame.Payload, client); err != nil {
				fail(err)
			}
			continue
		}
		flowID := flowForPeer[peer]
		if flowID == 0 {
			flowID = nextFlow
			nextFlow++
			if nextFlow == 0 {
				nextFlow = 1
			}
			flowForPeer[peer] = flowID
			peerForFlow[flowID] = peer
		}
		frame, err := platformproxy.Marshal(platformproxy.Frame{
			Kind:    platformproxy.KindUDPDatagram,
			FlowID:  flowID,
			Peer:    targetAddr,
			Payload: buf[:n],
		})
		if err != nil {
			fail(err)
		}
		if _, err := conn.WriteToUDPAddrPort(frame, wbdAddr); err != nil {
			fail(err)
		}
	}
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "WBD_BENCH_PLATFORM_ADAPTER_FAIL time=%s err=%v\n", time.Now().UTC().Format(time.RFC3339Nano), err)
	os.Exit(1)
}
