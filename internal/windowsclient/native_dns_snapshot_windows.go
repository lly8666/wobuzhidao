//go:build windows

package windowsclient

import (
 "context"
 "fmt"
 "net/netip"
 "syscall"
 "unicode/utf16"
 "unsafe"

 "golang.org/x/sys/windows"
)

// dnsInterfaceSettingsV1 is the 64-byte Win64 DNS_INTERFACE_SETTINGS V1
// structure from Microsoft netioapi.h, Windows 10 build 19041+.
// FreeInterfaceDnsSettings is mandatory after a successful query.
type n4DNSInterfaceSettingsV1 struct {
 Version uint32
 _ uint32
 Flags uint64
 Domain *uint16
 NameServer *uint16
 SearchList *uint16
 RegistrationEnabled uint32
 RegisterAdapterName uint32
 EnableLLMNR uint32
 QueryAdapterName uint32
 ProfileNameServer *uint16
}

const (
 n4DNSSettingsVersion1 uint32 = 1
 n4DNSSettingNameServer uint64 = 0x2
)
var (
 n4ConvertIndexLuid=n4Iphlpapi.NewProc("ConvertInterfaceIndexToLuid")
 n4ConvertLuidGuid=n4Iphlpapi.NewProc("ConvertInterfaceLuidToGuid")
 n4GetInterfaceDNS=n4Iphlpapi.NewProc("GetInterfaceDnsSettings")
 n4FreeInterfaceDNS=n4Iphlpapi.NewProc("FreeInterfaceDnsSettings")
)

// N4DNSBaseline is a copied read-only snapshot; it owns no pointer to DLL
// memory and can be journaled in a later native configuration transaction.
// Strings must not be logged since they may contain an internal DNS domain.
type N4DNSBaseline struct {
 Interface windows.GUID
 NameServers []netip.Addr
 RawNameServer string
 RawSearchList string
 RawDomain string
 RawProfileNameServer string
 Flags uint64
 RegistrationEnabled uint32
 RegisterAdapterName uint32
 EnableLLMNR uint32
 QueryAdapterName uint32
}

func n4DNSSafeUTF16(p *uint16) (string,error){
 if p==nil {return "",nil}
 // Read at most 1024 UTF-16 units; refuse unexpected unbounded Windows
 // string data rather than scanning arbitrary memory or using it in logs.
 const maxUnits=1024
 vals:=make([]uint16,0,64)
 for i:=uintptr(0);i<maxUnits;i++{
  u:=*(*uint16)(unsafe.Pointer(uintptr(unsafe.Pointer(p))+i*2))
  if u==0{return string(utf16.Decode(vals)),nil}
  vals=append(vals,u)
 }
 return "",fmt.Errorf("%w: DNS native string exceeds bound",ErrNetworkPlan)
}

func n4NativeInterfaceGUID(index uint32)(windows.GUID,error){
 if index==0{return windows.GUID{},ErrNetworkPlan}
 if err:=n4ConvertIndexLuid.Find();err!=nil{return windows.GUID{},err}
 if err:=n4ConvertLuidGuid.Find();err!=nil{return windows.GUID{},err}
 var luid uint64
 rc,_,_:=n4ConvertIndexLuid.Call(uintptr(index),uintptr(unsafe.Pointer(&luid)))
 if rc!=0{return windows.GUID{},fmt.Errorf("ConvertInterfaceIndexToLuid: %w",syscall.Errno(rc))}
 var guid windows.GUID
 rc,_,_ =n4ConvertLuidGuid.Call(uintptr(unsafe.Pointer(&luid)),uintptr(unsafe.Pointer(&guid)))
 if rc!=0{return windows.GUID{},fmt.Errorf("ConvertInterfaceLuidToGuid: %w",syscall.Errno(rc))}
 return guid,nil
}

type N4NativeDNSReader struct{}
func (N4NativeDNSReader) Read(ctx context.Context,index uint32)(N4DNSBaseline,error){
 if err:=ctx.Err();err!=nil{return N4DNSBaseline{},err}
 v:=windows.RtlGetVersion()
 if err:=n4RequireNativeDNSBuild(v.MajorVersion,v.BuildNumber);err!=nil{return N4DNSBaseline{},err}
 guid,err:=n4NativeInterfaceGUID(index);if err!=nil{return N4DNSBaseline{},err}
 if err:=n4GetInterfaceDNS.Find();err!=nil{return N4DNSBaseline{},err}
 if err:=n4FreeInterfaceDNS.Find();err!=nil{return N4DNSBaseline{},err}
 var native n4DNSInterfaceSettingsV1
 native.Version=n4DNSSettingsVersion1 // flags must remain zero on Get
 // MS Windows x64 ABI passes 16-byte GUID values by reference to caller
 // memory. Do not confuse this with a generic pointer API.
 rc,_,_:=n4GetInterfaceDNS.Call(uintptr(unsafe.Pointer(&guid)),uintptr(unsafe.Pointer(&native)))
 if rc!=0{return N4DNSBaseline{},fmt.Errorf("GetInterfaceDnsSettings: %w",syscall.Errno(rc))}
 defer n4FreeInterfaceDNS.Call(uintptr(unsafe.Pointer(&native)))
 out:=N4DNSBaseline{
  Interface:guid,Flags:native.Flags,
  RegistrationEnabled:native.RegistrationEnabled,
  RegisterAdapterName:native.RegisterAdapterName,
  EnableLLMNR:native.EnableLLMNR,QueryAdapterName:native.QueryAdapterName,
 }
 if out.RawDomain,err=n4DNSSafeUTF16(native.Domain);err!=nil{return N4DNSBaseline{},err}
 if out.RawSearchList,err=n4DNSSafeUTF16(native.SearchList);err!=nil{return N4DNSBaseline{},err}
 if out.RawNameServer,err=n4DNSSafeUTF16(native.NameServer);err!=nil{return N4DNSBaseline{},err}
 if out.RawProfileNameServer,err=n4DNSSafeUTF16(native.ProfileNameServer);err!=nil{return N4DNSBaseline{},err}
 // Strict interpretation of comma or whitespace separated adapter servers
 // is only needed when a static nameserver flag is set. Other historical
 // resolver states may be obtained by DHCP and must NOT be misread as static.
 if out.Flags&n4DNSSettingNameServer!=0 && out.RawNameServer!="" {
  addrs,parseErr:=parseN4DNSNameserverList(out.RawNameServer)
  if parseErr!=nil{return N4DNSBaseline{},parseErr}
  out.NameServers=addrs
 }
 return out,nil
}
