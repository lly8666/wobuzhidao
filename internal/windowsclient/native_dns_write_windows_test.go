//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "net/netip"
 "testing"
 "unicode/utf16"
 "unsafe"

 "golang.org/x/sys/windows"
)
func TestN4NativeDNSOwnedWriteRequestNoMutation(t *testing.T) {
 server:=[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")}
 req,wide,err:=n4StaticDNSRequest(server)
 if err!=nil{t.Fatal(err)}
 if req.Version!=1 || req.Flags!=n4DNSSettingNameServer {t.Fatalf("unexpected request flags %v",req)}
 if req.NameServer==nil || unsafe.Pointer(req.NameServer)!=unsafe.Pointer(&wide[0]) {t.Fatal("request lost owned UTF-16 memory")}
 got:=string(utf16.Decode(wide[:len(wide)-1]))
 if got!="1.1.1.1,8.8.8.8" {t.Fatalf("encoded native resolver=%q",got)}
 if req.Domain!=nil||req.SearchList!=nil||req.ProfileNameServer!=nil ||
  req.EnableLLMNR!=0||req.RegistrationEnabled!=0||req.QueryAdapterName!=0{
  t.Fatal("native DNS request tried to overwrite unrelated settings")
 }
 testcases:=[][]netip.Addr{
  nil,
  {},
  {netip.MustParseAddr("::1")},
  {netip.MustParseAddr("192.168.0.1"),netip.MustParseAddr("192.168.0.1")},
  {netip.MustParseAddr("0.0.0.0")},
  {netip.MustParseAddr("224.0.0.1")},
 }
 for _,tc:=range testcases {
  if _,_,err:=n4StaticDNSRequest(tc);!errors.Is(err,ErrNetworkPlan) {t.Fatalf("invalid DNS request %v: %v",tc,err)}
 }
 many:=make([]netip.Addr,9)
 for i:=range many{many[i]=netip.AddrFrom4([4]byte{1,1,1,byte(i+1)})}
 if _,_,err:=n4StaticDNSRequest(many);!errors.Is(err,ErrNetworkPlan){t.Fatalf("overlimit: %v",err)}
}
func TestN4NativeDNSPreexistingForeignBaselineFailsClosed(t *testing.T){
 guid:=windows.GUID{Data1:23}
 baseline:=N4DNSBaseline{Interface:guid}
 if err:=n4ReadOwnedDNSBaseline(baseline);err!=nil{t.Fatalf("clean owned interface baseline rejected: %v",err)}
 for _,change:=range []struct{name string;f func(*N4DNSBaseline)}{
  {"empty-guid",func(b *N4DNSBaseline){b.Interface=windows.GUID{}}},
  {"static-ip",func(b *N4DNSBaseline){b.RawNameServer="9.9.9.9"}},
  {"dns-flags",func(b *N4DNSBaseline){b.Flags=n4DNSSettingNameServer}},
  {"profile-server",func(b *N4DNSBaseline){b.RawProfileNameServer="1.1.1.1"}},
  {"domain",func(b *N4DNSBaseline){b.RawDomain="corp.local"}},
  {"suffix",func(b *N4DNSBaseline){b.RawSearchList="corp.local"}},
  {"ipv6",func(b *N4DNSBaseline){b.Flags=1}},
  {"llmnr",func(b *N4DNSBaseline){b.Flags=128}},
 }{
  t.Run(change.name,func(t *testing.T){
   copy:=baseline;change.f(&copy)
   if err:=n4ReadOwnedDNSBaseline(copy);!errors.Is(err,ErrN4ForeignRoute){t.Fatalf("foreign DNS baseline accepted: %v",err)}
  })
 }
}
func TestN4NativeDNSSetterDeniedBeforeDLLSideEffects(t *testing.T){
 g:=windows.GUID{Data1:7}
 ctx,cancel:=context.WithCancel(context.Background())
 cancel()
 if err:=(n4OwnedDNSWrite{}).SetIPv4(ctx,nil,g,[]netip.Addr{netip.MustParseAddr("1.1.1.1")});!errors.Is(err,context.Canceled){
  t.Fatalf("cancel not enforced before DLL: %v",err)
 }
 if err:=(n4OwnedDNSWrite{}).SetIPv4(context.Background(),&n4FakeGuard{durable:true,active:true},windows.GUID{},nil);!errors.Is(err,ErrNetworkPlan){
  t.Fatalf("invalid Wintun GUID accepted: %v",err)
 }
 if err:=(n4OwnedDNSWrite{}).SetIPv4(context.Background(),&n4FakeGuard{durable:true,active:true},g,nil);!errors.Is(err,ErrNetworkPlan){
  t.Fatalf("empty DNS list accepted: %v",err)
 }
 guard:=&n4FakeGuard{durable:false}
 if err:=(n4OwnedDNSWrite{}).SetIPv4(context.Background(),guard,g,[]netip.Addr{netip.MustParseAddr("1.1.1.1")});!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("volatile guard permitted DNS write: %v",err)
 }
 guard.durable=true
 if err:=(n4OwnedDNSWrite{}).SetIPv4(context.Background(),guard,g,[]netip.Addr{netip.MustParseAddr("1.1.1.1")});!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("durable but unarmed guard permitted DNS write: %v",err)
 }
 // No successful SetInterfaceDnsSettings call is made in this test.
 _=strings.TrimSpace
}
