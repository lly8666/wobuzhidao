//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "os"
 "testing"
)

func TestN4WindowsPersistedOriginalPreRouteStrictWALReceipts(t *testing.T){
 for _,n:=range []int{2,3,4}{
  for _,state:=range []string{"owned","intent"}{
   path,id:=n4PreRouteFixture(t,n,state)
   j,err:=n4UnifiedRead(path)
   if err!=nil{t.Fatal(err)}
   if err:=n4ValidatePersistedPreRouteJournal(j,id);err!=nil{
    t.Fatalf("valid original pre-route stage count=%d last=%s refused: %v",n,state,err)
   }
   j.Phase="cleaning"
   if err:=n4ValidatePersistedPreRouteJournal(j,id);err!=nil{
    t.Fatalf("interrupted cleanup must be idempotently valid: %v",err)
   }
   j.Records=j.Records[:1]
   if err:=n4ValidatePersistedPreRouteJournal(j,id);err!=nil{
    t.Fatalf("after restored stages guard-only cleaning receipt: %v",err)
   }
   j.Phase="applying"
   if err:=n4ValidatePersistedPreRouteJournal(j,id);!errors.Is(err,ErrN4JournalConflict){
    t.Fatalf("guard-only applying incorrectly accepted: %v",err)
   }
  }
 }
}

func TestN4WindowsPersistedOriginalPreRouteRefusesUntrustedStages(t *testing.T){
 path,id:=n4PreRouteFixture(t,4,"owned")
 j,err:=n4UnifiedRead(path);if err!=nil{t.Fatal(err)}
 cases:=[]struct{name string;mutate func(*n4UnifiedJournal)}{
  {"foreign-generation",func(q *n4UnifiedJournal){q.Identity.Generation[0]^=1}},
  {"active-phase",func(q *n4UnifiedJournal){q.Phase="active"}},
  {"guard-not-owned",func(q *n4UnifiedJournal){q.Records[0].State="intent"}},
  {"route-intent",func(q *n4UnifiedJournal){
   q.Records=append(q.Records,n4UnifiedRecord{Name:"routes",State:"intent",Before:json.RawMessage(`{}`)})
  }},
  {"wrong-lease-index",func(q *n4UnifiedJournal){
   var b n4LeaseBaseline;_=json.Unmarshal(q.Records[1].Before,&b);b.Index++
   q.Records[1].Before=mustN4Raw(t,b)
  }},
  {"wrong-mtu-target",func(q *n4UnifiedJournal){
   var b n4MTUStageBaseline;_=json.Unmarshal(q.Records[2].Before,&b);b.Target=b.Before.MTU
   q.Records[2].Before=mustN4Raw(t,b)
  }},
  {"dns-foreign",func(q *n4UnifiedJournal){
   var b n4DNSStageBaseline;_=json.Unmarshal(q.Records[3].Before,&b)
   b.Before.RawDomain="foreign.example";q.Records[3].Before=mustN4Raw(t,b)
  }},
 }
 for _,tt:=range cases{
  t.Run(tt.name,func(t *testing.T){
   changed:=j
   changed.Records=append([]n4UnifiedRecord(nil),j.Records...)
   tt.mutate(&changed)
   if err:=n4ValidatePersistedPreRouteJournal(changed,id);!errors.Is(err,ErrN4JournalConflict){
    t.Fatalf("foreign or impossible receipt accepted: %v",err)
   }
  })
 }
 // The absent original is no reason to acquire a similarly named NIC.
 // Native OS read-only identity proof rejects a synthetic absent index,
 // and does not inspect/delete the fake WFP guard or remove the WAL.
 g:=&n4FakeGuard{durable:true,active:true}
 if err:=n4RecoverPersistedPreRouteWAL(context.Background(),path,id,g);err==nil{
  t.Fatal("missing original adapter was falsely authorized for native rollback")
 }
 if g.disarmCount!=0 || !g.active{t.Fatal("missing original reached WFP mutation")}
 if _,err:=os.Stat(path);err!=nil{t.Fatalf("ambiguous journal lost: %v",err)}
}

func TestN4WindowsPersistedOriginalPreRouteRequiresDurableGuard(t *testing.T){
 path,id:=n4PreRouteFixture(t,3,"intent")
 g:=&n4FakeGuard{durable:false,active:true}
 if err:=n4RecoverPersistedPreRouteWAL(context.Background(),path,id,g);!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("transient fake guard accepted: %v",err)
 }
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=n4RecoverPersistedPreRouteWAL(ctx,path,id,g);!errors.Is(err,context.Canceled){
  t.Fatalf("cancelled recovery continued: %v",err)
 }
 if _,err:=os.Stat(path);err!=nil{t.Fatal(err)}
}
