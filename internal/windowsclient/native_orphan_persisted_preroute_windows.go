//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "net/netip"
 "os"
)

// n4PersistedOriginalProof does NOT open a Wintun or trust its alias. The
// original physical OS adapter must still own the exact GUID+LUID+IPv4 index
// from the checksum-validated original-generation WAL at EVERY operation.
type n4PersistedOriginalProof struct{ ID N4UnifiedIdentity }

func (p n4PersistedOriginalProof) IsCurrentOwnedWintun(ctx context.Context,index uint32)(bool,error){
 if index==0 || index!=p.ID.WintunIndex{return false,nil}
 state,err:=n4InspectOriginalWintun(ctx,p.ID)
 if err!=nil{return false,err}
 return state==N4UnifiedOwned,nil
}

// N4WindowsRecoverPersistedPreRouteWAL is deliberately limited to an
// original still-live GUID/LUID/index and lease/MTU/DNS-only parent WAL.
// No new Wintun handle, no alias adoption, no system route and no host-wide
// WFP rule is installed here. The persistent WFP must ALREADY be armed.
// Production activation is still disabled elsewhere.
func N4WindowsRecoverPersistedPreRouteWAL(ctx context.Context,path string,id N4UnifiedIdentity)error{
 guard:=&n4WindowsPersistentWFPGuard{
  InstallationID:id.Installation,WintunLUID:id.WintunLUID,
  ExclusiveSingleWintun:true, // selects exact legacy plan; never permits Arm
 }
 return n4RecoverPersistedPreRouteWAL(ctx,path,id,guard)
}

func n4ValidatePersistedPreRouteJournal(j n4UnifiedJournal,id N4UnifiedIdentity)error{
 if j.Identity!=id || (j.Phase!="applying" && j.Phase!="cleaning") ||
  len(j.Records)==0 || len(j.Records)>4 ||
  j.Records[0].Name!="guard" ||j.Records[0].State!="owned" ||
  string(j.Records[0].Before)!=`{"absent":true}`{
  return ErrN4JournalConflict
 }
 if len(j.Records)==1{
  // Only an interrupted *cleaning* transaction may contain guard alone.
  if j.Phase!="cleaning"{return ErrN4JournalConflict}
  return nil
 }
 copyJ:=j
 copyJ.Phase="applying"
 // Reuse the exact receipt parser for all immutable lease/MTU/DNS
 // baselines; a valid SHA on altered data is NEVER sufficient authority.
 return n4ValidatePreRouteOrphanReceipts(copyJ,id)
}

func n4PersistedPreRouteStage(id N4UnifiedIdentity,rec n4UnifiedRecord,g CrashPersistentGuard)(N4UnifiedStage,error){
 proof:=n4PersistedOriginalProof{ID:id}
 switch rec.Name{
 case "lease":
  var b n4LeaseBaseline
  if err:=json.Unmarshal(rec.Before,&b);err!=nil{return nil,ErrN4JournalConflict}
  addr,err:=netip.ParsePrefix(b.Address)
  if err!=nil{return nil,ErrN4JournalConflict}
  intent,err:=n4StageLeaseIntent(id.WintunIndex,addr)
  if err!=nil{return nil,ErrN4JournalConflict}
  return n4WindowsLeaseStage{Lease:intent,Writer:N4WindowsLeaseWriter{
   Guard:g,Adapter:proof,Snapshot:N4WindowsAddressSnapshot{},
  }},nil
 case "mtu":
  var b n4MTUStageBaseline
  if err:=json.Unmarshal(rec.Before,&b);err!=nil{return nil,ErrN4JournalConflict}
  return n4WindowsMTUStage{Target:b.Target,Writer:n4WindowsMTUWriter{Guard:g,Adapter:proof}},nil
 case "dns":
  var b n4DNSStageBaseline
  if err:=json.Unmarshal(rec.Before,&b);err!=nil{return nil,ErrN4JournalConflict}
  return n4WindowsDNSStage{Servers:b.Servers,Guard:g,Writer:n4OwnedDNSWrite{Adapter:proof}},nil
 default:return nil,ErrN4JournalConflict
 }
}

// Caller-injected guard is for isolated Windows test ONLY; the public
// production entry constructs the real persistent WFP identity above.
// Fail closed on unfinished, foreign or ambiguous state. Do not reconstruct
// a transient lease ownership token from a mere IP match: only a fully
// durable 'owned' stage combined with the exact original NIC and guard
// authorizes the existing CAS-protected Undo.
func n4RecoverPersistedPreRouteWAL(ctx context.Context,path string,id N4UnifiedIdentity,g CrashPersistentGuard)error{
 if err:=ctx.Err();err!=nil{return err}
 if path=="" || !id.valid() || id.WintunGUID=="" || id.AdapterAlias=="" ||
  g==nil || !g.Durable(){return ErrN4NativeGuard}
 release,err:=(N4WindowsMachineGate{}).Acquire(ctx)
 if err!=nil{return err}
 defer release()
 j,err:=n4UnifiedRead(path)
 if errors.Is(err,os.ErrNotExist){return nil}
 if err!=nil{return err}
 if err:=n4ValidatePersistedPreRouteJournal(j,id);err!=nil{return err}
 if _,err:=os.Lstat(path+".routes");err==nil{return ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return err}
 if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return err}
 // The verified full persistent protection must stay armed until all
 // interface-local stage writes have been undone and independently read.
 if len(j.Records)>1{
  ok,err:=g.Verify(ctx)
  if err!=nil{return err}
  if !ok{return ErrN4NativeGuard}
 }
 // Preflight EVERY live receipt before any OS setter/deletion, including
 // untouched stages, to avoid a partial rollback across a foreign stage.
 for i:=1;i<len(j.Records);i++{
  rec:=j.Records[i]
  stage,err:=n4PersistedPreRouteStage(id,rec,g)
  if err!=nil{return err}
  state,err:=stage.Inspect(ctx,id,rec.Before)
  if err!=nil{return err}
  if state==N4UnifiedForeign || state==N4UnifiedPartial{return ErrN4RouteAmbiguous}
  if rec.State=="intent" && state!=N4UnifiedAbsent{return ErrN4RouteAmbiguous}
  if rec.State=="owned" && state==N4UnifiedAbsent && j.Phase=="applying"{
   return ErrN4RouteAmbiguous
  }
 }
 if j.Phase!="cleaning"{
  j.Phase="cleaning"
  if err:=n4UnifiedWrite(path,j,false);err!=nil{return err}
 }
 // The route stage can never be entered: even a sidecar without its
 // parent receipt was refused before this point.
 for len(j.Records)>1{
  if err:=ctx.Err();err!=nil{return err}
  i:=len(j.Records)-1
  rec:=j.Records[i]
  if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return err}
  verified,err:=g.Verify(ctx)
  if err!=nil{return err}
  if !verified{return ErrN4NativeGuard}
  stage,err:=n4PersistedPreRouteStage(id,rec,g)
  if err!=nil{return err}
  state,err:=stage.Inspect(ctx,id,rec.Before)
  if err!=nil{return err}
  if state==N4UnifiedForeign || state==N4UnifiedPartial{return ErrN4RouteAmbiguous}
  if rec.State=="intent" && state!=N4UnifiedAbsent{return ErrN4RouteAmbiguous}
  if rec.State=="owned" && state==N4UnifiedOwned{
   if err:=stage.Undo(ctx,id,rec.Before);err!=nil{return err}
  }
  if state,err=stage.Inspect(ctx,id,rec.Before);err!=nil{return err
  }else if state!=N4UnifiedAbsent{return ErrN4RouteAmbiguous}
  if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return err}
  j.Records=j.Records[:i]
  if err:=n4UnifiedWrite(path,j,false);err!=nil{return err}
 }
 // The confirmed original remains present. No route was ever started;
 // ONLY now can the independent, exact-ID WFP backend be disarmed.
 if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return err}
 if err:=n4FinishGuardAfterRoutes(ctx,g);err!=nil{return err}
 if ok,err:=g.Verify(ctx);err!=nil{return err
 }else if ok{return ErrN4NativeGuard}
 if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return err}
 if _,err:=os.Lstat(path+".routes");err==nil{return ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return err}
 return os.Remove(path)
}
