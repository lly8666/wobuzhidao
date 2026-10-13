//go:build windows

package windowsclient

import (
 "context"
 "fmt"
 "net/netip"
 "syscall"
 "unsafe"

 "golang.org/x/sys/windows"
)

// N4WindowsAddressSnapshot is read-only; it deliberately does NOT toggle
// DHCP, remove foreign APIPA addresses, or claim ownership of Wintun. A
// future native lease transaction must journal every individual OS change.
type N4WindowsAddressSnapshot struct{}

var (
 n4GetUnicastTable=n4Iphlpapi.NewProc("GetUnicastIpAddressTable")
 n4FreeMibTable=n4Iphlpapi.NewProc("FreeMibTable")
)
type N4IPv4AddressView struct {
 InterfaceIndex uint32
 Address netip.Addr
 DADState uint32
 PrefixOrigin uint32
}

// GetUnicastIpAddressTable's MIB_UNICASTIPADDRESS_TABLE stores a uint32 count
// and 4 bytes of padding before its 8-byte-aligned row array (64-bit Win).
const n4UnicastTableRowsOffset = uintptr(8)
func n4ToAddressView(row windows.MibUnicastIpAddressRow) N4IPv4AddressView {
 raw:=(*n4SockaddrInet)(unsafe.Pointer(&row.Address))
 return N4IPv4AddressView{
  InterfaceIndex:row.InterfaceIndex,
  Address:n4ReadSockaddrIPv4(raw),
  DADState:row.DadState,
  PrefixOrigin:row.PrefixOrigin,
 }
}
func (N4WindowsAddressSnapshot) IPv4(ctx context.Context,interfaceIndex uint32)([]N4IPv4AddressView,error) {
 if interfaceIndex==0{return nil,ErrNetworkPlan}
 if err:=ctx.Err();err!=nil{return nil,err}
 if err:=n4GetUnicastTable.Find();err!=nil{return nil,err}
 if err:=n4FreeMibTable.Find();err!=nil{return nil,err}
 var table uintptr
 rc,_,_:=n4GetUnicastTable.Call(windows.AF_INET,uintptr(unsafe.Pointer(&table)))
 if rc!=0{return nil,fmt.Errorf("GetUnicastIpAddressTable: %w",syscall.Errno(rc))}
 if table==0{return nil,fmt.Errorf("GetUnicastIpAddressTable returned null table")}
 defer n4FreeMibTable.Call(table)
 count:=*(*uint32)(unsafe.Pointer(table))
 // An oversized OS response must not cause unsafe.Slice to walk arbitrarily
 // far into memory, even if a corrupt native result slips past ABI checks.
 if count>65536{return nil,ErrNetworkPlan}
 rows:=unsafe.Slice((*windows.MibUnicastIpAddressRow)(unsafe.Pointer(table+n4UnicastTableRowsOffset)),int(count))
 results:=make([]N4IPv4AddressView,0)
 for _,row:=range rows {
  if row.InterfaceIndex!=interfaceIndex{continue}
  a:=n4ToAddressView(row)
  if !a.Address.Is4(){return nil,ErrNetworkPlan}
  results=append(results,a)
 }
 return results,nil
}
const n4DADPreferred uint32=4
func N4VerifyExclusivePreferredLease(rows []N4IPv4AddressView,ifIndex uint32,lease netip.Addr)(bool,error){
 if ifIndex==0 || !lease.Is4() || !lease.IsGlobalUnicast() {return false,ErrNetworkPlan}
 present:=false
 for _,a:=range rows{
  if a.InterfaceIndex!=ifIndex {continue}
  if !a.Address.Is4() || a.Address!=lease {
   return false,fmt.Errorf("%w: foreign Wintun IPv4 %v",ErrN4ForeignRoute,a.Address)
  }
  if present {return false,ErrNetworkPlan}
  present=true
  if a.DADState!=n4DADPreferred {return false,ErrNetworkPlan}
 }
 return present,nil
}
