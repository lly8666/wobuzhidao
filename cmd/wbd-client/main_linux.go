//go:build linux

package main

import (
	"context"
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

	"github.com/lly8666/wobuzhidao/internal/configfile"
	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/dnsroute"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/openwrtclient"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
	"github.com/lly8666/wobuzhidao/internal/qualificationdiag"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/runtimeentry"
	"github.com/lly8666/wobuzhidao/internal/splitroute"
)

func main() {
	if err := runLinuxClient(); err != nil {
		log.Fatal(err)
	}
}

func runLinuxClient() error {
	var (
		deadAfter          = flag.Duration("dead-after", runtimeentry.DefaultDeadAfter, "reconnect after no authenticated lane records; at least 3 keepalive intervals")
		reconnectMin       = flag.Duration("reconnect-min", runtimeentry.DefaultReconnectMin, "minimum retry delay after failed lane admission")
		reconnectMax       = flag.Duration("reconnect-max", runtimeentry.DefaultReconnectMax, "maximum retry delay after failed lane admission")
		configPath         = flag.String("config", "", "JSON configuration file; CLI flags override matching keys")
		dnsHijack          = flag.Bool("dns-hijack", true, "capture ordinary TCP/UDP DNS and use dns4 through the tunnel; default on")
		dnsText            = flag.String("dns4", "1.1.1.1,8.8.8.8", "primary,backup IPv4 DNS; transparent queries retain original reply address")
		routeMode          = flag.String("route-mode", "bypass-lan-cn", "IPv4 capture policy: all, bypass-lan, bypass-lan-cn")
		chinaIPFile        = flag.String("china-ip-file", "", "optional China IPv4 CIDR snapshot; empty uses embedded list; restart to apply")
		updateChinaIP      = flag.String("update-china-ip", "", "download and validate China IPv4 list into this file, then exit; no tunnel required")
		keepalive          = flag.Duration("keepalive-interval", runtimeentry.DefaultKeepaliveInterval, "authenticated lane heartbeat interval; minimum 1s")
		rawIface           = flag.String("raw-interface", "", "Linux/OpenWrt underlay interface")
		localIPText        = flag.String("local-ip", "", "underlay source IPv4")
		sourcePort         = flag.Uint("source-port", 40000, "FakeTCP source port")
		serverIPText       = flag.String("server-ip", "", "server public IPv4")
		serverPort         = flag.Uint("server-port", 443, "server FakeTCP port")
		tunnelText         = flag.String("tunnel-id", "", "32-hex Logical Tunnel ID; empty derives from account and installation in automatic lease mode")
		leaseText          = flag.String("lease4", "", "leased IPv4 /32; empty requests automatic server allocation")
		account            = flag.String("account", "", "account identity; empty uses username")
		installationText   = flag.String("installation-id", "", "32-hex installation ID")
		serverName         = flag.String("server-name", "", "recognized TLS server name")
		routeKeyHex        = flag.String("route-key-hex", "", "hex route key")
		username           = flag.String("username", "", "protected admission username")
		password           = flag.String("password", "", "protected admission password")
		clientLimit        = flag.Uint("client-record-limit", 1300, "server-to-client TLS-like record wire limit")
		mtu                = flag.Int("mtu", 1500, "connection MTU")
		tlsStartupPadding  = flag.Bool("tls-startup-padding", false, "bounded passive inner TLS startup padding; no waiting; default off")
		fecParity          = flag.Int("fec-parity", 0, "fixed FEC parity shards: 0=off; allowed 4,8,10,12,16,20")
		tproxyPort         = flag.Uint("tproxy-port", 12345, "transparent TCP/UDP capture port")
		mark               = flag.Uint("mark", 0x42, "TPROXY fwmark")
		table              = flag.Uint("route-table", 1066, "TPROXY policy route table")
		priority           = flag.Uint("rule-priority", 1066, "TPROXY policy rule priority")
		lanes              = flag.Int("lanes", 1, "authoritative transport lanes: 1=Normal, 2..4=Game racing")
		idleDormant        = flag.Duration("idle-dormant", 0, "enter DORMANT after payload idle duration; 0 disables")
		rotateMin          = flag.Duration("rotate-min", 0, "minimum lane rotation interval; 0 disables rotation")
		rotateMax          = flag.Duration("rotate-max", 0, "maximum lane rotation interval; must pair with rotate-min")
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
	if *updateChinaIP != "" {
		if err := splitroute.Update(*updateChinaIP); err != nil {
			return err
		}
		return nil
	}
	direct, err := splitroute.Direct(*routeMode, *chinaIPFile)
	if err != nil {
		return err
	}
	defer startQualificationCPUProfile()()
	if *rawIface == "" || *localIPText == "" || *serverIPText == "" || *serverName == "" ||
		*routeKeyHex == "" || *username == "" || *password == "" {
		flag.Usage()
		return errors.New("required identity and server settings are missing")
	}
	if *sourcePort == 0 || *sourcePort > 65535 || *serverPort == 0 || *serverPort > 65535 ||
		*tproxyPort == 0 || *tproxyPort > 65535 || *clientLimit > 65535 {
		return errors.New("invalid port or record limit")
	}
	if err := logicaltunnel.ValidateProductTransportLaneCount(*lanes); err != nil {
		return err
	}
	fecFlushAfter, fecMaxBlocks, err := datapath.FixedFECRuntimeDefaults(*fecParity)
	if err != nil {
		return err
	}
	if *idleDormant < 0 || *rotateMin < 0 || *rotateMax < 0 ||
		(*rotateMin == 0) != (*rotateMax == 0) || (*rotateMin > 0 && *rotateMax < *rotateMin) {
		return errors.New("invalid lifecycle durations")
	}
	if *diagnosticJSONL != "" && *diagnosticInterval <= 0 {
		return errors.New("diagnostic-interval must be positive")
	}
	if _, err := runtimeentry.RotatingSourcePort(uint16(*sourcePort), 1); err != nil {
		return errors.New("source-port must leave a 1024-port bounded rotation window")
	}

	localIP, err := netip.ParseAddr(*localIPText)
	if err != nil || !localIP.Is4() {
		return errors.New("local-ip must be IPv4")
	}
	serverIP, err := netip.ParseAddr(*serverIPText)
	if err != nil || !serverIP.Is4() {
		return errors.New("server-ip must be IPv4")
	}
	resolvedInstallation, identityErr := logicaltunnel.ResolveInstallation(*installationText, "/var/lib/wbd-client/installation-id", false)
	if identityErr != nil {
		return identityErr
	}
	*installationText = resolvedInstallation.String()
	if *account == "" {
		*account = *username
	}
	autoLease := *leaseText == ""
	if autoLease {
		installation, identityErr := logicaltunnel.ParseInstallationID(*installationText)
		if identityErr != nil {
			return identityErr
		}
		expected := logicaltunnel.DerivedTunnelID(*account, installation)
		if *tunnelText == "" {
			*tunnelText = expected.String()
		}
		if *account != *username || *tunnelText != expected.String() {
			return errors.New("automatic lease requires account=username and installation-derived tunnel-id")
		}
	}
	parseLease := *leaseText
	if autoLease {
		parseLease = "0.0.0.0/32"
	}
	leasePrefix, err := netip.ParsePrefix(parseLease)
	if err != nil || !leasePrefix.Addr().Is4() || leasePrefix.Bits() != 32 {
		return errors.New("lease4 must be IPv4 /32")
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

	plan, err := openwrtclient.BuildNetworkPlan(uint16(*tproxyPort), uint32(*mark), uint32(*table), uint32(*priority), serverIP)
	if err != nil {
		return err
	}
	dns, err := dnsroute.ParseServers(*dnsText)
	if err != nil || (*dnsHijack && len(dns) == 0) {
		return errors.New("invalid DNS servers")
	}
	if !*dnsHijack {
		dns = nil
	}
	for _, resolver := range dns {
		if resolver == serverIP {
			return errors.New("dns4 must not equal the underlay server address")
		}
	}
	plan.DNSHijack = *dnsHijack
	plan.Direct4 = direct
	log.Printf("WBD_ROUTE_POLICY mode=%s direct_prefixes=%d source=%s", *routeMode, len(direct), chinaListSource(*chinaIPFile))
	netRuntime, err := openwrtclient.OpenRuntime(plan)
	if err != nil {
		return err
	}
	defer netRuntime.Close()

	raw, err := faketcp.OpenRawIPv4Endpoint(*rawIface, localIP.As4(), faketcp.PacketPersonaLegacy)
	if err != nil {
		return err
	}
	baseIO := runtimeentry.SegmentIO{
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
	mux, err := runtimeentry.NewSegmentMux(baseIO)
	if err != nil {
		return err
	}
	mux.SetTimingDiagnostics(*diagnosticJSONL != "")
	defer mux.Close()

	var adapter *openwrtclient.SocketAdapter
	client, err := runtimeentry.DialTunnelClient(context.Background(), runtimeentry.TunnelClientConfig{
		OpenLane: func(_ uint8, incarnation uint64) (runtimeentry.SegmentIO, faketcp.ClientFlow, error) {
			port, err := runtimeentry.RotatingSourcePort(uint16(*sourcePort), incarnation)
			if err != nil {
				return runtimeentry.SegmentIO{}, faketcp.ClientFlow{}, err
			}
			flow := faketcp.ClientFlow{
				LocalIP: localIP.As4(), PeerIP: serverIP.As4(),
				LocalPort: port, PeerPort: uint16(*serverPort),
			}
			laneIO, err := mux.Open(flow)
			return laneIO, flow, err
		},
		Lease:             lease,
		TLSStartupPadding: *tlsStartupPadding,
		DesiredLanes:      *lanes,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{
				ServerName: *serverName, RouteKey: routeKey, Timeout: 15 * time.Second,
			},
			AutoLease: autoLease, InstallationID: installation[:], Username: *username, Password: *password,
			TunnelID: tunnelID.Bytes(), ClientLimit: uint16(*clientLimit),
		},
		Lane: datapath.ClientLaneParams{
			ConnectionMTU:   *mtu,
			TxIPv4HeaderLen: 20, TxTCPHeaderLen: 20,
			RxIPv4HeaderLen: 20, RxTCPHeaderLen: 20,
			ParityShards: *fecParity, FlushAfter: fecFlushAfter, MaxBlocks: fecMaxBlocks,
		},
		Deliver: func(packets [][]byte, now time.Time) error {
			if adapter == nil {
				return platformflow.ErrClosed
			}
			return adapter.DeliverFromOwner(packets, now)
		},
		DormantAfter:      *idleDormant,
		KeepaliveInterval: *keepalive, DeadAfter: *deadAfter, ReconnectMin: *reconnectMin, ReconnectMax: *reconnectMax,
		RotateMin: *rotateMin,
		RotateMax: *rotateMax,
	})
	if err != nil {
		return err
	}
	defer client.Close()

	channel, err := platformflow.NewTunnelChannel(client.Owner(), client.PlatformWireSink)
	if err != nil {
		return err
	}
	adapter, err = openwrtclient.OpenSocketAdapter(openwrtclient.SocketConfig{
		ListenPort:      uint16(*tproxyPort),
		DNSServers:      dns,
		ReplyBypassMark: uint32(*mark) | 0x80000000,
		Channel:         channel,
		Client:          platformflow.DefaultClientConfig(),
		BeforeBusiness: func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return client.PrepareBusiness(ctx)
		},
	})
	if err != nil {
		return err
	}
	defer adapter.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	errCh := make(chan error, 4)
	if *diagnosticJSONL != "" {
		go func() {
			errCh <- qualificationdiag.Run(ctx, *diagnosticJSONL, *diagnosticInterval, func(now time.Time) any {
				return struct {
					runtimeentry.TunnelDiagnostic
					SegmentMux runtimeentry.SegmentMuxDiagnostic  `json:"segment_mux"`
					ClientUDP  openwrtclient.UDPIngressDiagnostic `json:"client_udp"`
					RawIO      faketcp.RawIODiagnostic            `json:"raw_io"`
				}{
					TunnelDiagnostic: client.DiagnosticSnapshot(now),
					SegmentMux:       mux.DiagnosticSnapshot(),
					ClientUDP:        adapter.UDPIngressDiagnostic(),
					RawIO:            raw.IODiagnostic(),
				}
			})
		}()
	}
	go func() { errCh <- adapter.Run(ctx) }()
	go func() {
		select {
		case err := <-client.Errors():
			errCh <- err
		case <-ctx.Done():
		}
	}()
	go func() {
		select {
		case err := <-mux.Errors():
			errCh <- err
		case <-ctx.Done():
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, openwrtclient.ErrSocketClosed) {
			log.Printf("wbd-client stopped: %v", err)
			runErr = err
		}
		cancel()
	}
	fmt.Printf("WBD_OPENWRT_CLIENT_STOPPED cleanup=owned-only lanes=%d ipv6=BLACKHOLE_DROPPED\n", *lanes)
	return runErr
}
