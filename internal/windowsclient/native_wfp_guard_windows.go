//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "reflect"

 "github.com/tailscale/wf"
)

// n4WindowsPersistentWFPGuard is an unconnected native backend. It uses a
// persistent, NON-dynamic WFP sublayer and three crash-stable owned rule IDs.
// Product code must never construct it until exclusive/shared Wintun ownership,
// DNS restore, and the network transaction are collectively qualified.
// Do not enable multiple guards with differing WintunLUID: each rule blocks
// IPv6/physical DNS on OTHER interfaces, including another Wintun adapter.
// Durable checks persistent metadata; it is not permission to install WFP.
type n4WindowsPersistentWFPGuard struct {
 InstallationID [16]byte
 WintunLUID uint64
 ExclusiveSingleWintun bool
}
func (g *n4WindowsPersistentWFPGuard) Durable()bool{
 return g!=nil && g.InstallationID!=([16]byte{}) &&g.WintunLUID!=0 && g.ExclusiveSingleWintun
}
func (g *n4WindowsPersistentWFPGuard) plan()(n4WFPRulePlan,error){
 if !g.Durable(){return n4WFPRulePlan{},ErrN4NativeGuard}
 return n4PlanWFPGuard(g.InstallationID,g.WintunLUID)
}
func n4WFPSublayerEqual(have,want *wf.Sublayer)bool{
 return have!=nil && want!=nil && have.ID==want.ID && have.Name==want.Name &&
  have.Persistent && have.Weight==want.Weight &&
  have.Provider==(wf.ProviderID{})
}
func n4WFPRuleEqual(have,want *wf.Rule)bool{
 return have!=nil && want!=nil && have.ID==want.ID && have.Name==want.Name &&
  have.Layer==want.Layer && have.Sublayer==want.Sublayer &&
  have.Persistent && !have.BootTime && !have.Disabled &&
  have.Action==wf.ActionBlock && !have.HardAction &&
  reflect.DeepEqual(have.Conditions,want.Conditions)
}
func n4WFPReadOwned(s *wf.Session,plan n4WFPRulePlan)(bool,map[wf.RuleID]bool,error){
 layers,err:=s.Sublayers();if err!=nil{return false,nil,err}
 subPresent:=false
 for _,sl:=range layers {
  if sl.ID!=plan.Sublayer.ID {continue}
  if !n4WFPSublayerEqual(sl,plan.Sublayer) {return false,nil,ErrN4ForeignRoute}
  subPresent=true
 }
 rules,err:=s.Rules();if err!=nil{return false,nil,err}
 matching:=make(map[wf.RuleID]bool,len(plan.Rules))
 for _,have:=range rules {
  if have.Sublayer==plan.Sublayer.ID {
   known:=false
   for _,want:=range plan.Rules{
    if have.ID!=want.ID {continue}
    known=true
    if !n4WFPRuleEqual(have,want) {return false,nil,ErrN4ForeignRoute}
    matching[have.ID]=true
    break
   }
   if !known{return false,nil,ErrN4ForeignRoute}
  }else{
   for _,want:=range plan.Rules{
    if have.ID==want.ID{return false,nil,ErrN4ForeignRoute}
   }
  }
 }
 return subPresent,matching,nil
}
// Verify opens a NEW engine handle, so success cannot be caused by a WFP
// dynamic session still held open in this process. It also rejects any
// foreign object reusing our deterministic install-scoped keys.
func (g *n4WindowsPersistentWFPGuard) Verify(ctx context.Context)(bool,error){
 if err:=ctx.Err();err!=nil{return false,err}
 plan,err:=g.plan();if err!=nil{return false,err}
 s,err:=wf.New(&wf.Options{Dynamic:false});if err!=nil{return false,err}
 defer s.Close()
 sub,ids,err:=n4WFPReadOwned(s,plan)
 if err!=nil{return false,err}
 if !sub{return false,nil}
 for _,want:=range plan.Rules{if !ids[want.ID]{return false,nil}}
 return true,nil
}
// Arm refuses an unsafe host-global LUID-not-equal WFP guard.
// A caller boolean cannot prove absence of independently managed Wintun
// adapters now or in future. Refuse BEFORE opening the WFP engine.
func (g *n4WindowsPersistentWFPGuard) Arm(ctx context.Context)error{
 if err:=ctx.Err();err!=nil{return err}
 return ErrN4NativeGuard
}
// Disarm only removes exact owned persistent objects; if any rule is foreign
// or the guard is unexpectedly incomplete, fail closed and keep host policy.
// Idempotent removal of an already absent owned installation is permitted.
func n4WFPDeletionIDs(p n4WFPRulePlan, sub bool, matching map[wf.RuleID]bool, partialOK bool)([]wf.RuleID,error){
 if !sub{
  if len(matching)!=0{return nil,ErrN4ForeignRoute}
  return nil,nil
 }
 out:=make([]wf.RuleID,0,len(p.Rules))
 for i:=len(p.Rules)-1;i>=0;i--{
  r:=p.Rules[i]
  if !matching[r.ID] {
   if !partialOK{return nil,errors.Join(ErrN4NativeGuard,errors.New("owned WFP rule missing during guarded disarm"))}
   continue
  }
  out=append(out,r.ID)
 }
 return out,nil
}

func (g *n4WindowsPersistentWFPGuard) disarm(ctx context.Context,partialOK bool)error{
 if err:=ctx.Err();err!=nil{return err}
 plan,err:=g.plan();if err!=nil{return err}
 return n4WFPDisarmExactPlan(ctx,plan,partialOK)
}

// n4WFPDisarmExactPlan is the same native WFP deletion path used by the
// product guard and the TEST-NET-only Actions lab. Callers may NOT use the
// lab to infer production-wide filter qualification. The independently
// reopened engine after deletion catches a failed or partial host mutation.
func n4WFPDisarmExactPlan(ctx context.Context,plan n4WFPRulePlan,partialOK bool)error{
 if err:=ctx.Err();err!=nil{return err}
 if plan.Sublayer==nil || len(plan.Rules)==0{return ErrN4NativeGuard}
 for _,r:=range plan.Rules{
  if r==nil || r.ID==(wf.RuleID{}) || r.Sublayer!=plan.Sublayer.ID{
   return ErrN4NativeGuard
  }
 }
 s,err:=wf.New(&wf.Options{Dynamic:false});if err!=nil{return err}
 // Exact read-before-write CAS rejects unknown rules, altered conditions,
 // foreign sublayers and moved rule IDs prior to ANY deletion.
 sub,matching,err:=n4WFPReadOwned(s,plan)
 if err!=nil{s.Close();return err}
 ids,err:=n4WFPDeletionIDs(plan,sub,matching,partialOK)
 if err!=nil{s.Close();return err}
 if sub{
  for _,id:=range ids{
   if err:=ctx.Err();err!=nil{s.Close();return err}
   if err:=s.DeleteRule(id);err!=nil{s.Close();return err}
  }
  if err:=ctx.Err();err!=nil{s.Close();return err}
  if err:=s.DeleteSublayer(plan.Sublayer.ID);err!=nil{s.Close();return err}
 }
 if err:=s.Close();err!=nil{return err}
 if err:=ctx.Err();err!=nil{return err}
 // WFP persistence is OS-owned, not session-owned. Reopen independently,
 // reject an unexpectedly surviving object, retain WAL on uncertainty.
 check,err:=wf.New(&wf.Options{Dynamic:false});if err!=nil{return err}
 defer check.Close()
 present,remaining,err:=n4WFPReadOwned(check,plan);if err!=nil{return err}
 if present || len(remaining)!=0{return ErrN4NativeGuard}
 return ctx.Err()
}

// Disarm refuses partial filtering state. This is appropriate if captured
// routes may still exist: any missing protective filter means a leak risk.
func (g *n4WindowsPersistentWFPGuard) Disarm(ctx context.Context)error{
 return g.disarm(ctx,false)
}
// DisarmAfterRoutesRemoved may ONLY be called after an owned durable journal
// has recorded complete removal of EVERY capture and underlay route. It
// removes remaining exact-owned filter IDs from an interrupted deletion and
// never accepts foreign objects; it is not a generic WFP reset operation.
func (g *n4WindowsPersistentWFPGuard) DisarmAfterRoutesRemoved(ctx context.Context)error{
 return g.disarm(ctx,true)
}
