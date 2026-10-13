//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "net/netip"
 "testing"
)
func TestN4WindowsRouteMatchStrictOwnedIdentity(t *testing.T){
 key:=NativeRouteKey{Prefix:netip.MustParsePrefix("128.0.0.0/1"),
  Interface:25,NextHop:netip.MustParseAddr("0.0.0.0"),Metric:5}
 row,err:=N4NativeRouteEntry(key.Prefix,key.Interface,key.NextHop)
 if err!=nil{t.Fatal(err)}
 if !n4NativeRouteMatches(row,key){t.Fatal("native route identity does not roundtrip")}
 cases:=[]struct{name string;change func(*n4ForwardRow)}{
  {"wrong-interface",func(r *n4ForwardRow){r.Index=26}},
  {"wrong-destination",func(r *n4ForwardRow){r.Destination.Address=n4NativeRouteIPv4(netip.MustParseAddr("0.0.0.0"))}},
  {"wrong-prefix",func(r *n4ForwardRow){r.Destination.Bits=2}},
  {"wrong-gateway",func(r *n4ForwardRow){r.NextHop=n4NativeRouteIPv4(netip.MustParseAddr("192.0.2.1"))}},
  {"wrong-metric",func(r *n4ForwardRow){r.Metric=6}},
  {"loopback",func(r *n4ForwardRow){r.Loopback=1}},
  {"published",func(r *n4ForwardRow){r.Publish=1}},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  clone:=row;tc.change(&clone)
  if n4NativeRouteMatches(clone,key){t.Fatal("foreign route incorrectly matches owned identity")}
 })}
}
func TestN4WindowsBackendNonElevatedInvalidKeysAndContext(t *testing.T){
 var b N4WindowsRouteBackend
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 key:=NativeRouteKey{Prefix:netip.MustParsePrefix("0.0.0.0/1"),
  Interface:5,NextHop:netip.MustParseAddr("0.0.0.0"),Metric:5}
 if err:=b.CreateIfAbsent(ctx,key);!errors.Is(err,context.Canceled){t.Fatalf("cancelled create: %v",err)}
 if err:=b.RemoveOwned(ctx,key);!errors.Is(err,context.Canceled){t.Fatalf("cancelled delete: %v",err)}
 _,_,err:=b.Lookup(ctx,key)
 if !errors.Is(err,context.Canceled){t.Fatalf("cancelled read: %v",err)}
 key.Interface=0
 if _,_,err=b.Lookup(context.Background(),key);!errors.Is(err,ErrNetworkPlan){t.Fatalf("invalid interface accepted: %v",err)}
 // No Create/Delete calls on runner; admin/WinTun route mutation NOT_RUN.
}
