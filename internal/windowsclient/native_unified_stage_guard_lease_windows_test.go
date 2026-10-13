//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "net/netip"
 "testing"

 "github.com/tailscale/wf"
)

func n4TestConcreteNativeIdentity() N4UnifiedIdentity{
 var id N4UnifiedIdentity
 id.Installation[0]=219
 id.Generation[0]=102
 id.WintunLUID=0x98765
 id.WintunIndex=21
 return id
}
func TestN4WindowsGuardStageObservedPartialNeverConfusedWithAbsent(t *testing.T){
 id:=n4TestConcreteNativeIdentity()
 p,err:=n4PlanWFPGuard(id.Installation,id.WintunLUID)
 if err!=nil{t.Fatal(err)}
 if s:=n4WFPClassifyOwned(false,nil,p);s!=N4UnifiedAbsent{t.Fatal(s)}
 if s:=n4WFPClassifyOwned(false,map[wf.RuleID]bool{p.Rules[0].ID:true},p);s!=N4UnifiedPartial{t.Fatal(s)}
 if s:=n4WFPClassifyOwned(true,nil,p);s!=N4UnifiedPartial{t.Fatal(s)}
 if s:=n4WFPClassifyOwned(true,map[wf.RuleID]bool{p.Rules[0].ID:true},p);s!=N4UnifiedPartial{t.Fatal(s)}
 full:=map[wf.RuleID]bool{}
 for _,v:=range p.Rules{full[v.ID]=true}
 if s:=n4WFPClassifyOwned(true,full,p);s!=N4UnifiedOwned{t.Fatal(s)}
}
func TestN4WindowsGuardStageForeignIdentityRefusedBeforeNativeWFP(t *testing.T){
 id:=n4TestConcreteNativeIdentity()
 g:=&n4WindowsPersistentWFPGuard{InstallationID:id.Installation,WintunLUID:id.WintunLUID,ExclusiveSingleWintun:true}
 stage:=n4WindowsGuardStage{Guard:g}
 if stage.Name()!="guard"{t.Fatal("wrong unified sequence")}
 wrong:=id;wrong.Installation[0]++
 if _,err:=stage.Snapshot(context.Background(),wrong);!errors.Is(err,ErrN4NativeGuard){t.Fatalf("foreign install %v",err)}
 wrong=id;wrong.WintunLUID++
 if err:=stage.Apply(context.Background(),wrong,json.RawMessage(`{"absent":true}`));!errors.Is(err,ErrN4NativeGuard){t.Fatalf("foreign LUID %v",err)}
 if err:=stage.Undo(context.Background(),id,json.RawMessage(`{"absent":false}`));!errors.Is(err,ErrN4JournalConflict){t.Fatalf("altered WAL %v",err)}
 g.ExclusiveSingleWintun=false
 if _,err:=stage.Inspect(context.Background(),id,json.RawMessage(`{"absent":true}`));!errors.Is(err,ErrN4NativeGuard){t.Fatalf("non-exclusive broad guard accepted %v",err)}
}
func TestN4WindowsGuardStageReadOnlyHostWithoutInstallingPolicy(t *testing.T){
 id:=n4TestConcreteNativeIdentity()
 stage:=n4WindowsGuardStage{Guard:&n4WindowsPersistentWFPGuard{
  InstallationID:id.Installation,WintunLUID:id.WintunLUID,ExclusiveSingleWintun:true,
 }}
 // Session enumeration only. No persistent sublayer/rules are created.
 b,err:=stage.Snapshot(context.Background(),id)
 if err!=nil{t.Fatalf("native WFP read-only stage snapshot: %v",err)}
 if string(b)!=`{"absent":true}`{t.Fatalf("bad receipt %q",b)}
 state,err:=stage.Inspect(context.Background(),id,b)
 if err!=nil || state!=N4UnifiedAbsent{t.Fatalf("unexpected WFP state %s %v",state,err)}
}
func TestN4WindowsLeaseStageProofAndReceiptPrecedeAnyWindowsMutation(t *testing.T){
 id:=n4TestConcreteNativeIdentity()
 l:=N4NativeLeaseIntent{InterfaceIndex:id.WintunIndex,Prefix:netip.MustParsePrefix("10.66.0.7/32")}
 guard:=&n4FakeGuard{durable:true,active:true}
 owner:=&n4LeaseFakeOwner{}
 stage:=n4WindowsLeaseStage{Lease:l,Writer:N4WindowsLeaseWriter{Guard:guard,Adapter:owner}}
 if stage.Name()!="lease"{t.Fatal("wrong unified sequence")}
 if _,err:=stage.Snapshot(context.Background(),id);!errors.Is(err,ErrN4ForeignRoute){t.Fatalf("borrowed adapter read native state: %v",err)}
 if owner.calls!=1{t.Fatal("missing owner proof")}
 wrong:=id;wrong.WintunIndex++
 if _,err:=stage.Snapshot(context.Background(),wrong);!errors.Is(err,ErrN4ForeignRoute){t.Fatalf("wrong index queried: %v",err)}
 if owner.calls!=1{t.Fatal("wrong index must refuse before owner query")}
 valid:=json.RawMessage(`{"index":21,"address":"10.66.0.7/32","absent":true}`)
 if err:=stage.receipt(valid);err!=nil{t.Fatal(err)}
 for _,unsafeReceipt:=range []json.RawMessage{
  json.RawMessage(`{"index":22,"address":"10.66.0.7/32","absent":true}`),
  json.RawMessage(`{"index":21,"address":"10.66.0.0/24","absent":true}`),
  json.RawMessage(`{"index":21,"address":"10.66.0.7/32","absent":false}`),
  json.RawMessage(`{broken`),
 }{
  if err:=stage.Apply(context.Background(),id,unsafeReceipt);!errors.Is(err,ErrN4JournalConflict){
   t.Fatalf("bad WAL receipt reached writer: %s err=%v",unsafeReceipt,err)
  }
 }
 if owner.calls!=1{t.Fatal("unsafe receipt reached adapter or Win32")}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=stage.Apply(ctx,id,valid);!errors.Is(err,context.Canceled){t.Fatalf("canceled apply %v",err)}
 if owner.calls!=1{t.Fatal("canceled apply reached adapter")}
}
func TestN4WindowsLeaseStageRejectsMissingDurableGuardAndInvalidCIDR(t *testing.T){
 id:=n4TestConcreteNativeIdentity()
 l,err:=n4StageLeaseIntent(id.WintunIndex,netip.MustParsePrefix("10.66.0.7/32"))
 if err!=nil || l.InterfaceIndex!=id.WintunIndex{t.Fatalf("valid lease: %+v %v",l,err)}
 for _,p:=range []netip.Prefix{
  netip.MustParsePrefix("10.66.0.0/24"),
  netip.MustParsePrefix("::1/128"),
  netip.MustParsePrefix("127.0.0.1/32"),
 }{
  if _,err:=n4StageLeaseIntent(id.WintunIndex,p);!errors.Is(err,ErrNetworkPlan){t.Fatalf("invalid CIDR accepted %s",p)}
 }
 owner:=&n4LeaseFakeOwner{owned:true}
 stage:=n4WindowsLeaseStage{Lease:l,Writer:N4WindowsLeaseWriter{Adapter:owner}}
 if _,err:=stage.Snapshot(context.Background(),id);!errors.Is(err,ErrN4NativeGuard){t.Fatalf("missing persistent WFP guard accepted: %v",err)}
 if owner.calls!=0{t.Fatal("missing guard reached adapter")}
}
