//go:build linux

package main

import (
	"context"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/netip"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/linuxserver"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/runtimeentry"
)

func main() {
	var (
		rawIface = flag.String("raw-interface", "", "Linux interface used for FakeTCP raw IPv4 I/O")
		listenIPText = flag.String("listen-ip", "", "public IPv4 address bound by the raw endpoint")
		listenPort = flag.Uint("listen-port", 443, "public FakeTCP/TLS port")
		tunName = flag.String("tun-name", "wbdg0", "shared server TUN name")
		leasePoolText = flag.String("lease-pool", "10.66.0.0/16", "server IPv4 lease pool")
		leaseText = flag.String("lease4", "", "static client lease /32 for this minimal entry")
		tunnelText = flag.String("tunnel-id", "", "32-hex Logical Tunnel ID")
		account = flag.String("account", "", "account identity bound to the static lease")
		installationText = flag.String("installation-id", "", "32-hex installation ID")
		serverName = flag.String("server-name", "", "recognized TLS server name")
		routeKeyHex = flag.String("route-key-hex", "", "hex route key used by Reality-like recognition")
		certPath = flag.String("tls-cert", "", "server certificate PEM")
		keyPath = flag.String("tls-key", "", "server private key PEM")
		username = flag.String("username", "", "protected admission username")
		password = flag.String("password", "", "protected admission password")
		decoy = flag.String("decoy", "", "ordinary TLS fallback target host:port")
		serverLimit = flag.Uint("server-record-limit", 1250, "client-to-server TLS-like record wire limit")
		mtu = flag.Int("mtu", 1500, "connection/shared-TUN MTU")
		firewall = flag.String("firewall", "auto", "shared-TUN firewall backend: auto|nft|iptables")
		nftForward = flag.String("nft-forward", "", "optional family:table:chain for nft forward policy")
	)
	flag.Parse()

	if *rawIface == "" || *listenIPText == "" || *leaseText == "" || *tunnelText == "" ||
		*account == "" || *installationText == "" || *serverName == "" || *routeKeyHex == "" ||
		*certPath == "" || *keyPath == "" || *username == "" || *password == "" || *decoy == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *listenPort == 0 || *listenPort > 65535 || *serverLimit > 65535 {
		log.Fatal("invalid port or record limit")
	}

	listenIP, err := netip.ParseAddr(*listenIPText)
	if err != nil || !listenIP.Is4() {
		log.Fatal("listen-ip must be IPv4")
	}
	leasePrefix, err := netip.ParsePrefix(*leaseText)
	if err != nil || !leasePrefix.Addr().Is4() || leasePrefix.Bits() != 32 {
		log.Fatal("lease4 must be IPv4 /32")
	}
	leasePool, err := netip.ParsePrefix(*leasePoolText)
	if err != nil || !leasePool.Addr().Is4() {
		log.Fatal("lease-pool must be IPv4")
	}
	tunnelID, err := logicaltunnel.ParseTunnelID(*tunnelText)
	if err != nil {
		log.Fatal(err)
	}
	installation, err := logicaltunnel.ParseInstallationID(*installationText)
	if err != nil {
		log.Fatal(err)
	}
	routeKey, err := hex.DecodeString(*routeKeyHex)
	if err != nil || len(routeKey) == 0 {
		log.Fatal("route-key-hex must decode to non-empty bytes")
	}
	cert, err := tls.LoadX509KeyPair(*certPath, *keyPath)
	if err != nil {
		log.Fatal(err)
	}
	lease := logicaltunnel.Lease{
		Account: *account,
		InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: leasePrefix.String(),
			Routes4: []string{"0.0.0.0/0"},
		},
	}
	if err := lease.Validate(); err != nil {
		log.Fatal(err)
	}

	plan, err := linuxserver.BuildNetworkPlan(*tunName, leasePool, *mtu, linuxserver.FirewallBackend(*firewall), *nftForward)
	if err != nil {
		log.Fatal(err)
	}
	network, err := linuxserver.OpenRuntime(plan)
	if err != nil {
		log.Fatal(err)
	}
	defer network.Close()

	router, err := linuxserver.NewSharedTUNRouter(leasePool, 4096, network.TUN())
	if err != nil {
		log.Fatal(err)
	}
	raw, err := faketcp.OpenRawIPv4Endpoint(*rawIface, listenIP.As4(), faketcp.PacketPersonaLegacy)
	if err != nil {
		log.Fatal(err)
	}
	io := runtimeentry.SegmentIO{
		Read: func() (faketcp.Segment, error) {
			seg, _, err := raw.ReadSegment()
			return seg, err
		},
		Emit: func(seg faketcp.Segment) error {
			_, err := raw.WriteSegment(seg)
			return err
		},
		Close: raw.Close,
	}
	server, err := runtimeentry.NewServer(runtimeentry.ServerConfig{
		IO: io,
		ListenPort: uint16(*listenPort),
		Admission: realityfront.ServerAdmissionConfig{
			TLS: realityfront.ServerConfig{
				ServerName: *serverName,
				RouteKey: routeKey,
				TLSConfig: &tls.Config{Certificates: []tls.Certificate{cert}},
				Timeout: 15 * time.Second,
			},
			ExpectedUsername: *username,
			ExpectedPassword: *password,
			ServerLimit: uint16(*serverLimit),
		},
		Fallback: realityfront.FallbackConfig{
			Target: *decoy, ServerName: *serverName,
			DialTimeout: 5 * time.Second, SessionTimeout: 2 * time.Minute,
			MaxBytes: 64 << 20,
		},
		LookupLease: func(id logicaltunnel.TunnelID) (logicaltunnel.Lease, error) {
			if id != tunnelID {
				return logicaltunnel.Lease{}, logicaltunnel.ErrUnknownTunnel
			}
			return lease.Clone(), nil
		},
		Lane: datapath.ServerLaneParams{
			ConnectionMTU: *mtu,
			TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
			RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
		},
		Router: router,
		Service: platformflow.DefaultServerConfig(),
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	errCh := make(chan error, 2)
	go func() { errCh <- server.Run(ctx) }()
	go func() {
		buf := make([]byte, 65535)
		for {
			n, err := network.TUN().ReadPacket(buf)
			if err != nil {
				errCh <- err
				return
			}
			packet := append([]byte(nil), buf[:n]...)
			if err := server.RoutePacket(packet, time.Now()); err != nil {
				if errors.Is(err, runtimeentry.ErrTunnelNotQualified) || errors.Is(err, linuxserver.ErrNoLeaseRoute) {
					continue
				}
				errCh <- err
				return
			}
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("wbd-server stopped: %v", err)
		}
		cancel()
	}
	_ = server.Close()
	fmt.Println("WBD_SERVER_STOPPED cleanup=owned-only")
}
