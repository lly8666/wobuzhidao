//go:build windows

package windowsclient

import (
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
 sub:=&wf.Sublayer{ID:subID,Name:"WBD N4 TEST-NET scoped temporary lab",Description:"CI-only WFP persistence/ownership experiment",Persistent:true,Weight:1}
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
 // Only successful owned rule lookup permits deletion. Cleanup always
 // takes place through a new session, i.e. no dynamic policy dependency.
 cleanup,err:=wf.New(&wf.Options{Dynamic:false})
 if err!=nil{t.Fatal(err)}
 if err:=cleanup.DeleteRule(ruleID);err!=nil{cleanup.Close();t.Fatal(err)}
 ruleAdded=false
 if err:=cleanup.DeleteSublayer(subID);err!=nil{cleanup.Close();t.Fatal(err)}
 sublayerAdded=false
 if err:=cleanup.Close();err!=nil{t.Fatal(err)}
 t.Log("Persistent TEST-NET scoped filter survived WFP session close/reopen and was removed by GUID; not a production leak guard")
}
