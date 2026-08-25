package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

type resultLog struct {
	Remote       string   `json:"remote"`
	OK           bool     `json:"ok"`
	Error        string   `json:"error,omitempty"`
	ServerName   string   `json:"server_name,omitempty"`
	ALPN         []string `json:"alpn,omitempty"`
	Target       string   `json:"target"`
	UpBytes      int64    `json:"up_bytes"`
	DownBytes    int64    `json:"down_bytes"`
	DurationMS   float64  `json:"duration_ms"`
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "server":
		runServer(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
}

func runServer(args []string) {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	listen := fs.String("listen", "127.0.0.1:9443", "TCP listen address; bind publicly only for deliberate testing")
	target := fs.String("target", "", "fixed genuine TLS target host:port")
	serverName := fs.String("server-name", "", "exact SNI allowed and expected at the target")
	helloTimeout := fs.Duration("hello-timeout", 5*time.Second, "maximum time to receive ClientHello")
	dialTimeout := fs.Duration("dial-timeout", 5*time.Second, "target TCP dial timeout")
	sessionTimeout := fs.Duration("session-timeout", 30*time.Second, "hard lifetime of one mirrored session; 0 disables")
	maxHello := fs.Int("max-hello-bytes", 64<<10, "maximum buffered TLS ClientHello bytes")
	maxBytes := fs.Int64("max-bytes", 32<<20, "maximum bytes copied per direction after ClientHello; 0 disables")
	maxConns := fs.Int("max-conns", 32, "maximum concurrent mirror sessions")
	_ = fs.Parse(args)

	if strings.TrimSpace(*target) == "" || strings.TrimSpace(*serverName) == "" || *maxConns <= 0 || *maxConns > 4096 {
		fatal(errors.New("server requires -target, -server-name, and a sane positive -max-conns"))
	}
	cfg := realitymirror.Config{
		Target:         *target,
		ServerName:     *serverName,
		HelloTimeout:   *helloTimeout,
		DialTimeout:    *dialTimeout,
		SessionTimeout: *sessionTimeout,
		MaxHelloBytes:  *maxHello,
		MaxBytes:       *maxBytes,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", *listen)
	if err != nil {
		fatal(err)
	}
	defer ln.Close()
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	ready := map[string]any{
		"listen": ln.Addr().String(), "target": *target, "server_name": *serverName,
		"session_timeout_ms": float64(sessionTimeout.Milliseconds()), "max_bytes_per_direction": *maxBytes,
	}
	printJSON("WBD_REALITY_MIRROR_READY", ready)

	sem := make(chan struct{}, *maxConns)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			fmt.Fprintln(os.Stderr, "WBD_REALITY_MIRROR_ACCEPT_FAIL", err)
			continue
		}
		select {
		case sem <- struct{}{}:
			go func(c net.Conn) {
				defer func() { <-sem }()
				handleOne(ctx, c, cfg)
			}(conn)
		default:
			_ = conn.Close()
			printJSON("WBD_REALITY_MIRROR_RESULT", resultLog{Remote: conn.RemoteAddr().String(), OK: false, Error: "concurrency limit", Target: cfg.Target})
		}
	}
}

func handleOne(ctx context.Context, conn net.Conn, cfg realitymirror.Config) {
	remote := conn.RemoteAddr().String()
	defer conn.Close()
	start := time.Now()
	result, err := realitymirror.Handle(ctx, conn, cfg)
	row := resultLog{
		Remote: remote, OK: err == nil, Target: cfg.Target,
		ServerName: result.Hello.ServerName, ALPN: result.Hello.ALPN,
		UpBytes: result.UpBytes, DownBytes: result.DownBytes,
		DurationMS: float64(time.Since(start)) / float64(time.Millisecond),
	}
	if err != nil {
		row.Error = err.Error()
	}
	printJSON("WBD_REALITY_MIRROR_RESULT", row)
}

func printJSON(prefix string, value any) {
	b, err := json.Marshal(value)
	if err != nil {
		fmt.Fprintln(os.Stderr, prefix, err)
		return
	}
	fmt.Printf("%s %s\n", prefix, b)
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  wbd-reality-mirror server -listen 127.0.0.1:9443 -target HOST:443 -server-name HOST")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "The server mirrors one fixed genuine TLS target. Use wbd-tls-diag or a normal TLS/HTTP client against the mirror address while keeping SNI/hostname set to -server-name.")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "WBD_REALITY_MIRROR_FAIL", err)
	os.Exit(1)
}
