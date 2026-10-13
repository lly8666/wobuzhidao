//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "os"
 "net/netip"
)

// A minimal assembly of the ALREADY IMPLEMENTED five native Windows stages.
// This is deliberately package-private: the product must not call Apply
// until independently verified exclusive-safe production WFP (or a shared
// multi-Wintun policy) and crash-orphan ownership have been qualified.
// No extra journal, adapter service, daemon, or configuration knob is added.
func n4AssembleWindowsNativeOwnedTxn(
 ctx context.Context, tun *TUN, plan NetworkPlan, id N4UnifiedIdentity,
 guard *n4WindowsPersistentWFPGuard,
)(*N4NativeUnifiedTxn,error){
 if err:=ctx.Err();err!=nil{return nil,err}
 if !id.valid() || tun==nil || !tun.createdByProcess ||
  plan.AdapterAlias=="" || plan.AdapterAlias!=tun.Name() {
  return nil,ErrN4ForeignRoute
 }
 if guard==nil || !guard.Durable() ||
  guard.InstallationID!=id.Installation || guard.WintunLUID!=id.WintunLUID{
  return nil,ErrN4NativeGuard
 }
 if plan.StatePath=="" || plan.Schema!=StateSchema {
  return nil,ErrNetworkPlan
 }
 // Never silently cross over a pre-existing PowerShell-owned state file,
 // even if this session can still create a fresh native journal.
 if _,err:=os.Stat(plan.StatePath);err==nil{return nil,ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return nil,err}
 native,err:=tun.NativeIdentity(ctx)
 if err!=nil{return nil,err}
 if !native.CreatedByProcess || native.Index!=id.WintunIndex ||
  native.LUID!=id.WintunLUID{return nil,ErrN4ForeignRoute}
 if _,err:=N4BuildCompactNativeRouteKeys(plan,id.WintunIndex);err!=nil{return nil,err}
 lease,err:=n4StageLeaseIntent(id.WintunIndex,plan.Lease4)
 if err!=nil{return nil,err}
 if _,_,err:=n4StaticDNSRequest(plan.DNSServers4);err!=nil{return nil,err}
 owner:=N4WindowsCreatedWintunReservation{
  Tunnel:tun,Installation:id.Installation,Generation:id.Generation,
 }
 walPath:=plan.StatePath+".n4-owned-wal"
 n:=&N4NativeUnifiedTxn{
  Path:walPath,Identity:id,Owner:owner,Gate:N4WindowsMachineGate{},
  assemblyOnly:true,
  Stages:[5]N4UnifiedStage{
   n4WindowsGuardStage{Guard:guard},
   n4WindowsLeaseStage{Lease:lease,Writer:N4WindowsLeaseWriter{
    Guard:guard,Adapter:tun,Snapshot:N4WindowsAddressSnapshot{},
   }},
   n4WindowsMTUStage{Target:plan.TunnelMTU,Writer:n4WindowsMTUWriter{
    Guard:guard,Adapter:tun,
   }},
   n4WindowsDNSStage{Servers:append([]netip.Addr(nil),plan.DNSServers4...),
    Guard:guard,Writer:n4OwnedDNSWrite{Adapter:tun}},
   n4WindowsCompactRouteStage{
    Plan:plan,Path:walPath+".routes",Backend:N4WindowsRouteBackend{},Guard:guard,
   },
  },
 }
 if err:=n.preflight();err!=nil{return nil,err}
 return n,nil
}
