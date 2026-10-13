//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "net/netip"
 "os"

 "golang.org/x/sys/windows"
)

// N4WindowsRetirePreRouteOrphanWAL is a crash-only, no-alias-adoption
// extension for a destroyed process-created Wintun whose parent WAL
// advanced through lease/MTU/DNS but NEVER began any route stage.
// Windows removes these interface-local settings with the original NIC.
// A physical underlay/server /32 can only have been touched by stage 5;
// ANY route receipt or route stage is therefore an absolute veto.
//
// This function never deletes a route or alters a NIC. It may delete only
// exact owned WFP objects after every pre-route journal constraint and OS
// adapter absence are proven. Production Arm and full-network Apply stay
// disabled until separately qualified.
func N4WindowsRetirePreRouteOrphanWAL(ctx context.Context,path string,id N4UnifiedIdentity)error{
 g:=&n4WindowsPersistentWFPGuard{
  InstallationID:id.Installation,WintunLUID:id.WintunLUID,
  ExclusiveSingleWintun:true, // selects old crash-stable IDs; NOT Arm permission
 }
 return n4RetirePreRouteOrphanWAL(ctx,path,id,g)
}

func n4ValidatePreRouteOrphanReceipts(j n4UnifiedJournal,id N4UnifiedIdentity)error{
 if j.Identity!=id || j.Phase!="applying" ||
  len(j.Records)<2 || len(j.Records)>4 ||
  j.Records[0].Name!="guard" || j.Records[0].State!="owned" ||
  string(j.Records[0].Before)!=`{"absent":true}`{
  return ErrN4JournalConflict
 }
 // No route stage is permissible. All records before the last must have
 // been confirmed, per the single WAL's original sequential apply order.
 for i,r:=range j.Records{
  if r.Name!=n4UnifiedOrder[i] ||
   (r.State!="owned" && !(i==len(j.Records)-1 && r.State=="intent")){
   return ErrN4JournalConflict
  }
 }
 var lease n4LeaseBaseline
 if err:=json.Unmarshal(j.Records[1].Before,&lease);err!=nil{return ErrN4JournalConflict}
 prefix,err:=netip.ParsePrefix(lease.Address)
 if err!=nil || !prefix.Addr().Is4() || prefix.Bits()!=32 ||
  !lease.Absent || lease.Index!=id.WintunIndex{return ErrN4JournalConflict}
 if len(j.Records)>2 {
  var mtu n4MTUStageBaseline
  if err:=json.Unmarshal(j.Records[2].Before,&mtu);err!=nil{return ErrN4JournalConflict}
  if mtu.Before.Index!=id.WintunIndex || mtu.Before.MTU==mtu.Target ||
   N4VerifyOwnedTunnelMTU(mtu.Before,id.WintunIndex,mtu.Target)!=nil{
   return ErrN4JournalConflict
  }
 }
 if len(j.Records)>3{
  var dns n4DNSStageBaseline
  if err:=json.Unmarshal(j.Records[3].Before,&dns);err!=nil{return ErrN4JournalConflict}
  guid,err:=windows.GUIDFromString(id.WintunGUID)
  if err!=nil || guid.String()!=id.WintunGUID ||
   dns.Index!=id.WintunIndex || dns.Before.Interface!=guid ||
   n4ReadOwnedDNSBaseline(dns.Before)!=nil{return ErrN4JournalConflict}
  if _,_,err:=n4StaticDNSRequest(dns.Servers);err!=nil{return ErrN4JournalConflict}
 }
 return nil
}

func n4RetirePreRouteOrphanWAL(ctx context.Context,path string,id N4UnifiedIdentity,g n4OwnedOrphanGuard)error{
 if err:=ctx.Err();err!=nil{return err}
 if path=="" || !id.valid() || id.WintunGUID=="" ||
  id.AdapterAlias=="" || g==nil{return ErrN4JournalConflict}
 release,err:=(N4WindowsMachineGate{}).Acquire(ctx)
 if err!=nil{return err}
 defer release()
 j,err:=n4UnifiedRead(path)
 if errors.Is(err,os.ErrNotExist){return nil} // idempotent after own success
 if err!=nil{return err}
 if err:=n4ValidatePreRouteOrphanReceipts(j,id);err!=nil{return err}
 // Even an unreadable sidecar means route writes MIGHT have occurred.
 if _,err:=os.Lstat(path+".routes");err==nil{return ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return err}
 // A recycled ifIndex belongs to another owner. Native full table scan
 // verifies GUID, LUID AND both IPv4/IPv6 interface indices are absent.
 if err:=n4OriginalWintunAbsent(ctx,id);err!=nil{return err}
 state,err:=g.ownedState(ctx)
 if err!=nil{return err} // includes any foreign/altered WFP object
 switch state{
 case N4UnifiedOwned,N4UnifiedPartial:
  // Safe to finish an interrupted exact-ID WFP deletion: there are no
  // underlay/capture routes to leak, and original interface-local settings
  // vanished with the exact dead adapter.
  if err:=g.DisarmAfterRoutesRemoved(ctx);err!=nil{return err}
 case N4UnifiedAbsent:
  // Prior cleanup already removed the owned guard.
 default:return ErrN4NativeGuard
 }
 if err:=ctx.Err();err!=nil{return err}
 state,err=g.ownedState(ctx)
 if err!=nil{return err}
 if state!=N4UnifiedAbsent{return ErrN4NativeGuard}
 if err:=n4OriginalWintunAbsent(ctx,id);err!=nil{return err}
 // A sidecar that appeared during inspection must not be discarded.
 if _,err:=os.Lstat(path+".routes");err==nil{return ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return err}
 return os.Remove(path)
}
