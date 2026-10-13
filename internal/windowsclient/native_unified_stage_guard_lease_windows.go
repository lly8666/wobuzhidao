//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "net/netip"
 "time"

 "github.com/tailscale/wf"
)

// Concrete Win32 stages for the existing five-stage WAL. Product activation
// remains prohibited until MTU/DNS/routes are concrete and WFP exclusivity is
// independently proven; these stages never run automatically on import.
type n4WindowsGuardStage struct{ Guard *n4WindowsPersistentWFPGuard }

func n4WFPClassifyOwned(sub bool, matches map[wf.RuleID]bool, plan n4WFPRulePlan) N4UnifiedState {
 if !sub { if len(matches)!=0{return N4UnifiedPartial};return N4UnifiedAbsent }
 for _,rule:=range plan.Rules {
  if !matches[rule.ID] { return N4UnifiedPartial }
 }
 return N4UnifiedOwned
}
func(g *n4WindowsPersistentWFPGuard) ownedState(ctx context.Context)(N4UnifiedState,error) {
 if err:=ctx.Err();err!=nil{return "",err}
 p,err:=g.plan();if err!=nil{return "",err}
 s,err:=wf.New(&wf.Options{Dynamic:false});if err!=nil{return "",err}
 defer s.Close()
 sub,matches,err:=n4WFPReadOwned(s,p)
 if err!=nil{return "",err} // foreign WFP object is never claimed
 return n4WFPClassifyOwned(sub,matches,p),nil
}
func(s n4WindowsGuardStage) Name()string{return "guard"}
func(s n4WindowsGuardStage) valid(id N4UnifiedIdentity,b json.RawMessage)error {
 if s.Guard==nil || !s.Guard.Durable() || !id.valid() ||
  id.Installation!=s.Guard.InstallationID || id.WintunLUID!=s.Guard.WintunLUID {
  return ErrN4NativeGuard
 }
 if b!=nil && string(b)!=`{"absent":true}` { return ErrN4JournalConflict }
 return nil
}
func(s n4WindowsGuardStage) Snapshot(ctx context.Context,id N4UnifiedIdentity)(json.RawMessage,error){
 if err:=s.valid(id,nil);err!=nil{return nil,err}
 state,err:=s.Guard.ownedState(ctx)
 if err!=nil{return nil,err}
 if state!=N4UnifiedAbsent{return nil,ErrN4JournalConflict}
 return json.RawMessage(`{"absent":true}`),nil
}
func(s n4WindowsGuardStage) Apply(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 if err:=s.valid(id,b);err!=nil{return err}
 state,err:=s.Guard.ownedState(ctx);if err!=nil{return err}
 if state!=N4UnifiedAbsent{return ErrN4JournalConflict}
 return s.Guard.Arm(ctx)
}
func(s n4WindowsGuardStage) Inspect(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)(N4UnifiedState,error){
 if err:=s.valid(id,b);err!=nil{return "",err}
 return s.Guard.ownedState(ctx)
}
func(s n4WindowsGuardStage) Undo(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 if err:=s.valid(id,b);err!=nil{return err}
 // Unified Recover calls guard Undo strictly last, once every later
 // route/DNS/MTU/lease receipt has been durably retired.
 return s.Guard.DisarmAfterRoutesRemoved(ctx)
}

type n4WindowsLeaseStage struct {
 Lease N4NativeLeaseIntent
 Writer N4WindowsLeaseWriter
}
type n4LeaseBaseline struct {
 Index uint32 `json:"index"`
 Address string `json:"address"`
 Absent bool `json:"absent"`
}
func(s n4WindowsLeaseStage) Name()string{return "lease"}
func(s n4WindowsLeaseStage) check(ctx context.Context,id N4UnifiedIdentity)error{
 if err:=ctx.Err();err!=nil{return err}
 if err:=n4ValidateLeaseIntent(s.Lease);err!=nil{return err}
 if !id.valid() || s.Lease.InterfaceIndex!=id.WintunIndex {return ErrN4ForeignRoute}
 if s.Writer.Guard==nil || !s.Writer.Guard.Durable() || s.Writer.Adapter==nil {
  return ErrN4NativeGuard
 }
 owned,err:=s.Writer.Adapter.IsCurrentOwnedWintun(ctx,id.WintunIndex)
 if err!=nil{return err}
 if !owned{return ErrN4ForeignRoute}
 return nil
}
func(s n4WindowsLeaseStage) receipt(b json.RawMessage)error{
 var rec n4LeaseBaseline
 if err:=json.Unmarshal(b,&rec);err!=nil{return ErrN4JournalConflict}
 if !rec.Absent || rec.Index!=s.Lease.InterfaceIndex ||
  rec.Address!=s.Lease.Prefix.String() {return ErrN4JournalConflict}
 return nil
}
func(s n4WindowsLeaseStage) Snapshot(ctx context.Context,id N4UnifiedIdentity)(json.RawMessage,error){
 if err:=s.check(ctx,id);err!=nil{return nil,err}
 rows,err:=s.Writer.Snapshot.IPv4(ctx,s.Lease.InterfaceIndex)
 if err!=nil{return nil,err}
 if len(rows)!=0{return nil,ErrN4ForeignRoute} // never delete DHCP/foreign IP
 return json.Marshal(n4LeaseBaseline{Index:id.WintunIndex,Address:s.Lease.Prefix.String(),Absent:true})
}
func(s n4WindowsLeaseStage) Inspect(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)(N4UnifiedState,error){
 if err:=s.receipt(b);err!=nil{return "",err}
 if err:=s.check(ctx,id);err!=nil{return "",err}
 rows,err:=s.Writer.Snapshot.IPv4(ctx,s.Lease.InterfaceIndex)
 if err!=nil{return "",err}
 if len(rows)==0{return N4UnifiedAbsent,nil}
 if len(rows)!=1 || rows[0].InterfaceIndex!=id.WintunIndex ||
  rows[0].Address!=s.Lease.Prefix.Addr(){return N4UnifiedForeign,nil}
 row,found,err:=(N4WindowsLeaseReader{}).Lookup(ctx,s.Lease)
 if err!=nil {
  if errors.Is(err,ErrN4ForeignRoute){return N4UnifiedForeign,nil}
  return "",err
 }
 if !found{return N4UnifiedPartial,nil}
 if err:=N4NativeLeaseDADGate(row,s.Lease);err!=nil{
  if errors.Is(err,ErrN4LeaseNotPreferred){return N4UnifiedPartial,nil}
  if errors.Is(err,ErrN4ForeignRoute){return N4UnifiedForeign,nil}
  return "",err
 }
 return N4UnifiedOwned,nil
}
func(s n4WindowsLeaseStage) Apply(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 if err:=s.receipt(b);err!=nil{return err}
 if err:=s.check(ctx,id);err!=nil{return err}
 // The WAL has persisted lease "intent" BEFORE invoking Create. The
 // writer enforces guard.Verify, current Wintun owner, empty adapter, /32.
 if _,err:=s.Writer.Create(ctx,s.Lease);err!=nil{return err}
 // DAD is asynchronous on Windows. Never arm capture while tentative.
 timer:=time.NewTimer(8*time.Second);defer timer.Stop()
 ticker:=time.NewTicker(100*time.Millisecond);defer ticker.Stop()
 for {
  status,err:=s.Inspect(ctx,id,b)
  if err!=nil{return err}
  switch status {
  case N4UnifiedOwned:return nil
  case N4UnifiedPartial:
  default:return ErrN4RouteAmbiguous
  }
  select {
  case <-ctx.Done():return ctx.Err()
  case <-timer.C:return ErrN4LeaseNotPreferred
  case <-ticker.C:
  }
 }
}
func(s n4WindowsLeaseStage) Undo(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 if err:=s.receipt(b);err!=nil{return err}
 if err:=s.check(ctx,id);err!=nil{return err}
 // Only the unified journal's already-confirmed "owned" receipt may reach
 // Undo. A crash after Create but before durable "owned" remains AMBIGUOUS.
 if err:=s.Writer.DeleteConfirmed(ctx,n4NativeLeaseOwnerToken{intent:s.Lease,created:true});err!=nil{return err}
 state,err:=s.Inspect(ctx,id,b)
 if err!=nil{return err}
 if state!=N4UnifiedAbsent{return ErrN4RouteAmbiguous}
 return nil
}

// Confirm the stage's selected host address never becomes a prefix route.
// LAN/CN direct policy is in process memory, not an OS route exception.
func n4StageLeaseIntent(index uint32,p netip.Prefix)(N4NativeLeaseIntent,error){
 l:=N4NativeLeaseIntent{InterfaceIndex:index,Prefix:p}
 return l,n4ValidateLeaseIntent(l)
}
