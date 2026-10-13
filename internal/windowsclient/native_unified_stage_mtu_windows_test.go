//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "testing"
)

func n4StageMTUFixture()(n4WindowsMTUStage,N4UnifiedIdentity){
 id:=n4TestConcreteNativeIdentity()
 return n4WindowsMTUStage{Target:1249,Writer:n4WindowsMTUWriter{
  Guard:&n4FakeGuard{durable:true,active:true},
  Adapter:&n4LeaseFakeOwner{owned:false},
 }},id
}
func TestN4WindowsMTUStageRefusesBorrowedAndStaleOwners(t *testing.T){
 stage,id:=n4StageMTUFixture()
 if stage.Name()!="mtu"{t.Fatal(stage.Name())}
 if _,err:=stage.Snapshot(context.Background(),id);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("borrowed adapter queried native MTU: %v",err)
 }
 owner:=stage.Writer.Adapter.(*n4LeaseFakeOwner)
 if owner.calls!=1{t.Fatalf("no current-owner gate: %d",owner.calls)}
 stage.Writer.Adapter=nil
 if _,err:=stage.Snapshot(context.Background(),id);!errors.Is(err,ErrN4NativeGuard){t.Fatalf("missing owner: %v",err)}
 if owner.calls!=1{t.Fatal("owner queried after missing gate")}
}
func TestN4WindowsMTUStageJournalMustMatchFullBaselineAndTarget(t *testing.T){
 stage,id:=n4StageMTUFixture()
 original:=N4InterfaceIPv4Snapshot{Index:id.WintunIndex,MTU:1500,Metric:25,Connected:true}
 b,err:=json.Marshal(n4MTUStageBaseline{Before:original,Target:1249})
 if err!=nil{t.Fatal(err)}
 if _,err:=stage.decode(id,b);err!=nil{t.Fatal(err)}
 for _,raw:=range []json.RawMessage{
  json.RawMessage(`{"before":{"Index":22,"MTU":1500},"target":1249}`),
  json.RawMessage(`{"before":{"Index":21,"MTU":1500},"target":1400}`),
  json.RawMessage(`{"before":{"Index":21,"MTU":1249},"target":1249}`),
  json.RawMessage(`bad`),
 } {
  if err:=stage.Apply(context.Background(),id,raw);!errors.Is(err,ErrN4JournalConflict){
   t.Fatalf("tampered journal reached SetIpInterfaceEntry: %s %v",raw,err)
  }
 }
 if owner:=stage.Writer.Adapter.(*n4LeaseFakeOwner);owner.calls!=0{t.Fatal("untrusted receipt reached OS owner proof")}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=stage.Apply(ctx,id,b);!errors.Is(err,context.Canceled){t.Fatalf("canceled native MTU: %v",err)}
}
