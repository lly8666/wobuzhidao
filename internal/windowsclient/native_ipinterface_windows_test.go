//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "net"
 "testing"

 "golang.org/x/sys/windows"
)
func TestN4NativeIPInterfaceMTUPreflightMatrix(t *testing.T){
 baseline:=N4InterfaceIPv4Snapshot{Index:17,MTU:1500,Metric:25,Connected:true}
 cases:=[]struct{idx,mtu uint32;ok bool}{
  {17,576,true},{17,1249,true},{17,9000,true},
  {17,575,false},{17,9001,false},{0,1249,false},{18,1249,false},
 }
 for _,c:=range cases{
  err:=N4VerifyOwnedTunnelMTU(baseline,c.idx,c.mtu)
  if (err==nil)!=c.ok {t.Fatalf("idx=%d mtu=%d err=%v",c.idx,c.mtu,err)}
 }
 baseline.MTU=0
 if err:=N4VerifyOwnedTunnelMTU(baseline,17,1249);!errors.Is(err,ErrNetworkPlan){t.Fatal("broken NIC baseline accepted")}
}
func TestN4NativeIPInterfaceWindowsReadOnly(t *testing.T){
 reader:=N4WindowsInterfaceReader{}
 if _,err:=reader.ReadIPv4(context.Background(),0);!errors.Is(err,ErrNetworkPlan){t.Fatal("index zero accepted")}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=reader.ReadIPv4(ctx,25);!errors.Is(err,context.Canceled){t.Fatalf("cancelled native read: %v",err)}
 interfaces,err:=net.Interfaces();if err!=nil{t.Fatal(err)}
 var found int
 for _,ifc:=range interfaces{
  if ifc.Index<=0 || ifc.Flags&net.FlagLoopback!=0 || ifc.Flags&net.FlagUp==0 {continue}
  snap,e:=reader.ReadIPv4(context.Background(),uint32(ifc.Index))
  if e!=nil{t.Logf("native read of ifIndex=%d unavailable: %v",ifc.Index,e);continue}
  if snap.Index!=uint32(ifc.Index) || snap.MTU<576 || snap.MTU>65535 {
   t.Fatalf("native MTU suspicious: %+v",snap)
  }
  found++
 }
 if found==0{t.Fatal("no nonloopback Windows interface yielded native MTU baseline")}
 var r windows.MibIpInterfaceRow
 if r.NlMtu!=0 {t.Fatal("SDK row zero value not empty")}
 // Does NOT call SetIpInterfaceEntry or change adapter configuration.
}
