//go:build ignore

// single_flow_decoy is an integration-test helper for the single-flow active-
// probe/fallback gate. It is deliberately not a product binary. The helper
// accepts one ordinary TCP/TLS 1.3 connection from the mux fallback path and
// speaks the current encrypted V2 admission exchange so a wrong-marker
// FakeTCP client can prove that its original ClientHello and subsequent TLS
// stream were transparently proxied to the decoy without opening another
// public flow.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"os"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:44444", "ordinary TCP decoy listen address")
	certPath := flag.String("cert", "", "TLS certificate")
	keyPath := flag.String("key", "", "TLS private key")
	username := flag.String("username", "probe", "test admission username")
	password := flag.String("password", "probe-password", "test admission password")
	ticketDir := flag.String("ticket-dir", "", "test-only ticket directory")
	flag.Parse()
	if *certPath == "" || *keyPath == "" || *ticketDir == "" || *username == "" || *password == "" {
		fmt.Fprintln(os.Stderr, "single_flow_decoy: cert, key, ticket-dir and credentials are required")
		os.Exit(2)
	}
	cert, err := tls.LoadX509KeyPair(*certPath, *keyPath)
	if err != nil { fail(err) }
	pool := netip.MustParsePrefix("10.77.0.0/24")
	routes := []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}
	leases, err := logicaltunnel.NewManager(pool, routes)
	if err != nil { fail(err) }
	ln, err := net.Listen("tcp4", *listen)
	if err != nil { fail(err) }
	defer ln.Close()
	fmt.Printf("WBD_SINGLE_FLOW_DECOY_READY listen=%s logical_tunnel_v2=1\n", *listen)
	conn, err := ln.Accept()
	if err != nil { fail(err) }
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	tlsConn := tls.Server(conn, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion: tls.VersionTLS13,
		MaxVersion: tls.VersionTLS13,
	})
	if err := tlsConn.Handshake(); err != nil { fail(err) }
	result, err := realityfront.BootstrapServerSimpleV2(tlsConn, *username, *password, *ticketDir, leases, time.Now())
	if err != nil { fail(err) }
	state := tlsConn.ConnectionState()
	fmt.Printf("WBD_SINGLE_FLOW_DECOY_AUTH_PASS version=%x server_name=%s ticket_nonzero=%t tunnel_id=%s address4=%s\n",
		state.Version, state.ServerName, result.Ticket != (realityfront.Ticket{}), result.Config.TunnelID, result.Config.Address4)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "single_flow_decoy:", err)
	os.Exit(1)
}
