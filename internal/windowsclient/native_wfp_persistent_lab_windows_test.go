//go:build windows

package windowsclient

import (
 "context"
 "crypto/rand"
 "encoding/binary"
 "errors"
 "net/netip"
 "os"
 "testing"

 "github.com/tailscale/wf"
 "golang.org/x/sys/windows"
)

// This is a self-cleaning GitHub-hosted WFP **LAB** test. It is NOT the
// production IPv6/DNS guard. Its block is limited to TEST-NET-3 203.0.113.254
// and unused TCP/UDP destination port 65530. The filter is persistent to
// validate it survives closing and reopening a non-dynamic WFP session.
// Never run this test on the user's system or in the packaged client.
func n4WFPTestGUID(t *testing.T) windows.GUID {
 t.Helper()
 var b [16]byte
 if _,err:=rand.Read(b[:]);err!=nil{t.Fatal(err)}
 b[6]=(b[6]&0x0f)|0x40
 b[8]=(b[8]&0x3f)|0x80
 return windows.GUID{
  Data1:binary.LittleEndian.Uint32(b[0:4]),
  Data2:binary.LittleEndian.Uint16(b[4:6]),
  Data3:binary.LittleEndian.Uint16(b[6:8]),
  Data4:[8]byte(b[8:16]),
 }
}
func n4LabRulePresent(s *wf.Session,want *wf.Rule) (bool,error) {
 rules,err:=s.Rules();if err!=nil{return false,err}
 for _,r:=range rules{
  if r.ID==want.ID {
   if !n4WFPRuleEqual(r,want) {
    return false,errors.New("WFP LAB rule found but owner/conditions/persistence differ")
   }
   return true,nil
  }
 }
 return false,nil
}
func TestN4WindowsWFPPersistentFilterHostLab(t *testing.T) {
 if os.Getenv("WBD_N4_WFP_ISOLATED_LAB")!="1"{
  t.Skip("host WFP mutation only in explicitly isolated Actions runner")
 }
 subID:=wf.SublayerID(n4WFPTestGUID(t))
 ruleID:=wf.RuleID(n4WFPTestGUID(t))
 // A persistent sublayer with a random GUID must not collide with or
 // impersonate the user's firewall policy. Test a single non-routable
 // destination+port instead of installing blanket IPv6/DNS restrictions.
 session,err:=wf.New(&wf.Options{Dynamic:false,Name:"WBD N4 GitHub hosted WFP non-production LAB"})
 if err!=nil{t.Fatal(err)}
 sublayerAdded,ruleAdded:=false,false
 session.Close()
 t.Cleanup(func(){
  cleanup,err:=wf.New(&wf.Options{Dynamic:false,Name:"WBD N4 CI WFP cleanup"})
  if err!=nil{t.Errorf("WFP LAB leaked filter (cleanup engine open): %v",err);return}
  defer cleanup.Close()
  if ruleAdded {
   if err:=cleanup.DeleteRule(ruleID);err!=nil{t.Errorf("WFP LAB filter deletion failed: %v",err)}
  }
  if sublayerAdded {
   if err:=cleanup.DeleteSublayer(subID);err!=nil{t.Errorf("WFP LAB sublayer deletion failed: %v",err)}
  }
 })
 // The WFP engine is reopened after test setup to prove handle lifetime
 // alone does not keep a persistent filter alive.
 setup,err:=wf.New(&wf.Options{Dynamic:false})
 if err!=nil{t.Fatal(err)}
 sub:=&wf.Sublayer{ID:subID,Name:"WBD N4 TEST-NET scoped temporary lab",Description:"CI-only WFP persistence/ownership experiment",Persistent:true,Weight:11}
 if err:=setup.AddSublayer(sub);err!=nil{setup.Close();t.Fatal(err)}
 sublayerAdded=true
 rule:=&wf.Rule{
  ID:ruleID,
  Name:"WBD N4 TEST-NET 203.0.113.254:65530 blocker",
  Description:"CI-only native persistent rule; not a product DNS/IPv6 filter",
  Layer:wf.LayerALEAuthConnectV4,Sublayer:subID,
  Weight:1,Persistent:true,Action:wf.ActionBlock,
  Conditions:[]*wf.Match{
   {Field:wf.FieldIPRemoteAddress,Op:wf.MatchTypeEqual,Value:netip.MustParseAddr("203.0.113.254")},
   {Field:wf.FieldIPRemotePort,Op:wf.MatchTypeEqual,Value:uint16(65530)},
   // Exercise the native non-tunnel interface condition without affecting
   // real destinations: the rule still requires TEST-NET+port65530.
   {Field:wf.FieldIPLocalInterface,Op:wf.MatchTypeNotEqual,Value:uint64(0xA11CE)},
  },
 }
 if err:=setup.AddRule(rule);err!=nil{setup.Close();t.Fatal(err)}
 ruleAdded=true
 if err:=setup.Close();err!=nil{t.Fatal(err)}
 reopened,err:=wf.New(&wf.Options{Dynamic:false})
 if err!=nil{t.Fatal(err)}
 found,err:=n4LabRulePresent(reopened,rule)
 if err!=nil{reopened.Close();t.Fatal(err)}
 if !found{reopened.Close();t.Fatal("CI scoped persistent rule vanished across WFP engine reopen")}
 if err:=reopened.Close();err!=nil{t.Fatal(err)}
 // Use exactly the same independent engine/CAS/readback deletion routine
 // as the product guard, but only on THIS TEST-NET+port scoped lab filter.
 plan:=n4WFPRulePlan{Sublayer:sub,Rules:[]*wf.Rule{rule}}
 n4LogExactLabSubDiff(t,sub)
 if err:=n4WFPDisarmExactPlan(context.Background(),plan,false);err!=nil{
  t.Fatalf("real WFP exact-owned cleanup failed: %v",err)
 }
 ruleAdded, sublayerAdded=false,false
 if err:=n4WFPDisarmExactPlan(context.Background(),plan,false);err!=nil{
  t.Fatalf("repeated real WFP cleanup not idempotent: %v",err)
 }
 t.Log("Real WFP persisted scoped TEST-NET filter, then production-shared exact CAS deletion and independent reopened readback PASS; not host-wide DNS/IPv6")
}


func TestN4WindowsWFPExactOwnedDeletePartialHostLab(t *testing.T){
 if os.Getenv("WBD_N4_WFP_ISOLATED_LAB")!="1"{
  t.Skip("real TEST-NET WFP mutation only on explicitly gated Actions lab")
 }
 sid:=wf.SublayerID(n4WFPTestGUID(t))
 rid:=wf.RuleID(n4WFPTestGUID(t))
 sub:=&wf.Sublayer{ID:sid,Name:"WBD N4 TEST-NET partial-deletion lab",Persistent:true,Weight:11}
 rule:=&wf.Rule{
  ID:rid,Name:"WBD N4 TEST-NET partial-deletion rule",
  Layer:wf.LayerALEAuthConnectV4,Sublayer:sid,
  Persistent:true,Weight:1,Action:wf.ActionBlock,
  Conditions:[]*wf.Match{
   {Field:wf.FieldIPRemoteAddress,Op:wf.MatchTypeEqual,Value:netip.MustParseAddr("203.0.113.254")},
   {Field:wf.FieldIPRemotePort,Op:wf.MatchTypeEqual,Value:uint16(65530)},
  },
 }
 s,err:=wf.New(&wf.Options{Dynamic:false})
 if err!=nil{t.Fatal(err)}
 ruleAdded,subAdded:=false,false
 t.Cleanup(func(){
  cleanup,e:=wf.New(&wf.Options{Dynamic:false})
  if e!=nil{t.Errorf("cannot reopen scoped WFP lab cleanup: %v",e);return}
  defer cleanup.Close()
  if ruleAdded {if e=cleanup.DeleteRule(rid);e!=nil{t.Errorf("WFP scoped rule leftover: %v",e)}}
  if subAdded {if e=cleanup.DeleteSublayer(sid);e!=nil{t.Errorf("WFP scoped sublayer leftover: %v",e)}}
 })
 if err=s.AddSublayer(sub);err!=nil{s.Close();t.Fatal(err)}
 subAdded=true
 if err=s.AddRule(rule);err!=nil{s.Close();t.Fatal(err)}
 ruleAdded=true
 if err=s.Close();err!=nil{t.Fatal(err)}
 plan:=n4WFPRulePlan{Sublayer:sub,Rules:[]*wf.Rule{rule}}
 n4LogExactLabSubDiff(t,sub)
 foreign:=*rule
 foreign.Conditions=[]*wf.Match{
  {Field:wf.FieldIPRemoteAddress,Op:wf.MatchTypeEqual,Value:netip.MustParseAddr("203.0.113.254")},
  {Field:wf.FieldIPRemotePort,Op:wf.MatchTypeEqual,Value:uint16(65529)},
 }
 if e:=n4WFPDisarmExactPlan(context.Background(),
  n4WFPRulePlan{Sublayer:sub,Rules:[]*wf.Rule{&foreign}},false);!errors.Is(e,ErrN4ForeignRoute){
  t.Fatalf("foreign plan unexpectedly deleted OS rule: %v",e)
 }
 verify,e:=wf.New(&wf.Options{Dynamic:false})
 if e!=nil{t.Fatal(e)}
 exists,e:=n4LabRulePresent(verify,rule)
 verify.Close()
 if e!=nil || !exists{t.Fatalf("foreign-plan call mutated real scoped WFP rule: %v %v",exists,e)}
 // Simulate an interrupted exact-owned rule deletion. A strict disarm
 // refuses the partial state; the separately gated after-routes cleanup
 // is permitted to remove ONLY the known empty owned sublayer.
 s,err=wf.New(&wf.Options{Dynamic:false});if err!=nil{t.Fatal(err)}
 if err=s.DeleteRule(rid);err!=nil{s.Close();t.Fatal(err)}
 ruleAdded=false
 if err=s.Close();err!=nil{t.Fatal(err)}
 if e:=n4WFPDisarmExactPlan(context.Background(),plan,false);!errors.Is(e,ErrN4NativeGuard){
  t.Fatalf("strict disarm accepted interrupted WFP rule state: %v",e)
 }
 if e:=n4WFPDisarmExactPlan(context.Background(),plan,true);e!=nil{
  t.Fatalf("exact owned partial WFP cleanup did not finish: %v",e)
 }
 subAdded=false
 if e:=n4WFPDisarmExactPlan(context.Background(),plan,true);e!=nil{
  t.Fatalf("partial WFP cleanup retry failed: %v",e)
 }
 t.Log("real TEST-NET WFP foreign-rule CAS, partial failure, and exact-owned idempotent delete PASS; no host-wide filter")
}

func n4LogExactLabSubDiff(t *testing.T,want *wf.Sublayer) {
 t.Helper()
 s,err:=wf.New(&wf.Options{Dynamic:false})
 if err!=nil{t.Fatalf("TEST-NET-only WFP diagnostic engine: %v",err)}
 defer s.Close()
 all,err:=s.Sublayers()
 if err!=nil{t.Fatalf("TEST-NET-only WFP diagnostic read: %v",err)}
 found:=false
 for _,have:=range all {
  if have.ID!=want.ID{continue}
  found=true
  t.Logf("OWN_SCOPED_WFP_SUB_DIFF name_eq=%t persistent_have=%t persistent_want=%t weight_have=%d weight_want=%d provider_zero=%t provider_data_len=%d exact=%t",
   have.Name==want.Name,have.Persistent,want.Persistent,have.Weight,want.Weight,
   have.Provider==(wf.ProviderID{}),len(have.ProviderData),
   n4WFPSublayerEqual(have,want))
 }
 if !found{t.Log("OWN_SCOPED_WFP_SUB_DIFF owned randomized lab sublayer absent")}
}
