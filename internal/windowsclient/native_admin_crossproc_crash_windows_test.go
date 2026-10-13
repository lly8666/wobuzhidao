//go:build windows

package windowsclient

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "testing"
 "time"
)

// Only an ephemeral hosted Windows runner runs the real child-process kill.
// This proves no-create read-only OS identity, NOT native crash recovery.
func n4HostedCrashChild(t *testing.T){
 alias,wal,ready:=os.Getenv("WBD_N4_CRASH_ALIAS"),os.Getenv("WBD_N4_CRASH_WAL"),os.Getenv("WBD_N4_CRASH_READY")
 if alias==""||wal==""||ready==""{t.Fatal("child state missing")}
 tun,err:=OpenTUN(alias);if err!=nil{t.Fatal(err)}
 if !tun.createdByProcess{t.Fatal("borrowed child adapter")}
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second);defer cancel()
 live,err:=tun.NativeIdentity(ctx);if err!=nil{t.Fatal(err)}
 guid,err:=n4NativeInterfaceGUID(live.Index);if err!=nil{t.Fatal(err)}
 var ins,gen [16]byte
 if _,err:=rand.Read(ins[:]);err!=nil{t.Fatal(err)}
 if _,err:=rand.Read(gen[:]);err!=nil{t.Fatal(err)}
 id:=N4UnifiedIdentity{Installation:ins,Generation:gen,WintunIndex:live.Index,WintunLUID:live.LUID,WintunGUID:guid.String(),AdapterAlias:alias}
 if err:=n4UnifiedWrite(wal,n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:id,Phase:"applying"},true);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(ready,[]byte("WAL_SYNCED\n"),0600);err!=nil{t.Fatal(err)}
 // No Wintun.Close: parent intentionally kills the process.
 for{time.Sleep(time.Second)}
}

func TestN4WindowsHostedCrossProcessCrashReadOnlyIdentity(t *testing.T){
 if os.Getenv("WBD_N4_CRASH_CHILD")=="1"{n4HostedCrashChild(t);return}
 if os.Getenv("WBD_N4_ADMIN_CRASH_PROBE")!="1"||os.Getenv("GITHUB_ACTIONS")!="true"{
  t.Skip("NOT_RUN: only disposable hosted Windows runner")
 }
 report:=map[string]string{
  "schema":"wbd-n4-crash-readonly/v1","source_sha":os.Getenv("GITHUB_SHA"),"runner":os.Getenv("RUNNER_OS"),
  "result":"FAIL_UNFINISHED","child_crash":"NOT_RUN","no_create_reopen":"NOT_RUN","journal_guid":"NOT_RUN",
  "borrowed_cannot_mutate":"NOT_RUN","real_apply_recover":"NOT_RUN","wfp":"NOT_RUN",
  "routes":"NOT_RUN","dns":"NOT_RUN","physical":"NOT_RUN",
 }
 defer func(){
  path:=os.Getenv("WBD_N4_ADMIN_CRASH_PROBE_REPORT")
  if path==""{t.Error("missing receipt path");return}
  b,err:=json.MarshalIndent(report,"","  ");if err!=nil{t.Error(err);return}
  if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{t.Error(err);return}
  if err:=os.WriteFile(path,append(b,'\n'),0600);err!=nil{t.Error(err)}
 }()
 if report["source_sha"]==""||report["runner"]!="Windows"{t.Fatal("exact CI SOURCE/runner mandatory")}
 var salt [8]byte
 if _,err:=rand.Read(salt[:]);err!=nil{t.Fatal(err)}
 alias:="WBDN4-CRH-"+hex.EncodeToString(salt[:])
 dir:=t.TempDir()
 wal,ready:=filepath.Join(dir,"owner.json"),filepath.Join(dir,"ready")
 child:=exec.Command(os.Args[0],"-test.run=^TestN4WindowsHostedCrossProcessCrashReadOnlyIdentity$","-test.timeout=3m")
 child.Env=append(os.Environ(),"WBD_N4_CRASH_CHILD=1","WBD_N4_CRASH_ALIAS="+alias,
  "WBD_N4_CRASH_WAL="+wal,"WBD_N4_CRASH_READY="+ready)
 child.Stdout,child.Stderr=io.Discard,io.Discard
 if err:=child.Start();err!=nil{t.Fatal(err)}
 done:=false
 defer func(){if !done{_ =child.Process.Kill();_ =child.Wait()}}()
 deadline:=time.Now().Add(25*time.Second)
 for{
  if b,err:=os.ReadFile(ready);err==nil{
   if string(b)!="WAL_SYNCED\n"{t.Fatal("malformed readiness")}
   break
  }
  if time.Now().After(deadline){t.Fatal("real child failed to commit WAL")}
  time.Sleep(100*time.Millisecond)
 }
 if err:=child.Process.Kill();err!=nil{t.Fatal(err)}
 err:=child.Wait();done=true
 if err==nil{t.Fatal("expected abrupt child termination")}
 report["child_crash"]="PASS_REAL_CHILD_KILLED_AFTER_WAL_SYNC"
 var tunnel *TUN
 limit:=time.Now().Add(5*time.Second)
 for{
  tunnel,err=OpenExistingTUN(alias)
  if err==nil{break}
  if time.Now().After(limit){
   report["result"]="UNSUPPORTED_ORPHAN_ADAPTER_NOT_PERSISTED"
   t.Skipf("UNSUPPORTED: crash removed Windows interface, no replacement created: %v",err)
  }
  time.Sleep(100*time.Millisecond)
 }
 defer func(){if err:=tunnel.Close();err!=nil{t.Error(err)}}()
 if tunnel.createdByProcess{t.Fatal("borrowed orphan claimed newly created ownership")}
 report["no_create_reopen"]="PASS_EXISTING_ONLY_BORROWED"
 j,err:=n4UnifiedRead(wal);if err!=nil{t.Fatal(err)}
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second);defer cancel()
 if err:=n4VerifyJournalBoundLiveWintun(ctx,tunnel,wal,j.Identity);err!=nil{
  t.Fatalf("after real child crash WAL identity mismatched: %v",err)
 }
 report["journal_guid"]="PASS_EXACT_OS_GUID_GENERATION_LUID_INDEX_ALIAS"
 owner:=N4WindowsCreatedWintunReservation{
  Tunnel:tunnel,Installation:j.Identity.Installation,Generation:j.Identity.Generation,
 }
 if err:=owner.Verify(ctx,j.Identity);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("borrowed Wintun improperly gained native write permission: %v",err)
 }
 report["borrowed_cannot_mutate"]="PASS_ORIGINAL_OWNER_REFUSES_BORROWED"
 report["result"]="SCOPED_CROSSPROCESS_CRASH_READONLY_IDENTITY_PASS"
 t.Log("real TerminateProcess/read-only recovered adapter identity; no WFP/route/OS restore")
}

func TestN4WindowsOpenExistingRequiresExplicitAlias(t *testing.T){
 for _,name:=range []string{"","WBD\rforeign","WBD\nforeign"}{
  if _,err:=OpenExistingTUN(name);!errors.Is(err,ErrInvalidBinding){
   t.Fatalf("unsafe alias %q not refused before Win32: %v",name,err)
  }
 }
}
