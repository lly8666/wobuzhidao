//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "fmt"
 "net/netip"
 "syscall"
 "unsafe"

 "golang.org/x/sys/windows"
)

var (
 n4InitUnicast=n4Iphlpapi.NewProc("InitializeUnicastIpAddressEntry")
 n4GetUnicast=n4Iphlpapi.NewProc("GetUnicastIpAddressEntry")
 n4CreateUnicast=n4Iphlpapi.NewProc("CreateUnicastIpAddressEntry")
 n4DeleteUnicast=n4Iphlpapi.NewProc("DeleteUnicastIpAddressEntry")
)

var ErrN4LeaseNotPreferred=errors.New("windowsclient: native IPv4 lease DAD not yet preferred")

// N4NativeLeaseIntent represents one *host* IPv4 address on a WBD-owned,
// independently verified Wintun adapter. /32 avoids generating an unmanaged
// on-link network route. China policy IP ranges NEVER become OS route entries.
type N4NativeLeaseIntent struct {
 InterfaceIndex uint32
 Prefix netip.Prefix
}

func n4ValidateLeaseIntent(l N4NativeLeaseIntent) error {
 if l.InterfaceIndex==0 || !l.Prefix.IsValid() || !l.Prefix.Addr().Is4() ||
  l.Prefix.Bits()!=32 || !l.Prefix.Addr().IsGlobalUnicast() ||
  l.Prefix.Addr().IsLoopback() || l.Prefix.Addr().IsLinkLocalUnicast() {
  return ErrNetworkPlan
 }
 return nil
}

func n4NativeLeaseRow(l N4NativeLeaseIntent) (windows.MibUnicastIpAddressRow,error) {
 if err:=n4ValidateLeaseIntent(l);err!=nil{return windows.MibUnicastIpAddressRow{},err}
 if err:=n4InitUnicast.Find();err!=nil{return windows.MibUnicastIpAddressRow{},err}
 var row windows.MibUnicastIpAddressRow
 n4InitUnicast.Call(uintptr(unsafe.Pointer(&row)))
 raw:=(*n4SockaddrInet)(unsafe.Pointer(&row.Address))
 *raw=n4NativeRouteIPv4(l.Prefix.Addr())
 row.InterfaceLuid=0
 row.InterfaceIndex=l.InterfaceIndex
 row.OnLinkPrefixLength=32
 row.SkipAsSource=0
 // Windows performs duplicate address detection; do NOT claim Preferred
 // before the OS actually reports IpDadStatePreferred.
 return row,nil
}

// N4WindowsLeaseReader is safe on a non-admin Actions runner: it only
// queries the exact IPv4/interface tuple. It does not create an address.
type N4WindowsLeaseReader struct{}
func (N4WindowsLeaseReader) Lookup(ctx context.Context,l N4NativeLeaseIntent)(windows.MibUnicastIpAddressRow,bool,error){
 if err:=ctx.Err();err!=nil{return windows.MibUnicastIpAddressRow{},false,err}
 row,err:=n4NativeLeaseRow(l);if err!=nil{return row,false,err}
 if err:=n4GetUnicast.Find();err!=nil{return row,false,err}
 rc,_,_:=n4GetUnicast.Call(uintptr(unsafe.Pointer(&row)))
 if syscall.Errno(rc)==n4ErrorNotFound{return row,false,nil}
 if rc!=0{return row,false,fmt.Errorf("GetUnicastIpAddressEntry: %w",syscall.Errno(rc))}
 if row.InterfaceIndex!=l.InterfaceIndex || row.Address.Family!=windows.AF_INET ||
   n4ReadSockaddrIPv4((*n4SockaddrInet)(unsafe.Pointer(&row.Address)))!=l.Prefix.Addr() ||
   row.OnLinkPrefixLength!=32 {return row,false,ErrN4ForeignRoute}
 return row,true,nil
}

// N4NativeLeaseDADGate does not paper over a tentative, duplicate,
// deprecated, or invalid address. Parent network txn must NOT enable capture
// or TCP Direct until the returned row has reached Preferred.
func N4NativeLeaseDADGate(row windows.MibUnicastIpAddressRow,l N4NativeLeaseIntent)error {
 if err:=n4ValidateLeaseIntent(l);err!=nil{return err}
 if row.InterfaceIndex!=l.InterfaceIndex || row.Address.Family!=windows.AF_INET ||
  n4ReadSockaddrIPv4((*n4SockaddrInet)(unsafe.Pointer(&row.Address)))!=l.Prefix.Addr() ||
  row.OnLinkPrefixLength!=32 || row.SkipAsSource!=0 {
  return ErrN4ForeignRoute
 }
 if row.DadState!=n4DADPreferred {return ErrN4LeaseNotPreferred}
 return nil
}

// n4NativeLeaseOwnerToken only proves that THIS process observed a
// successful Create. It is NOT durable across a crash; real product Apply
// requires an intent-before-create journal and adapter owner verification.
// Consequently this backend is NOT CONNECTED to cmd/wbd-client yet.
type n4NativeLeaseOwnerToken struct{ intent N4NativeLeaseIntent; created bool }
type N4WindowsLeaseWriter struct {
 Guard CrashPersistentGuard
 Snapshot N4WindowsAddressSnapshot
}

func (w N4WindowsLeaseWriter) preflight(ctx context.Context,l N4NativeLeaseIntent)error {
 if err:=ctx.Err();err!=nil{return err}
 if err:=n4ValidateLeaseIntent(l);err!=nil{return err}
 if w.Guard==nil || !w.Guard.Durable(){return ErrN4NativeGuard}
 ok,err:=w.Guard.Verify(ctx)
 if err!=nil{return fmt.Errorf("%w: %v",ErrN4NativeGuard,err)}
 if !ok{return ErrN4NativeGuard}
 return nil
}

// Create refuses existing foreign, DHCP, link-local and even exact matching
// leases. It does not delete any addresses to make room. Failure remains
// unowned/ambiguous and must be resolved by the durable parent txn.
func (w N4WindowsLeaseWriter) Create(ctx context.Context,l N4NativeLeaseIntent)(n4NativeLeaseOwnerToken,error){
 var token n4NativeLeaseOwnerToken
 if err:=w.preflight(ctx,l);err!=nil{return token,err}
 rows,err:=w.Snapshot.IPv4(ctx,l.InterfaceIndex);if err!=nil{return token,err}
 if len(rows)!=0{return token,ErrN4ForeignRoute}
 row,err:=n4NativeLeaseRow(l);if err!=nil{return token,err}
 if err:=ctx.Err();err!=nil{return token,err}
 if err:=n4CreateUnicast.Find();err!=nil{return token,err}
 rc,_,_:=n4CreateUnicast.Call(uintptr(unsafe.Pointer(&row)))
 if rc!=0{return token,fmt.Errorf("CreateUnicastIpAddressEntry: %w",syscall.Errno(rc))}
 return n4NativeLeaseOwnerToken{intent:l,created:true},nil
}

// DeleteConfirmed cannot operate merely on a matching host address. Caller
// must supply the fresh proof returned by successful Create, prove WBD
// adapter ownership independently and have a durable persistent guard.
func (w N4WindowsLeaseWriter) DeleteConfirmed(ctx context.Context,token n4NativeLeaseOwnerToken)error{
 if !token.created{return ErrN4ForeignRoute}
 l:=token.intent
 if err:=w.preflight(ctx,l);err!=nil{return err}
 row,exists,err:=(N4WindowsLeaseReader{}).Lookup(ctx,l)
 if err!=nil{return err}
 if !exists{return nil}
 if err:=N4NativeLeaseDADGate(row,l);err!=nil{return err}
 if err:=n4DeleteUnicast.Find();err!=nil{return err}
 rc,_,_:=n4DeleteUnicast.Call(uintptr(unsafe.Pointer(&row)))
 if rc!=0{return fmt.Errorf("DeleteUnicastIpAddressEntry: %w",syscall.Errno(rc))}
 return nil
}
