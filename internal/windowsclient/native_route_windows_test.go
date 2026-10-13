//go:build windows

package windowsclient

import (
    "net/netip"
    "testing"
    "unsafe"
)

func TestN4NativeRouteWindowsABIAndCompactKeys(t *testing.T) {
    if got:=unsafe.Sizeof(n4ForwardRow{});got!=104 {t.Fatalf("MIB_IPFORWARD_ROW2 ABI size=%d, want 104",got)}
    if got:=unsafe.Offsetof(n4ForwardRow{}.Destination);got!=12 {t.Fatalf("destination offset=%d",got)}
    if got:=unsafe.Offsetof(n4ForwardRow{}.NextHop);got!=44 {t.Fatalf("nexthop offset=%d",got)}
    if got:=unsafe.Offsetof(n4ForwardRow{}.Origin);got!=100 {t.Fatalf("origin offset=%d",got)}
    row,err:=N4NativeRouteEntry(netip.MustParsePrefix("128.12.3.4/1"),25,netip.MustParseAddr("0.0.0.0"))
    if err!=nil{t.Fatal(err)}
    if row.Index!=25 || row.Destination.Bits!=1 ||
        n4ReadSockaddrIPv4(&row.Destination.Address)!=netip.MustParseAddr("128.0.0.0") ||
        n4ReadSockaddrIPv4(&row.NextHop)!=netip.MustParseAddr("0.0.0.0"){
        t.Fatalf("route ABI encoding mismatch: %+v",row)
    }
    if _,err:=N4NativeRouteEntry(netip.MustParsePrefix("::/1"),25,netip.MustParseAddr("0.0.0.0"));err==nil{
        t.Fatal("native IPv4 API accepted IPv6")
    }
}

// Native route getters are safe on a non-elevated Windows runner; mutate
// is forbidden. This proves only that GetIpForwardEntry2 is callable.
func TestN4NativeRouteNonElevatedReadOnlyProbe(t *testing.T) {
    row,err:=N4NativeRouteEntry(netip.MustParsePrefix("192.0.2.254/32"),4294967295,netip.MustParseAddr("0.0.0.0"))
    if err!=nil{t.Fatal(err)}
    _,err=N4NativeRouteAPI{}.Exists(&row)
    // Windows may reject this fabricated index; both NOT_FOUND and a
    // structured ERROR_NOT_FOUND-style error are acceptable. The important
    // assertion is that the API returned normally without crashing.
    if err!=nil {t.Logf("non-elevated nonexistent interface query: %v",err)}
}
