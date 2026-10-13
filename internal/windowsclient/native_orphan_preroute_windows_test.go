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

 "golang.org/x/sys/windows"
)

func n4PreRouteFixture(t *testing.T,n int,finalState string)(string,N4UnifiedIdentity){
 t.Helper()
 id:=n4GuardOwnedOrphanID()
 guid,err:=windows.GUIDFromString(id.WintunGUID)
 if err!=nil{t.Fatal(err)}
 records:=[]n4UnifiedRecord{
  {Name:"guard",State:"owned",Before:json.RawMessage(`{"absent":true}`)},
  {Name:"lease",State:"owned",Before:mustN4Raw(t,n4LeaseBaseline{
   Index:id.WintunIndex,Address:"10.234.12.34/32",Absent:true,
  })},
  {Name:"mtu",State:"owned",Before:mustN4Raw(t,n4MTUStageBaseline{
   Before:N4InterfaceIPv4Snapshot{Index:id.WintunIndex,MTU:1500},
   Target:1400,
  })},
  {Name:"dns",State:"owned",Before:mustN4Raw(t,n4DNSStageBaseline{
   Index:id.WintunIndex,Before:N4DNSBaseline{Interface:guid},
   Servers:[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")},
  })},
 }
 records=records[:n]
 records[len(records)-1].State=finalState
 path:=filepath.Join(t.TempDir(),"preroute-wal.json")
 j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:id,Phase:"applying",Records:records}
 if err:=n4UnifiedWrite(path,j,true);err!=nil{t.Fatal(err)}
 return path,id
}
func mustN4Raw(t *testing.T,v any)json.RawMessage{
 t.Helper()
 b,err:=json.Marshal(v)
 if err!=nil{t.Fatal(err)}
 return b
}
func TestN4WindowsPreRouteOrphanRetireScopedFakeWFP(t *testing.T){
 for _,n:=range []int{2,3,4}{
  for _,lastState:=range []string{"owned","intent"}{
   for _,state:=range []N4UnifiedState{N4UnifiedOwned,N4UnifiedPartial,N4UnifiedAbsent}{
    t.Run(string(rune('0'+n))+"-"+lastState+"-"+string(state),func(t *testing.T){
     path,id:=n4PreRouteFixture(t,n,lastState)
     g:=&n4OwnedOrphanFakeGuard{state:state}
     err:=n4RetirePreRouteOrphanWAL(context.Background(),path,id,g)
     if err!=nil{t.Fatalf("verified dead-index OS probe and fake-WFP cleanup: %v",err)}
     expect:=1
     if state==N4UnifiedAbsent{expect=0}
     if g.disarmCalls!=expect || g.inspectCalls<2{
      t.Fatalf("no post-owned disarm/readback gate: %+v",g)
     }
     if _,err:=os.Stat(path);!errors.Is(err,os.ErrNotExist){
      t.Fatalf("WAL survived confirmed cleanup: %v",err)
     }
     if err:=n4RetirePreRouteOrphanWAL(context.Background(),path,id,g);err!=nil{
      t.Fatalf("second Stop not idempotent: %v",err)
     }
    })
   }
  }
 }
}
func TestN4WindowsPreRouteOrphanRefusesRouteSidecarOrForeignReceipt(t *testing.T){
 for _,test:=range []struct{name string;modify func(*n4UnifiedJournal)}{
  {"cleaning-phase",func(j *n4UnifiedJournal){j.Phase="cleaning"}},
  {"wrong-lease-index",func(j *n4UnifiedJournal){
   var v n4LeaseBaseline
   _=json.Unmarshal(j.Records[1].Before,&v)
   v.Index++
   b,_:=json.Marshal(v);j.Records[1].Before=b
  }},
  {"foreign-mtu-index",func(j *n4UnifiedJournal){
   var v n4MTUStageBaseline
   _=json.Unmarshal(j.Records[2].Before,&v)
   v.Before.Index++
   b,_:=json.Marshal(v);j.Records[2].Before=b
  }},
  {"foreign-dns-guid",func(j *n4UnifiedJournal){
   var v n4DNSStageBaseline
   _=json.Unmarshal(j.Records[3].Before,&v)
   v.Before.Interface.Data1++
   b,_:=json.Marshal(v);j.Records[3].Before=b
  }},
  {"corrupt-dns-server",func(j *n4UnifiedJournal){
   var v n4DNSStageBaseline
   _=json.Unmarshal(j.Records[3].Before,&v)
   v.Servers=[]netip.Addr{netip.MustParseAddr("::1")}
   b,_:=json.Marshal(v);j.Records[3].Before=b
  }},
  {"unexpected-guard-intent",func(j *n4UnifiedJournal){j.Records[0].State="intent"}},
  {"unexpected-route-stage",func(j *n4UnifiedJournal){
   j.Records=append(j.Records,n4UnifiedRecord{Name:"routes",State:"intent",Before:json.RawMessage(`{}`)})
  }},
 }{
  t.Run(test.name,func(t *testing.T){
   path,id:=n4PreRouteFixture(t,4,"owned")
   j,err:=n4UnifiedRead(path);if err!=nil{t.Fatal(err)}
   test.modify(&j)
   if err:=n4UnifiedWrite(path,j,false);err!=nil{
    // Impossible journal state is itself a hard reject before OS access.
    if !errors.Is(err,ErrN4JournalConflict){t.Fatal(err)}
    return
   }
   g:=&n4OwnedOrphanFakeGuard{state:N4UnifiedOwned}
   if err:=n4RetirePreRouteOrphanWAL(context.Background(),path,id,g);!errors.Is(err,ErrN4JournalConflict){
    t.Fatalf("ambiguous modified WAL retired: %v",err)
   }
   if g.inspectCalls!=0 || g.disarmCalls!=0{t.Fatal("untrusted WAL reached WFP backend")}
   if _,err:=os.Stat(path);err!=nil{t.Fatal("WAL not retained",err)}
  })
 }
 path,id:=n4PreRouteFixture(t,4,"intent")
 guard:=&n4OwnedOrphanFakeGuard{state:N4UnifiedOwned}
 if err:=os.WriteFile(path+".routes",[]byte("untrusted route sidecar"),0600);err!=nil{t.Fatal(err)}
 if err:=n4RetirePreRouteOrphanWAL(context.Background(),path,id,guard);!errors.Is(err,ErrN4JournalConflict){t.Fatalf("route sidecar ignored: %v",err)}
 if guard.inspectCalls!=0{t.Fatal("sidecar reached WFP backend")}
 if err:=os.Remove(path+".routes");err!=nil{t.Fatal(err)}
 wrong:=id;wrong.Generation[0]^=1
 if err:=n4RetirePreRouteOrphanWAL(context.Background(),path,wrong,guard);!errors.Is(err,ErrN4JournalConflict){t.Fatalf("foreign generation retired: %v",err)}
 cancelled,cancel:=context.WithCancel(context.Background());cancel()
 if err:=n4RetirePreRouteOrphanWAL(cancelled,path,id,guard);!errors.Is(err,context.Canceled){t.Fatalf("cancelled recovery continued: %v",err)}
 if _,err:=os.Stat(path);err!=nil{t.Fatal("refused WAL disappeared",err)}
}
func TestN4WindowsPreRouteOrphanWFPFailureRetainsJournal(t *testing.T){
 for _,g:=range []*n4OwnedOrphanFakeGuard{
  {state:N4UnifiedForeign},
  {state:N4UnifiedOwned,disarmErr:ErrN4NativeGuard},
  {state:N4UnifiedPartial,inspectErr:ErrN4ForeignRoute},
 }{
  path,id:=n4PreRouteFixture(t,4,"owned")
  if err:=n4RetirePreRouteOrphanWAL(context.Background(),path,id,g);err==nil{t.Fatal("failed/foreign guard retired WAL")}
  if _,err:=os.Stat(path);err!=nil{t.Fatal("failed guard lost WAL",err)}
 }
}
