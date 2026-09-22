package linuxserver

import (
	"errors"
	"net/netip"
	"reflect"
	"testing"
)

func TestNetworkPlanOwnsOnlySharedTUNRouteForwardNATAndSysctls(t *testing.T) {
	plan, err := BuildNetworkPlan(
		"wbdg0",
		netip.MustParsePrefix("10.66.0.0/16"),
		1400,
		FirewallIPTables,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.DNSMode != DNSUnchanged {
		t.Fatalf("dns mode=%q", plan.DNSMode)
	}
	if len(plan.Setup) != 2 || !reflect.DeepEqual(plan.Setup[0], Command{Name: "ip", Args: []string{"link", "set", "wbdg0", "mtu", "1400", "up"}}) {
		t.Fatalf("setup=%+v", plan.Setup)
	}
	if !reflect.DeepEqual(plan.Setup[1], Command{Name: "ip", Args: []string{"route", "replace", "10.66.0.0/16", "dev", "wbdg0"}}) {
		t.Fatalf("route setup=%+v", plan.Setup[1])
	}
	if len(plan.Teardown) != 1 || !reflect.DeepEqual(plan.Teardown[0], Command{Name: "ip", Args: []string{"route", "del", "10.66.0.0/16", "dev", "wbdg0"}}) {
		t.Fatalf("teardown=%+v", plan.Teardown)
	}
	if len(plan.Sysctls) != 3 || !plan.Sysctls[0].RestoreSaved || !plan.Sysctls[1].RestoreSaved || !plan.Sysctls[2].RestoreSaved {
		t.Fatalf("sysctls=%+v", plan.Sysctls)
	}
	if plan.Sysctls[0].Key != "net.ipv4.ip_forward" || plan.Sysctls[0].Value != "1" ||
		plan.Sysctls[1].Key != "net.ipv4.conf.wbdg0.rp_filter" || plan.Sysctls[1].Value != "0" ||
		plan.Sysctls[2].Key != "net.ipv6.conf.wbdg0.disable_ipv6" || plan.Sysctls[2].Value != "1" {
		t.Fatalf("sysctls=%+v", plan.Sysctls)
	}
	if len(plan.Firewall.Rules) != 3 {
		t.Fatalf("firewall rules=%+v", plan.Firewall.Rules)
	}
	gotMarkers := []string{
		plan.Firewall.Rules[0].Marker,
		plan.Firewall.Rules[1].Marker,
		plan.Firewall.Rules[2].Marker,
	}
	wantMarkers := []string{"wbd-shared-tun-out", "wbd-shared-tun-in", "wbd-shared-tun-nat"}
	if !reflect.DeepEqual(gotMarkers, wantMarkers) {
		t.Fatalf("markers=%v want=%v", gotMarkers, wantMarkers)
	}
	for _, rule := range plan.Firewall.Rules {
		if rule.Marker == "" {
			t.Fatalf("unowned firewall rule=%+v", rule)
		}
	}
}

func TestNetworkPlanNFTForwardAndValidation(t *testing.T) {
	plan, err := BuildNetworkPlan(
		"wbdg0",
		netip.MustParsePrefix("10.66.0.0/16"),
		1280,
		FirewallNFT,
		"inet:filter:forward",
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Firewall.NFTForward != "inet:filter:forward" || plan.Firewall.Backend != FirewallNFT {
		t.Fatalf("firewall=%+v", plan.Firewall)
	}

	bad := []struct {
		name    string
		prefix  netip.Prefix
		mtu     int
		backend FirewallBackend
		nft     string
	}{
		{"", netip.MustParsePrefix("10.66.0.0/16"), 1400, FirewallAuto, ""},
		{"name too long for tun", netip.MustParsePrefix("10.66.0.0/16"), 1400, FirewallAuto, ""},
		{"wbdg0", netip.MustParsePrefix("2001:db8::/64"), 1400, FirewallAuto, ""},
		{"wbdg0", netip.MustParsePrefix("10.66.0.1/32"), 1400, FirewallAuto, ""},
		{"wbdg0", netip.MustParsePrefix("10.66.0.0/16"), 500, FirewallAuto, ""},
		{"wbdg0", netip.MustParsePrefix("10.66.0.0/16"), 1400, "bogus", ""},
		{"wbdg0", netip.MustParsePrefix("10.66.0.0/16"), 1400, FirewallIPTables, "inet:filter:forward"},
		{"wbdg0", netip.MustParsePrefix("10.66.0.0/16"), 1400, FirewallNFT, "bad"},
	}
	for _, tc := range bad {
		if _, err := BuildNetworkPlan(tc.name, tc.prefix, tc.mtu, tc.backend, tc.nft); !errors.Is(err, ErrNetworkPlan) {
			t.Fatalf("case=%+v err=%v", tc, err)
		}
	}
}
