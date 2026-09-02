package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/realitymirror"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "server":
		err = runServer(os.Args[2:])
	case "client":
		err = runClient(os.Args[2:])
	default:
		usage()
		err = errors.New("mode must be client or server")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "WBD_REALITY_FRONT_FAIL", err)
		os.Exit(1)
	}
}

func runServer(args []string) error {
	fs := flag.NewFlagSet("server", flag.ContinueOnError)
	listen := fs.String("listen", "127.0.0.1:9443", "single TCP listener used for WBD takeover and genuine-target fallback")
	target := fs.String("target", "", "fixed fallback target host:port")
	serverName := fs.String("server-name", "", "target-looking SNI used by the WBD client and fallback")
	certFile := fs.String("cert", "", "local TLS certificate used only after a recognized WBD ClientHello")
	keyFile := fs.String("key", "", "private key for -cert")
	routeKey := fs.String("route-key", "", "classifier secret shared by WBD client/server; not account authentication")
	username := fs.String("username", "", "shared personal account username; concurrent devices may reuse it")
	password := fs.String("password", "", "shared personal account password; concurrent devices may reuse it")
	ticketDir := fs.String("ticket-dir", "/tmp/wbd-reality-front-tickets", "0700 local directory shared with wbd-link-proxy ticket admission")
	maxConns := fs.Int("max-conns", 64, "maximum concurrent front sessions")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *target == "" || *serverName == "" || *certFile == "" || *keyFile == "" || len(*routeKey) < 16 || *username == "" || *password == "" || *maxConns <= 0 {
		return errors.New("server requires target/server-name/cert/key, route-key >=16 bytes, username/password and positive max-conns")
	}
	cert, err := tls.LoadX509KeyPair(*certFile, *keyFile)
	if err != nil {
		return err
	}
	cfg := realityfront.ServerConfig{
		RouteKey: []byte(*routeKey), ServerName: *serverName,
		TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13},
		ExpectedUsername: *username, ExpectedPassword: *password, TicketDir: *ticketDir,
		HelloTimeout: 5 * time.Second,
		Mirror: realitymirror.Config{
			Target: *target, ServerName: *serverName,
			HelloTimeout: 5 * time.Second, DialTimeout: 5 * time.Second,
			SessionTimeout: 30 * time.Second, MaxHelloBytes: 64 << 10, MaxBytes: 32 << 20,
		},
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	defer ln.Close()
	go func() { <-ctx.Done(); _ = ln.Close() }()
	fmt.Printf("WBD_REALITY_FRONT_READY listen=%s target=%s server_name=%s takeover=tls13 fallback=mirror verify_client_cert=none auth=simple-userpass multi_session=1\n", ln.Addr(), *target, *serverName)
	sem := make(chan struct{}, *maxConns)
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			continue
		}
		select {
		case sem <- struct{}{}:
			go func(c net.Conn) {
				defer func() { <-sem; _ = c.Close() }()
				res, err := realityfront.HandleServerConnSimple(ctx, c, cfg)
				if err != nil {
					fmt.Fprintf(os.Stderr, "WBD_REALITY_FRONT_SESSION remote=%s err=%q\n", c.RemoteAddr(), err)
					return
				}
				if res.Branch == "wbd" {
					fmt.Printf("WBD_REALITY_FRONT_AUTH_OK remote=%s account=%s ticket=%s\n", c.RemoteAddr(), *username, res.Ticket.Hex())
				}
			}(conn)
		default:
			_ = conn.Close()
		}
	}
}

func runClient(args []string) error {
	fs := flag.NewFlagSet("client", flag.ContinueOnError)
	addr := fs.String("addr", "", "WBD server TCP address")
	serverName := fs.String("server-name", "", "target-looking SNI")
	routeKey := fs.String("route-key", "", "classifier secret shared with server")
	username := fs.String("username", "", "shared account username")
	password := fs.String("password", "", "shared account password")
	verifyServer := fs.Bool("verify-server", false, "verify certificate/hostname using system roots; default false accepts any cert/domain")
	ticketOut := fs.String("ticket-out", "", "optional 0600 file receiving only the one-time ticket hex")
	timeout := fs.Duration("timeout", 10*time.Second, "overall TCP/TLS/bootstrap timeout")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *addr == "" || *serverName == "" || len(*routeKey) < 16 || *username == "" || *password == "" || *timeout <= 0 {
		return errors.New("client requires addr/server-name, route-key >=16 bytes, username/password and positive timeout")
	}
	mr, err := realityfront.NewMarkerRand(rand.Reader, []byte(*routeKey), *serverName)
	if err != nil {
		return err
	}
	dialer := net.Dialer{Timeout: *timeout}
	raw, err := dialer.Dial("tcp", *addr)
	if err != nil {
		return err
	}
	defer raw.Close()
	_ = raw.SetDeadline(time.Now().Add(*timeout))
	cfg := &tls.Config{
		ServerName: *serverName,
		InsecureSkipVerify: !*verifyServer,
		MinVersion: tls.VersionTLS13, MaxVersion: tls.VersionTLS13,
		Rand: mr,
	}
	conn := tls.Client(raw, cfg)
	if err := conn.Handshake(); err != nil {
		return err
	}
	ticket, err := realityfront.BootstrapClientSimple(conn, *username, *password)
	if err != nil {
		return err
	}
	if strings.TrimSpace(*ticketOut) != "" {
		if err := os.WriteFile(*ticketOut, []byte(ticket.Hex()+"\n"), 0o600); err != nil {
			return err
		}
	}
	fmt.Printf("WBD_REALITY_FRONT_OK ticket=%s tls=%x verify_server=%t auth=simple-userpass\n", ticket.Hex(), conn.ConnectionState().Version, *verifyServer)
	return nil
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage:")
	fmt.Fprintln(os.Stderr, "  wbd-reality-front server -listen :443 -target HOST:443 -server-name HOST -cert self.pem -key self.key -route-key SECRET -username USER -password PASS -ticket-dir DIR")
	fmt.Fprintln(os.Stderr, "  wbd-reality-front client -addr SERVER:443 -server-name HOST -route-key SECRET -username USER -password PASS [-verify-server=false] [-ticket-out FILE]")
	fmt.Fprintln(os.Stderr, "Recognized WBD ClientHello sessions are taken over on the same TCP connection; unrecognized sessions are byte-preserving fallback to the fixed target. Shared username/password admission is one request inside TLS and may issue many independent concurrent session tickets. Sustained VPN payload never uses this TCP stream.")
}
