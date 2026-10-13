//go:build windows

package windowsclient

import (
    "fmt"
    "net/netip"
    "syscall"
    "unsafe"

    "golang.org/x/sys/windows"
)

// Windows netioapi definitions compatible with the documented, 104-byte
// MIB_IPFORWARD_ROW2 ABI. x/sys v0.33.0 includes the socket union but not
// MIB_IPFORWARD_ROW2. Keep this independently layout-tested on Windows.
// No route mutation is performed by construction or the ABI tests.
// SOCKADDR_INET is a 28-byte union in the Windows SDK; this pinned
// Go module predates the x/sys exported RawSockaddrInet helper.
type n4SockaddrInet struct {
    Family uint16
    Port uint16
    Data [6]uint32
}

type n4RoutePrefix struct {
    Address n4SockaddrInet
    Bits uint8
    _ [3]byte
}
type n4ForwardRow struct {
    Luid uint64
    Index uint32
    Destination n4RoutePrefix
    NextHop n4SockaddrInet
    SitePrefixLength uint8
    _ [3]byte
    ValidLifetime uint32
    PreferredLifetime uint32
    Metric uint32
    Protocol uint32
    Loopback uint8
    AutoconfigureAddress uint8
    Publish uint8
    Immortal uint8
    Age uint32
    Origin uint32
}

var n4Iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
var (
    n4RouteInitialize = n4Iphlpapi.NewProc("InitializeIpForwardEntry")
    n4RouteGet = n4Iphlpapi.NewProc("GetIpForwardEntry2")
    n4RouteCreate = n4Iphlpapi.NewProc("CreateIpForwardEntry2")
    n4RouteDelete = n4Iphlpapi.NewProc("DeleteIpForwardEntry2")
)
const n4ErrorNotFound syscall.Errno = 1168

func n4NativeRouteIPv4(ip netip.Addr) n4SockaddrInet {
    var out n4SockaddrInet
    out.Family = windows.AF_INET
    copy(unsafe.Slice((*byte)(unsafe.Pointer(&out.Data[0])),4),ip.AsSlice())
    return out
}
func n4ReadSockaddrIPv4(s *n4SockaddrInet) netip.Addr {
    if s == nil || s.Family!=windows.AF_INET {return netip.Addr{}}
    var data [4]byte
    copy(data[:],unsafe.Slice((*byte)(unsafe.Pointer(&s.Data[0])),4))
    return netip.AddrFrom4(data)
}

// N4NativeRouteEntry forms the exact-key native record. The target interface
// must be a snapshot acquired from an already opened Wintun adapter, never a
// mutable user-controlled PowerShell adapter alias.
func N4NativeRouteEntry(prefix netip.Prefix, index uint32, nextHop netip.Addr) (n4ForwardRow,error) {
    if index==0 || !prefix.IsValid() || !prefix.Addr().Is4() || !nextHop.Is4() {
        return n4ForwardRow{},ErrNetworkPlan
    }
    prefix=prefix.Masked()
    var row n4ForwardRow
    // InitializeIpForwardEntry sets all documented default field values.
    // Don't call it until all API symbol lookups have succeeded.
    if err:=n4RouteInitialize.Find();err!=nil{return row,fmt.Errorf("native route initializer: %w",err)}
    n4RouteInitialize.Call(uintptr(unsafe.Pointer(&row)))
    row.Index=index
    row.Destination=n4RoutePrefix{Address:n4NativeRouteIPv4(prefix.Addr()),Bits:uint8(prefix.Bits())}
    row.NextHop=n4NativeRouteIPv4(nextHop)
    row.Metric=5
    return row,nil
}

// N4NativeRouteAPI is deliberately a small native Win32 boundary. A caller
// must provide its own durable ownership journal, DNS/IPv6 guard and rollback.
// Until those are in place the active CLI does NOT invoke these methods.
type N4NativeRouteAPI struct{}

func (N4NativeRouteAPI) Exists(row *n4ForwardRow) (bool,error) {
    if row==nil{return false,ErrNetworkPlan}
    if err:=n4RouteGet.Find();err!=nil{return false,err}
    rc,_,_:=n4RouteGet.Call(uintptr(unsafe.Pointer(row)))
    switch syscall.Errno(rc) {
    case 0: return true,nil
    case n4ErrorNotFound: return false,nil
    default: return false,fmt.Errorf("GetIpForwardEntry2: %w",syscall.Errno(rc))
    }
}

// Create refuses to take ownership of a pre-existing route (journal remains
// empty in that case). It intentionally does not delete a conflicting route.
func (a N4NativeRouteAPI) Create(row *n4ForwardRow) (created bool,err error) {
    exists,err:=a.Exists(row);if err!=nil{return false,err}
    if exists{return false,fmt.Errorf("%w: exact route preexists",ErrNetworkPlan)}
    if err:=n4RouteCreate.Find();err!=nil{return false,err}
    rc,_,_:=n4RouteCreate.Call(uintptr(unsafe.Pointer(row)))
    if rc!=0{return false,fmt.Errorf("CreateIpForwardEntry2: %w",syscall.Errno(rc))}
    return true,nil
}
func (a N4NativeRouteAPI) DeleteOwned(row *n4ForwardRow) error {
    exists,err:=a.Exists(row);if err!=nil{return err}
    if !exists{return nil} // replay safe for an already absent owned route
    if err:=n4RouteDelete.Find();err!=nil{return err}
    rc,_,_:=n4RouteDelete.Call(uintptr(unsafe.Pointer(row)))
    if rc!=0{return fmt.Errorf("DeleteIpForwardEntry2: %w",syscall.Errno(rc))}
    return nil
}
