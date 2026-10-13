//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "reflect"
 "testing"

 "golang.org/x/sys/windows"
)

type n4MTUFakeOwner struct {
 owned bool
 calls int
}
func (f *n4MTUFakeOwner) IsCurrentOwnedWintun(_ context.Context,idx uint32)(bool,error){
 f.calls++
 if idx==0{return false,ErrNetworkPlan}
 return f.owned,nil
}

func TestN4NativeMTUWriterSafetyGateNoMutation(t *testing.T){
 old:=N4InterfaceIPv4Snapshot{Index:15,MTU:1500,Metric:25,Connected:true}
 w:=n4WindowsMTUWriter{}
 if _,err:=w.Apply(context.Background(),old,1249);!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("no durable guard+owner should deny: %v",err)
 }
 w.Guard=&n4FakeGuard{durable:false,active:true}
 w.Adapter=&n4MTUFakeOwner{owned:true}
 if _,err:=w.Apply(context.Background(),old,1249);!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("volatile guard should deny: %v",err)
 }
 guard:=&n4FakeGuard{durable:true,active:false}
 w.Guard=guard
 owner:=&n4MTUFakeOwner{owned:false}
 w.Adapter=owner
 if _,err:=w.Apply(context.Background(),old,1249);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("foreign interface accepted: %v",err)
 }
 owner.owned=true
 if _,err:=w.Apply(context.Background(),old,1249);!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("unarmed guard accepted: %v",err)
 }
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=w.Apply(ctx,old,1249);!errors.Is(err,context.Canceled){
  t.Fatalf("canceled MTU update accepted: %v",err)
 }
 if err:=w.Restore(context.Background(),n4MTUWriteReceipt{});!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("unowned MTU restore accepted: %v",err)
 }
 if _,err:=w.Apply(context.Background(),old,99999);!errors.Is(err,ErrNetworkPlan){
  t.Fatalf("out-of-range MTU accepted: %v",err)
 }
}
func TestN4NativeMTUSnapshotExactAndWindowsReadOnly(t *testing.T){
 row:=windows.MibIpInterfaceRow{
  Family:windows.AF_INET,InterfaceIndex:13,Metric:17,NlMtu:1250,
  Connected:1,WeakHostSend:0,WeakHostReceive:0,
  UseAutomaticMetric:1,DisableDefaultRoutes:1,
 }
 s:=n4MTUSnapshotFromRow(row)
 if s.Index!=13 || s.MTU!=1250 || s.Metric!=17 || !s.Connected || !s.AutomaticMetric || !s.DisableDefaultRoutes{
  t.Fatalf("snapshot lost required identity: %+v",s)
 }
 next:=s
 next.MTU++
 if n4SameInterfaceBaseline(s,next){t.Fatal("changed MTU passed CAS")}
 next=s
 next.Metric++
 if n4SameInterfaceBaseline(s,next){t.Fatal("foreign route metric passed CAS")}
 next=s
 next.WeakHostSend=true
 if n4SameInterfaceBaseline(s,next){t.Fatal("foreign weak-host policy passed CAS")}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=n4GetIPv4InterfaceRow(ctx,13);!errors.Is(err,context.Canceled){
  t.Fatalf("canceled native row read accepted: %v",err)
 }
 if _,err:=n4GetIPv4InterfaceRow(context.Background(),0);!errors.Is(err,ErrNetworkPlan){
  t.Fatalf("zero interface row read accepted: %v",err)
 }
 // This test invokes GetIpInterfaceEntry only, never SetIpInterfaceEntry.
}

func TestN4NativeMTUSetterNormalizesOnlyIPv4SitePrefixLength(t *testing.T){
 original:=windows.MibIpInterfaceRow{
  Family:windows.AF_INET,InterfaceIndex:31,
  SitePrefixLength:64,NlMtu:65535,Metric:5,
  Connected:1,UseAutomaticMetric:1,DisableDefaultRoutes:1,
 }
 normalized:=original
 n4PrepareIPv4MTUSetRow(&normalized,1249)
 if normalized.SitePrefixLength!=0 || normalized.NlMtu!=1249{
  t.Fatalf("invalid SetIpInterfaceEntry IPv4 input: %+v",normalized)
 }
 original.SitePrefixLength=0
 original.NlMtu=1249
 if !reflect.DeepEqual(normalized,original) {t.Fatal("input normalization changed foreign row fields")}
 n4PrepareIPv4MTUSetRow(&normalized,65535)
 if normalized.SitePrefixLength!=0 || normalized.NlMtu!=65535{
  t.Fatal("MTU restore did not normalize IPv4 Set input")
 }
}

func TestN4NativeMTURestoreOriginalWintun65535GuardWithoutOSWrite(t *testing.T){
 baseline:=N4InterfaceIPv4Snapshot{Index:88,MTU:65535,Metric:5,Connected:true,AutomaticMetric:true}
 after:=baseline
 after.MTU=1249
 guard:=&n4FakeGuard{durable:true,active:true}
 owner:=&n4MTUFakeOwner{owned:true}
 writer:=n4WindowsMTUWriter{Guard:guard,Adapter:owner}
 // Restore must check the native current row using the bounded, actually
 // applied MTU; it must not pass the 65535 original as a new target.
 if err:=writer.preflight(context.Background(),after,1249);err!=nil{t.Fatalf("owned current state rejected: %v",err)}
 if err:=writer.preflight(context.Background(),after,baseline.MTU);!errors.Is(err,ErrNetworkPlan){
  t.Fatalf("unsafe NEW MTU 65535 should remain disallowed: %v",err)
 }
 if owner.calls!=1{t.Fatalf("foreign check moved behind invalid target: %d",owner.calls)}
}
