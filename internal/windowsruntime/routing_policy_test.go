package windowsruntime

import (
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/ipset"
)

func TestRoutingPolicyMatrix(t *testing.T) {
	cases := []struct {
		name                                     string
		policy                                   RoutingPolicy
		mode                                     string
		needsCN, cnCapture, cnDirect, captureLAN bool
	}{
		{"000", RoutingPolicy{false, false, false}, "None", false, false, false, false},
		{"001", RoutingPolicy{false, false, true}, "Full", true, false, true, false},
		{"010", RoutingPolicy{false, true, false}, "Split", true, true, false, false},
		{"011", RoutingPolicy{false, true, true}, "Full", false, false, false, false},
		{"100", RoutingPolicy{true, false, false}, "Split", false, false, false, true},
		{"101", RoutingPolicy{true, false, true}, "Full", true, false, true, true},
		{"110", RoutingPolicy{true, true, false}, "Split", true, true, false, true},
		{"111", RoutingPolicy{true, true, true}, "Full", false, false, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := testProfile()
			p.CNSetDir = filepath.Join("C:\\", "wbd-cn")
			p.RoutingPolicy = &tc.policy
			plan := buildRoutingPlan(p)
			if plan.Mode != tc.mode || p.RequiresCNSet() != tc.needsCN || plan.CaptureLAN != tc.captureLAN {
				t.Fatalf("plan=%+v needsCN=%v", plan, p.RequiresCNSet())
			}
			if (plan.PrefixFile4 != "") != tc.cnCapture || (plan.DirectPrefixFile4 != "") != tc.cnDirect {
				t.Fatalf("CN route plan=%+v", plan)
			}
		})
	}
}

func TestLegacyRouteModesMapToExplicitPolicy(t *testing.T) {
	cases := []struct {
		mode string
		want RoutingPolicy
	}{{RouteFull, RoutingPolicy{false, true, true}}, {RouteForeign, RoutingPolicy{false, false, true}}, {RouteChina, RoutingPolicy{false, true, false}}}
	for _, tc := range cases {
		p := testProfile()
		p.RouteMode = tc.mode
		if got := p.EffectiveRoutingPolicy(); got != tc.want {
			t.Fatalf("mode=%s got=%+v want=%+v", tc.mode, got, tc.want)
		}
	}
}

func TestBuildPlanExplicitRoutingPolicyUsesCNAndLANArguments(t *testing.T) {
	dir := t.TempDir()
	if _, err := ipset.WriteCNBundle(dir, "test", []netip.Prefix{netip.MustParsePrefix("1.2.0.0/16")}); err != nil {
		t.Fatal(err)
	}
	policy := RoutingPolicy{ProxyLAN: true, ProxyChina: false, ProxyOther: true}
	p := testProfile()
	p.CNSetDir = dir
	p.RoutingPolicy = &policy
	plan, err := BuildPlan(p, testUnderlay(), strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	if !argPair(plan.RouteApply.Args, "-Mode", "Full") || !argPair(plan.RouteApply.Args, "-DirectPrefixFile4", filepath.Join(dir, ipset.CNIPv4File)) || !slices.Contains(plan.RouteApply.Args, "-CaptureLAN") {
		t.Fatalf("route args=%v", plan.RouteApply.Args)
	}
}

func TestBuildPlanLANDirectNeverEnablesCaptureLAN(t *testing.T) {
	p := testProfile()
	p.RoutingPolicy = &RoutingPolicy{ProxyLAN: false, ProxyChina: true, ProxyOther: true}
	p.DNSMode = DNSSystem
	plan, err := BuildPlan(p, testUnderlay(), strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(plan.RouteApply.Args, "-CaptureLAN") {
		t.Fatalf("LAN-direct policy unexpectedly enabled -CaptureLAN: %v", plan.RouteApply.Args)
	}
	if !argPair(plan.RouteApply.Args, "-Mode", "Full") {
		t.Fatalf("unexpected route mode for LAN-direct full proxy: %v", plan.RouteApply.Args)
	}
}

func TestRoutingPolicyAutoDNSFollowsOtherTraffic(t *testing.T) {
	for _, tc := range []struct {
		other bool
		want  bool
	}{{false, false}, {true, true}} {
		p := testProfile()
		policy := RoutingPolicy{ProxyLAN: false, ProxyChina: !tc.other, ProxyOther: tc.other}
		p.RoutingPolicy = &policy
		p.DNSMode = DNSAuto
		got := resolvedDNSServers(p)
		if (len(got) > 0) != tc.want {
			t.Fatalf("other=%v DNS=%v", tc.other, got)
		}
	}
}

func TestRoutingPolicyActionCase(t *testing.T) {
	raw := os.Getenv("WBD_ROUTING_POLICY_CASE")
	if raw == "" {
		t.Skip("Actions-only policy selector")
	}
	if len(raw) != 3 {
		t.Fatalf("case=%q", raw)
	}
	policy := RoutingPolicy{ProxyLAN: raw[0] == '1', ProxyChina: raw[1] == '1', ProxyOther: raw[2] == '1'}
	p := testProfile()
	p.CNSetDir = `C:\wbd-cn`
	p.RoutingPolicy = &policy
	plan := buildRoutingPlan(p)
	wantMode := "None"
	if policy.ProxyOther {
		wantMode = "Full"
	} else if policy.ProxyChina || policy.ProxyLAN {
		wantMode = "Split"
	}
	if plan.Mode != wantMode || plan.CaptureLAN != policy.ProxyLAN || p.RequiresCNSet() != (policy.ProxyChina != policy.ProxyOther) {
		t.Fatalf("case=%s plan=%+v needsCN=%v", raw, plan, p.RequiresCNSet())
	}
}
