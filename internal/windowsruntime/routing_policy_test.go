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

func TestRoutingPolicyMatrixUsesOneHostRouteShape(t *testing.T) {
	for bits := 0; bits < 8; bits++ {
		policy := RoutingPolicy{ProxyLAN: bits&4 != 0, ProxyChina: bits&2 != 0, ProxyOther: bits&1 != 0}
		p := testProfile()
		p.CNSetDir = filepath.Join("C:\\", "wbd-cn")
		p.RoutingPolicy = &policy
		plan := buildRoutingPlan(p)
		if plan.Mode != "Full" || !plan.CaptureLAN {
			t.Fatalf("case=%03b host route plan=%+v; split policy must stay inside TUN", bits, plan)
		}
		if p.RequiresCNSet() != (policy.ProxyChina != policy.ProxyOther) {
			t.Fatalf("case=%03b needsCN=%v", bits, p.RequiresCNSet())
		}
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

func TestBuildPlanExplicitRoutingPolicyMovesDecisionIntoTun(t *testing.T) {
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
	if !argPair(plan.RouteApply.Args, "-Mode", "Full") || !slices.Contains(plan.RouteApply.Args, "-CaptureLAN") {
		t.Fatalf("host route args=%v", plan.RouteApply.Args)
	}
	for _, forbidden := range []string{"-PrefixFile4", "-DirectPrefixFile4"} {
		if slices.Contains(plan.RouteApply.Args, forbidden) {
			t.Fatalf("host route args leaked split prefix %q: %v", forbidden, plan.RouteApply.Args)
		}
	}
	for _, want := range []string{"-proxy-lan=true", "-proxy-china=false", "-proxy-other=true"} {
		if !slices.Contains(plan.TUN.Args, want) {
			t.Fatalf("TUN args=%v missing %q", plan.TUN.Args, want)
		}
	}
	if !argPair(plan.TUN.Args, "-direct-ifindex", "12") ||
		!argPair(plan.TUN.Args, "-route-state", p.RouteState) ||
		!argPair(plan.TUN.Args, "-cn4", filepath.Join(dir, ipset.CNIPv4File)) {
		t.Fatalf("TUN split inputs=%v", plan.TUN.Args)
	}
}

func TestBuildPlanLANDirectStillCapturesLANIntoTun(t *testing.T) {
	p := testProfile()
	p.RoutingPolicy = &RoutingPolicy{ProxyLAN: false, ProxyChina: true, ProxyOther: true}
	p.DNSMode = DNSSystem
	plan, err := BuildPlan(p, testUnderlay(), strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.RouteApply.Args, "-CaptureLAN") || !argPair(plan.RouteApply.Args, "-Mode", "Full") {
		t.Fatalf("LAN must be captured before in-TUN classification: %v", plan.RouteApply.Args)
	}
	if !slices.Contains(plan.TUN.Args, "-proxy-lan=false") {
		t.Fatalf("LAN-direct decision missing from TUN args: %v", plan.TUN.Args)
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
	if plan.Mode != "Full" || !plan.CaptureLAN || p.RequiresCNSet() != (policy.ProxyChina != policy.ProxyOther) {
		t.Fatalf("case=%s plan=%+v needsCN=%v", raw, plan, p.RequiresCNSet())
	}
}
