package windowsclient

import (
	"net/netip"
	"strings"
	"testing"
	"github.com/lly8666/wobuzhidao/internal/splitroute"
)

func TestSplitCaptureRetainsPhysicalLANAndForcesExplicitDNS(t *testing.T) {
	bypass,err:=splitroute.Direct("bypass-lan-cn","");if err!=nil{t.Fatal(err)}
	p,err:=BuildNetworkPlan(Config{AdapterAlias:"WBD",TunnelMTU:1249,Lease4:netip.MustParsePrefix("10.66.0.2/32"),Server4:netip.MustParseAddr("8.8.8.8"),Physical:PhysicalPath{InterfaceIndex:12,NextHop4:netip.MustParseAddr("192.168.1.1")},StatePath:"state.json",Bypass4:bypass,DNSServers4:[]netip.Addr{netip.MustParseAddr("223.5.5.5")}});if err!=nil{t.Fatal(err)}
	var cap []netip.Prefix;for _,r:=range p.CaptureRoutes{cap=append(cap,r.Prefix)}
	for _,s:=range []string{"192.168.1.5","127.0.0.1","8.8.8.8","1.0.1.1"}{if splitroute.Contains(cap,netip.MustParseAddr(s)){t.Fatal("captured bypass",s)}}
	for _,s:=range []string{"1.1.1.1","223.5.5.5"}{if !splitroute.Contains(cap,netip.MustParseAddr(s)){t.Fatal("lost capture",s)}}
	if _,err=p.PowerShellArgs("Apply","script.ps1");err==nil{t.Fatal("large set sent as huge argv")}
	p.CapturePrefixFile4="C:\\capture.txt";args,err:=p.PowerShellArgs("Apply","script.ps1");if err!=nil||!strings.Contains(strings.Join(args," "),"-CapturePrefixFile4 C:\\capture.txt"){t.Fatal(args,err)}
	t.Logf("kernel capture prefixes=%d; no per-packet userspace classification",len(cap))
}
