//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "testing"

 "github.com/tailscale/wf"
)
func TestN4WFPGuardRequiresVerifiedExclusiveAdapter(t *testing.T){
 var inst [16]byte;inst[0]=7
 guard:=&n4WindowsPersistentWFPGuard{InstallationID:inst,WintunLUID:1234}
 if guard.Durable(){t.Fatal("multi-tunnel unsafe guard marked durable")}
 if _,err:=guard.Verify(context.Background());!errors.Is(err,ErrN4NativeGuard){t.Fatalf("missing exclusive NIC not refused: %v",err)}
 guard.ExclusiveSingleWintun=true
 if !guard.Durable(){t.Fatal("valid exclusive guard rejected")}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=guard.Arm(ctx);!errors.Is(err,context.Canceled){t.Fatalf("cancel must halt before installing WFP rules: %v",err)}
 if err:=guard.Disarm(ctx);!errors.Is(err,context.Canceled){t.Fatalf("cancel must halt before WFP deletion: %v",err)}
}
func TestN4WFPGuardRefusesAlteredForeignRule(t *testing.T){
 var inst [16]byte;inst[0]=19
 p,err:=n4PlanWFPGuard(inst,72);if err!=nil{t.Fatal(err)}
 r:=*p.Rules[2]
 if !n4WFPRuleEqual(&r,p.Rules[2]) {t.Fatal("exact owner rule not accepted")}
 r.Persistent=false
 if n4WFPRuleEqual(&r,p.Rules[2]) {t.Fatal("transient WFP rule falsely trusted")}
 r=*p.Rules[2];r.Action=wf.ActionPermit
 if n4WFPRuleEqual(&r,p.Rules[2]) {t.Fatal("foreign Permit rule falsely trusted")}
 r=*p.Rules[2];r.Sublayer=wf.SublayerID(n4WFPTestGUID(t))
 if n4WFPRuleEqual(&r,p.Rules[2]) {t.Fatal("wrong sublayer trusted")}
 r=*p.Rules[2];r.Conditions=append([]*wf.Match(nil),r.Conditions...)
 wrong:=*r.Conditions[1];wrong.Value=uint16(5353);r.Conditions[1]=&wrong
 if n4WFPRuleEqual(&r,p.Rules[2]) {t.Fatal("foreign DNS port condition accepted")}
 sl:=*p.Sublayer;sl.Persistent=false
 if n4WFPSublayerEqual(&sl,p.Sublayer){t.Fatal("volatile foreign sublayer trusted")}
}
func TestN4WFPGuardMissingRulesReadOnly(t *testing.T){
 // Just opens a native WFP session and enumerates its existing objects.
 // No broad DNS/IPv6 block rule is ever installed on the Actions runner.
 var inst [16]byte;inst[0]=241;inst[1]=22
 guard:=&n4WindowsPersistentWFPGuard{InstallationID:inst,WintunLUID:0xA11CE,ExclusiveSingleWintun:true}
 ok,err:=guard.Verify(context.Background())
 if err!=nil{t.Fatal(err)}
 if ok{t.Fatal("unknown installation unexpectedly already has production WFP guard")}
}
