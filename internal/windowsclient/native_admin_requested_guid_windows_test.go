//go:build windows

package windowsclient

import (
 "crypto/rand"
 "encoding/binary"
 "encoding/hex"
 "encoding/json"
 "os"
 "path/filepath"
 "syscall"
 "testing"
 "unsafe"

 "golang.org/x/sys/windows"
)

// This is an isolated, disposable Windows ACTIONS capability only.
// RequestedGUID is an undocumented Wintun API knob; the OS must independently
// return an identical interface GUID via the native NET_LUID API before it
// can be considered a candidate for durable journal identity.
// No recovery, foreign adapter adoption, host routing, DNS or WFP mutations.
func TestN4WindowsHostedRequestedGUIDNativeReadback(t *testing.T){
 if os.Getenv("WBD_N4_ADMIN_GUID_PROBE")!="1" || os.Getenv("GITHUB_ACTIONS")!="true"{
  t.Skip("NOT_RUN: isolated GitHub Windows Wintun GUID probe")
 }
 receipt:=map[string]string{
  "schema":"wbd-n4-requested-guid-readback/v1",
  "source_sha":os.Getenv("GITHUB_SHA"),"runner":os.Getenv("RUNNER_OS"),
  "result":"FAIL_UNFINISHED","requested_guid":"NOT_RUN",
  "native_luid_guid":"NOT_RUN","cross_process_crash":"NOT_RUN",
  "os_route":"NOT_RUN","wfp":"NOT_RUN","physical":"NOT_RUN",
 }
 defer func(){
  target:=os.Getenv("WBD_N4_ADMIN_GUID_PROBE_REPORT")
  if target==""{t.Error("GUID proof report path missing");return}
  b,err:=json.MarshalIndent(receipt,"","  ");if err!=nil{t.Error(err);return}
  if err:=os.MkdirAll(filepath.Dir(target),0700);err!=nil{t.Error(err);return}
  if err:=os.WriteFile(target,append(b,'\n'),0600);err!=nil{t.Error(err)}
 }()
 if receipt["source_sha"]=="" || receipt["runner"]!="Windows"{
  t.Fatal("exact Windows Actions SOURCE mandatory")
 }
 var entropy [16]byte
 if _,err:=rand.Read(entropy[:]);err!=nil{t.Fatal(err)}
 // Fresh, random RFC4122 v4 GUID: not derived from account, password or
 // installation ID, and not a substitute for durable ACL owner proof.
 entropy[6]=(entropy[6]&0x0f)|0x40
 entropy[8]=(entropy[8]&0x3f)|0x80
 guid:=windows.GUID{
  Data1:binary.LittleEndian.Uint32(entropy[:4]),
  Data2:binary.LittleEndian.Uint16(entropy[4:6]),
  Data3:binary.LittleEndian.Uint16(entropy[6:8]),
  Data4:[8]byte(entropy[8:]),
 }
 alias:="WBDN4-GUID-"+hex.EncodeToString(entropy[:6])
 name,err:=syscall.UTF16PtrFromString(alias);if err!=nil{t.Fatal(err)}
 kind,err:=syscall.UTF16PtrFromString("WBD");if err!=nil{t.Fatal(err)}
 if err:=wintunCreateAdapter.Find();err!=nil{
  receipt["result"]="UNSUPPORTED_WINTUN_DLL"
  t.Skipf("UNSUPPORTED native Wintun create: %v",err)
 }
 adapter,_,callErr:=wintunCreateAdapter.Call(
  uintptr(unsafe.Pointer(name)),uintptr(unsafe.Pointer(kind)),uintptr(unsafe.Pointer(&guid)))
 if adapter==0{
  receipt["result"]="UNSUPPORTED_REQUESTED_GUID_CREATE"
  t.Skipf("UNSUPPORTED Wintun requested GUID create: %v",callErr)
 }
 defer wintunCloseAdapter.Call(adapter)
 receipt["requested_guid"]="PASS_CREATED_FRESH_ADAPTER"
 if err:=n4WintunGetAdapterLuid.Find();err!=nil{t.Fatal(err)}
 var luid uint64
 n4WintunGetAdapterLuid.Call(adapter,uintptr(unsafe.Pointer(&luid)))
 if luid==0{t.Fatal("created Wintun returned zero LUID")}
 // Reuse the existing Windows DNS interface reader's OS-backed
 // index -> LUID -> GUID conversion, avoiding duplicate ABI declarations.
 if err:=n4ConvertInterfaceLuidIndex.Find();err!=nil{t.Fatal(err)}
 var index uint32
 rc,_,_:=n4ConvertInterfaceLuidIndex.Call(uintptr(unsafe.Pointer(&luid)),uintptr(unsafe.Pointer(&index)))
 if rc!=0 || index==0{t.Fatalf("ConvertInterfaceLuidToIndex: status=%d index=%d",rc,index)}
 observed,err:=n4NativeInterfaceGUID(index)
 if err!=nil{t.Fatalf("existing native interface GUID lookup: %v",err)}
 if observed!=guid{
  t.Fatalf("requested/OS interface GUID differ; no crash-owner entitlement: requested=%v read=%v",guid,observed)
 }
 // The same OS LUID cannot be treated as a proof for a different adapter
 // generation; this test proves ONLY the native GUID readback on a live handle.
 receipt["native_luid_guid"]="PASS_MATCHES_REQUESTED_GUID"
 receipt["result"]="SCOPED_REQUESTED_GUID_NATIVE_MATCH_PASS"
 t.Log("SCOPED requested GUID matched native LUID readback for disposable adapter; no crash recovery or network writes")
}
