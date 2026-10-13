//go:build windows

package windowsclient

import (
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "testing"
 "time"
)

type n4HostedDriverCapabilityReceipt struct {
 Schema string `json:"schema"`
 SourceSHA string `json:"source_sha"`
 Runner string `json:"runner"`
 Result string `json:"result"`
 Driver string `json:"driver"`
 CreatedAdapter string `json:"created_adapter"`
 NativeLUID string `json:"native_luid"`
 IPv4AddressRead string `json:"ipv4_address_read"`
 IPv4InterfaceRead string `json:"ipv4_interface_read"`
 NativeDNSRead string `json:"native_dns_read"`
 RealWrite string `json:"real_write"`
 WFPProduction string `json:"wfp_production"`
 Physical string `json:"physical"`
}
func n4WriteHostedDriverCapability(r n4HostedDriverCapabilityReceipt)error{
 target:=os.Getenv("WBD_N4_ADMIN_PROBE_REPORT")
 if target==""{return errors.New("N4 driver capability report path required")}
 b,err:=json.MarshalIndent(r,"","  ");if err!=nil{return err}
 if err:=os.MkdirAll(filepath.Dir(target),0700);err!=nil{return err}
 return os.WriteFile(target,append(b,'\n'),0600)
}
// A REAL Wintun driver-adapter creation/read capability probe, NOT a simulated
// Wintun test and NOT the product's persistent WFP/network configuration.
// No address/MTU/DNS/route/firewall writes are performed. Runs only on an
// isolated disposable GitHub Actions Windows runner with pinned official DLL.
func TestN4WindowsHostedCreatedWintunReadOnlyCapability(t *testing.T){
 if os.Getenv("WBD_N4_ADMIN_ISOLATED")!="1" || os.Getenv("GITHUB_ACTIONS")!="true"{
  t.Skip("NOT_RUN: requires isolated GH Windows opt-in")
 }
 rec:=n4HostedDriverCapabilityReceipt{
  Schema:"wbd-n4-hosted-created-wintun-readonly/v1",
  SourceSHA:os.Getenv("GITHUB_SHA"),Runner:os.Getenv("RUNNER_OS"),
  Result:"UNSUPPORTED",Driver:"NOT_RUN",CreatedAdapter:"NOT_RUN",
  NativeLUID:"NOT_RUN",IPv4AddressRead:"NOT_RUN",IPv4InterfaceRead:"NOT_RUN",
  NativeDNSRead:"NOT_RUN",RealWrite:"NOT_RUN",WFPProduction:"NOT_RUN",Physical:"NOT_RUN",
 }
 save:=func(){t.Helper();if err:=n4WriteHostedDriverCapability(rec);err!=nil{t.Fatal(err)}}
 if rec.SourceSHA=="" || rec.Runner!="Windows"{
  t.Fatal("SOURCE and Windows runner identity are mandatory")
 }
 if err:=wintunCreateAdapter.Find();err!=nil{
  rec.Driver="UNSUPPORTED_DLL_OR_EXPORT";save()
  t.Skipf("UNSUPPORTED: official wintun.dll create export unavailable: %v",err)
 }
 var entropy [8]byte
 if _,err:=rand.Read(entropy[:]);err!=nil{t.Fatal(err)}
 alias:="WBDN4CI-"+hex.EncodeToString(entropy[:])
 tun,err:=OpenTUN(alias)
 if err!=nil{
  rec.Driver="UNSUPPORTED_DRIVER_OR_PRIVILEGE";save()
  t.Skipf("UNSUPPORTED: unable to create unique isolated Wintun adapter: %v",err)
 }
 // Cleanup is unconditional, including error paths. WintunCloseAdapter
 // release is not by itself a proven persistent OS-adapter removal.
 defer func(){
  if err:=tun.Close();err!=nil{t.Errorf("created Wintun close: %v",err)}
 }()
 if !tun.createdByProcess{
  rec.Driver="FAIL_FOREIGN_ALIAS";save()
  t.Fatal("random-name opened/borrowed adapter must never be treated as owned")
 }
 rec.Driver="PASS_REAL_CREATED_WINTUN"
 rec.CreatedAdapter="PASS_PROCESS_CREATED"
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second)
 defer cancel()
 id,err:=tun.NativeIdentity(ctx)
 if err!=nil || !id.CreatedByProcess || id.Index==0 || id.LUID==0{
  rec.NativeLUID="FAIL";save()
  t.Fatalf("WintunGetAdapterLuid/ConvertInterfaceLuidToIndex live proof: %+v %v",id,err)
 }
 var ownerID N4UnifiedIdentity
 ownerID.Installation[0]=42
 ownerID.Generation[0]=19
 ownerID.WintunIndex=id.Index
 ownerID.WintunLUID=id.LUID
 owner:=N4WindowsCreatedWintunReservation{
  Tunnel:tun,Installation:ownerID.Installation,Generation:ownerID.Generation,
 }
 release,err:=owner.Hold(ctx,ownerID)
 if err!=nil || release==nil{
  rec.NativeLUID="FAIL_OWNER_HOLD";save()
  t.Fatalf("created-Wintun generation hold: %v",err)
 }
 release()
 rec.NativeLUID="PASS_LIVE_LUID_INDEX_AND_GENERATION"
 rows,err:=(N4WindowsAddressSnapshot{}).IPv4(ctx,id.Index)
 if err!=nil{
  rec.IPv4AddressRead="UNSUPPORTED_READ";save()
  t.Skipf("UNSUPPORTED: created Wintun IPv4 table read: %v",err)
 }
 if len(rows)!=0{
  rec.IPv4AddressRead="FAIL_FOREIGN_EXISTING_ADDRESS";save()
  t.Fatalf("fresh Wintun inherited foreign address count=%d",len(rows))
 }
 rec.IPv4AddressRead="PASS_EMPTY_FRESH_ADAPTER"
 if _,err:=(N4WindowsInterfaceReader{}).ReadIPv4(ctx,id.Index);err!=nil{
  rec.IPv4InterfaceRead="UNSUPPORTED_READ";save()
  t.Skipf("UNSUPPORTED: native MTU row read: %v",err)
 }
 rec.IPv4InterfaceRead="PASS_READ_ONLY"
 if _,err:=(N4NativeDNSReader{}).Read(ctx,id.Index);err!=nil{
  rec.NativeDNSRead="UNSUPPORTED_READ";save()
  t.Skipf("UNSUPPORTED: native DNS interface snapshot: %v",err)
 }
 rec.NativeDNSRead="PASS_READ_ONLY"
 rec.Result="SCOPED_CREATED_WINTUN_READ_ONLY_PASS"
 if err:=n4WriteHostedDriverCapability(rec);err!=nil{t.Fatal(fmt.Errorf("capability evidence: %w",err))}
 t.Log("real created Wintun LUID, IPv4 row and DNS read supported; mutating Apply/Stop and protection remain NOT_RUN")
}
