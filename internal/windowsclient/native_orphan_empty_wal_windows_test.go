//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "testing"
)

func n4OrphanTestIdentity()N4UnifiedIdentity{
 id:=n4TestConcreteNativeIdentity()
 id.WintunGUID="{11111111-2222-3333-4444-555555555555}"
 id.AdapterAlias="WBDN4-retired"
 return id
}

func TestN4WindowsRetireEmptyOrphanWALRefusesUnboundOrNonemptyJournal(t *testing.T){
 id:=n4OrphanTestIdentity()
 path:=filepath.Join(t.TempDir(),"orphan-wal.json")
 ctx:=context.Background()
 legacy:=id;legacy.WintunGUID=""
 if err:=N4WindowsRetireEmptyOrphanWAL(ctx,path,legacy);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("legacy GUID-less WAL allowed: %v",err)
 }
 if err:=N4WindowsRetireEmptyOrphanWAL(ctx,"",id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("missing journal path accepted: %v",err)
 }
 // A crash with ANY stage intent is not the pre-write empty-WAL case.
 // Refuse without probing or writing any real network interface.
 j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:id,Phase:"applying",Records:[]n4UnifiedRecord{
  {Name:"guard",State:"intent",Before:json.RawMessage(`{"absent":true}`)},
 }}
 if err:=n4UnifiedWrite(path,j,true);err!=nil{t.Fatal(err)}
 if err:=N4WindowsRetireEmptyOrphanWAL(ctx,path,id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("nonempty native journal retired: %v",err)
 }
 if _,err:=os.Stat(path);err!=nil{t.Fatalf("failed recovery destroyed ambiguous WAL: %v",err)}
 wrong:=id;wrong.Generation[0]^=1
 if err:=N4WindowsRetireEmptyOrphanWAL(ctx,path,wrong);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("cross-generation cleanup allowed: %v",err)
 }
 ctx,cancel:=context.WithCancel(ctx);cancel()
 if err:=N4WindowsRetireEmptyOrphanWAL(ctx,path,id);!errors.Is(err,context.Canceled){
  t.Fatalf("cancelled cleanup accessed OS: %v",err)
 }
}

func TestN4WindowsRetireGuardIntentOrphanWALNeverDiscardsGuardOwnedOrSidecar(t *testing.T){
 id:=n4OrphanTestIdentity()
 ctx:=context.Background()
 dir:=t.TempDir()
 path:=filepath.Join(dir,"guard-intent.json")
 mk:=func(state string,before json.RawMessage) n4UnifiedJournal {
  return n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:id,Phase:"applying",Records:[]n4UnifiedRecord{
   {Name:"guard",State:state,Before:before},
  }}
 }
 // A malformed baseline is not equivalent to a proven pre-write absence.
 if err:=n4UnifiedWrite(path,mk("intent",json.RawMessage(`{}`)),true);err!=nil{t.Fatal(err)}
 if err:=N4WindowsRetireGuardIntentOrphanWAL(ctx,path,id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("bad guard before-state accepted: %v",err)
 }
 if err:=os.Remove(path);err!=nil{t.Fatal(err)}
 if err:=n4UnifiedWrite(path,mk("owned",json.RawMessage(`{"absent":true}`)),true);err!=nil{t.Fatal(err)}
 if err:=N4WindowsRetireGuardIntentOrphanWAL(ctx,path,id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("confirmed guard abandoned by orphan cleanup: %v",err)
 }
 if err:=os.Remove(path);err!=nil{t.Fatal(err)}
 if err:=n4UnifiedWrite(path,mk("intent",json.RawMessage(`{"absent":true}`)),true);err!=nil{t.Fatal(err)}
 // The old empty-WAL API must NOT silently widen its contract.
 if err:=N4WindowsRetireEmptyOrphanWAL(ctx,path,id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("nonempty guard intent passed empty-only API: %v",err)
 }
 sidecar:=path+".routes"
 if err:=os.WriteFile(sidecar,[]byte("unresolved native route intent"),0600);err!=nil{t.Fatal(err)}
 if err:=N4WindowsRetireGuardIntentOrphanWAL(ctx,path,id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("route sidecar ignored by pre-write crash recovery: %v",err)
 }
 if _,err:=os.Stat(path);err!=nil{t.Fatalf("WAL lost after refusal: %v",err)}
 if _,err:=os.Stat(sidecar);err!=nil{t.Fatalf("sidecar lost after refusal: %v",err)}
 if err:=os.Remove(sidecar);err!=nil{t.Fatal(err)}
 wrong:=id;wrong.Generation[0]^=1
 if err:=N4WindowsRetireGuardIntentOrphanWAL(ctx,path,wrong);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("foreign generation accepted: %v",err)
 }
 // Canceled calls cannot even start native inspection.
 canceled,cancel:=context.WithCancel(ctx);cancel()
 if err:=N4WindowsRetireGuardIntentOrphanWAL(canceled,path,id);!errors.Is(err,context.Canceled){
  t.Fatalf("cancelled orphan clean was allowed: %v",err)
 }
}
func TestN4WindowsRetireEmptyOrphanWALSidecarCannotBeIgnored(t *testing.T){
 id:=n4OrphanTestIdentity()
 path:=filepath.Join(t.TempDir(),"empty.json")
 if err:=n4UnifiedWrite(path,n4UnifiedJournal{
  Schema:n4UnifiedSchema,Identity:id,Phase:"applying",
 },true);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(path+".routes",[]byte("ambiguous child receipt"),0600);err!=nil{t.Fatal(err)}
 if err:=N4WindowsRetireEmptyOrphanWAL(context.Background(),path,id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("empty parent ignored child route journal: %v",err)
 }
 if _,err:=os.Stat(path);err!=nil{t.Fatalf("empty parent destroyed: %v",err)}
}
