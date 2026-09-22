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

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/openwrtclient"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
"github.com/lly8666/wobuzhidao/internal/qualificationdiag"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/runtimeentry"
)

func main() {
	var (
		rawIface = flag.String("raw-interface", "", "Linux/OpenWrt underlay interface")
		localIPText = flag.String("local-ip", "", "underlay source IPv4")
		sourcePort = flag.Uint("source-port", 40000, "FakeTCP source port")
		serverIPText = flag.String("server-ip", "", "server public IPv4")
		serverPort = flag.Uint("server-port", 443, "server FakeTCP port")
		tunnelText = flag.String("tunnel-id", "", "32-hex Logical Tunnel ID")
		leaseText = flag.String("lease4", "", "leased IPv4 /32")
		account = flag.String("account", "", "account identity")
		installationText = flag.String("installation-id", "", "32-hex installation ID")
		serverName = flag.String("server-name", "", "recognized TLS server name")
		routeKeyHex = flag.String("route-key-hex", "", "hex route key")
		username = flag.String("username", "", "protected admission username")
		password = flag.String("password", "", "protected admission password")
		clientLimit = flag.Uint("client-record-limit", 1300, "server-to-client TLS-like record wire limit")
		mtu = flag.Int("mtu", 1500, "connection MTU")
		tlsStartupPadding = flag.Bool("tls-startup-padding", false, "bounded passive inner TLS startup padding; no waiting; default off")
		fecParity = flag.Int("fec-parity", 0, "fixed FEC parity shards: 0=off; allowed 4,8,10,12,16,20")
		tproxyPort = flag.Uint("tproxy-port", 12345, "transparent TCP/UDP capture port")
		mark = flag.Uint("mark", 0x42, "TPROXY fwmark")
		table = flag.Uint("route-table", 1066, "TPROXY policy route table")
		priority = flag.Uint("rule-priority", 1066, "TPROXY policy rule priority")
		lanes = flag.Int("lanes", 1, "authoritative transport lanes: 1=Normal, 2..4=Game racing")
		idleDormant = flag.Duration("idle-dormant", 0, "enter DORMANT after payload idle duration; 0 disables")
		rotateMin = flag.Duration("rotate-min", 0, "minimum lane rotation interval; 0 disables rotation")
		rotateMax = flag.Duration("rotate-max", 0, "maximum lane rotation interval; must pair with rotate-min")
		diagnosticJSONL = flag.String("diagnostic-jsonl", "", "optional qualification diagnostics JSONL path; disabled by default")
		diagnosticInterval = flag.Duration("diagnostic-interval", time.Second, "qualification diagnostics sample interval")
	)
	flag.Parse()
	if handleVersion() {
		return
	}
	if *rawIface == "" || *localIPText == "" || *serverIPText == "" || *tunnelText == "" ||
		*leaseText == "" || *account == "" || *installationText == "" || *serverName == "" ||
		*routeKeyHex == "" || *username == "" || *password == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *sourcePort == 0 || *sourcePort > 65535 || *serverPort == 0 || *serverPort > 65535 ||
		*tproxyPort == 0 || *tproxyPort > 65535 || *clientLimit > 65535 {
		log.Fatal("invalid port or record limit")
	}
	if err := logicaltunnel.ValidateProductTransportLaneCount(*lanes); err != nil {
		log.Fatal(err)
	}
	fecFlushAfter, fecMaxBlocks, err := datapath.FixedFECRuntimeDefaults(*fecParity)
	if err != nil {
		log.Fatal(err)
	}
	if *idleDormant < 0 || *rotateMin < 0 || *rotateMax < 0 ||
		(*rotateMin == 0) != (*rotateMax == 0) || (*rotateMin > 0 && *rotateMax < *rotateMin) {
		log.Fatal("invalid lifecycle durations")
	}
	if *diagnosticJSONL != "" && *diagnosticInterval <= 0 {
		log.Fatal("diagnostic-interval must be positive")
	}
	if _, err := runtimeentry.RotatingSourcePort(uint16(*sourcePort), 1); err != nil {
		log.Fatal("source-port must leave a 1024-port bounded rotation window")
	}

	localIP, err := netip.ParseAddr(*localIPText)
	if err != nil || !localIP.Is4() {
		log.Fatal("local-ip must be IPv4")
	}
	serverIP, err := netip.ParseAddr(*serverIPText)
	if err != nil || !serverIP.Is4() {
		log.Fatal("server-ip must be IPv4")
	}
	leasePrefix, err := netip.ParsePrefix(*leaseText)
	if err != nil || !leasePrefix.Addr().Is4() || leasePrefix.Bits() != 32 {
		log.Fatal("lease4 must be IPv4 /32")
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

	plan, err := openwrtclient.BuildNetworkPlan(uint16(*tproxyPort), uint32(*mark), uint32(*table), uint32(*priority), serverIP)
	if err != nil {
		log.Fatal(err)
	}
	netRuntime, err := openwrtclient.OpenRuntime(plan)
	if err != nil {
		log.Fatal(err)
	}
	defer netRuntime.Close()

	raw, err := faketcp.OpenRawIPv4Endpoint(*rawIface, localIP.As4(), faketcp.PacketPersonaLegacy)
	if err != nil {
		log.Fatal(err)
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
		Close: raw.Close,
	}
	mux, err := runtimeentry.NewSegmentMux(baseIO)
	if err != nil {
		log.Fatal(err)
	}
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
		Lease: lease,
		TLSStartupPadding: *tlsStartupPadding,
		DesiredLanes: *lanes,
		Admission: realityfront.ClientAdmissionConfig{
			TLS: realityfront.ClientConfig{
				ServerName: *serverName, RouteKey: routeKey, Timeout: 15 * time.Second,
			},
			Username: *username, Password: *password,
			TunnelID: tunnelID.Bytes(), ClientLimit: uint16(*clientLimit),
		},
		Lane: datapath.ClientLaneParams{
			ConnectionMTU: *mtu,
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
		DormantAfter: *idleDormant,
		RotateMin: *rotateMin,
		RotateMax: *rotateMax,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	channel, err := platformflow.NewTunnelChannel(client.Owner(), client.PlatformWireSink)
	if err != nil {
		log.Fatal(err)
	}
	adapter, err = openwrtclient.OpenSocketAdapter(openwrtclient.SocketConfig{
		ListenPort: uint16(*tproxyPort),
		Channel: channel,
		Client: platformflow.DefaultClientConfig(),
		BeforeBusiness: func() error {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return client.PrepareBusiness(ctx)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer adapter.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	errCh := make(chan error, 4)
	if *diagnosticJSONL != "" {
		go func() {
			errCh <- qualificationdiag.Run(ctx, *diagnosticJSONL, *diagnosticInterval, func(now time.Time) any {
				return client.DiagnosticSnapshot(now)
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

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, openwrtclient.ErrSocketClosed) {
			log.Printf("wbd-client stopped: %v", err)
		}
		cancel()
	}
	fmt.Printf("WBD_OPENWRT_CLIENT_STOPPED cleanup=owned-only lanes=%d ipv6=NOT_IMPLEMENTED\n", *lanes)
}
