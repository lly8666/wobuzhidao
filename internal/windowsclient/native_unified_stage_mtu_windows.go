//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
)

// The MTU stage stores the *original* IPv4 interface baseline, not an alias
// or current OS pointer. The common WAL persists it before SetIpInterfaceEntry.
type n4WindowsMTUStage struct {
 Target uint32
 Writer n4WindowsMTUWriter
}
type n4MTUStageBaseline struct {
 Before N4InterfaceIPv4Snapshot `json:"before"`
 Target uint32 `json:"target"`
}
func(s n4WindowsMTUStage) Name()string{return "mtu"}
func(s n4WindowsMTUStage) currentOwner(ctx context.Context,id N4UnifiedIdentity)error{
 if err:=ctx.Err();err!=nil{return err}
 if !id.valid() || s.Writer.Adapter==nil || s.Writer.Guard==nil ||
  !s.Writer.Guard.Durable(){return ErrN4NativeGuard}
 owned,err:=s.Writer.Adapter.IsCurrentOwnedWintun(ctx,id.WintunIndex)
 if err!=nil{return err}
 if !owned{return ErrN4ForeignRoute}
 return nil
}
func(s n4WindowsMTUStage) decode(id N4UnifiedIdentity,b json.RawMessage)(n4MTUStageBaseline,error){
 var receipt n4MTUStageBaseline
 if err:=json.Unmarshal(b,&receipt);err!=nil{return receipt,ErrN4JournalConflict}
 if !id.valid() || receipt.Before.Index!=id.WintunIndex ||
  receipt.Target!=s.Target || receipt.Before.MTU==s.Target ||
  N4VerifyOwnedTunnelMTU(receipt.Before,id.WintunIndex,s.Target)!=nil{
  return receipt,ErrN4JournalConflict
 }
 return receipt,nil
}
func(s n4WindowsMTUStage) Snapshot(ctx context.Context,id N4UnifiedIdentity)(json.RawMessage,error){
 if err:=s.currentOwner(ctx,id);err!=nil{return nil,err}
 old,err:=(N4WindowsInterfaceReader{}).ReadIPv4(ctx,id.WintunIndex)
 if err!=nil{return nil,err}
 if err:=N4VerifyOwnedTunnelMTU(old,id.WintunIndex,s.Target);err!=nil{return nil,err}
 if old.MTU==s.Target{return nil,ErrNetworkPlan}
 return json.Marshal(n4MTUStageBaseline{Before:old,Target:s.Target})
}
func(s n4WindowsMTUStage) Inspect(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)(N4UnifiedState,error){
 rec,err:=s.decode(id,b);if err!=nil{return "",err}
 if err:=s.currentOwner(ctx,id);err!=nil{return "",err}
 got,err:=(N4WindowsInterfaceReader{}).ReadIPv4(ctx,id.WintunIndex)
 if err!=nil{return "",err}
 if got==rec.Before{return N4UnifiedAbsent,nil}
 expected:=rec.Before;expected.MTU=rec.Target
 if got==expected{return N4UnifiedOwned,nil}
 return N4UnifiedForeign,nil
}
func(s n4WindowsMTUStage) Apply(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 rec,err:=s.decode(id,b);if err!=nil{return err}
 if err:=s.currentOwner(ctx,id);err!=nil{return err}
 _,err=s.Writer.Apply(ctx,rec.Before,rec.Target)
 return err
}
func(s n4WindowsMTUStage) Undo(ctx context.Context,id N4UnifiedIdentity,b json.RawMessage)error{
 rec,err:=s.decode(id,b);if err!=nil{return err}
 if err:=s.currentOwner(ctx,id);err!=nil{return err}
 // Only a fully confirmed WAL "owned" marker may reach this code. OS
 // revalidation happens INSIDE Restore before any SetIpInterfaceEntry.
 if err:=s.Writer.Restore(ctx,n4MTUWriteReceipt{
  index:id.WintunIndex,before:rec.Before,after:rec.Target,confirmed:true,
 });err!=nil{return err}
 state,err:=s.Inspect(ctx,id,b)
 if err!=nil{return err}
 if state!=N4UnifiedAbsent{return ErrN4RouteAmbiguous}
 return nil
}
