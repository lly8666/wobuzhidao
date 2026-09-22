//go:build linux

package linuxserver

import (
	"bytes"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
)

func TestPrivilegedSharedTUNRuntime(t *testing.T) {
	if os.Getenv("WBD_P4_LINUX_TUN_NET") != "1" {
		t.Skip("set WBD_P4_LINUX_TUN_NET=1 inside an isolated root network namespace")
	}
	if os.Geteuid() != 0 {
		t.Fatal("privileged shared-TUN qualification requires root")
	}
	backend := FirewallBackend(os.Getenv("WBD_P4_FIREWALL_BACKEND"))
	if backend != FirewallIPTables && backend != FirewallNFT {
		t.Fatalf("unsupported qualification backend %q", backend)
	}
	nftForward := os.Getenv("WBD_P4_NFT_FORWARD")
	beforeForward, err := readSysctl("net.ipv4.ip_forward")
	if err != nil {
		t.Fatal(err)
	}

	plan, err := BuildNetworkPlan(
		"wbdg0",
		netip.MustParsePrefix("10.66.0.0/16"),
		1400,
		backend,
		nftForward,
	)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := OpenRuntime(plan)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = rt.Close()
		}
	}()

	if rt.Backend() != backend {
		t.Fatalf("backend=%q want=%q", rt.Backend(), backend)
	}
	if rt.TUN() == nil || rt.TUN().Name() != "wbdg0" {
		t.Fatalf("runtime TUN=%v", rt.TUN())
	}
	mustCommand(t, "ip", "link", "show", "dev", "wbdg0")
	route := mustCommand(t, "ip", "-4", "route", "show", "10.66.0.0/16", "dev", "wbdg0")
	if !strings.Contains(route, "10.66.0.0/16") {
		t.Fatalf("route=%q", route)
	}
	if got, err := readSysctl("net.ipv4.ip_forward"); err != nil || got != "1" {
		t.Fatalf("ip_forward=%q err=%v", got, err)
	}
	if got, err := readSysctl("net.ipv4.conf.wbdg0.rp_filter"); err != nil || got != "0" {
		t.Fatalf("rp_filter=%q err=%v", got, err)
	}
	if got, err := readSysctl("net.ipv6.conf.wbdg0.disable_ipv6"); err != nil || got != "1" {
		t.Fatalf("disable_ipv6=%q err=%v", got, err)
	}
	if out := mustCommand(t, "ip", "-6", "addr", "show", "dev", "wbdg0"); strings.Contains(out, "inet6") {
		t.Fatalf("IPv4-only shared TUN unexpectedly has IPv6 address: %q", out)
	}
	assertFirewallMarkers(t, backend, nftForward, true)

	router, err := NewSharedTUNRouter(plan.LeasePrefix, 4, rt.TUN())
	if err != nil {
		t.Fatal(err)
	}
	a := &fakeOwner{
		lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.1"),
		stats: datapath.TunnelOwnerStats{DesiredLanes: 1},
	}
	b := &fakeOwner{
		lease: testLease(t, "ffeeddccbbaa99887766554433221100", "10.66.0.2"),
		stats: datapath.TunnelOwnerStats{DesiredLanes: 3},
	}
	tokenA, err := router.Register(a)
	if err != nil {
		t.Fatal(err)
	}
	tokenB, err := router.Register(b)
	if err != nil {
		t.Fatal(err)
	}
	if router.ActiveTunnels() != 2 {
		t.Fatalf("active tunnels=%d", router.ActiveTunnels())
	}

	toInternetA := ipv4Packet(netip.MustParseAddr("10.66.0.1"), netip.MustParseAddr("203.0.113.10"), []byte("a"))
	toInternetB := ipv4Packet(netip.MustParseAddr("10.66.0.2"), netip.MustParseAddr("203.0.113.11"), []byte("b"))
	if err := router.DeliverFromOwner(tokenA, [][]byte{toInternetA}); err != nil {
		t.Fatal(err)
	}
	if err := router.DeliverFromOwner(tokenB, [][]byte{toInternetB}); err != nil {
		t.Fatal(err)
	}

	returnA := ipv4Packet(netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.66.0.1"), []byte("ra"))
	gotA, err := router.RouteFromTUN(returnA, time.Unix(800, 0))
	if err != nil {
		t.Fatal(err)
	}
	if gotA.IsGame || a.normalCalls != 1 || !bytes.Equal(a.lastPacket, returnA) {
		t.Fatalf("normal return=%+v calls=%d", gotA, a.normalCalls)
	}
	returnB := ipv4Packet(netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("10.66.0.2"), []byte("rb"))
	gotB, err := router.RouteFromTUN(returnB, time.Unix(801, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !gotB.IsGame || b.gameCalls != 1 || !bytes.Equal(b.lastPacket, returnB) {
		t.Fatalf("game return=%+v calls=%d", gotB, b.gameCalls)
	}

	if err := rt.Close(); err != nil {
		t.Fatal(err)
	}
	closed = true
	if err := exec.Command("ip", "link", "show", "dev", "wbdg0").Run(); err == nil {
		t.Fatal("shared TUN survived runtime Close")
	}
	if out := mustCommand(t, "ip", "-4", "route", "show", "10.66.0.0/16"); strings.Contains(out, "wbdg0") {
		t.Fatalf("shared TUN route survived cleanup: %q", out)
	}
	if got, err := readSysctl("net.ipv4.ip_forward"); err != nil || got != beforeForward {
		t.Fatalf("ip_forward after cleanup=%q want=%q err=%v", got, beforeForward, err)
	}
	assertFirewallMarkers(t, backend, nftForward, false)
	fmt.Printf("WBD_P4_LINUX_SHARED_TUN_PASS backend=%s active_tunnels=2 cleanup=owned-only dns=%s\n", backend, DNSUnchanged)
}

func mustCommand(t *testing.T, name string, args ...string) string {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, out)
	}
	return string(out)
}

func assertFirewallMarkers(t *testing.T, backend FirewallBackend, nftForward string, want bool) {
	t.Helper()
	switch backend {
	case FirewallIPTables:
		filter := mustCommand(t, "iptables", "-S", "FORWARD")
		nat := mustCommand(t, "iptables", "-t", "nat", "-S", "POSTROUTING")
		joined := filter + nat
		for _, marker := range []string{"wbd-shared-tun-out", "wbd-shared-tun-in", "wbd-shared-tun-nat"} {
			has := strings.Contains(joined, marker)
			if has != want {
				t.Fatalf("iptables marker=%s present=%v want=%v rules=%s", marker, has, want, joined)
			}
		}
	case FirewallNFT:
		if nftForward == "" {
			if want {
				table := mustCommand(t, "nft", "-a", "list", "table", "inet", nftOwnedTable)
				for _, marker := range []string{"wbd-shared-tun-out", "wbd-shared-tun-in", "wbd-shared-tun-nat"} {
					if !strings.Contains(table, marker) {
						t.Fatalf("owned nft table missing %s: %s", marker, table)
					}
				}
			} else if err := exec.Command("nft", "list", "table", "inet", nftOwnedTable).Run(); err == nil {
				t.Fatalf("owned nft table survived cleanup")
			}
			return
		}
		parts := strings.Split(nftForward, ":")
		if len(parts) != 3 {
			t.Fatalf("bad nft forward spec %q", nftForward)
		}
		forward := mustCommand(t, "nft", "-a", "list", "chain", parts[0], parts[1], parts[2])
		hasOut := strings.Contains(forward, "wbd-shared-tun-out")
		hasIn := strings.Contains(forward, "wbd-shared-tun-in")
		if hasOut != want || hasIn != want {
			t.Fatalf("nft forward markers out=%v in=%v want=%v: %s", hasOut, hasIn, want, forward)
		}
		if want {
			table := mustCommand(t, "nft", "-a", "list", "table", "inet", nftOwnedTable)
			if !strings.Contains(table, "wbd-shared-tun-nat") {
				t.Fatalf("nft NAT marker missing: %s", table)
			}
		} else {
			if err := exec.Command("nft", "list", "table", "inet", nftOwnedTable).Run(); err == nil {
				t.Fatal("nft owned table survived cleanup")
			}
			forward = mustCommand(t, "nft", "list", "chain", parts[0], parts[1], parts[2])
			if !strings.Contains(forward, "policy drop") {
				t.Fatalf("existing nft forward policy was not preserved: %s", forward)
			}
		}
	default:
		t.Fatalf("unexpected backend %q", backend)
	}
}
