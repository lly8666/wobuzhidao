//go:build windows

package windowsclient

import (
 "errors"
 "testing"
)
func TestN4ScriptlessWindowsNativeCapabilityMatrix(t *testing.T){
 checks:=[]struct{major,build uint32;supported bool}{
  {6,7601,false}, // Windows 7 cannot silently fall back to PowerShell
  {6,9600,false}, // Windows 8.1
  {10,18362,false},
  {10,19040,false},
  {10,19041,true},
  {10,19045,true},
  {10,22000,true},
  {10,26100,true},
 }
 for _,c:=range checks{
  err:=n4RequireNativeDNSBuild(c.major,c.build)
  if (err==nil)!=c.supported{t.Fatalf("version %d.%d support err=%v",c.major,c.build,err)}
  if !c.supported && !errors.Is(err,ErrN4UnsupportedWindows){
   t.Fatalf("unsupported OS did not fail closed: %v",err)
  }
 }
}
func TestN4ScriptlessWindowsDLLExportsOnly(t *testing.T){
 caps,err:=N4ScriptlessWindowsCapability()
 if err!=nil{t.Fatal(err)}
 if !caps.AddressAndRouteAPI || !caps.DNSInterfaceAPI || !caps.WFPDurableFilterAPI{
  t.Fatalf("incomplete native API set %+v",caps)
 }
 t.Logf("Native API export preflight Windows %d build %d; no WFP filter, DNS or route mutation attempted",caps.WindowsMajor,caps.WindowsBuild)
}
