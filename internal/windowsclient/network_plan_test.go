package windowsclient

import (
	"errors"
	"net/netip"
	"strings"
	"testing"
)

func TestBuildNetworkPlanOwnsLeaseRoutesDNSAndIPv6KillSwitch(t *testing.T) {
	plan,err:=BuildNetworkPlan(Config{
		AdapterAlias:"WBD",
		Lease4:netip.MustParsePrefix("10.66.0.7/32"),
		Server4:netip.MustParseAddr("198.51.100.10"),
		Physical:PhysicalPath{InterfaceIndex:12,NextHop4:netip.MustParseAddr("192.0.2.1")},
		DNSServers4:[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")},
		Direct4:[]netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")},
		StatePath:"C:\\ProgramData\\WBD\\route-state.json",
	})
	if err!=nil { t.Fatal(err) }
	if plan.Schema!=StateSchema || plan.Lease4.String()!="10.66.0.7/32" { t.Fatalf("plan=%+v",plan) }
	if plan.UnderlayRoute.Prefix.String()!="198.51.100.10/32" || plan.UnderlayRoute.InterfaceIndex!=12 || plan.UnderlayRoute.NextHop.String()!="192.0.2.1" { t.Fatalf("underlay=%+v",plan.UnderlayRoute) }
	if len(plan.DirectRoutes)!=1 || plan.DirectRoutes[0].Prefix.String()!="203.0.113.0/24" { t.Fatalf("direct=%+v",plan.DirectRoutes) }
	got:=map[string]bool{}
	for _,r:=range plan.CaptureRoutes { got[r.Prefix.String()]=true }
	for _,want:=range []string{"0.0.0.0/1","128.0.0.0/1","1.1.1.1/32","8.8.8.8/32"} {
		if !got[want] { t.Fatalf("capture missing %s: %+v",want,plan.CaptureRoutes) }
	}
	if plan.NRPTNamespace!="." || plan.NRPTDisplay!=NRPTDisplayName || plan.NRPTComment!=NRPTComment { t.Fatalf("nrpt=%+v",plan) }
	if len(plan.IPv6BlockPrefixes)!=2 || plan.IPv6BlockPrefixes[0].String()!="::/1" || plan.IPv6BlockPrefixes[1].String()!="8000::/1" { t.Fatalf("ipv6=%v",plan.IPv6BlockPrefixes) }
	args,err:=plan.PowerShellArgs("Apply","C:\\wbd\\windows_client_network.ps1")
	if err!=nil { t.Fatal(err) }
	joined:=strings.Join(args," ")
	for _,want:=range []string{"-AdapterAlias WBD","-TunnelAddress4 10.66.0.7/32","-Underlay4 198.51.100.10","-PhysicalInterfaceIndex 12","-PhysicalNextHop4 192.0.2.1","-DNSServer 1.1.1.1,8.8.8.8","-DirectPrefix4 203.0.113.0/24"} {
		if !strings.Contains(joined,want) { t.Fatalf("args missing %q: %s",want,joined) }
	}
}

func TestNetworkPlanRejectsUnsafeIdentityOrPhysicalPath(t *testing.T) {
	base:=Config{
		AdapterAlias:"WBD",
		Lease4:netip.MustParsePrefix("10.66.0.7/32"),
		Server4:netip.MustParseAddr("198.51.100.10"),
		Physical:PhysicalPath{InterfaceIndex:12,NextHop4:netip.MustParseAddr("192.0.2.1")},
		StatePath:"state.json",
	}
	cases:=[]Config{
		func() Config { c:=base;c.AdapterAlias="";return c }(),
		func() Config { c:=base;c.Lease4=netip.MustParsePrefix("10.66.0.0/24");return c }(),
		func() Config { c:=base;c.Server4=netip.MustParseAddr("2001:db8::1");return c }(),
		func() Config { c:=base;c.Physical.InterfaceIndex=0;return c }(),
		func() Config { c:=base;c.Physical.NextHop4=netip.MustParseAddr("2001:db8::1");return c }(),
		func() Config { c:=base;c.DNSServers4=[]netip.Addr{netip.MustParseAddr("2001:4860:4860::8888")};return c }(),
		func() Config { c:=base;c.StatePath="";return c }(),
	}
	for _,c:=range cases {
		if _,err:=BuildNetworkPlan(c); !errors.Is(err,ErrNetworkPlan) { t.Fatalf("cfg=%+v err=%v",c,err) }
	}
}
