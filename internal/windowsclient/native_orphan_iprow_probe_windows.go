//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "fmt"
 "syscall"
 "unsafe"

 "golang.org/x/sys/windows"
)

// n4OriginalIPRowProbe contains only classifications, never DNS values,
// addresses, packet contents, or names. An absent AF_INET MIB row is not
// evidence that its still-registered Wintun adapter was safely removed.
type n4OriginalIPRowProbe struct {
 IPv4 string
 IPv6 string
}

// n4ReadIPFamilyState makes an OS read only. Unlike a Delete/Set call,
// GetIpInterfaceEntry does not install an address or revive the ghost NIC.
func n4ReadIPFamilyState(ctx context.Context,index uint32,family uint16)(string,error){
 if err:=ctx.Err();err!=nil{return "",err}
 if index==0 || (family!=windows.AF_INET && family!=windows.AF_INET6){
  return "",ErrN4JournalConflict
 }
 if err:=n4InitIPInterface.Find();err!=nil{return "",err}
 if err:=n4GetIPInterface.Find();err!=nil{return "",err}
 var row windows.MibIpInterfaceRow
 n4InitIPInterface.Call(uintptr(unsafe.Pointer(&row)))
 row.Family=family
 row.InterfaceIndex=index
 rc,_,_:=n4GetIPInterface.Call(uintptr(unsafe.Pointer(&row)))
 if rc==uintptr(syscall.Errno(1168)){
  return "MISSING_1168",nil
 }
 if rc!=0{
  return "",fmt.Errorf("%w: GetIpInterfaceEntry family %d: %w",ErrN4NativeGuard,family,syscall.Errno(rc))
 }
 if row.Family!=family || row.InterfaceIndex!=index{
  return "",ErrN4ForeignRoute
 }
 return "PRESENT",nil
}

// n4ProbeOriginalIPRows requires exactly the same original GUID, NET_LUID,
// and IPv4 ifIndex before AND after the family queries. It cannot authorize
// an OS write, alias adoption, deleting a device, WAL retirement or WFP
// disarm. In particular "MISSING_1168" is a hard block for MTU rollback.
func n4ProbeOriginalIPRows(ctx context.Context,id N4UnifiedIdentity)(n4OriginalIPRowProbe,error){
 var out n4OriginalIPRowProbe
 if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return out,err}
 var err error
 out.IPv4,err=n4ReadIPFamilyState(ctx,id.WintunIndex,windows.AF_INET)
 if err!=nil{return n4OriginalIPRowProbe{},err}
 out.IPv6,err=n4ReadIPFamilyState(ctx,id.WintunIndex,windows.AF_INET6)
 if err!=nil{return n4OriginalIPRowProbe{},err}
 if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{
  return n4OriginalIPRowProbe{},errors.Join(ErrN4NativeGuard,err)
 }
 return out,nil
}
