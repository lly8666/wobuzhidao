package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/platformproxy"
)

func main() {
	var listen string
	var udpIdle time.Duration
	flag.StringVar(&listen, "listen", "", "local UDP service address used by wbd-link-server-mux")
	flag.DurationVar(&udpIdle, "udp-idle", 60*time.Second, "idle timeout for one proxied UDP flow")
	flag.Parse()

	if listen == "" || udpIdle <= 0 {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_SERVER_FAIL -listen and positive -udp-idle are required")
		os.Exit(2)
	}
	addr, err := net.ResolveUDPAddr("udp4", listen)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_SERVER_FAIL", err)
		os.Exit(1)
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_SERVER_FAIL", err)
		os.Exit(1)
	}
	defer conn.Close()

	relay, err := platformproxy.NewUDPRelay(conn, udpIdle)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_SERVER_FAIL", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	fmt.Printf("WBD_PLATFORM_PROXY_SERVER_READY listen=%s udp=1 tcp=0 session_isolation=service_peer+flow_id\n", conn.LocalAddr())
	if err := relay.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_SERVER_FAIL", err)
		os.Exit(1)
	}
}
