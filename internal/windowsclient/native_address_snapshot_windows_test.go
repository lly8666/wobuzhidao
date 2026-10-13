//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "net/netip"
 "testing"
 "unsafe"

 "golang.org/x/sys/windows"
)
func TestN4NativeAddressABIAndReadOnlyWindows(t *testing.T){
 if got:=unsafe.Sizeof(windows.MibUnicastIpAddressRow{});got<72||got>128{
  t.Fatalf("unexpected MIB_UNICASTIPADDRESS_ROW size %d",got)
 }
 if got:=unsafe.Sizeof(n4SockaddrInet{});got!=28{t.Fatalf("Win32 sockaddr union=%d",got)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:= (N4WindowsAddressSnapshot{}).IPv4(ctx,999999);!errors.Is(err,context.Canceled) {
  t.Fatalf("cancel not respected: %v",err)
 }
 // Non-elevated OS-native enumeration is safe and read-only: an impossible
 // interface index must have no address entries.
 v,err:=(N4WindowsAddressSnapshot{}).IPv4(context.Background(),4294967295)
 if err!=nil{t.Fatal(err)}
 if len(v)!=0{t.Fatalf("unexpected ifindex addr %v",v)}
}
func TestN4NativeLeasePreflightMatrix(t *testing.T){
 lease:=netip.MustParseAddr("10.66.0.7")
 good:=N4IPv4AddressView{InterfaceIndex:37,Address:lease,DADState:4,PrefixOrigin:1}
 cases:=[]struct{name string;rows []N4IPv4AddressView;wantPresent bool;wantErr bool}{
  {"empty",nil,false,false},
  {"ready",[]N4IPv4AddressView{good},true,false},
  {"other-nic-ignored",[]N4IPv4AddressView{{InterfaceIndex:39,Address:netip.MustParseAddr("169.254.1.9"),DADState:4}},false,false},
  {"foreign-APIPA",[]N4IPv4AddressView{good,{InterfaceIndex:37,Address:netip.MustParseAddr("169.254.1.9"),DADState:4}},false,true},
  {"foreign-LAN",[]N4IPv4AddressView{{InterfaceIndex:37,Address:netip.MustParseAddr("192.168.1.2"),DADState:4}},false,true},
  {"tentative",[]N4IPv4AddressView{{InterfaceIndex:37,Address:lease,DADState:1}},false,true},
  {"duplicate",[]N4IPv4AddressView{good,good},false,true},
 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  ok,err:=N4VerifyExclusivePreferredLease(tc.rows,37,lease)
  if ok!=tc.wantPresent || (err!=nil)!=tc.wantErr{t.Fatalf("ok=%v err=%v",ok,err)}
 })}
 if _,err:=N4VerifyExclusivePreferredLease(nil,0,lease);!errors.Is(err,ErrNetworkPlan){t.Fatal("missing NIC accepted")}
 if _,err:=N4VerifyExclusivePreferredLease(nil,37,netip.MustParseAddr("::1"));!errors.Is(err,ErrNetworkPlan){t.Fatal("IPv6 accepted")}
}
