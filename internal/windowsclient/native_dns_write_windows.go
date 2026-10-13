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
type n4OwnedDNSWrite struct{}

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

// SetIPv4 is a low-level primitive only. Never expose it through the CLI
// until an owned Wintun lease and persistent WFP guard implement a journaled
// DNS Apply/Recover transaction that survives power loss.
func (n4OwnedDNSWrite) SetIPv4(ctx context.Context, guard CrashPersistentGuard, iface windows.GUID, addrs []netip.Addr) error {
 if err:=ctx.Err();err!=nil{return err}
 if guard==nil || !guard.Durable() {return ErrN4NativeGuard}
 ok,err:=guard.Verify(ctx)
 if err!=nil || !ok {return fmt.Errorf("%w: guard not verified: %v",ErrN4NativeGuard,err)}
 if iface==(windows.GUID{}) {return ErrNetworkPlan}
 version:=windows.RtlGetVersion()
 if err:=n4RequireNativeDNSBuild(version.MajorVersion,version.BuildNumber);err!=nil{return err}
 req,storage,err:=n4StaticDNSRequest(addrs)
 if err!=nil{return err}
 if err:=n4SetInterfaceDNS.Find();err!=nil{return err}
 // GUID is 16 bytes by-value in the SDK signature and is passed by address
 // by the Windows x64 ABI; Settings is a pointer to the V1 64-byte request.
 ret,_,_:=n4SetInterfaceDNS.Call(uintptr(unsafe.Pointer(&iface)),uintptr(unsafe.Pointer(&req)))
 runtime.KeepAlive(storage)
 runtime.KeepAlive(iface)
 if ret!=0{return fmt.Errorf("SetInterfaceDnsSettings: %w",syscall.Errno(ret))}
 return nil
}
