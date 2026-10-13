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
 // This legacy guard-negative case must satisfy the NEW earlier
 // GUID/alias identity gate before it can exercise the absent WFP guard.
 // Missing GUID/alias is tested independently as ErrN4JournalConflict.
 id.WintunGUID="11111111-2222-3333-4444-555555555555"
 id.AdapterAlias="WBD"
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

func TestN4WindowsNativeAssemblyRejectsUnboundLegacyGUID(t *testing.T){
 id:=n4TestConcreteNativeIdentity()
 tun:=&TUN{name:"WBD",createdByProcess:true}
 guard:=&n4WindowsPersistentWFPGuard{InstallationID:id.Installation,WintunLUID:id.WintunLUID,ExclusiveSingleWintun:true}
 plan:=NetworkPlan{AdapterAlias:"WBD"}
 if _,err:=n4AssembleWindowsNativeOwnedTxn(context.Background(),tun,plan,id,guard);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("legacy LUID/index-only journal assembled: %v",err)
 }
 id.WintunGUID="11111111-2222-3333-4444-555555555555"
 if _,err:=n4AssembleWindowsNativeOwnedTxn(context.Background(),tun,plan,id,guard);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("missing alias assembled: %v",err)
 }
 id.AdapterAlias="not-WBD"
 if _,err:=n4AssembleWindowsNativeOwnedTxn(context.Background(),tun,plan,id,guard);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("foreign alias assembled: %v",err)
 }
 id.AdapterAlias="WBD"
 if _,err:=n4AssembleWindowsNativeOwnedTxn(context.Background(),tun,plan,id,guard);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("same-owner alias without durable PnP instance pin assembled: %v",err)
 }
 id.PnPInstanceSHA256="BAD"
 if _,err:=n4AssembleWindowsNativeOwnedTxn(context.Background(),tun,plan,id,guard);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("invalid PnP pin accepted for production native assembly: %v",err)
 }
}
