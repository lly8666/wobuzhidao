//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "fmt"
 "reflect"

 "github.com/tailscale/wf"
)

// n4WindowsPersistentWFPGuard is an unconnected native backend. It uses a
// persistent, NON-dynamic WFP sublayer and three crash-stable owned rule IDs.
// Product code must never construct it until exclusive/shared Wintun ownership,
// DNS restore, and the network transaction are collectively qualified.
// Do not enable multiple guards with differing WintunLUID: each rule blocks
// IPv6/physical DNS on OTHER interfaces, including another Wintun adapter.
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
// Arm is not used from cmd/wbd-client while scriptless Wintun/DNS lifecycle
// is incomplete. It is intentionally idempotent for same-installation
// partial-Arm crash repair, and refuses any mismatched/foreign WFP rule.
func (g *n4WindowsPersistentWFPGuard) Arm(ctx context.Context)error{
 if err:=ctx.Err();err!=nil{return err}
 plan,err:=g.plan();if err!=nil{return err}
 s,err:=wf.New(&wf.Options{Dynamic:false});if err!=nil{return err}
 defer s.Close()
 sub,existing,err:=n4WFPReadOwned(s,plan)
 if err!=nil{return err}
 if !sub {
  if err:=s.AddSublayer(plan.Sublayer);err!=nil{return fmt.Errorf("native WFP owned sublayer: %w",err)}
 }
 for _,r:=range plan.Rules {
  if err:=ctx.Err();err!=nil{return err}
  if existing[r.ID]{continue}
  if err:=s.AddRule(r);err!=nil{return fmt.Errorf("native WFP owned guard rule: %w",err)}
 }
 if err:=s.Close();err!=nil{return err}
 ok,err:=g.Verify(ctx)
 if err!=nil{return err}
 if !ok{return ErrN4NativeGuard}
 return nil
}
// Disarm only removes exact owned persistent objects; if any rule is foreign
// or the guard is unexpectedly incomplete, fail closed and keep host policy.
// Idempotent removal of an already absent owned installation is permitted.
func (g *n4WindowsPersistentWFPGuard) Disarm(ctx context.Context)error{
 if err:=ctx.Err();err!=nil{return err}
 plan,err:=g.plan();if err!=nil{return err}
 s,err:=wf.New(&wf.Options{Dynamic:false});if err!=nil{return err}
 defer s.Close()
 sub,matching,err:=n4WFPReadOwned(s,plan);if err!=nil{return err}
 if !sub {
  if len(matching)>0{return ErrN4ForeignRoute}
  return nil
 }
 // If a filter unexpectedly vanished while routes still existed, a caller
 // must treat this as lost protection and refuse regular network activation.
 for _,r:=range plan.Rules{
  if !matching[r.ID]{return errors.Join(ErrN4NativeGuard,errors.New("owned WFP rule missing during guard disarm"))}
 }
 for i:=len(plan.Rules)-1;i>=0;i-- {
  if err:=ctx.Err();err!=nil{return err}
  if err:=s.DeleteRule(plan.Rules[i].ID);err!=nil{return err}
 }
 if err:=s.DeleteSublayer(plan.Sublayer.ID);err!=nil{return err}
 return nil
}
