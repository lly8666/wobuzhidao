package windowsruntime

import (
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/ipset"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func testProfile() Profile {
	return Profile{
		BinDir:`C:\Program Files\WBD`, ServerFront:"198.51.100.10:40443", ServerName:"front.example", RouteKey:"0123456789abcdef", Username:"solo", Password:"shared-password", ServerRaw:"198.51.100.10:40443", FEC:"20:20", IfName:"WBD", MTU:1400, RouteMode:RouteFull,
		InstallationID:"00112233445566778899aabbccddeeff", Lanes:1, TunnelIPv4:"10.66.0.1/32",
		TicketPath:`C:\ProgramData\WBD\ticket.tmp`, TunnelConfigPath:`C:\ProgramData\WBD\tunnel-config.json`, RouteState:`C:\ProgramData\WBD\route-state.json`,
	}
}
func testUnderlay() Underlay { return Underlay{SourceIP:"192.0.2.20", PacketDevice:`\Device\NPF_{01234567-89AB-CDEF-0123-456789ABCDEF}`, SourceMAC:"00:11:22:33:44:55", NextHopMAC:"66:77:88:99:aa:bb"} }

func TestBuildPlanUsesGlobalSingleFlowWindowsStack(t *testing.T) {
	p, err := BuildPlan(testProfile(), testUnderlay(), strings.Repeat("ab", 32)); if err != nil { t.Fatal(err) }
	if p.FakeTCP.Name!="faketcp"||p.DTLS.Name!="dtls"||p.Link.Name!="link"||p.TUN.Name!="tun"{t.Fatalf("unexpected runtime commands: %+v",p)}
	if !slices.Contains(p.FakeTCP.Args,"legacy"){t.Fatalf("FakeTCP release recovery not pinned: %v",p.FakeTCP.Args)}
	if !slices.Contains(p.FakeTCP.Args,testUnderlay().PacketDevice)||!slices.Contains(p.FakeTCP.Args,testUnderlay().SourceMAC)||!slices.Contains(p.FakeTCP.Args,testUnderlay().NextHopMAC){t.Fatalf("Npcap underlay identity missing: %v",p.FakeTCP.Args)}
	if !argPair(p.FakeTCP.Args,"--remote",testProfile().ServerRaw){t.Fatalf("single public endpoint missing: %v",p.FakeTCP.Args)}
	if !argPair(p.FakeTCP.Args,"--reality-server-name",testProfile().ServerName)||!argPair(p.FakeTCP.Args,"--reality-route-key",testProfile().RouteKey)||!argPair(p.FakeTCP.Args,"--reality-username",testProfile().Username)||!argPair(p.FakeTCP.Args,"--reality-password",testProfile().Password)||!argPair(p.FakeTCP.Args,"--reality-ticket-out",testProfile().TicketPath)||!argPair(p.FakeTCP.Args,"--reality-installation-id",testProfile().InstallationID)||!argPair(p.FakeTCP.Args,"--reality-tunnel-config-out",testProfile().TunnelConfigPath){t.Fatalf("Reality bootstrap must be inside FakeTCP command: %v",p.FakeTCP.Args)}
	if got:=p.DTLS.Args;!slices.Equal(got,[]string{"client","46101","127.0.0.1","45101","none","none"}){t.Fatalf("DTLS client contract = %v",got)}
	if !slices.Contains(p.Link.Args,"20:20")||!slices.Contains(p.Link.Args,"1"){t.Fatalf("immutable LINK settings missing: %v",p.Link.Args)}
	if p.IPv6Apply.Name!="ipv6-apply"||!hasArgSuffix(p.IPv6Apply.Args,"windows_ipv6_killswitch.ps1")||!slices.Contains(p.IPv6Apply.Args,"Apply"){t.Fatalf("device IPv6 fail-close missing: %v",p.IPv6Apply)}
	if p.IPv6Cleanup.Name!="ipv6-cleanup"||!slices.Contains(p.IPv6Cleanup.Args,"Cleanup"){t.Fatalf("IPv6 cleanup missing: %v",p.IPv6Cleanup)}
	if !slices.Contains(p.RouteApply.Args,"198.51.100.10"){t.Fatalf("raw endpoint underlay escape missing: %v",p.RouteApply.Args)}
	if !argPair(p.RouteApply.Args,"-TunnelAddress4","10.66.0.1/32"){t.Fatalf("server-assigned tunnel address missing: %v",p.RouteApply.Args)}
	if !argPair(p.RouteApply.Args,"-DNSServer","1.1.1.1,1.0.0.1"){t.Fatalf("Full Auto DNS must use Cloudflare pair through WBD: %v",p.RouteApply.Args)}
}

func TestBuildFakeTCPCommandDoesNotDialSeparateRealityFront(t *testing.T) {
	cmd,err:=BuildFakeTCPCommand(testProfile(),testUnderlay());if err!=nil{t.Fatal(err)}
	if cmd.Name!="faketcp"{t.Fatalf("command=%+v",cmd)}
	if slices.Contains(cmd.Args,"reality-bootstrap"){t.Fatalf("unexpected separate bootstrap marker: %v",cmd.Args)}
	if !argPair(cmd.Args,"--remote","198.51.100.10:40443"){t.Fatalf("remote=%v",cmd.Args)}
	if !argPair(cmd.Args,"--reality-installation-id","00112233445566778899aabbccddeeff"){t.Fatalf("installation identity missing: %v",cmd.Args)}
	if !argPair(cmd.Args,"--reality-tunnel-config-out",testProfile().TunnelConfigPath){t.Fatalf("authenticated tunnel config output missing: %v",cmd.Args)}
}

func TestProfileAllowsExactlyOneProductPublicTransport(t *testing.T) {
	p:=testProfile();p.Lanes=0;if got:=p.normalized().Lanes;got!=1{t.Fatalf("default lane count normalized to %d want 1",got)};if err:=p.Validate();err!=nil{t.Fatalf("default one-flow product rejected: %v",err)}
	p=testProfile();p.Lanes=1;if err:=p.Validate();err!=nil{t.Fatalf("explicit one-flow product rejected: %v",err)}
	for _,lanes:=range []int{-1,2,3,4,5}{p:=testProfile();p.Lanes=lanes;if err:=p.Validate();!errorsIsTransportLane(err){t.Fatalf("lanes=%d unexpectedly accepted/error=%v",lanes,err)}}
}

func errorsIsTransportLane(err error) bool { return err != nil && strings.Contains(err.Error(), logicaltunnel.ErrTransportLanes.Error()) }

func TestProfileAllowsTunnelAddressToBeEmptyUntilAuthenticatedBootstrap(t *testing.T) {
	p:=testProfile();p.TunnelIPv4="";if err:=p.Validate();err!=nil{t.Fatalf("pre-bootstrap profile rejected: %v",err)}
	if _,err:=BuildPlan(p,testUnderlay(),strings.Repeat("ab",32));err==nil||!strings.Contains(err.Error(),"authenticated tunnel IPv4"){t.Fatalf("runtime plan accepted missing authenticated address: %v",err)}
}

func TestBuildPlanForeignAndChinaPoliciesUseVerifiedCNBundle(t *testing.T) {
	dir:=t.TempDir();if _,err:=ipset.WriteCNBundle(dir,"test",[]netip.Prefix{netip.MustParsePrefix("1.2.0.0/16")});err!=nil{t.Fatal(err)}
	foreign:=testProfile();foreign.RouteMode=RouteForeign;foreign.CNSetDir=dir
	p,err:=BuildPlan(foreign,testUnderlay(),strings.Repeat("ab",32));if err!=nil{t.Fatal(err)}
	cn4:=filepath.Join(dir,ipset.CNIPv4File)
	if !argPair(p.RouteApply.Args,"-Mode","Full")||!argPair(p.RouteApply.Args,"-DirectPrefixFile4",cn4){t.Fatalf("Foreign route args = %v",p.RouteApply.Args)}
	if !argPair(p.RouteApply.Args,"-DNSServer","1.1.1.1,1.0.0.1"){t.Fatalf("Foreign Auto DNS must stay inside WBD: %v",p.RouteApply.Args)}
	china:=testProfile();china.RouteMode=RouteChina;china.CNSetDir=dir
	p,err=BuildPlan(china,testUnderlay(),strings.Repeat("ab",32));if err!=nil{t.Fatal(err)}
	if !argPair(p.RouteApply.Args,"-Mode","Split")||!argPair(p.RouteApply.Args,"-PrefixFile4",cn4){t.Fatalf("China route args = %v",p.RouteApply.Args)}
	if slices.Contains(p.RouteApply.Args,"-DNSServer"){t.Fatalf("China Auto DNS should keep the system resolver: %v",p.RouteApply.Args)}
}

func TestBuildPlanRejectsTamperedCNBundle(t *testing.T) {
	dir:=t.TempDir();if _,err:=ipset.WriteCNBundle(dir,"test",[]netip.Prefix{netip.MustParsePrefix("1.2.0.0/16")});err!=nil{t.Fatal(err)}
	if err:=os.WriteFile(filepath.Join(dir,ipset.CNIPv4File),[]byte("8.8.8.0/24\n"),0o600);err!=nil{t.Fatal(err)}
	p:=testProfile();p.RouteMode=RouteForeign;p.CNSetDir=dir
	if _,err:=BuildPlan(p,testUnderlay(),strings.Repeat("ab",32));err==nil{t.Fatal("tampered CN bundle unexpectedly accepted")}
}

func TestPlanStartAndStopOrder(t *testing.T) {
	p,err:=BuildPlan(testProfile(),testUnderlay(),strings.Repeat("01",32));if err!=nil{t.Fatal(err)}
	wantStart:=[]string{"faketcp","dtls","link","tun","ipv6-apply","route-apply"};if got:=commandNames(p.StartSequence());!slices.Equal(got,wantStart){t.Fatalf("start order = %v want %v",got,wantStart)}
	wantStop:=[]string{"route-cleanup","ipv6-cleanup","tun","link","dtls","faketcp"};if got:=commandNames(p.StopSequence());!slices.Equal(got,wantStop){t.Fatalf("stop order = %v want %v",got,wantStop)}
}

func TestProfileRejectsUnfrozenOrUnsafeSettings(t *testing.T) {
	cases:=[]struct{name string;mutate func(*Profile)}{
		{"auto-fec",func(p *Profile){p.FEC="auto"}},
		{"bad-route-key",func(p *Profile){p.RouteKey="short"}},
		{"hostname-raw",func(p *Profile){p.ServerRaw="example.com:40443"}},
		{"two-public-endpoints",func(p *Profile){p.ServerRaw="198.51.100.10:40000"}},
		{"unknown-route-mode",func(p *Profile){p.RouteMode="Split"}},
		{"foreign-without-cn-set",func(p *Profile){p.RouteMode=RouteForeign;p.CNSetDir=""}},
		{"bad-dns-mode",func(p *Profile){p.DNSMode="Magic"}},
		{"bad-custom-dns",func(p *Profile){p.DNSMode=DNSCustom;p.DNSServer="resolver.example"}},
		{"bad-installation-id",func(p *Profile){p.InstallationID="bad"}},
		{"missing-tunnel-config-path",func(p *Profile){p.TunnelConfigPath=""}},
	}
	for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){p:=testProfile();tc.mutate(&p);if err:=p.Validate();err==nil{t.Fatalf("expected validation failure: %+v",p)}})}
}

func commandNames(commands []Command) []string { out:=make([]string,0,len(commands));for _,cmd:=range commands{out=append(out,cmd.Name)};return out }
func argPair(args []string,key,value string) bool { for i:=0;i+1<len(args);i++{if args[i]==key&&args[i+1]==value{return true}};return false }
func hasArgSuffix(args []string,suffix string) bool { for _,arg:=range args{if strings.HasSuffix(strings.ReplaceAll(arg,`\`,`/`),"/"+suffix)||arg==suffix{return true}};return false }
