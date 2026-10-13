//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "net/netip"
 "os"
 "path/filepath"
 "testing"
)

func n4TestConcreteRoutes(t *testing.T)(n4WindowsCompactRouteStage,*n4FakeRoute,*n4FakeGuard,N4UnifiedIdentity){
 t.Helper()
 id:=n4TestConcreteNativeIdentity()
 plan,err:=BuildCompactNetworkPlan(Config{
  AdapterAlias:"WBD",TunnelMTU:1249,
  Lease4:netip.MustParsePrefix("10.66.0.7/32"),
  Server4:netip.MustParseAddr("198.51.100.10"),
  Physical:PhysicalPath{InterfaceIndex:12,NextHop4:netip.MustParseAddr("192.0.2.1")},
  DNSServers4:[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")},
  StatePath:"test-owned-network.json",
 })
 if err!=nil{t.Fatal(err)}
 guard:=&n4FakeGuard{durable:true,active:true}
 back:=&n4FakeRoute{existing:map[NativeRouteKey]bool{}}
 stage:=n4WindowsCompactRouteStage{
  Plan:plan,Path:filepath.Join(t.TempDir(),"routes.json"),
  Backend:back,Guard:guard,
 }
 return stage,back,guard,id
}
func TestN4WindowsCompactRouteStageOwnsOnlySmallBatchAndRetainsParentGuard(t *testing.T){
 stage,backend,guard,id:=n4TestConcreteRoutes(t)
 if stage.Name()!="routes"{t.Fatal(stage.Name())}
 b,err:=stage.Snapshot(context.Background(),id);if err!=nil{t.Fatal(err)}
 var rec n4CompactRouteBaseline
 if err:=json.Unmarshal(b,&rec);err!=nil{t.Fatal(err)}
 if len(rec.Keys)!=5 || rec.Keys[0].Prefix!=netip.MustParsePrefix("198.51.100.10/32") ||
  rec.Keys[1].Prefix!=netip.MustParsePrefix("0.0.0.0/1") ||
  rec.Keys[2].Prefix!=netip.MustParsePrefix("128.0.0.0/1"){
  t.Fatalf("unexpected native compact routes %v",rec.Keys)
 }
 if err:=stage.Apply(context.Background(),id,b);err!=nil{t.Fatal(err)}
 if state,err:=stage.Inspect(context.Background(),id,b);err!=nil || state!=N4UnifiedOwned{t.Fatalf("not active: %v %v",state,err)}
 if err:=stage.Undo(context.Background(),id,b);err!=nil{t.Fatal(err)}
 if len(backend.existing)!=0{t.Fatalf("leaked routes: %v",backend.existing)}
 if !guard.active || guard.disarmCount!=0 {t.Fatal("route stage disarmed parent WFP guard before DNS/MTU/lease cleanup")}
 if err:=stage.Undo(context.Background(),id,b);err!=nil{t.Fatalf("second Stop: %v",err)}
}
func TestN4WindowsCompactRouteStagePartialWriteReclaimsOnlyConfirmed(t *testing.T){
 stage,backend,guard,id:=n4TestConcreteRoutes(t)
 b,err:=stage.Snapshot(context.Background(),id);if err!=nil{t.Fatal(err)}
 backend.createErrAt=2
 if err:=stage.Apply(context.Background(),id,b);err==nil{t.Fatal("injected partial route failure missing")}
 if len(backend.existing)!=1{t.Fatalf("expected only first confirmed physical escape: %v",backend.existing)}
 if err:=stage.RecoverUnconfirmedRoutes(context.Background(),id,b);err!=nil{t.Fatal(err)}
 if len(backend.existing)!=0 || !guard.active || guard.disarmCount!=0 {
  t.Fatalf("unsafe partial cleanup routes=%v guard=%+v",backend.existing,guard)
 }
 if _,err:=os.Stat(stage.Path);!errors.Is(err,os.ErrNotExist){t.Fatal(err)}
}
func TestN4WindowsCompactRouteStageAmbiguousIntentNeverDeletesForeign(t *testing.T){
 stage,backend,guard,id:=n4TestConcreteRoutes(t)
 b,err:=stage.Snapshot(context.Background(),id);if err!=nil{t.Fatal(err)}
 backend.createErrAt=2
 if err:=stage.Apply(context.Background(),id,b);err==nil{t.Fatal("missing partial failure")}
 keys,err:=stage.receipt(id,b);if err!=nil{t.Fatal(err)}
 backend.existing[keys[1]]=true // another actor made the unconfirmed intent exact
 if err:=stage.RecoverUnconfirmedRoutes(context.Background(),id,b);!errors.Is(err,ErrN4RouteAmbiguous){
  t.Fatalf("unconfirmed route deleted: %v",err)
 }
 if backend.removed!=0 || !guard.active {t.Fatal("ambiguous route bypassed parent WFP")}
 if _,err:=os.Stat(stage.Path);err!=nil{t.Fatal("lost per-route WAL")}
}
func TestN4WindowsCompactRouteStageUnifiedWALPartialFailureAndStopRestart(t *testing.T){
 stage,backend,guard,id:=n4TestConcreteRoutes(t)
 trace:=&[]string{}
 tx:=&N4NativeUnifiedTxn{
  Path:filepath.Join(t.TempDir(),"all.json"),Identity:id,
  Gate:&n4UnifiedTestGate{},Owner:&n4UnifiedTestOwner{},
 }
 var base [4]*n4UnifiedTestStage
 for i:=0;i<4;i++{
  base[i]=&n4UnifiedTestStage{name:n4UnifiedOrder[i],state:N4UnifiedAbsent,trace:trace}
  tx.Stages[i]=base[i]
 }
 tx.Stages[4]=stage
 backend.createErrAt=2
 if err:=tx.Apply(context.Background());err==nil{t.Fatal("missing partial route error")}
 if err:=tx.Recover(context.Background());err!=nil{t.Fatalf("two WALs failed exact per-route rollback: %v",err)}
 if len(backend.existing)!=0 || !guard.active || guard.disarmCount!=0{t.Fatal("parent guard disarmed by nested route journal")}
 for _,st:=range base{if st.state!=N4UnifiedAbsent{t.Fatal("parent stage leaked")}}
 if _,err:=os.Stat(tx.Path);!errors.Is(err,os.ErrNotExist){t.Fatal(err)}
 // A separate fully-confirmed Apply also survives interrupted Stop.
 backend.createErrAt=0
 if err:=tx.Apply(context.Background());err!=nil{t.Fatal(err)}
 backend.removeErrAt=2
 if err:=tx.Recover(context.Background());err==nil{t.Fatal("cleanup failure not propagated")}
 if !guard.active{t.Fatal("lost guard on partial Stop")}
 backend.removeErrAt=0
 fresh:=&N4NativeUnifiedTxn{Path:tx.Path,Identity:tx.Identity,Gate:tx.Gate,Owner:tx.Owner,Stages:tx.Stages}
 if err:=fresh.Recover(context.Background());err!=nil{t.Fatalf("cannot resume partial Stop: %v",err)}
 if len(backend.existing)!=0 || !guard.active{t.Fatal("unowned route or guard transition")}
}
