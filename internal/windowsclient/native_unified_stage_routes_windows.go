//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "os"
 "slices"
)

// The fifth WAL stage installs only server /32 escape + two /1 captures +
// configured DNS /32. It reuses the previously qualified per-route native
// owned journal for partial Write/Stop recovery instead of expanding 10k+
// China/LAN CIDRs or introducing a third transaction implementation.
type n4WindowsCompactRouteStage struct {
 Plan NetworkPlan
 Path string // independent bounded per-route receipts at parent WAL + ".routes"
 Backend NativeRouteBackend // production: N4WindowsRouteBackend{}
 Guard CrashPersistentGuard
}
type n4CompactRouteBaseline struct {
 Keys []NativeRouteKey `json:"keys"`
}
func(s n4WindowsCompactRouteStage) Name()string{return "routes"}
func(s n4WindowsCompactRouteStage) expected(id N4UnifiedIdentity)([]NativeRouteKey,error){
 if !id.valid() || s.Path=="" || s.Backend==nil || s.Guard==nil || !s.Guard.Durable(){
  return nil,ErrN4NativeGuard
 }
 return N4BuildCompactNativeRouteKeys(s.Plan,id.WintunIndex)
}
func(s n4WindowsCompactRouteStage) receipt(id N4UnifiedIdentity,b json.RawMessage)([]NativeRouteKey,error){
 keys,err:=s.expected(id);if err!=nil{return nil,err}
 var saved n4CompactRouteBaseline
 if err:=json.Unmarshal(b,&saved);err!=nil{return nil,ErrN4JournalConflict}
 if !slices.Equal(keys,saved.Keys){return nil,ErrN4JournalConflict}
 return keys,nil
}
func(s n4WindowsCompactRouteStage) inspectKeys(ctx context.Context,keys []NativeRouteKey)(N4UnifiedState,error){
 var present int
 for _,key:=range keys {
  if err:=ctx.Err();err!=nil{return "",err}
  found,exact,err:=s.Backend.Lookup(ctx,key)
  if err!=nil{return "",err}
  if !found{continue}
  if !exact{return N4UnifiedForeign,nil}
  present++
 }
 if present==0{return N4UnifiedAbsent,nil}
 if present==len(keys){return N4UnifiedOwned,nil}
 return N4UnifiedPartial,nil
}
func(s n4WindowsCompactRouteStage) Snapshot(ctx context.Context,id N4UnifiedIdentity)(json.RawMessage,error){
 keys,err:=s.expected(id);if err!=nil{return nil,err}
 if err:=ctx.Err();err!=nil{return nil,err}
 if _,err:=os.Stat(s.Path);err==nil{return nil,ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return nil,err}
 status,err:=s.inspectKeys(ctx,keys)
 if err!=nil{return nil,err}
 if status!=N4UnifiedAbsent{return nil,ErrN4ForeignRoute}
 return json.Marshal(n4CompactRouteBaseline{Keys:keys})
}
func(s n4WindowsCompactRouteStage) Inspect(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)(N4UnifiedState,error){
 keys,err:=s.receipt(id,b);if err!=nil{return "",err}
 return s.inspectKeys(ctx,keys)
}

// The per-route journal MUST verify (and retain) the parent's persistent
// guard, but must NOT Arm or Disarm it: parent owns that lifetime and leaves
// it installed until AFTER lease, MTU and DNS are also restored.
type n4SharedPersistentGuard struct{ parent CrashPersistentGuard }
func(g n4SharedPersistentGuard) Durable()bool{return g.parent!=nil && g.parent.Durable()}
func(g n4SharedPersistentGuard) Arm(ctx context.Context)error{
 ok,err:=g.Verify(ctx);if err!=nil{return err}
 if !ok{return ErrN4NativeGuard}
 return nil
}
func(g n4SharedPersistentGuard) Verify(ctx context.Context)(bool,error){
 if err:=ctx.Err();err!=nil{return false,err}
 if !g.Durable(){return false,ErrN4NativeGuard}
 return g.parent.Verify(ctx)
}
func(g n4SharedPersistentGuard) Disarm(ctx context.Context)error{
 return ctx.Err() // NOOP: parent releases this guard only after other stages
}
func(s n4WindowsCompactRouteStage) ownedRoutes()*N4OwnedRoutes{
 return &N4OwnedRoutes{Path:s.Path,Route:s.Backend,Guard:n4SharedPersistentGuard{parent:s.Guard}}
}
func(s n4WindowsCompactRouteStage) Apply(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 keys,err:=s.receipt(id,b);if err!=nil{return err}
 // Parent WAL persisted route intent BEFORE this separate per-entry journal.
 return s.ownedRoutes().ApplyRoutes(ctx,keys)
}
func(s n4WindowsCompactRouteStage) Undo(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 keys,err:=s.receipt(id,b);if err!=nil{return err}
 if err:=s.ownedRoutes().Recover(ctx);err!=nil{return err}
 state,err:=s.inspectKeys(ctx,keys)
 if err!=nil{return err}
 if state!=N4UnifiedAbsent{return ErrN4RouteAmbiguous}
 return nil
}
func(s n4WindowsCompactRouteStage) RecoverUnconfirmedRoutes(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 // Called ONLY for the final stage's unconfirmed parent intent and only
 // while the parent has independently verified the full WFP guard remains.
 return s.Undo(ctx,id,b)
}
