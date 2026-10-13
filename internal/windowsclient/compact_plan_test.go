package windowsclient

import (
    "net/netip"
    "testing"
)
func TestN4CompactRoutesDoNotMaterializeChinaCIDRs(t *testing.T) {
    base:=Config{
        AdapterAlias:"WBD",TunnelMTU:1249,
        Lease4:netip.MustParsePrefix("10.66.0.7/32"),
        Server4:netip.MustParseAddr("198.51.100.10"),
        Physical:PhysicalPath{InterfaceIndex:12,NextHop4:netip.MustParseAddr("192.0.2.1")},
        StatePath:"state.json",
        DNSServers4:[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")},
    }
    base.Bypass4=make([]netip.Prefix,8192)
    for i:=range base.Bypass4 {
        base.Bypass4[i]=netip.PrefixFrom(netip.AddrFrom4([4]byte{byte(i/256),byte(i%256),0,0}),24)
    }
    base.Direct4=[]netip.Prefix{netip.MustParsePrefix("203.0.113.0/24")}
    plan,err:=BuildCompactNetworkPlan(base)
    if err!=nil{t.Fatal(err)}
    if len(plan.CaptureRoutes)!=4 || len(plan.DirectRoutes)!=0 {
        t.Fatalf("CN prefix array became Windows routes: capture=%d direct=%d",len(plan.CaptureRoutes),len(plan.DirectRoutes))
    }
    if plan.UnderlayRoute.Prefix.String()!="198.51.100.10/32" {
        t.Fatalf("physical server escape lost: %+v",plan.UnderlayRoute)
    }
}
