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

// This test intentionally mutates ONLY a NEW randomly named Wintun adapter
// on an isolated disposable GitHub Windows runner. It never installs host
// routes, production WFP filters, DNS on a physical NIC, or product traffic.
// It validates native IP Helper/DNS mechanics, not the full protective policy.
func TestN4WindowsHostedOwnedAdapterNativeIPWritesAndRestore(t *testing.T){
 if os.Getenv("WBD_N4_ADMIN_MUTATION")!="1" || os.Getenv("GITHUB_ACTIONS")!="true"{
  t.Skip("NOT_RUN: isolated Windows Actions adapter mutation only")
 }
 receipt:=map[string]string{
  "schema":"wbd-n4-created-wintun-native-ip-mutation/v1",
  "source_sha":os.Getenv("GITHUB_SHA"),
  "runner":os.Getenv("RUNNER_OS"),
  "result":"FAIL_UNFINISHED",
  "adapter":"NOT_RUN","lease":"NOT_RUN","mtu":"NOT_RUN",
  "dns":"NOT_RUN","guard":"TEST_ONLY_FAKE_NOT_PRODUCTION_WFP",
  "routes":"NOT_RUN","physical":"NOT_RUN",
 }
 defer func(){
  target:=os.Getenv("WBD_N4_ADMIN_MUTATION_REPORT")
  if target==""{t.Errorf("mutation report path missing");return}
  raw,err:=json.MarshalIndent(receipt,"","  ")
  if err!=nil{t.Errorf("encode report: %v",err);return}
  if err:=os.MkdirAll(filepath.Dir(target),0700);err!=nil{t.Errorf("report dir: %v",err);return}
  if err:=os.WriteFile(target,append(raw,'\n'),0600);err!=nil{t.Errorf("write report: %v",err)}
 }()
 if receipt["source_sha"]=="" || receipt["runner"]!="Windows"{t.Fatal("exact Windows Actions SOURCE required")}
 var random [8]byte
 if _,err:=rand.Read(random[:]);err!=nil{t.Fatal(err)}
 tun,err:=OpenTUN("WBDN4-MUT-"+hex.EncodeToString(random[:]))
 if err!=nil{
  receipt["result"]="UNSUPPORTED_DRIVER_OR_ADMIN"
  t.Skipf("UNSUPPORTED: isolated Wintun Create/Session failed: %v",err)
 }
 defer func(){
  if err:=tun.Close();err!=nil{t.Errorf("close created test Wintun: %v",err)}
 }()
 if !tun.createdByProcess{t.Fatal("borrowed adapter must not be mutated")}
 ctx,cancel:=context.WithTimeout(context.Background(),45*time.Second)
 defer cancel()
 native,err:=tun.NativeIdentity(ctx)
 if err!=nil || !native.CreatedByProcess || native.Index==0 || native.LUID==0{
  t.Fatalf("real Wintun owned LUID: %+v %v",native,err)
 }
 receipt["adapter"]="PASS_NEW_CREATED_LUID"
 // A fake WFP guard here is test-only and protects NO real traffic. This
 // isolated test intentionally leaves the host routing table untouched.
 guard:=&n4FakeGuard{durable:true,active:true}
 lease:=N4NativeLeaseIntent{InterfaceIndex:native.Index,Prefix:netip.MustParsePrefix("198.18.0.77/32")}
 writer:=N4WindowsLeaseWriter{Guard:guard,Adapter:tun,Snapshot:N4WindowsAddressSnapshot{}}
 token,err:=writer.Create(ctx,lease)
 if err!=nil{t.Fatalf("real CreateUnicastIpAddressEntry on owned test adapter: %v",err)}
 deadline:=time.Now().Add(8*time.Second)
 for{
  row,found,err:=(N4WindowsLeaseReader{}).Lookup(ctx,lease)
  if err!=nil{t.Fatalf("real native lease row: %v",err)}
  if found && N4NativeLeaseDADGate(row,lease)==nil{break}
  if time.Now().After(deadline){t.Fatal("real isolated lease DAD never reached Preferred")}
  time.Sleep(100*time.Millisecond)
 }
 receipt["lease"]="PASS_CREATE_DAD_PREFERRED"
 old,err:=(N4WindowsInterfaceReader{}).ReadIPv4(ctx,native.Index)
 if err!=nil{t.Fatalf("read baseline MTU: %v",err)}
 target:=uint32(1249)
 if old.MTU==target{target=1250}
 mtuWriter:=n4WindowsMTUWriter{Guard:guard,Adapter:tun}
 // Real native diagnostic only: never weaken the full-row CAS on a new
 // driver adapter to accommodate asynchronous Windows interface changes.
 initialRow,initialErr:=n4GetIPv4InterfaceRow(ctx,native.Index)
 currentOwner,ownerErr:=tun.IsCurrentOwnedWintun(ctx,native.Index)
 t.Logf("WBD_N4_MTU_BEFORE expected=%+v observed=%+v site_prefix=%d read_err=%v live_created_owner=%v owner_err=%v",
  old,n4MTUSnapshotFromRow(initialRow),initialRow.SitePrefixLength,initialErr,currentOwner,ownerErr)
 mtur,err:=mtuWriter.Apply(ctx,old,target)
 if err!=nil{
  lastRow,lastErr:=n4GetIPv4InterfaceRow(ctx,native.Index)
  t.Fatalf("real owned Wintun SetIpInterfaceEntry: %v; expected=%+v observed=%+v site_prefix=%d native_read_err=%v",
   err,old,n4MTUSnapshotFromRow(lastRow),lastRow.SitePrefixLength,lastErr)
 }
 if err:=mtuWriter.Restore(ctx,mtur);err!=nil{t.Fatalf("real owned Wintun MTU restore: %v",err)}
 afterMTU,err:=(N4WindowsInterfaceReader{}).ReadIPv4(ctx,native.Index)
 if err!=nil || afterMTU!=old{t.Fatalf("original native MTU row not restored: %+v %+v %v",old,afterMTU,err)}
 receipt["mtu"]="PASS_SET_CAS_AND_RESTORE"
 beforeDNS,err:=(N4NativeDNSReader{}).Read(ctx,native.Index)
 if err!=nil{t.Fatalf("GetInterfaceDnsSettings before Set: %v",err)}
 if err:=n4ReadOwnedDNSBaseline(beforeDNS);err!=nil{
  receipt["dns"]="UNSUPPORTED_NONEMPTY_FRESH_DNS_BASELINE"
  receipt["result"]="UNSUPPORTED_FRESH_DNS_BASELINE"
  t.Skipf("UNSUPPORTED: fresh adapter has nonempty/foreign DNS baseline: %v",err)
 }
 dnsWriter:=n4OwnedDNSWrite{Adapter:tun}
 if err:=dnsWriter.SetIPv4(ctx,guard,beforeDNS.Interface,native.Index,
   []netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")});err!=nil{
  t.Fatalf("real native SetInterfaceDnsSettings: %v",err)
 }
 rec:=n4DNSStageBaseline{
  Index:native.Index,Before:beforeDNS,
  Servers:[]netip.Addr{netip.MustParseAddr("1.1.1.1"),netip.MustParseAddr("8.8.8.8")},
 }
 gotDNS,err:=(N4NativeDNSReader{}).Read(ctx,native.Index)
 if err!=nil || n4DNSClassifyState(gotDNS,rec)!=N4UnifiedOwned{
  t.Fatalf("real static DNS write readback mismatch: %+v %v",gotDNS,err)
 }
 if err:=dnsWriter.ClearIPv4(ctx,guard,beforeDNS.Interface,native.Index);err!=nil{
  t.Fatalf("real static DNS ClearIPv4: %v",err)
 }
 restored,err:=(N4NativeDNSReader{}).Read(ctx,native.Index)
 if err!=nil || n4DNSClassifyState(restored,rec)!=N4UnifiedAbsent{
  t.Fatalf("real dynamic DNS baseline not restored: %+v %v",restored,err)
 }
 receipt["dns"]="PASS_SET_CLEAR_GET_EXACT_BASELINE"
 if err:=writer.DeleteConfirmed(ctx,token);err!=nil{t.Fatalf("real native owned address Delete: %v",err)}
 rows,err:=(N4WindowsAddressSnapshot{}).IPv4(ctx,native.Index)
 if err!=nil || len(rows)!=0{t.Fatalf("native address cleanup failed rows=%d err=%v",len(rows),err)}
 receipt["lease"]="PASS_CREATE_DAD_DELETE_EXACT"
 receipt["result"]="SCOPED_ISOLATED_NATIVE_IP_WRITE_RESTORE_PASS"
 t.Log("actual disposable Wintun native lease/MTU/DNS mutation restored; persistent WFP, routes and physical NOT_RUN")
}
