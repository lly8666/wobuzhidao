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
	"path/filepath"
	"encoding/json"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/configfile"
	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/linuxserver"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/qualificationdiag"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/runtimeentry"
)

func main() { if err := runServer(); err != nil { log.Fatal(err) } }

func runServer() error {
	var (
		checkConfig        = flag.Bool("check-config", false, "validate server configuration without network changes; credentials redacted")
		statePath = flag.String("state-path", "", "absolute owned network journal path; empty keeps legacy unmanaged launch")
		recoverNetwork = flag.Bool("recover-network", false, "recover owned network journal and exit; requires state-path")
		maxClients = flag.Int("max-clients", 256, "maximum seven-day in-memory automatic client leases; 1..4096")
		configPath         = flag.String("config", "", "JSON configuration file; CLI flags override matching keys")
		keepalive          = flag.Duration("keepalive-interval", runtimeentry.DefaultKeepaliveInterval, "authenticated lane heartbeat interval; minimum 1s")
		rawIface           = flag.String("raw-interface", "", "Linux interface used for FakeTCP raw IPv4 I/O")
		listenIPText       = flag.String("listen-ip", "", "public IPv4 address bound by the raw endpoint")
		listenPort         = flag.Uint("listen-port", 443, "public FakeTCP/TLS port")
		tunName            = flag.String("tun-name", "wbdg0", "shared server TUN name")
		leasePoolText      = flag.String("lease-pool", "10.66.0.0/16", "server IPv4 lease pool")
		leaseText          = flag.String("lease4", "", "static client lease /32 for this minimal entry")
		tunnelText         = flag.String("tunnel-id", "", "32-hex Logical Tunnel ID")
		account            = flag.String("account", "", "account identity bound to the static lease")
		installationText   = flag.String("installation-id", "", "32-hex installation ID")
		serverName         = flag.String("server-name", "", "recognized TLS server name")
		routeKeyHex        = flag.String("route-key-hex", "", "hex route key used by Reality-like recognition")
		certPath           = flag.String("tls-cert", "", "server certificate PEM")
		keyPath            = flag.String("tls-key", "", "server private key PEM")
		username           = flag.String("username", "", "protected admission username")
		password           = flag.String("password", "", "protected admission password")
		decoy              = flag.String("decoy", "", "ordinary TLS fallback target host:port")
		serverLimit        = flag.Uint("server-record-limit", 1250, "client-to-server TLS-like record wire limit")
		mtu                = flag.Int("mtu", 1500, "connection/shared-TUN MTU")
		tlsStartupPadding  = flag.Bool("tls-startup-padding", false, "bounded passive inner TLS startup padding; no waiting; default off")
		fecParity          = flag.Int("fec-parity", 0, "fixed FEC parity shards: 0=off; allowed 4,8,10,12,16,20")
		firewall           = flag.String("firewall", "auto", "shared-TUN firewall backend: auto|nft|iptables")
		nftForward         = flag.String("nft-forward", "", "optional family:table:chain for nft forward policy")
		lanes              = flag.Int("lanes", 1, "authoritative transport lanes: 1=Normal, 2..4=Game racing")
		idleDormant        = flag.Duration("idle-dormant", 0, "enter DORMANT after payload idle duration; 0 disables")
		diagnosticJSONL    = flag.String("diagnostic-jsonl", "", "optional qualification diagnostics JSONL path; disabled by default")
		diagnosticInterval = flag.Duration("diagnostic-interval", time.Second, "qualification diagnostics sample interval")
	)
	flag.Parse()
	if err := configfile.ApplyFile(flag.CommandLine, *configPath); err != nil {
		return err
	}
	if handleVersion() {
		return nil
	}
	if *recoverNetwork { return linuxserver.RecoverManagedNetwork(*statePath) }

	defer startQualificationCPUProfile()()
	if *rawIface == "" || *listenIPText == "" || *serverName == "" || *routeKeyHex == "" ||
		*certPath == "" || *keyPath == "" || *username == "" || *password == "" || *decoy == "" {
		flag.Usage()
		return errors.New("required server settings are missing")
	}
	if *listenPort == 0 || *listenPort > 65535 || *serverLimit > 65535 {
		return errors.New("invalid port or record limit")
	}
	if err := logicaltunnel.ValidateProductTransportLaneCount(*lanes); err != nil {
		return err
	}
	fecFlushAfter, fecMaxBlocks, err := datapath.FixedFECRuntimeDefaults(*fecParity)
	if err != nil {
		return err
	}
	if *idleDormant < 0 {
		return errors.New("idle-dormant must be non-negative")
	}
	if *diagnosticJSONL != "" && *diagnosticInterval <= 0 {
		return errors.New("diagnostic-interval must be positive")
	}

	listenIP, err := netip.ParseAddr(*listenIPText)
	if err != nil || !listenIP.Is4() {
		return errors.New("listen-ip must be IPv4")
	}
	staticLease := *leaseText != ""
	if !staticLease { explicitLanes:=false; flag.Visit(func(f *flag.Flag) { if f.Name=="lanes" { explicitLanes=true } }); if !explicitLanes { *lanes=4 }; *leaseText = "0.0.0.0/32"; *account = *username; *installationText = "00000000000000000000000000000000"; *tunnelText = "00000000000000000000000000000000" }
	if *maxClients < 1 || *maxClients > 4096 { return errors.New("max-clients must be 1..4096") }
	leasePrefix, err := netip.ParsePrefix(*leaseText)
	if err != nil || !leasePrefix.Addr().Is4() || leasePrefix.Bits() != 32 {
		return errors.New("lease4 must be IPv4 /32")
	}
	leasePool, err := netip.ParsePrefix(*leasePoolText)
	if err != nil || !leasePool.Addr().Is4() {
		return errors.New("lease-pool must be IPv4")
	}
	tunnelID, err := logicaltunnel.ParseTunnelID(*tunnelText)
	if err != nil {
		return err
	}
	installation, err := logicaltunnel.ParseInstallationID(*installationText)
	if err != nil {
		return err
	}
	routeKey, err := hex.DecodeString(*routeKeyHex)
	if err != nil || len(routeKey) == 0 {
		return errors.New("route-key-hex must decode to non-empty bytes")
	}
	if *configPath != "" {
		base, err := filepath.Abs(filepath.Dir(*configPath)); if err != nil { return err }
		if !filepath.IsAbs(*certPath) { *certPath = filepath.Join(base,*certPath) }
		if !filepath.IsAbs(*keyPath) { *keyPath = filepath.Join(base,*keyPath) }
		
	}
	cert, err := tls.LoadX509KeyPair(*certPath, *keyPath)
	if err != nil {
		return err
	}
	lease := logicaltunnel.Lease{
		Account:        *account,
		InstallationID: installation,
		Config: logicaltunnel.TunnelConfig{
			TunnelID: tunnelID,
			Address4: leasePrefix.String(),
			Routes4:  []string{"0.0.0.0/0"},
		},
	}
	if err := lease.Validate(); err != nil {
		return err
	}

	plan, err := linuxserver.BuildNetworkPlan(*tunName, leasePool, *mtu, linuxserver.FirewallBackend(*firewall), *nftForward)
	if err != nil {
		return err
	}
	if *keepalive < time.Second || *keepalive > time.Hour || len(routeKey) < 16 || len(*username) > 255 || len(*password) > 1024 || *serverLimit < 31 { return errors.New("invalid admission or keepalive configuration") }
	if staticLease && !leasePool.Contains(leasePrefix.Addr()) { return errors.New("static lease is outside lease-pool") }
	if *checkConfig {
		out := map[string]string{}
		flag.VisitAll(func(f *flag.Flag) { if f.Name == "password" || f.Name == "route-key-hex" { out[f.Name] = "configured" } else { out[f.Name] = f.Value.String() } })
		out["lease-mode"] = "automatic"; if staticLease { out["lease-mode"] = "static" }
		return json.NewEncoder(os.Stdout).Encode(out)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	var leaseStore *logicaltunnel.LeaseRegistry
	if !staticLease {
		leaseStore, err = logicaltunnel.NewLeaseRegistry(leasePool,*maxClients)
		if err != nil { return err }
	}
	var network interface { TUN() *linuxserver.TUN; Close() error }
	if *statePath!="" { network,err=linuxserver.OpenManagedRuntime(plan,*statePath,listenIP,uint16(*listenPort)) } else { network,err=linuxserver.OpenRuntime(plan) }
	if err != nil {
		return err
	}
	defer func() { if err := network.Close(); err != nil { log.Printf("network cleanup failed: %v",err) } }()

	router, err := linuxserver.NewSharedTUNRouter(leasePool, 4096, network.TUN())
	if err != nil {
		return err
	}
	raw, err := faketcp.OpenRawIPv4Endpoint(*rawIface, listenIP.As4(), faketcp.PacketPersonaLegacy)
	if err != nil {
		return err
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
		Close:     raw.Close,
		EmitBatch: raw.WriteSegments,
	}
	raw.SetIODiagnostics(*diagnosticJSONL != "")
	defer raw.Close()
	var server *runtimeentry.LifecycleServer
	server, err = runtimeentry.NewLifecycleServer(runtimeentry.LifecycleServerConfig{
		ServerConfig: runtimeentry.ServerConfig{
			TLSStartupPadding: *tlsStartupPadding,
			IO:                io,
			ListenPort:        uint16(*listenPort),
			Admission: realityfront.ServerAdmissionConfig{
				AllocateLease: func(req realityfront.AdmissionRequest) (string,error) {
					if leaseStore == nil || int(req.DesiredLanes)>*lanes { return "", realityfront.ErrAdmissionParams }
					installation, err := logicaltunnel.ParseInstallationID(hex.EncodeToString(req.InstallationID)); if err != nil { return "",err }
					id, err := logicaltunnel.TunnelIDFromBytes(req.TunnelID); if err != nil { return "",err }
					assigned, err := leaseStore.Acquire(req.Username,installation,id); return assigned.Config.Address4,err
				},
				TLS: realityfront.ServerConfig{
					ServerName: *serverName,
					RouteKey:   routeKey,
					TLSConfig:  &tls.Config{Certificates: []tls.Certificate{cert}},
					Timeout:    15 * time.Second,
				},
				ExpectedUsername: *username,
				ExpectedPassword: *password,
				ServerLimit:      uint16(*serverLimit),
			},
			Fallback: realityfront.FallbackConfig{
				Target: *decoy, ServerName: *serverName,
				DialTimeout: 5 * time.Second, SessionTimeout: 2 * time.Minute,
				MaxBytes: 64 << 20,
			},
			LookupLease: func(id logicaltunnel.TunnelID) (logicaltunnel.Lease, error) {
				if leaseStore != nil { return leaseStore.Lookup(id) }
				if id != tunnelID {
					return logicaltunnel.Lease{}, logicaltunnel.ErrUnknownTunnel
				}
				return lease.Clone(), nil
			},
			Lane: datapath.ServerLaneParams{
				ConnectionMTU:   *mtu,
				TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
				RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
				ParityShards: *fecParity, FlushAfter: fecFlushAfter, MaxBlocks: fecMaxBlocks,
			},
			Router:  router,
			Service: platformflow.DefaultServerConfig(),
		},
		DesiredLanes:      *lanes,
		DormantAfter:      *idleDormant,
		KeepaliveInterval: *keepalive,
		ObserveTiming:     *diagnosticJSONL != "",
	})
	if err != nil {
		return err
	}

	if leaseStore != nil { leaseStore.BeforeExpire = server.ForgetInactiveTunnel }
	errCh := make(chan error, 3)
	if *diagnosticJSONL != "" {
		go func() {
			errCh <- qualificationdiag.Run(ctx, *diagnosticJSONL, *diagnosticInterval, func(now time.Time) any {
				snapshot, ok := server.TunnelDiagnosticSnapshot(tunnelID, now)
				return struct {
					Present bool                          `json:"present"`
					Tunnel  runtimeentry.TunnelDiagnostic `json:"tunnel"`
					RawIO   faketcp.RawIODiagnostic       `json:"raw_io"`
				}{Present: ok, Tunnel: snapshot, RawIO: raw.IODiagnostic()}
			})
		}()
	}
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

	var runErr error
	if err:=linuxserver.NotifyReady();err!=nil{ _=server.Close(); return err }
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			runErr = err
		}
		cancel()
	}
	runErr = errors.Join(runErr,server.Close(),network.Close())
	fmt.Printf("WBD_SERVER_STOPPED cleanup=owned-only lanes=%d\n", *lanes)
	return runErr
}
