//go:build windows

package windowsclient

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "testing"
 "time"
)

// Real native Win2022/25 adapter and READ-ONLY journal-bound identity probe.
// NO permission to mutate OS networking, to recover a crash, or adopt any
// foreign adapter. The journal stores only a random test identity and OS NIC
// metadata; it is not a credential/secret.
func TestN4WindowsHostedGUIDBoundJournalReadOnlyProof(t *testing.T){
 if os.Getenv("WBD_N4_ADMIN_GUID_JOURNAL")!="1" || os.Getenv("GITHUB_ACTIONS")!="true" {
  t.Skip("NOT_RUN: only disposable GitHub Windows Wintun GUID/WAL proof")
 }
 report:=map[string]string{
  "schema":"wbd-n4-real-journal-guid-readonly/v1",
  "source_sha":os.Getenv("GITHUB_SHA"),"runner":os.Getenv("RUNNER_OS"),
  "result":"FAIL_UNFINISHED","native_wintun":"NOT_RUN","guid_journal":"NOT_RUN",
  "legacy":"NOT_RUN","foreign":"NOT_RUN","corruption":"NOT_RUN",
  "cross_process_crash":"NOT_RUN","wfp":"NOT_RUN","routes":"NOT_RUN",
  "physical":"NOT_RUN",
 }
 defer func(){
  file:=os.Getenv("WBD_N4_ADMIN_GUID_JOURNAL_REPORT")
  if file==""{t.Error("GUID journal report path missing");return}
  raw,err:=json.MarshalIndent(report,"","  ");if err!=nil{t.Error(err);return}
  if err:=os.MkdirAll(filepath.Dir(file),0700);err!=nil{t.Error(err);return}
  if err:=os.WriteFile(file,append(raw,'\n'),0600);err!=nil{t.Error(err)}
 }()
 if report["source_sha"]=="" || report["runner"]!="Windows"{t.Fatal("exact SOURCE Windows Actions mandatory")}
 var suffix [8]byte
 if _,err:=rand.Read(suffix[:]);err!=nil{t.Fatal(err)}
 name:="WBDN4-JRN-"+hex.EncodeToString(suffix[:])
 tun,err:=OpenTUN(name)
 if err!=nil{
  report["result"]="UNSUPPORTED_DRIVER_OR_ADMIN"
  t.Skipf("UNSUPPORTED created Wintun: %v",err)
 }
 defer func(){if err:=tun.Close();err!=nil{t.Errorf("close isolated Wintun: %v",err)}}()
 if !tun.createdByProcess{t.Fatal("borrowed adapter unexpectedly created")}
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 native,err:=tun.NativeIdentity(ctx)
 if err!=nil || !native.CreatedByProcess || native.Index==0 || native.LUID==0{
  t.Fatalf("actual Windows Wintun identity: %+v %v",native,err)
 }
 osGUID,err:=n4NativeInterfaceGUID(native.Index)
 if err!=nil{t.Fatal(err)}
 report["native_wintun"]="PASS_LIVE_CREATED_LUID_INDEX_OS_GUID"
 var inst,gen [16]byte
 if _,err:=rand.Read(inst[:]);err!=nil{t.Fatal(err)}
 if _,err:=rand.Read(gen[:]);err!=nil{t.Fatal(err)}
 id:=N4UnifiedIdentity{
  Installation:inst,Generation:gen,WintunIndex:native.Index,
  WintunLUID:native.LUID,WintunGUID:osGUID.String(),AdapterAlias:name,
 }
 wal:=filepath.Join(t.TempDir(),"read-only-owner.json")
 j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:id,Phase:"applying"}
 if err:=n4UnifiedWrite(wal,j,true);err!=nil{t.Fatal(err)}
 if err:=n4VerifyJournalBoundLiveWintun(ctx,tun,wal,id);err!=nil{t.Fatalf("real journal+NIC identity: %v",err)}
 report["guid_journal"]="PASS_READONLY_LIVE_WAL_AND_OS_GUID"
 legacy:=id;legacy.WintunGUID=""
 legacyPath:=filepath.Join(t.TempDir(),"legacy.json")
 if err:=n4UnifiedWrite(legacyPath,n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:legacy,Phase:"applying"},true);err!=nil{t.Fatal(err)}
 if err:=n4VerifyJournalBoundLiveWintun(ctx,tun,legacyPath,legacy);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("legacy WAL with no GUID promoted to recovered owner: %v",err)
 }
 report["legacy"]="PASS_FAIL_CLOSED"
 foreign:=id;foreign.WintunGUID=func()string{
  copy:=osGUID;copy.Data1^=1;return copy.String()
 }()
 badPath:=filepath.Join(t.TempDir(),"foreign.json")
 if err:=n4UnifiedWrite(badPath,n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:foreign,Phase:"applying"},true);err!=nil{t.Fatal(err)}
 if err:=n4VerifyJournalBoundLiveWintun(ctx,tun,badPath,foreign);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("valid journal with different Windows interface GUID accepted: %v",err)
 }
 altered:=id;altered.Generation[0]^=1
 if err:=n4VerifyJournalBoundLiveWintun(ctx,tun,wal,altered);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("wrong generation accepted: %v",err)
 }
 report["foreign"]="PASS_GUID_AND_GENERATION_MISMATCH_REFUSED"
 raw,err:=os.ReadFile(wal);if err!=nil{t.Fatal(err)}
 // Keep valid JSON but break the SHA-256 envelope. A tampered WAL never
 // authorizes a foreign adapter; here no OS mutation follows the check.
 raw=append(raw,byte(' '))
 if err:=os.WriteFile(wal,raw,0600);err!=nil{t.Fatal(err)}
 // Trailing JSON whitespace does NOT invalidate checksum; alter a
 // significant field by writing a *new* valid envelope then one byte.
 // Never mutate the actual adapter while probing.
 raw,err=os.ReadFile(wal);if err!=nil{t.Fatal(err)}
 for i:=range raw {
  if raw[i]=='a' {raw[i]='b';break}
 }
 if err:=os.WriteFile(wal,raw,0600);err!=nil{t.Fatal(err)}
 if err:=n4VerifyJournalBoundLiveWintun(ctx,tun,wal,id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("tampered WAL accepted: %v",err)
 }
 report["corruption"]="PASS_CHECKSUM_OR_PARSE_REFUSED"
 report["result"]="SCOPED_REAL_JOURNAL_GUID_READ_ONLY_PROOF_PASS"
 t.Log("real Wintun GUID+generation WAL read-only proof PASS, no cross-process orphan recovery or OS network mutations")
}
