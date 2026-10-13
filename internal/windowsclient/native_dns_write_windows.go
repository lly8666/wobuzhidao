//go:build windows

package windowsclient

import (
 "context"
 "fmt"
 "net/netip"
 "runtime"
 "strings"
 "syscall"
 "unsafe"

 "golang.org/x/sys/windows"
)

var n4SetInterfaceDNS = n4Iphlpapi.NewProc("SetInterfaceDnsSettings")

// n4OwnedDNSWrite is not called by the product yet: its caller MUST first
// prove that the target adapter is WBD-owned and that a crash-persistent WFP
// DNS/IPv6 protection guard is armed. The old runtime is left untouched until
// those independently qualified preconditions exist.
type n4OwnedDNSWrite struct { Adapter n4OwnedInterfaceProof }

const n4DNSOwnedStaticMask = n4DNSSettingNameServer

// Only the precise static IPv4 NameServer field may be changed. Windows DNS
// interface settings are a bitmap-based partial update: do not silently
// overwrite suffix, search path, mDNS, LLMNR, dynamic DNS registration, IPv6,
// profile DNS or any other owner-controlled interface property.
func n4StaticDNSRequest(addrs []netip.Addr) (n4DNSInterfaceSettingsV1, []uint16, error) {
 var req n4DNSInterfaceSettingsV1
 req.Version=n4DNSSettingsVersion1
 req.Flags=n4DNSOwnedStaticMask
 if len(addrs)<1 || len(addrs)>8 {return req,nil,ErrNetworkPlan}
 var names []string
 seen:=make(map[netip.Addr]struct{},len(addrs))
 for _,a:=range addrs {
  if !a.Is4() || !a.IsGlobalUnicast() {
   return n4DNSInterfaceSettingsV1{},nil,ErrNetworkPlan
  }
  if _,ok:=seen[a];ok{return n4DNSInterfaceSettingsV1{},nil,ErrNetworkPlan}
  seen[a]=struct{}{}
  names=append(names,a.String())
 }
 raw,err:=windows.UTF16FromString(strings.Join(names,","))
 if err!=nil{return n4DNSInterfaceSettingsV1{},nil,fmt.Errorf("DNS native UTF16: %w",err)}
 req.NameServer=&raw[0]
 return req,raw,nil
}

func n4ReadOwnedDNSBaseline(b N4DNSBaseline)error {
 // Updating a Wintun adapter which has a foreign/static resolver, a DNS
 // search suffix, profile server, registered dynamic names, or IPv6 settings
 // could hijack another app's state or make correct rollback impossible.
 // The caller must separately attest that this interface is a WBD-owned
 // adapter for the *current generation*, not merely matching a display name.
 if b.Interface==(windows.GUID{}) {return ErrN4ForeignRoute}
 if b.RawNameServer!="" || b.RawProfileNameServer!="" ||
  b.RawSearchList!="" || b.RawDomain!="" ||
  b.Flags&(^uint64(n4DNSOwnedStaticMask))!=0 ||
  b.Flags&n4DNSOwnedStaticMask!=0 {
  return ErrN4ForeignRoute
 }
 return nil
}

// SetIPv4 installs only the IPv4 static NameServer field. Its caller must
// already have persisted a WAL intent and armed an owned persistent WFP guard.
func (w n4OwnedDNSWrite) SetIPv4(ctx context.Context, guard CrashPersistentGuard, iface windows.GUID, index uint32, addrs []netip.Addr) error {
 if err:=ctx.Err();err!=nil{return err}
 req,storage,err:=n4StaticDNSRequest(addrs)
 if err!=nil{return err}
 return w.writeSetting(ctx,guard,iface,index,req,storage)
}

// ClearIPv4 requests removal of the static IPv4 NameServer override only:
// DNS_SETTING_NAMESERVER with a nil NameServer leaves every OTHER DNS field
// unspecified. The caller must independently read back and compare with the
// original empty/static-free baseline before retiring any WAL receipt.
// This Windows behavior needs a real driver/admin positive capability check;
// do not activate the official product until that test is available.
func (w n4OwnedDNSWrite) ClearIPv4(ctx context.Context,guard CrashPersistentGuard,iface windows.GUID,index uint32)error{
 if err:=ctx.Err();err!=nil{return err}
 req:=n4DNSInterfaceSettingsV1{Version:n4DNSSettingsVersion1,Flags:n4DNSOwnedStaticMask}
 return w.writeSetting(ctx,guard,iface,index,req,nil)
}
func (w n4OwnedDNSWrite) writeSetting(ctx context.Context, guard CrashPersistentGuard, iface windows.GUID, index uint32, req n4DNSInterfaceSettingsV1,storage []uint16)error{
 if err:=ctx.Err();err!=nil{return err}
 if guard==nil || !guard.Durable(){return ErrN4NativeGuard}
 ok,err:=guard.Verify(ctx)
 if err!=nil || !ok{return fmt.Errorf("%w: guard not verified: %v",ErrN4NativeGuard,err)}
 if iface==(windows.GUID{}) || index==0{return ErrNetworkPlan}
 if req.Version!=n4DNSSettingsVersion1 || req.Flags!=n4DNSOwnedStaticMask{
  return ErrNetworkPlan
 }
 // A GUID supplied by a caller is NOT adapter ownership.
 if w.Adapter==nil{return ErrN4NativeGuard}
 owned,err:=w.Adapter.IsCurrentOwnedWintun(ctx,index)
 if err!=nil{return err}
 if !owned{return ErrN4ForeignRoute}
 actual,err:=n4NativeInterfaceGUID(index)
 if err!=nil{return err}
 if actual!=iface{return ErrN4ForeignRoute}
 if err:=ctx.Err();err!=nil{return err}
 version:=windows.RtlGetVersion()
 if err:=n4RequireNativeDNSBuild(version.MajorVersion,version.BuildNumber);err!=nil{return err}
 if err:=n4SetInterfaceDNS.Find();err!=nil{return err}
 // Win64 ABI: GUID passed by reference, Settings points to 64-byte V1.
 ret,_,_:=n4SetInterfaceDNS.Call(uintptr(unsafe.Pointer(&iface)),uintptr(unsafe.Pointer(&req)))
 runtime.KeepAlive(storage)
 runtime.KeepAlive(iface)
 if ret!=0{return fmt.Errorf("SetInterfaceDnsSettings: %w",syscall.Errno(ret))}
 return nil
}
