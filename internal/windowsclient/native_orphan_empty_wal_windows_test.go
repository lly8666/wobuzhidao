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
