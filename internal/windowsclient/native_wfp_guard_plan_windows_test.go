//go:build windows

package windowsclient

import (
 "errors"
 "testing"

 "github.com/tailscale/wf"
)
func TestN4WFPGuardPlanDeterministicIsolation(t *testing.T){
 var a,b [16]byte
 a[0]=1;b[0]=2
 p,err:=n4PlanWFPGuard(a,987654321)
 if err!=nil{t.Fatal(err)}
 q,err:=n4PlanWFPGuard(a,987654321)
 if err!=nil{t.Fatal(err)}
 if p.Sublayer.ID!=q.Sublayer.ID {t.Fatal("guard identity not crash stable")}
 for i:=range p.Rules{
  if p.Rules[i].ID!=q.Rules[i].ID {t.Fatal("rule ID changed between process generations")}
 }
 other,err:=n4PlanWFPGuard(b,987654321)
 if err!=nil{t.Fatal(err)}
 if other.Sublayer.ID==p.Sublayer.ID || other.Rules[0].ID==p.Rules[0].ID {
  t.Fatal("installation-scoped WFP rule identities overlap")
 }
 if _,err:=n4PlanWFPGuard([16]byte{},987654321);!errors.Is(err,ErrN4NativeGuard){t.Fatal("zero installation allowed")}
 if _,err:=n4PlanWFPGuard(a,0);!errors.Is(err,ErrN4NativeGuard){t.Fatal("zero tunnel LUID allowed")}
 if !p.Sublayer.Persistent || len(p.Rules)!=3 {t.Fatal("persistent guard plan missing rules")}
}
func TestN4WFPGuardPlanCannotDisableLeakFilters(t *testing.T){
 var install [16]byte; install[0]=42
 p,err:=n4PlanWFPGuard(install,17)
 if err!=nil{t.Fatal(err)}
 corrupted:=n4WFPRulePlan{Sublayer:p.Sublayer,Rules:append([]*wf.Rule(nil),p.Rules...)}
 copy0:=*corrupted.Rules[0]
 corrupted.Rules[0]=&copy0
 corrupted.Rules[0].Persistent=false
 if !errors.Is(n4ValidateWFPGuardPlan(corrupted,17),ErrN4NativeGuard) {t.Fatal("nonpersistent filter accepted")}
 corrupted.Rules[0]=p.Rules[0]
 corruptDNS:=*p.Rules[2]
 corrupted.Rules[2]=&corruptDNS
 corruptDNS.Conditions=[]*wf.Match{p.Rules[2].Conditions[0],{Field:wf.FieldIPRemotePort,Op:wf.MatchTypeEqual,Value:uint16(5353)}}
 if !errors.Is(n4ValidateWFPGuardPlan(corrupted,17),ErrN4NativeGuard) {t.Fatal("wrong DNS port accepted")}
 corrupted.Rules[2]=p.Rules[2]
 corruptIface:=*p.Rules[1]
 corrupted.Rules[1]=&corruptIface
 corruptIface.Conditions=[]*wf.Match{{Field:wf.FieldIPLocalInterface,Op:wf.MatchTypeEqual,Value:uint64(17)}}
 if !errors.Is(n4ValidateWFPGuardPlan(corrupted,17),ErrN4NativeGuard) {t.Fatal("non-tunnel predicate reversed")}
}
