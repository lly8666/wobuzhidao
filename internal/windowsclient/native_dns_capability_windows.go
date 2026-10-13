//go:build windows

package windowsclient

import (
 "errors"
 "fmt"

 "golang.org/x/sys/windows"
)

var ErrN4UnsupportedWindows = errors.New("windowsclient: required native scriptless DNS/WFP APIs unsupported by this Windows release")
const n4MinNativeDNSBuild uint32 = 19041

type N4NativePlatformCapability struct {
 WindowsMajor uint32
 WindowsBuild uint32
 AddressAndRouteAPI bool
 DNSInterfaceAPI bool
 WFPDurableFilterAPI bool
}

// Platform capability is a necessary condition only. It does NOT claim
// effective IPv6/DNS guards, admin rights, Wintun installed, or physical path.
func N4ScriptlessWindowsCapability()(N4NativePlatformCapability,error){
 v:=windows.RtlGetVersion()
 c:=N4NativePlatformCapability{WindowsMajor:v.MajorVersion,WindowsBuild:v.BuildNumber}
 if err:=n4RequireNativeDNSBuild(c.WindowsMajor,c.WindowsBuild);err!=nil{return c,err}
 for _,name:=range []string{
  "GetIpForwardEntry2","CreateIpForwardEntry2","DeleteIpForwardEntry2",
  "GetUnicastIpAddressTable","GetUnicastIpAddressEntry",
  "CreateUnicastIpAddressEntry","DeleteUnicastIpAddressEntry",
 }{
  if err:=n4Iphlpapi.NewProc(name).Find();err!=nil{
   return c,fmt.Errorf("%w: missing iphlpapi.dll!%s: %v",ErrN4UnsupportedWindows,name,err)
  }
 }
 c.AddressAndRouteAPI=true
 for _,name:=range []string{"GetInterfaceDnsSettings","SetInterfaceDnsSettings"}{
  if err:=n4Iphlpapi.NewProc(name).Find();err!=nil{
   return c,fmt.Errorf("%w: missing native per-interface DNS %s: %v",ErrN4UnsupportedWindows,name,err)
  }
 }
 c.DNSInterfaceAPI=true
 wfp:=windows.NewLazySystemDLL("fwpuclnt.dll")
 for _,name:=range []string{
  "FwpmEngineOpen0","FwpmEngineClose0",
  "FwpmFilterAdd0","FwpmFilterGetByKey0","FwpmFilterDeleteByKey0",
  "FwpmSubLayerAdd0","FwpmSubLayerGetByKey0","FwpmSubLayerDeleteByKey0",
 }{
  if err:=wfp.NewProc(name).Find();err!=nil{
   return c,fmt.Errorf("%w: missing native WFP %s: %v",ErrN4UnsupportedWindows,name,err)
  }
 }
 c.WFPDurableFilterAPI=true
 return c,nil
}
func n4RequireNativeDNSBuild(major,build uint32)error{
 if major!=10 || build<n4MinNativeDNSBuild{
  return fmt.Errorf("%w: Windows major=%d build=%d; need Windows 10 build >=%d",
   ErrN4UnsupportedWindows,major,build,n4MinNativeDNSBuild)
 }
 return nil
}
