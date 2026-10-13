//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "net/netip"
 "reflect"
 "slices"
)

// Native IPv4 DNS is the fourth existing WAL stage. Never modify another
// adapter's search list, profile DNS, registration, suffix, LLMNR or IPv6.
type n4WindowsDNSStage struct {
 Servers []netip.Addr
 Guard CrashPersistentGuard
 Writer n4OwnedDNSWrite
}
type n4DNSStageBaseline struct {
 Index uint32 `json:"index"`
 Before N4DNSBaseline `json:"before"`
 Servers []netip.Addr `json:"servers"`
}
func(s n4WindowsDNSStage) Name()string{return "dns"}
func(s n4WindowsDNSStage) check(ctx context.Context,id N4UnifiedIdentity)error{
 if err:=ctx.Err();err!=nil{return err}
 if !id.valid() || s.Guard==nil || !s.Guard.Durable() ||
  s.Writer.Adapter==nil{return ErrN4NativeGuard}
 if _,_,err:=n4StaticDNSRequest(s.Servers);err!=nil{return err}
 owned,err:=s.Writer.Adapter.IsCurrentOwnedWintun(ctx,id.WintunIndex)
 if err!=nil{return err}
 if !owned{return ErrN4ForeignRoute}
 return nil
}
func(s n4WindowsDNSStage) decode(id N4UnifiedIdentity,b json.RawMessage)(n4DNSStageBaseline,error){
 var rec n4DNSStageBaseline
 if err:=json.Unmarshal(b,&rec);err!=nil{return rec,ErrN4JournalConflict}
 if !id.valid() || rec.Index!=id.WintunIndex ||
  !slices.Equal(rec.Servers,s.Servers) ||
  n4ReadOwnedDNSBaseline(rec.Before)!=nil{return rec,ErrN4JournalConflict}
 return rec,nil
}
func(s n4WindowsDNSStage) Snapshot(ctx context.Context,id N4UnifiedIdentity)(json.RawMessage,error){
 if err:=s.check(ctx,id);err!=nil{return nil,err}
 before,err:=(N4NativeDNSReader{}).Read(ctx,id.WintunIndex)
 if err!=nil{return nil,err}
 if err:=n4ReadOwnedDNSBaseline(before);err!=nil{return nil,err}
 return json.Marshal(n4DNSStageBaseline{
  Index:id.WintunIndex,Before:before,Servers:append([]netip.Addr(nil),s.Servers...),
 })
}
func n4DNSExceptStaticEqual(now,before N4DNSBaseline)bool{
 now.Flags &^=n4DNSOwnedStaticMask
 before.Flags &^=n4DNSOwnedStaticMask
 now.NameServers=nil;before.NameServers=nil
 now.RawNameServer="";before.RawNameServer=""
 return reflect.DeepEqual(now,before)
}
func n4DNSClassifyState(now N4DNSBaseline,rec n4DNSStageBaseline)N4UnifiedState{
 if !n4DNSExceptStaticEqual(now,rec.Before){return N4UnifiedForeign}
 if now.Flags&n4DNSOwnedStaticMask==0 && now.RawNameServer=="" &&
  len(now.NameServers)==0{return N4UnifiedAbsent}
 // Live Get may report Flags=0 even for a just-set NameServer.
 // Ownership is inferred only from exact parsed static addresses under
 // the independently verified created-Wintun identity and WAL baseline.
 if now.RawNameServer!="" && slices.Equal(now.NameServers,rec.Servers){
  return N4UnifiedOwned
 }
 return N4UnifiedForeign
}
func(s n4WindowsDNSStage) Inspect(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)(N4UnifiedState,error){
 rec,err:=s.decode(id,b);if err!=nil{return "",err}
 if err:=s.check(ctx,id);err!=nil{return "",err}
 now,err:=(N4NativeDNSReader{}).Read(ctx,id.WintunIndex)
 if err!=nil{return "",err}
 return n4DNSClassifyState(now,rec),nil
}
func(s n4WindowsDNSStage) Apply(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 rec,err:=s.decode(id,b);if err!=nil{return err}
 current,err:=s.Inspect(ctx,id,b);if err!=nil{return err}
 if current!=N4UnifiedAbsent{return ErrN4ForeignRoute}
 // Preceded by durable WAL intent; SetIPv4 verifies WFP and live Wintun.
 return s.Writer.SetIPv4(ctx,s.Guard,rec.Before.Interface,id.WintunIndex,s.Servers)
}
func(s n4WindowsDNSStage) Undo(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 rec,err:=s.decode(id,b);if err!=nil{return err}
 state,err:=s.Inspect(ctx,id,b);if err!=nil{return err}
 if state!=N4UnifiedOwned{return ErrN4ForeignRoute}
 // Clear requests only DNS_SETTING_NAMESERVER with nil nameserver. This
 // operation's semantics MUST be verified on an admin/Wintun capable runner;
 // no claim of success is made without independent native readback.
 if err:=s.Writer.ClearIPv4(ctx,s.Guard,rec.Before.Interface,id.WintunIndex);err!=nil{return err}
 state,err=s.Inspect(ctx,id,b)
 if err!=nil{return err}
 if state!=N4UnifiedAbsent{return ErrN4RouteAmbiguous}
 return nil
}
