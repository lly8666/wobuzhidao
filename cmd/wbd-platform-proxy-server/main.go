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
	var tcpIdle time.Duration
	flag.StringVar(&listen, "listen", "", "local UDP service address used by wbd-link-server-mux")
	flag.DurationVar(&udpIdle, "udp-idle", 60*time.Second, "idle timeout for one full-cone UDP mapping")
	flag.DurationVar(&tcpIdle, "tcp-idle", 90*time.Second, "idle timeout for one proxied TCP flow")
	flag.Parse()

	if listen == "" || udpIdle <= 0 || tcpIdle <= 0 {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_SERVER_FAIL -listen and positive -udp-idle/-tcp-idle are required")
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

	cfg := platformproxy.DefaultRelayConfig()
	cfg.UDPIdleTimeout = udpIdle
	cfg.TCP.IdleTimeout = tcpIdle
	relay, err := platformproxy.NewRelay(conn, cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_SERVER_FAIL", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	fmt.Printf("WBD_PLATFORM_PROXY_SERVER_READY listen=%s udp=1 udp_fullcone=1 tcp=1 session_isolation=service_peer+flow_id\n", conn.LocalAddr())
	if err := relay.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, net.ErrClosed) {
		fmt.Fprintln(os.Stderr, "WBD_PLATFORM_PROXY_SERVER_FAIL", err)
		os.Exit(1)
	}
}
