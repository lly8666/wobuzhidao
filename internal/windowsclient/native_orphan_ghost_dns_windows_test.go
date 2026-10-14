//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "os"
 "strings"
 "testing"
)

func TestN4WindowsGhostDNSPartialRecoveryRequiresOriginalPnPPin(t *testing.T){
 path,id:=n4PreRouteFixture(t,4,"owned")
 g:=&n4FakeGuard{durable:true,active:true}
 if err:=n4RestoreGhostDNSOnly(context.Background(),path,id,g);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("legacy no-PnP WAL granted ghost-DNS cleanup: %v",err)
 }
 if _,err:=os.Stat(path);err!=nil{t.Fatalf("refused legacy WAL deleted: %v",err)}
 id.PnPInstanceSHA256=strings.Repeat("a",64)
 if err:=n4RestoreGhostDNSOnly(context.Background(),path,id,g);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("generation with newly fabricated PnP pin accepted: %v",err)
 }
 if !g.active || g.disarmCount!=0{t.Fatal("fake WFP mutated on invalid WAL")}
}

func TestN4WindowsGhostDNSPartialRecoveryRefusesRouteSidecarAndCancellation(t *testing.T){
 path,id:=n4PreRouteFixture(t,4,"owned")
 j,err:=n4UnifiedRead(path);if err!=nil{t.Fatal(err)}
 id.PnPInstanceSHA256=strings.Repeat("b",64)
 j.Identity=id
 if err:=n4UnifiedWrite(path,j,false);err!=nil{t.Fatal(err)}
 g:=&n4FakeGuard{durable:true,active:true}
 if err:=os.WriteFile(path+".routes",[]byte("unknown native route intent"),0600);err!=nil{t.Fatal(err)}
 if err:=n4RestoreGhostDNSOnly(context.Background(),path,id,g);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("route sidecar bypassed ghost-DNS gate: %v",err)
 }
 if err:=os.Remove(path+".routes");err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=n4RestoreGhostDNSOnly(ctx,path,id,g);!errors.Is(err,context.Canceled){
  t.Fatalf("cancelled native probe proceeded: %v",err)
 }
 if err:=n4RestoreGhostDNSOnly(context.Background(),path,id,g);err==nil ||
  errors.Is(err,ErrN4GhostDNSPartialOnly){
  t.Fatalf("synthetic missing original device authorized DNS clearing: %v",err)
 }
 if _,err:=os.Stat(path);err!=nil{t.Fatal("foreign WAL deleted",err)}
 if !g.active || g.disarmCount!=0{t.Fatal("fake WFP disarmed after negative tests")}
}
