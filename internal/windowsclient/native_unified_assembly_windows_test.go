//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "testing"
)

func TestN4WindowsNativeWALAssemblerRequiresOwnedWintunAndPersistentGuard(t *testing.T){
 id:=n4TestConcreteNativeIdentity()
 tun:=&TUN{name:"WBD",createdByProcess:false}
 if _,err:=n4AssembleWindowsNativeOwnedTxn(context.Background(),tun,
   NetworkPlan{AdapterAlias:"WBD"},id,nil);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("borrowed same-name Wintun accepted: %v",err)
 }
 tun.createdByProcess=true
 if _,err:=n4AssembleWindowsNativeOwnedTxn(context.Background(),tun,
   NetworkPlan{AdapterAlias:"WBD"},id,nil);!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("unverified WFP guard accepted: %v",err)
 }
 foreign:=&n4WindowsPersistentWFPGuard{
  InstallationID:id.Installation,WintunLUID:id.WintunLUID+1,
  ExclusiveSingleWintun:true,
 }
 if _,err:=n4AssembleWindowsNativeOwnedTxn(context.Background(),tun,
   NetworkPlan{AdapterAlias:"WBD"},id,foreign);!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("foreign WFP LUID accepted: %v",err)
 }
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=n4AssembleWindowsNativeOwnedTxn(ctx,tun,
   NetworkPlan{AdapterAlias:"WBD"},id,foreign);!errors.Is(err,context.Canceled){
  t.Fatalf("cancelled assembly accessed driver: %v",err)
 }
 // None of the negative checks above loads Wintun DLL, installs WFP,
 // changes NIC state or creates a journal.
}
