//go:build windows

package windowsclient

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "io"
 "net/netip"
 "os"
 "os/exec"
 "path/filepath"
 "testing"
 "time"
)

func n4HostedPreRouteRealChild(t *testing.T){
 alias,wal,ready:=os.Getenv("WBD_N4_PREROUTE_ALIAS"),os.Getenv("WBD_N4_PREROUTE_WAL"),os.Getenv("WBD_N4_PREROUTE_READY")
 if alias=="" || wal=="" || ready==""{t.Fatal("missing exact child inputs")}
 tun,err:=OpenTUN(alias)
 if err!=nil{t.Fatal(err)}
 // Child is intentionally TerminateProcess'ed: no deferred Close and no
 // automatic rollback. The OS must destroy the original owned Wintun.
 ctx,cancel:=context.WithTimeout(context.Background(),40*time.Second)
 defer cancel()
 native,err:=tun.NativeIdentity(ctx)
 if err!=nil || !native.CreatedByProcess || native.LUID==0 || native.Index==0{t.Fatalf("invalid real Wintun identity %+v: %v",native,err)}
 guid,err:=n4NativeInterfaceGUID(native.Index)
 if err!=nil{t.Fatal(err)}
 id:=N4UnifiedIdentity{WintunGUID:guid.String(),WintunLUID:native.LUID,WintunIndex:native.Index,AdapterAlias:alias}
 if _,err:=rand.Read(id.Installation[:]);err!=nil{t.Fatal(err)}
 if _,err:=rand.Read(id.Generation[:]);err!=nil{t.Fatal(err)}
 if !id.valid(){t.Fatal("invalid installation/generation")}
 // This LAB ONLY uses a fake WFP guard. It validates the real native
 // IP/MTU/DNS side effects, not any production host-global leak defense.
 g:=&n4FakeGuard{durable:true,active:true}
 j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:id,Phase:"applying",
  Records:[]n4UnifiedRecord{{Name:"guard",State:"owned",Before:json.RawMessage(`{"absent":true}`)}}}
 if err:=n4UnifiedWrite(wal,j,true);err!=nil{t.Fatal(err)}
 mark:=func(name string,before json.RawMessage,write func()error){
  j.Records=append(j.Records,n4UnifiedRecord{Name:name,State:"intent",Before:before})
  if err:=n4UnifiedWrite(wal,j,false);err!=nil{t.Fatal(err)}
  if err:=write();err!=nil{t.Fatalf("native %s write failed: %v",name,err)}
  j.Records[len(j.Records)-1].State="owned"
  if err:=n4UnifiedWrite(wal,j,false);err!=nil{t.Fatal(err)}
 }
 lease:=N4NativeLeaseIntent{InterfaceIndex:native.Index,Prefix:netip.MustParsePrefix("198.18.18.77/32")}
 rows,err:=(N4WindowsAddressSnapshot{}).IPv4(ctx,native.Index)
 if err!=nil || len(rows)!=0{t.Fatalf("original Wintun lease baseline not empty: %v rows %d",err,len(rows))}
 writer:=N4WindowsLeaseWriter{Guard:g,Adapter:tun,Snapshot:N4WindowsAddressSnapshot{}}
 mark("lease",mustN4Raw(t,n4LeaseBaseline{Index:native.Index,Address:lease.Prefix.String(),Absent:true}),func()error{
  _,err:=writer.Create(ctx,lease);if err!=nil{return err}
  deadline:=time.Now().Add(8*time.Second)
  for{
   row,found,e:=(N4WindowsLeaseReader{}).Lookup(ctx,lease)
   if e!=nil{return e}
   if found && N4NativeLeaseDADGate(row,lease)==nil{return nil}
   if time.Now().After(deadline){return ErrN4LeaseNotPreferred}
   time.Sleep(100*time.Millisecond)
  }
 })
 oldMTU,err:=(N4WindowsInterfaceReader{}).ReadIPv4(ctx,native.Index)
 if err!=nil{t.Fatal(err)}
 target:=uint32(1249);if oldMTU.MTU==target{target=1250}
 mtuWriter:=n4WindowsMTUWriter{Guard:g,Adapter:tun}
 mark("mtu",mustN4Raw(t,n4MTUStageBaseline{Before:oldMTU,Target:target}),func()error{
  _,err:=mtuWriter.Apply(ctx,oldMTU,target)
  return err
 })
 beforeDNS,err:=(N4NativeDNSReader{}).Read(ctx,native.Index)
 if err!=nil{t.Fatal(err)}
 if err:=n4ReadOwnedDNSBaseline(beforeDNS);err!=nil{t.Fatalf("fresh Wintun DNS baseline unsafe: %v",err)}
 servers:=[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")}
 dnsWriter:=n4OwnedDNSWrite{Adapter:tun}
 receipt:=n4DNSStageBaseline{Index:native.Index,Before:beforeDNS,Servers:servers}
 mark("dns",mustN4Raw(t,receipt),func()error{
  if err:=dnsWriter.SetIPv4(ctx,g,beforeDNS.Interface,native.Index,servers);err!=nil{return err}
  got,err:=(N4NativeDNSReader{}).Read(ctx,native.Index)
  if err!=nil{return err}
  if n4DNSClassifyState(got,receipt)!=N4UnifiedOwned{return ErrN4RouteAmbiguous}
  return nil
 })
 if err:=os.WriteFile(ready,[]byte("REAL_LEASE_MTU_DNS_WAL_SYNCED\n"),0600);err!=nil{t.Fatal(err)}
 for{time.Sleep(time.Second)} // parent kills child; never stage routes
}

// This test checks REAL lease/MTU/DNS writes and REAL adapter destruction;
// only the WFP guard is fake, so its result must never be called a complete
// production guard / true whole-network crash Recover qualification.
func TestN4WindowsHostedRealPreRouteIPCrashWAL(t *testing.T){
 if os.Getenv("WBD_N4_PREROUTE_CHILD")=="1"{n4HostedPreRouteRealChild(t);return}
 if os.Getenv("WBD_N4_ADMIN_PREROUTE_CRASH_PROBE")!="1" || os.Getenv("GITHUB_ACTIONS")!="true"{
  t.Skip("NOT_RUN: only real disposable GitHub hosted Wintun child")
 }
 report:=map[string]string{
  "schema":"wbd-n4-real-preroute-crash/v1",
  "source_sha":os.Getenv("GITHUB_SHA"),"runner":os.Getenv("RUNNER_OS"),
  "result":"FAIL_UNFINISHED","real_wintun":"NOT_RUN",
  "lease_mtu_dns":"NOT_RUN","original_retired":"NOT_RUN",
  "wal_recovery":"NOT_RUN","wfp":"FAKE_ONLY",
  "routes":"NOT_RUN","physical":"NOT_RUN","full_network_recover":"NOT_RUN",
 }
 defer func(){
  p:=os.Getenv("WBD_N4_ADMIN_PREROUTE_CRASH_REPORT")
  if p==""{t.Error("missing report path");return}
  b,err:=json.MarshalIndent(report,"","  ");if err!=nil{t.Error(err);return}
  if err:=os.MkdirAll(filepath.Dir(p),0700);err!=nil{t.Error(err);return}
  if err:=os.WriteFile(p,append(b,'\n'),0600);err!=nil{t.Error(err)}
 }()
 if report["source_sha"]=="" || report["runner"]!="Windows"{t.Fatal("exact Windows Actions source required")}
 var salt [8]byte
 if _,err:=rand.Read(salt[:]);err!=nil{t.Fatal(err)}
 alias:="WBDN4-PRC-"+hex.EncodeToString(salt[:])
 dir:=t.TempDir()
 wal,ready:=filepath.Join(dir,"preroute.json"),filepath.Join(dir,"ready")
 child:=exec.Command(os.Args[0],"-test.run=^TestN4WindowsHostedRealPreRouteIPCrashWAL$","-test.timeout=3m")
 child.Env=append(os.Environ(),"WBD_N4_PREROUTE_CHILD=1","WBD_N4_PREROUTE_ALIAS="+alias,
  "WBD_N4_PREROUTE_WAL="+wal,"WBD_N4_PREROUTE_READY="+ready)
 child.Stdout,child.Stderr=io.Discard,io.Discard
 if err:=child.Start();err!=nil{t.Fatal(err)}
 stopped:=false
 defer func(){if !stopped{_ =child.Process.Kill();_ =child.Wait()}}()
 deadline:=time.Now().Add(35*time.Second)
 for{
  b,err:=os.ReadFile(ready)
  if err==nil{
   if string(b)!="REAL_LEASE_MTU_DNS_WAL_SYNCED\n"{t.Fatalf("bad child ready proof %q",b)}
   break
  }
  if time.Now().After(deadline){t.Fatal("child failed to perform fsynced real IP/MTU/DNS stages")}
  time.Sleep(100*time.Millisecond)
 }
 j,err:=n4UnifiedRead(wal)
 if err!=nil || len(j.Records)!=4 || j.Records[3].State!="owned"{t.Fatalf("real child WAL not fully committed: %+v %v",j,err)}
 report["real_wintun"]="PASS_CHILD_CREATED_ORIGINAL_GUID_LUID_INDEX"
 report["lease_mtu_dns"]="PASS_REAL_NATIVE_WRITES_FSYNCED"
 g:=&n4OwnedOrphanFakeGuard{state:N4UnifiedOwned}
 preCtx,stop:=context.WithTimeout(context.Background(),10*time.Second)
 if err:=n4OriginalWintunPersistedExact(preCtx,j.Identity);err!=nil{
  stop()
  t.Fatalf("live original GUID/LUID/ifIndex triple not independently corroborated: %v",err)
 }
 err=n4RetirePreRouteOrphanWAL(preCtx,wal,j.Identity,g)
 stop()
 if !errors.Is(err,ErrN4ForeignRoute){t.Fatalf("still-live original Wintun incorrectly retired: %v",err)}
 if g.inspectCalls!=0{t.Fatal("live Wintun reached WFP deletion")}
 if _,err:=os.Stat(wal);err!=nil{t.Fatalf("live child journal disappeared: %v",err)}
 if err:=child.Process.Kill();err!=nil{t.Fatal(err)}
 if err:=child.Wait();err==nil{t.Fatal("child did not TerminateProcess")}
 stopped=true
 ctx,cancel:=context.WithTimeout(context.Background(),15*time.Second)
 defer cancel()
 for {
  err=n4OriginalWintunAbsent(ctx,j.Identity)
  if err==nil{break}
  if ctx.Err()!=nil {
   // A timeout by itself cannot prove persistence or absence. Re-query OS
   // with a fresh bounded context and require the WHOLE original native
   // GUID+LUID+ifIndex, not an alias or an overlapping foreign index.
   checkCtx,done:=context.WithTimeout(context.Background(),3*time.Second)
   state,checkErr:=n4InspectOriginalWintun(checkCtx,j.Identity)
   done()
   if checkErr==nil && state==N4UnifiedAbsent{
    ctx,cancel=context.WithTimeout(context.Background(),10*time.Second)
    defer cancel()
    break
   }
   switch{
   case checkErr==nil && state==N4UnifiedOwned:
    // This is the *same original* OS adapter (not a same-name reopening).
    // The guard is a TEST-ONLY fake; any real post-crash native setter error
    // is a genuine FAIL that retains WAL, not UNSUPPORTED or a mocked PASS.
    rollbackCtx,rollbackDone:=context.WithTimeout(context.Background(),30*time.Second)
    fake:=&n4FakeGuard{durable:true,active:true}
    // Read-only triage: report ONLY exact-CAS state categories, never IP,
    // DNS, profile values or packet data. No mutation is done by Inspect.
    for i:=1;i<len(j.Records);i++{
     st,stageErr:=n4PersistedPreRouteStage(j.Identity,j.Records[i],fake)
     if stageErr!=nil{t.Fatalf("read-only triage stage constructor %s: %v",j.Records[i].Name,stageErr)}
     state,inspectErr:=st.Inspect(rollbackCtx,j.Identity,j.Records[i].Before)
     if inspectErr!=nil {
      // Native OS error is a code/operation only; no actual DNS/IP
      // settings, packet content, key or account data is logged.
      t.Logf("N4_REAL_POSTCRASH_STAGE=%s WAL=%s CAS_STATE=%s READ_OK=false ERROR=%v",j.Records[i].Name,j.Records[i].State,state,inspectErr)
     }else{
      t.Logf("N4_REAL_POSTCRASH_STAGE=%s WAL=%s CAS_STATE=%s READ_OK=true",j.Records[i].Name,j.Records[i].State,state)
     }
    }
    rollbackErr:=n4RecoverPersistedPreRouteWAL(rollbackCtx,wal,j.Identity,fake)
    rollbackDone()
    if rollbackErr!=nil{
     t.Fatalf("same-original Win32 CAS rollback after real child crash failed (WAL retained): %v",rollbackErr)
    }
    if fake.active || fake.disarmCount!=1{t.Fatal("test guard not disarmed exactly once after all real IP cleanup")}
    if _,e:=os.Stat(wal);!errors.Is(e,os.ErrNotExist){t.Fatalf("native CAS rollback did not retire WAL: %v",e)}
    report["original_retired"]="PASS_EXACT_SAME_ORIGINAL_GUARD_CAS"
    report["wal_recovery"]="PASS_NATIVE_LEASE_MTU_DNS_REVERSED_AFTER_CRASH"
    report["result"]="SCOPED_REAL_PERSISTED_ORIGINAL_IP_ROLLBACK_FAKE_WFP_PASS"
    t.Log("real destroyed child left original Wintun present; exact-GUID/LUID/index native lease/MTU/DNS rollback PASS, WFP FAKE and routes NOT_RUN")
    return
   case errors.Is(checkErr,ErrN4ForeignRoute):
    report["result"]="UNSUPPORTED_ORIGINAL_IDENTITY_CONFLICT"
    report["original_retired"]="UNSUPPORTED_FOREIGN_OR_REUSED_IDENTITY"
   default:
    report["result"]="UNSUPPORTED_NATIVE_OS_ABSENCE_UNPROVEN"
    report["original_retired"]="UNSUPPORTED_OS_NATIVE_SCAN"
   }
   if _,e:=os.Stat(wal);e!=nil{t.Fatalf("ambiguous OS identity lost WAL: %v",e)}
   if g.inspectCalls!=0 || g.disarmCalls!=0{t.Fatal("ambiguous adapter reached WFP deletion")}
   t.Skipf("UNSUPPORTED: after real IP writes and TerminateProcess original adapter absent proof not met; parent WAL retained, state=%s err=%v",state,checkErr)
  }
  if !errors.Is(err,ErrN4ForeignRoute){t.Fatalf("OS original identity scan failed unexpectedly: %v",err)}
  time.Sleep(100*time.Millisecond)
 }
 report["original_retired"]="PASS_REAL_TERMINATEPROCESS_OS_ABSENCE"
 if err:=n4RetirePreRouteOrphanWAL(ctx,wal,j.Identity,g);err!=nil{t.Fatalf("dead original pre-route WAL not safely retired: %v",err)}
 if _,err:=os.Stat(wal);!errors.Is(err,os.ErrNotExist){t.Fatalf("WAL not gone: %v",err)}
 if g.disarmCalls!=1 || g.inspectCalls<2{t.Fatal("fake guard did not verify removal")}
 report["wal_recovery"]="PASS_DEAD_ORIGINAL_PREROUTE_WAL_ONLY"
 report["result"]="SCOPED_REAL_WINTUN_IP_STAGE_CRASH_FAKE_WFP_PASS"
 t.Log("real Wintun native lease/MTU/DNS writes followed by TerminateProcess, OS old interface removal, and journal-only cleanup with fake WFP: PASS; routes/global WFP/physical NOT_RUN")
}
