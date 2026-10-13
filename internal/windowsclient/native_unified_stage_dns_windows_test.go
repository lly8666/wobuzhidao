//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "net/netip"
 "testing"

 "golang.org/x/sys/windows"
)
func n4DNSStageTestFixture()(n4WindowsDNSStage,N4UnifiedIdentity,n4DNSStageBaseline){
 id:=n4TestConcreteNativeIdentity()
 servers:=[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")}
 stage:=n4WindowsDNSStage{
  Servers:servers,Guard:&n4FakeGuard{durable:true,active:true},
  Writer:n4OwnedDNSWrite{Adapter:&n4DNSFakeInterfaceOwner{owned:false}},
 }
 rec:=n4DNSStageBaseline{Index:id.WintunIndex,
  Before:N4DNSBaseline{Interface:windows.GUID{Data1:173}},Servers:servers}
 return stage,id,rec
}
func TestN4WindowsDNSStageClassifiesOnlyExactStaticOverride(t *testing.T){
 stage,_,rec:=n4DNSStageTestFixture()
 if stage.Name()!="dns"{t.Fatal(stage.Name())}
 now:=rec.Before
 if got:=n4DNSClassifyState(now,rec);got!=N4UnifiedAbsent{t.Fatal(got)}
 now.Flags=n4DNSOwnedStaticMask
 now.NameServers=append([]netip.Addr(nil),rec.Servers...)
 now.RawNameServer="1.1.1.1,8.8.8.8"
 if got:=n4DNSClassifyState(now,rec);got!=N4UnifiedOwned{t.Fatal(got)}
 now.EnableLLMNR=1
 if got:=n4DNSClassifyState(now,rec);got!=N4UnifiedForeign{t.Fatal("foreign LLMNR changed")}
 now.EnableLLMNR=0;now.RawDomain="foreign.local"
 if got:=n4DNSClassifyState(now,rec);got!=N4UnifiedForeign{t.Fatal("foreign suffix changed")}
 now.RawDomain="";now.NameServers[0]=netip.MustParseAddr("9.9.9.9")
 if got:=n4DNSClassifyState(now,rec);got!=N4UnifiedForeign{t.Fatal("foreign resolver changed")}
 now.NameServers=rec.Servers;now.Flags=0
 if got:=n4DNSClassifyState(now,rec);got!=N4UnifiedOwned{
  t.Fatal("real Windows Get Flags=0 must still classify exact raw static DNS as owned")
 }
 now.RawNameServer=""
 if got:=n4DNSClassifyState(now,rec);got!=N4UnifiedForeign{
  t.Fatal("parsed entries without a real native nameserver string are not owned")
 }
}
func TestN4WindowsDNSStageDeniedBeforeNativeAPIsOnBorrowedOrTampered(t *testing.T){
 stage,id,rec:=n4DNSStageTestFixture()
 if _,err:=stage.Snapshot(context.Background(),id);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("borrowed Windows adapter reached DNS API: %v",err)
 }
 owner:=stage.Writer.Adapter.(*n4DNSFakeInterfaceOwner)
 if owner.called!=1{t.Fatal("owner proof bypassed")}
 b,err:=json.Marshal(rec);if err!=nil{t.Fatal(err)}
 if _,err:=stage.decode(id,b);err!=nil{t.Fatal(err)}
 for _,mutate:=range []func(*n4DNSStageBaseline){
  func(r *n4DNSStageBaseline){r.Index++},
  func(r *n4DNSStageBaseline){r.Servers=[]netip.Addr{netip.MustParseAddr("9.9.9.9")}},
  func(r *n4DNSStageBaseline){r.Before.Flags=n4DNSOwnedStaticMask},
  func(r *n4DNSStageBaseline){r.Before.Interface=windows.GUID{}},
 }{
  altered:=rec;mutate(&altered)
  raw,err:=json.Marshal(altered);if err!=nil{t.Fatal(err)}
  if err:=stage.Apply(context.Background(),id,raw);!errors.Is(err,ErrN4JournalConflict){
   t.Fatalf("tampered DNS WAL reached native setter: %v",err)
  }
 }
 if owner.called!=1{t.Fatal("invalid WAL accessed network")}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=stage.Apply(ctx,id,b);!errors.Is(err,context.Canceled){
  t.Fatalf("canceled DNS Apply touched Win32: %v",err)
 }
}
func TestN4WindowsDNSClearIPv4RequiresArmedPersistentGuardAndCurrentOwner(t *testing.T){
 iface:=windows.GUID{Data1:13}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=(n4OwnedDNSWrite{}).ClearIPv4(ctx,nil,iface,21);!errors.Is(err,context.Canceled){
  t.Fatalf("cancelled ClearIPv4: %v",err)
 }
 if err:=(n4OwnedDNSWrite{}).ClearIPv4(context.Background(),nil,iface,21);!errors.Is(err,ErrN4NativeGuard){t.Fatal(err)}
 armed:=&n4FakeGuard{durable:true,active:true}
 if err:=(n4OwnedDNSWrite{}).ClearIPv4(context.Background(),armed,iface,21);!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("missing owner allowed DNS reset: %v",err)
 }
 owner:=&n4DNSFakeInterfaceOwner{owned:false}
 w:=n4OwnedDNSWrite{Adapter:owner}
 if err:=w.ClearIPv4(context.Background(),armed,iface,21);!errors.Is(err,ErrN4ForeignRoute){t.Fatal(err)}
 if owner.called!=1{t.Fatal("missing direct owner check")}
 // None of these invokes SetInterfaceDnsSettings.
}
func TestN4WindowsDNSResetRequestIsNameserverOnly(t *testing.T){
 req:=n4DNSInterfaceSettingsV1{Version:n4DNSSettingsVersion1,Flags:n4DNSOwnedStaticMask}
 if req.NameServer!=nil || req.Domain!=nil || req.SearchList!=nil ||
  req.ProfileNameServer!=nil || req.Flags!=n4DNSSettingNameServer{
  t.Fatal("DNS reset could overwrite foreign fields")
 }
 // The actual reset's success must be qualified with readback on a real
 // admin/Wintun host; a request layout test never counts as live recovery.
}

func TestN4WindowsDNSEmptyClearFallbackOnlyTouchesNameServerField(t *testing.T){
 req,storage:=n4DNSEmptyStringClearRequest()
 if req.Version!=n4DNSSettingsVersion1 ||
  req.Flags!=n4DNSSettingNameServer || req.NameServer==nil ||
  len(storage)!=1 || storage[0]!=0 || *req.NameServer!=0 ||
  req.Domain!=nil || req.SearchList!=nil ||
  req.ProfileNameServer!=nil ||
  req.RegistrationEnabled!=0 || req.EnableLLMNR!=0 {
  t.Fatalf("invalid Win2022 DNS static-clear fallback partial request: %+v",req)
 }
 // The real admin GH Windows2022 test must still observe exact
 // post-clear native Get baseline and reject a no-op as a failure.
}
