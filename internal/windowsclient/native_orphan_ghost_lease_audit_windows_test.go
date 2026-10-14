//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "os"
 "strings"
 "testing"
)

func TestN4GhostLeaseAuditFailClosedBeforeNativeAccess(t *testing.T){
 path,id:=n4PreRouteFixture(t,4,"owned")
 j,err:=n4UnifiedRead(path);if err!=nil{t.Fatal(err)}
 g:=&n4FakeGuard{durable:true,active:true}
 if _,err:=n4AuditPinnedGhostLease(context.Background(),path,id,g);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("unpinned original authorized: %v",err)
 }
 id.PnPInstanceSHA256=strings.Repeat("a",64)
 j.Identity=id;j.Phase="cleaning";j.Records=j.Records[:3]
 if err:=n4UnifiedWrite(path,j,false);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path+".routes",[]byte("unknown route intent"),0600);err!=nil{t.Fatal(err)}
 if _,err:=n4AuditPinnedGhostLease(context.Background(),path,id,g);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("sidecar reached OS audit: %v",err)
 }
 if err:=os.Remove(path+".routes");err!=nil{t.Fatal(err)}
 wrong:=id;wrong.Generation[0]^=1
 if _,err:=n4AuditPinnedGhostLease(context.Background(),path,wrong,g);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("foreign generation accepted: %v",err)
 }
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=n4AuditPinnedGhostLease(ctx,path,id,g);!errors.Is(err,context.Canceled){
  t.Fatalf("cancelled audit continued: %v",err)
 }
 if _,err:=n4AuditPinnedGhostLease(context.Background(),path,id,g);err==nil{
  t.Fatal("synthetic original PnP pin without OS evidence passed")
 }
 after,err:=n4UnifiedRead(path)
 if err!=nil || len(after.Records)!=3 || after.Phase!="cleaning"{
  t.Fatalf("audit modified WAL %+v: %v",after,err)
 }
 if !g.active || g.disarmCount!=0{
  t.Fatal("read-only audit disarmed fake persistent guard")
 }
}

func TestN4GhostLeaseAuditNeverTreatsUnconfirmedStageAsRestored(t *testing.T){
 path,id:=n4PreRouteFixture(t,4,"owned")
 j,err:=n4UnifiedRead(path);if err!=nil{t.Fatal(err)}
 id.PnPInstanceSHA256=strings.Repeat("b",64)
 j.Identity=id;j.Phase="cleaning";j.Records=j.Records[:3]
 j.Records[2].State="intent"
 if err:=n4UnifiedWrite(path,j,false);err!=nil{t.Fatal(err)}
 g:=&n4FakeGuard{durable:true,active:true}
 if _,err:=n4AuditPinnedGhostLease(context.Background(),path,id,g);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("unconfirmed MTU write treated as original baseline: %v",err)
 }
 if _,err:=os.Stat(path);err!=nil{t.Fatalf("unfinished WAL disappeared: %v",err)}
 if g.disarmCount!=0{t.Fatal("unconfirmed stage disarmed WFP")}
}
