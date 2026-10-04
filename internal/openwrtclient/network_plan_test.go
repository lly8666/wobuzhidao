package openwrtclient

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func testPlan(t *testing.T) NetworkPlan {
	t.Helper()
	plan, err := BuildNetworkPlan(12345, 0x66, 1066, 1066, netip.MustParseAddr("198.51.100.10"))
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestDefaultNetworkDNSBeforeDirectAndIPv6Blackhole(t *testing.T) {
	p := testPlan(t)
	p.DNSHijack = true
	p.Direct4 = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}
	s, e := p.NFTScript()
	if e != nil {
		t.Fatal(e)
	}
	for _, x := range []string{"wbd-dns-capture", "wbd-dns-output-route", "type route hook output", "wbd-reply-output-bypass", "wbd-ipv6-output-drop", "wbd-ipv6-drop"} {
		if !strings.Contains(s, x) {
			t.Fatalf("missing %s", x)
		}
	}
	if strings.Index(s, "wbd-dns-capture") > strings.Index(s, "wbd-direct4") {
		t.Fatal("LAN DNS escaped redirect")
	}
	if got := strings.Join(p.IPv6Blackhole.Args, " "); got != "-6 route add blackhole ::/0 table 1066" {
		t.Fatal(got)
	}
	if got := strings.Join(p.IPv6Rule.Args, " "); got != "-6 rule add priority 1066 lookup 1066" {
		t.Fatal(got)
	}
	p.DNSHijack = false
	s, e = p.NFTScript()
	if e != nil || strings.Contains(s, "wbd-dns-capture") || !strings.Contains(s, "wbd-ipv6-drop") {
		t.Fatal("DNS toggle changed IPv6 policy")
	}
}

func TestNetworkPlanOrdersBypassBeforeTCPAndUDP(t *testing.T) {
	plan := testPlan(t)
	if len(plan.Rules) != 6 {
		t.Fatalf("rules=%d", len(plan.Rules))
	}
	wantPurposes := []string{
		"already-marked-bypass",
		"underlay-source-bypass",
		"local-destination-bypass",
		"underlay-destination-bypass",
		"tcp-tproxy-capture",
		"udp-tproxy-capture",
	}
	for i, want := range wantPurposes {
		if plan.Rules[i].Purpose != want {
			t.Fatalf("rule[%d]=%q want=%q", i, plan.Rules[i].Purpose, want)
		}
	}
	script, err := plan.NFTScript()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"table inet wbd_tproxy",
		"meta mark 0x66 return",
		"ip saddr 198.51.100.10 return",
		"fib daddr type local return",
		"ip daddr 198.51.100.10 return",
		"meta nfproto ipv4 meta l4proto tcp tproxy ip to :12345 meta mark set 0x66 accept",
		"meta nfproto ipv4 meta l4proto udp tproxy ip to :12345 meta mark set 0x66 accept",
		"comment \"wbd-tproxy-tcp\"",
		"comment \"wbd-tproxy-udp\"",
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
	dst := strings.Index(script, "ip daddr 198.51.100.10 return")
	tcp := strings.Index(script, "meta l4proto tcp")
	udp := strings.Index(script, "meta l4proto udp")
	if dst < 0 || tcp < 0 || udp < 0 || dst > tcp || dst > udp {
		t.Fatalf("underlay destination bypass must precede captures:\n%s", script)
	}
	if got := strings.Join(plan.PolicyRule.Args, " "); got != "-4 rule add priority 1066 fwmark 0x66 lookup 1066" {
		t.Fatalf("policy rule=%q", got)
	}
	if got := strings.Join(plan.LocalRoute.Args, " "); got != "-4 route add local 0.0.0.0/0 dev lo table 1066" {
		t.Fatalf("local route=%q", got)
	}
}

func TestNetworkPlanRejectsUnsafeOrNonIPv4Inputs(t *testing.T) {
	for _, tc := range []struct {
		port     uint16
		mark     uint32
		table    uint32
		priority uint32
		underlay string
	}{
		{0, 0x66, 1066, 1066, "198.51.100.10"},
		{12345, 0, 1066, 1066, "198.51.100.10"},
		{12345, 0x66, 0, 1066, "198.51.100.10"},
		{12345, 0x66, 1066, 0, "198.51.100.10"},
		{12345, 0x66, 1066, 1066, "::1"},
		{12345, 0x66, 1066, 1066, "0.0.0.0"},
		{12345, 0x66, 1066, 1066, "224.0.0.1"},
	} {
		_, err := BuildNetworkPlan(tc.port, tc.mark, tc.table, tc.priority, netip.MustParseAddr(tc.underlay))
		if !errors.Is(err, ErrNetworkPlan) {
			t.Fatalf("%+v err=%v", tc, err)
		}
	}
}
