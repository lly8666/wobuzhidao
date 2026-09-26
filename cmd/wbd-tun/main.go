package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/lly8666/wobuzhidao/internal/tunnel"
)

func main() {
	var (
		mode      = flag.String("mode", "client", "client or server")
		ifname    = flag.String("ifname", "wbd0", "TUN interface name")
		mtu       = flag.Int("mtu", 1400, "maximum IP packet size")
		transport = flag.String("transport", "127.0.0.1:4090", "client: local UDP transport target")
		local     = flag.String("local", "", "client: optional local UDP bind address")
		listen    = flag.String("listen", "127.0.0.1:4091", "server: UDP listen address for decoded transport")
		runFor    = flag.Duration("run-for", 0, "optional qualification lifetime; 0 runs until signal")
	)
	flag.Parse()

	tunDev, err := tunnel.OpenTUN(*ifname)
	fatalIf(err)

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
	bridge := &tunnel.Bridge{
		TUN:       tunDev,
		Transport: transportEndpoint,
		MTU:       *mtu,
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
