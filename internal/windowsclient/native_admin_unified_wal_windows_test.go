//go:build windows

package windowsclient

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "net/netip"
 "os"
 "path/filepath"
 "testing"
 "time"
)

// Validates REAL native lease/MTU/DNS under the ONE existing crash-durable WAL,
// Wintun lifetime reservation and machine mutex. WFP and routes are faked
// intentionally: NEVER install global filters or two /1 routes on a hosted
// shared Actions runner. This is not production N4 qualification.
func TestN4WindowsHostedUnifiedWALRealLeaseMTUDNS(t *testing.T) {
 if os.Getenv("WBD_N4_ADMIN_UNIFIED_WAL")!="1" || os.Getenv("GITHUB_ACTIONS")!="true" {
  t.Skip("NOT_RUN: isolated hosted Wintun WAL integration only")
 }
 receipt:=map[string]string{
  "schema":"wbd-n4-hosted-real-wintun-unified-wal/v1",
  "source_sha":os.Getenv("GITHUB_SHA"),"runner":os.Getenv("RUNNER_OS"),
  "result":"FAIL_UNFINISHED","adapter":"NOT_RUN",
  "lease":"NOT_RUN","mtu":"NOT_RUN","dns":"NOT_RUN","stage_assembly":"NOT_RUN",
  "guard":"TEST_ONLY_FAKE_NOT_PRODUCTION_WFP",
  "routes":"TEST_ONLY_FAKE_NO_CAPTURE_ROUTES","physical":"NOT_RUN",
  "cross_process_crash":"NOT_RUN",
 }
 defer func(){
  target:=os.Getenv("WBD_N4_ADMIN_UNIFIED_WAL_REPORT")
  if target=="" {t.Error("missing owned WAL test report path");return}
  b,err:=json.MarshalIndent(receipt,"","  ")
  if err!=nil{t.Error(err);return}
  if err:=os.MkdirAll(filepath.Dir(target),0700);err!=nil{t.Error(err);return}
  if err:=os.WriteFile(target,append(b,'\n'),0600);err!=nil{t.Error(err)}
 }()
 if receipt["source_sha"]=="" || receipt["runner"]!="Windows" {
  t.Fatal("exact Windows GitHub Actions source required")
 }
 var entropy [8]byte
 if _,err:=rand.Read(entropy[:]);err!=nil{t.Fatal(err)}
 tun,err:=OpenTUN("WBDN4-WAL-"+hex.EncodeToString(entropy[:]))
 if err!=nil{
  receipt["result"]="UNSUPPORTED_DRIVER_OR_ADMIN"
  t.Skipf("UNSUPPORTED: isolated Wintun adapter not available: %v",err)
 }
 defer func(){if err:=tun.Close();err!=nil{t.Errorf("Wintun close: %v",err)}}()
 if !tun.createdByProcess{t.Fatal("borrowed Wintun was not created by this process")}
 ctx,cancel:=context.WithTimeout(context.Background(),75*time.Second)
 defer cancel()
 live,err:=tun.NativeIdentity(ctx)
 if err!=nil || !live.CreatedByProcess || live.Index==0 || live.LUID==0{
  t.Fatalf("native created Wintun LUID: %+v %v",live,err)
 }
 receipt["adapter"]="PASS_NEW_CREATED_WINTUN_LUID"
 var install,gen [16]byte
 if _,err:=rand.Read(install[:]);err!=nil{t.Fatal(err)}
 if _,err:=rand.Read(gen[:]);err!=nil{t.Fatal(err)}
 id:=N4UnifiedIdentity{Installation:install,Generation:gen,
  WintunIndex:live.Index,WintunLUID:live.LUID}
 owner:=N4WindowsCreatedWintunReservation{
  Tunnel:tun,Installation:install,Generation:gen,
 }
 fakeWFP:=&n4FakeGuard{durable:true,active:true}
 // These are ONLY test stage stand-ins. Their state must not be mistaken
 // for a real WFP persistent IPv6/DNS guard or a host capture route.
 trace:=&[]string{}
 guardStage:=&n4UnifiedTestStage{name:"guard",state:N4UnifiedAbsent,trace:trace}
 routesStage:=&n4UnifiedTestStage{name:"routes",state:N4UnifiedAbsent,trace:trace}
 lease:=N4NativeLeaseIntent{InterfaceIndex:live.Index,
  Prefix:netip.MustParsePrefix("198.18.0.77/32")}
 targetMTU:=uint32(1249)
 baseline,err:=(N4WindowsInterfaceReader{}).ReadIPv4(ctx,live.Index)
 if err!=nil{t.Fatalf("native fresh Wintun MTU read: %v",err)}
 if baseline.MTU==targetMTU{targetMTU=1250}
 beforeDNS,err:=(N4NativeDNSReader{}).Read(ctx,live.Index)
 if err!=nil{t.Fatalf("native fresh Wintun DNS read: %v",err)}
 if err:=n4ReadOwnedDNSBaseline(beforeDNS);err!=nil{
  receipt["result"]="UNSUPPORTED_FRESH_DNS_BASELINE"
  t.Skipf("UNSUPPORTED: fresh Wintun DNS baseline not empty: %v",err)
 }
 servers:=[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")}
 path:=filepath.Join(t.TempDir(),"actual-owned-unified.json")
 stages:=[5]N4UnifiedStage{
  guardStage,
  n4WindowsLeaseStage{Lease:lease,Writer:N4WindowsLeaseWriter{
   Guard:fakeWFP,Adapter:tun,Snapshot:N4WindowsAddressSnapshot{},
  }},
  n4WindowsMTUStage{Target:targetMTU,Writer:n4WindowsMTUWriter{
   Guard:fakeWFP,Adapter:tun,
  }},
  n4WindowsDNSStage{Servers:servers,Guard:fakeWFP,Writer:n4OwnedDNSWrite{Adapter:tun}},
  routesStage,
 }
 txn:=&N4NativeUnifiedTxn{
  Path:path,Identity:id,Gate:N4WindowsMachineGate{},Owner:owner,Stages:stages,
 }
 // Leave durable evidence intact after any ambiguous write. A best-effort
 // same-process exact-owner cleanup is attempted, NEVER a foreign NIC reset.
 cleanupRequired:=true
 defer func(){
  if !cleanupRequired {
   return
  }
  cleanupCtx,done:=context.WithTimeout(context.Background(),30*time.Second)
  defer done()
  if err:=txn.Recover(cleanupCtx);err!=nil{
   t.Errorf("isolated WAL rollback not provably complete (journal retained): %v",err)
  }
 }()
 if err:=txn.Apply(ctx);err!=nil{
  t.Fatalf("real created-Wintun native WAL Apply: %v",err)
 }
 j,err:=n4UnifiedRead(path)
 if err!=nil || j.Phase!="active" || len(j.Records)!=5{
  t.Fatalf("unified real native WAL not fully active: %+v %v",j,err)
 }
 receipt["lease"]="PASS_WAL_DAD_PREFERRED"
 receipt["mtu"]="PASS_WAL_NATIVE_MTU_CAS"
 receipt["dns"]="PASS_WAL_NATIVE_SET_GET"
 // Deliberately recover with a distinct controller instance while the
 // original created Wintun handle is still alive; no cross-process crash.
 fresh:=&N4NativeUnifiedTxn{
  Path:path,Identity:id,Gate:N4WindowsMachineGate{},Owner:owner,Stages:stages,
 }
 if err:=fresh.Recover(ctx);err!=nil{t.Fatalf("actual native WAL Recover: %v",err)}
 if err:=fresh.Recover(ctx);err!=nil{t.Fatalf("repeat Stop is not idempotent: %v",err)}
 if _,err:=os.Stat(path);!os.IsNotExist(err){
  t.Fatalf("owned WAL not removed after confirmed restore: %v",err)
 }
 rows,err:=(N4WindowsAddressSnapshot{}).IPv4(ctx,live.Index)
 if err!=nil || len(rows)!=0{
  t.Fatalf("lease still present after unified Recover rows=%d err=%v",len(rows),err)
 }
 mtu,err:=(N4WindowsInterfaceReader{}).ReadIPv4(ctx,live.Index)
 if err!=nil || mtu!=baseline{
  t.Fatalf("original MTU baseline not restored: original=%+v now=%+v err=%v",baseline,mtu,err)
 }
 dns,err:=(N4NativeDNSReader{}).Read(ctx,live.Index)
 if err!=nil || !n4DNSExceptStaticEqual(dns,beforeDNS) ||
  dns.RawNameServer!="" || len(dns.NameServers)!=0 {
  t.Fatalf("original DNS baseline not restored: %+v",err)
 }
 if guardStage.state!=N4UnifiedAbsent || routesStage.state!=N4UnifiedAbsent{
  t.Fatal("test-only staged guard or routes not reversed")
 }
 // Dry-run the actual product five-stage composition without EVER
 // arming the host-global WFP filter or writing two /1 capture routes.
 // The exclusive flag here is a TEST-ONLY placeholder, not proof that a
 // production machine has no other independently managed Wintun.
 plan,err:=BuildCompactNetworkPlan(Config{
  AdapterAlias:tun.Name(),Lease4:lease.Prefix,
  Server4:netip.MustParseAddr("198.51.100.9"),
  Physical:PhysicalPath{InterfaceIndex:12,NextHop4:netip.MustParseAddr("192.0.2.1")},
  TunnelMTU:targetMTU,DNSServers4:servers,
  StatePath:filepath.Join(t.TempDir(),"unused-product-legacy-state.json"),
 })
 if err!=nil{t.Fatalf("compact native composition plan: %v",err)}
 inactiveWFP:=&n4WindowsPersistentWFPGuard{
  InstallationID:install,WintunLUID:live.LUID,ExclusiveSingleWintun:true,
 }
 composed,err:=n4AssembleWindowsNativeOwnedTxn(ctx,tun,plan,id,inactiveWFP)
 if err!=nil{t.Fatalf("actual native stage wiring preflight: %v",err)}
 for i,stage:=range composed.Stages{
  if stage==nil || stage.Name()!=n4UnifiedOrder[i]{
   t.Fatalf("missing actual concrete stage at %d: %v",i,stage)
  }
 }
 receipt["stage_assembly"]="PASS_NO_HOST_WFP_OR_CAPTURE_ROUTE_WRITE"
 cleanupRequired=false
 receipt["lease"]="PASS_WAL_DAD_AND_OWNED_DELETE"
 receipt["mtu"]="PASS_WAL_CAS_AND_EXACT_RESTORE"
 receipt["dns"]="PASS_WAL_SET_CLEAR_GET_ORIGINAL"
 receipt["result"]="SCOPED_REAL_WINTUN_NATIVE_WAL_APPLY_RECOVER_PASS"
 t.Log("REAL created-Wintun five-stage WAL: 3 native stages PASS, fake WFP/routes; no host default route, WFP, or physical traffic")
}
