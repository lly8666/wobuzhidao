//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "net"
 "net/netip"
 "testing"
 "unsafe"

 "golang.org/x/sys/windows"
)
func TestN4NativeDNSReadOnlySDKLayout(t *testing.T){
 var s n4DNSInterfaceSettingsV1
 if got:=unsafe.Sizeof(s);got!=64 {t.Fatalf("DNS_INTERFACE_SETTINGS Win64 size=%d; need64",got)}
 if got:=unsafe.Offsetof(s.Flags);got!=8 {t.Fatalf("flags=%d",got)}
 if got:=unsafe.Offsetof(s.NameServer);got!=24 {t.Fatalf("nameserver offset=%d",got)}
 if got:=unsafe.Offsetof(s.ProfileNameServer);got!=56 {t.Fatalf("profile offset=%d",got)}
 if _,err:=n4NativeInterfaceGUID(0);!errors.Is(err,ErrNetworkPlan){t.Fatal("zero interface index accepted")}
 v:=[]uint16{'1','.','1','.','1','.','1',0}
 got,err:=n4DNSSafeUTF16(&v[0]);if err!=nil||got!="1.1.1.1"{t.Fatalf("UTF16 %q error %v",got,err)}
 if str,err:=n4DNSSafeUTF16(nil);err!=nil||str!=""{t.Fatalf("nil string %q/%v",str,err)}
}
func TestN4NativeDNSReadOnlyLiveHostNoMutation(t *testing.T){
 v:=windows.RtlGetVersion()
 if err:=n4RequireNativeDNSBuild(v.MajorVersion,v.BuildNumber);err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=(N4NativeDNSReader{}).Read(ctx,7);!errors.Is(err,context.Canceled){t.Fatalf("cancelled DNS read error=%v",err)}
 interfaces,err:=net.Interfaces()
 if err!=nil{t.Fatal(err)}
 var queried int
 for _,ifc:=range interfaces {
  if ifc.Index<=0 || ifc.Flags&net.FlagLoopback!=0 || ifc.Flags&net.FlagUp==0 {continue}
  baseline,e:=(N4NativeDNSReader{}).Read(context.Background(),uint32(ifc.Index))
  if e!=nil {t.Logf("DNS read unsupported index %d: %v",ifc.Index,e);continue}
  if baseline.Interface==(windows.GUID{}){t.Fatal("empty GUID")}
  if baseline.Flags&n4DNSSettingNameServer!=0 {
   for _,a:=range baseline.NameServers {if !a.IsValid() {t.Fatalf("invalid DNS netip addr: %v",a)}}
  }
  queried++
 }
 if queried==0 {t.Fatal("no nonloopback Windows interface offered a native DNS snapshot; fail closed")}
}
func TestN4NativeDNSListParser(t *testing.T){
 v,err:=parseN4DNSNameserverList("1.1.1.1,8.8.8.8 9.9.9.9")
 if err!=nil||len(v)!=3||v[0]!=netip.MustParseAddr("1.1.1.1"){t.Fatalf("parsed=%v err=%v",v,err)}
 for _,s:=range []string{"1.1.1.1,notIP","", "2001:db8::1", "1.1.1.1,1.1.1.1"}{
  if _,err:=parseN4DNSNameserverList(s);err==nil{t.Fatalf("unsafe DNS accepted %q",s)}
 }
}
