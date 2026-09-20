//go:build windows

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
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
	"github.com/lly8666/wobuzhidao/internal/runtimeentry"
	"github.com/lly8666/wobuzhidao/internal/windowsclient"
)

const npcapGeneration uint64 = 1

func main() {
	var (
		serverIPText = flag.String("server-ip", "", "server public IPv4")
		serverPort = flag.Uint("server-port", 443, "server FakeTCP port")
		sourcePort = flag.Uint("source-port", 40000, "FakeTCP source port")
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
		adapterAlias = flag.String("adapter", "WBD", "Wintun adapter alias")
		dnsText = flag.String("dns4", "", "comma-separated IPv4 DNS servers")
		directText = flag.String("direct4", "", "comma-separated direct IPv4 prefixes")
		statePath = flag.String("state-path", "wbd-windows-client-state.json", "owned Windows network state file")
		scriptPath = flag.String("network-script", "scripts/windows_client_network.ps1", "Windows network Apply/Cleanup script")
	)
	flag.Parse()
	if *serverIPText == "" || *tunnelText == "" || *leaseText == "" || *account == "" ||
		*installationText == "" || *serverName == "" || *routeKeyHex == "" ||
		*username == "" || *password == "" {
		flag.Usage()
		os.Exit(2)
	}
	if *serverPort == 0 || *serverPort > 65535 || *sourcePort == 0 || *sourcePort > 65535 ||
		*clientLimit > 65535 {
		log.Fatal("invalid port or record limit")
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
	dns, err := parseIPv4Addrs(*dnsText)
	if err != nil {
		log.Fatal(err)
	}
	direct, err := parseIPv4Prefixes(*directText)
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

	underlay, err := windowsclient.DiscoverPhysicalUnderlay(serverIP, 0)
	if err != nil {
		log.Fatal(err)
	}
	npcapCfg, err := underlay.NpcapConfig(uint16(*sourcePort), uint16(*serverPort), npcapGeneration, faketcp.PacketPersonaWindows11)
	if err != nil {
		log.Fatal(err)
	}
	npcap, err := faketcp.OpenNpcapEndpoint(npcapCfg)
	if err != nil {
		log.Fatal(err)
	}
	io := runtimeentry.SegmentIO{
		Read: func() (faketcp.Segment, error) {
			seg, _, err := npcap.ReadSegment(npcapGeneration)
			return seg, err
		},
		Emit: func(seg faketcp.Segment) error {
			_, err := npcap.WriteSegment(npcapGeneration, seg)
			return err
		},
		Close: npcap.Close,
	}

	var router *windowsclient.Router
	client, err := runtimeentry.DialClient(context.Background(), runtimeentry.ClientConfig{
		IO: io,
		Flow: faketcp.ClientFlow{
			LocalIP: underlay.Source4.As4(), PeerIP: serverIP.As4(),
			LocalPort: uint16(*sourcePort), PeerPort: uint16(*serverPort),
		},
		Lease: lease,
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
		},
		Deliver: func(packets [][]byte, _ time.Time) error {
			if router == nil {
				return windowsclient.ErrInvalidBinding
			}
			return router.DeliverFromOwner(packets)
		},
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	tun, err := windowsclient.OpenTUN(*adapterAlias)
	if err != nil {
		log.Fatal(err)
	}
	defer tun.Close()
	router, err = windowsclient.NewRouter(client.Owner(), tun)
	if err != nil {
		log.Fatal(err)
	}

	physical, err := underlay.PhysicalPath()
	if err != nil {
		log.Fatal(err)
	}
	networkPlan, err := windowsclient.BuildNetworkPlan(windowsclient.Config{
		AdapterAlias: *adapterAlias,
		Lease4: leasePrefix,
		Server4: serverIP,
		Physical: physical,
		DNSServers4: dns,
		Direct4: direct,
		StatePath: *statePath,
	})
	if err != nil {
		log.Fatal(err)
	}
	if err := runNetworkAction(networkPlan, "Apply", *scriptPath); err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := runNetworkAction(networkPlan, "Cleanup", *scriptPath); err != nil {
			log.Printf("Windows cleanup: %v", err)
		}
	}()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	errCh := make(chan error, 2)
	go func() {
		buf := make([]byte, 65535)
		for {
			n, err := tun.ReadPacket(buf)
			if err != nil {
				errCh <- err
				return
			}
			out, err := router.RouteFromTUN(append([]byte(nil), buf[:n]...), time.Now())
			if err != nil {
				errCh <- err
				return
			}
			if out.IsGame {
				err = client.SendGame(out.Game, time.Now())
			} else {
				err = client.SendNormal(out.Normal, time.Now())
			}
			if err != nil {
				errCh <- err
				return
			}
		}
	}()
	go func() {
		select {
		case err := <-client.Errors():
			errCh <- err
		case <-ctx.Done():
		}
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("wbd-client stopped: %v", err)
		}
		cancel()
	}
	fmt.Println("WBD_WINDOWS_CLIENT_STOPPED cleanup=state-owned-only physical=NOT_RUN")
}

func runNetworkAction(plan windowsclient.NetworkPlan, action, script string) error {
	args, err := plan.PowerShellArgs(action, script)
	if err != nil {
		return err
	}
	out, err := exec.Command("powershell.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("powershell %s: %w: %s", action, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func parseIPv4Addrs(raw string) ([]netip.Addr, error) {
	var out []netip.Addr
	for _, part := range splitCSV(raw) {
		addr, err := netip.ParseAddr(part)
		if err != nil || !addr.Is4() {
			return nil, fmt.Errorf("invalid IPv4 address %q", part)
		}
		out = append(out, addr.Unmap())
	}
	return out, nil
}

func parseIPv4Prefixes(raw string) ([]netip.Prefix, error) {
	var out []netip.Prefix
	for _, part := range splitCSV(raw) {
		prefix, err := netip.ParsePrefix(part)
		if err != nil || !prefix.Addr().Is4() {
			return nil, fmt.Errorf("invalid IPv4 prefix %q", part)
		}
		out = append(out, prefix.Masked())
	}
	return out, nil
}

func splitCSV(raw string) []string {
	var out []string
	for _, part := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' }) {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
