//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "os"
 "unsafe"

 "golang.org/x/sys/windows"
)

// n4InspectOriginalWintun enumerates every live Win32 interface without
// opening or adopting a Wintun handle. It distinguishes the SAME surviving
// old GUID+LUID+ifIndex (owned), true absence, and ambiguous/reused identity
// (foreign/error). A mere display alias or one matching index is NEVER proof
// of an owned adapter. No mutation or key material is involved.
func n4InspectOriginalWintun(ctx context.Context,id N4UnifiedIdentity)(N4UnifiedState,error){
 if err:=ctx.Err();err!=nil{return "",err}
 if !id.valid() || id.AdapterAlias=="" || len(id.AdapterAlias)>128 ||
  id.WintunGUID=="" || len(id.WintunGUID)>64{return "",ErrN4JournalConflict}
 guid,err:=windows.GUIDFromString(id.WintunGUID)
 if err!=nil || guid.String()!=id.WintunGUID{return "",ErrN4JournalConflict}
 // GAA_FLAG_INCLUDE_ALL_INTERFACES + skip optional address lists. It is
 // essential to enumerate down/unbound NICS rather than infer disappearance.
 const flags uint32=0x100 | 0x1 | 0x2 | 0x4 | 0x8 | 0x20
 const maxBytes=1024*1024
 buf:=make([]byte,16*1024)
 for attempt:=0;attempt<3;attempt++{
  if err:=ctx.Err();err!=nil{return "",err}
  size:=uint32(len(buf))
  first:=(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
  err:=windows.GetAdaptersAddresses(0,flags,0,first,&size)
  if errors.Is(err,windows.ERROR_BUFFER_OVERFLOW){
   if size>maxBytes || int(size)<=len(buf){return "",ErrN4NativeGuard}
   buf=make([]byte,int(size))
   continue
  }
  if err!=nil{return "",errors.Join(ErrN4NativeGuard,err)}
  count,matched:=0,false
  for row:=first;row!=nil;row=row.Next{
   if err:=ctx.Err();err!=nil{return "",err}
   count++
   if count>4096 || row.AdapterName==nil{return "",ErrN4NativeGuard}
   name:=""
   end:=false
   for i:=uintptr(0);i<128;i++{
    ch:=*(*byte)(unsafe.Add(unsafe.Pointer(row.AdapterName),i))
    if ch==0{end=true;break}
    name+=string(ch)
   }
   if !end{return "",ErrN4NativeGuard}
   observed,parseErr:=windows.GUIDFromString(name)
   if parseErr!=nil{return "",ErrN4NativeGuard}
   hit:=observed==guid || row.IfIndex==id.WintunIndex ||
    row.Ipv6IfIndex==id.WintunIndex || row.Luid==id.WintunLUID
   if !hit{continue}
   if matched || observed!=guid || row.Luid!=id.WintunLUID ||
    row.IfIndex!=id.WintunIndex {
    return N4UnifiedForeign,ErrN4ForeignRoute
   }
   matched=true
  }
  if matched{return N4UnifiedOwned,nil}
  return N4UnifiedAbsent,nil
 }
 return "",ErrN4NativeGuard
}

// n4OriginalWintunAbsent is the strict dead-NIC precondition for WAL-only
// cleanup. It never treats converter status=87 or a reused ifIndex as absent.
func n4OriginalWintunAbsent(ctx context.Context,id N4UnifiedIdentity)error{
 state,err:=n4InspectOriginalWintun(ctx,id)
 if err!=nil{return err}
 if state!=N4UnifiedAbsent{return ErrN4ForeignRoute}
 return nil
}

// n4OriginalWintunPersistedExact is READ-ONLY. It does NOT authorize WFP,
// lease, MTU, DNS, route mutation, WintunOpenAdapter or interface takeover.
// A future crash recovery must prove its own per-stage write authorization.
func n4OriginalWintunPersistedExact(ctx context.Context,id N4UnifiedIdentity)error{
 state,err:=n4InspectOriginalWintun(ctx,id)
 if err!=nil{return err}
 if state!=N4UnifiedOwned{return ErrN4NativeGuard}
 return nil
}

// N4WindowsRetireEmptyOrphanWAL handles ONLY the crash gap after an empty,
// fsynced native WAL was created but BEFORE any guard/lease/MTU/DNS/route
// intent. Such a WAL authorizes no OS mutations. On a fresh process,
// deleting it is safe only with machine-wide exclusion, exact checksum and
// generation identity, proven ABSENCE of the original GUID/LUID/index, and
// a separate read-only check that no installation-owned WFP guard remains.
//
// This method NEVER calls WFP Arm/Disarm, route deletion, Windows IP/DNS
// setters or OpenTUN. A nonempty/ambiguous WAL is deliberately retained;
// later production recovery must resolve all five stages separately.
func N4WindowsRetireEmptyOrphanWAL(ctx context.Context,path string,id N4UnifiedIdentity)error{
 return n4WindowsRetireBeforeFirstOSWrite(ctx,path,id,false)
}

// N4WindowsRetireGuardIntentOrphanWAL is a deliberately narrower extension
// for a crash AFTER persisting guard intent but BEFORE confirmation. It is
// not native five-stage crash recovery, and can ONLY remove the WAL if the
// guard was never installed (including partial owned WFP objects).
// A lease/MTU/DNS/route intent or an owned guard is never discarded.
func N4WindowsRetireGuardIntentOrphanWAL(ctx context.Context,path string,id N4UnifiedIdentity)error{
 return n4WindowsRetireBeforeFirstOSWrite(ctx,path,id,true)
}

func n4WindowsRetireBeforeFirstOSWrite(ctx context.Context,path string,id N4UnifiedIdentity,guardIntent bool)error{
 if err:=ctx.Err();err!=nil{return err}
 if path=="" || !id.valid() || id.WintunGUID=="" || id.AdapterAlias==""{
  return ErrN4JournalConflict
 }
 release,err:=(N4WindowsMachineGate{}).Acquire(ctx)
 if err!=nil{return err}
 defer release()
 j,err:=n4UnifiedRead(path)
 if errors.Is(err,os.ErrNotExist){return nil} // repeated Stop is idempotent
 if err!=nil{return err}
 if j.Identity!=id || j.Phase!="applying" {return ErrN4JournalConflict}
 switch len(j.Records){
 case 0: // fsynced WAL, no stage intent, no OS mutation
 case 1:
  if !guardIntent || j.Records[0].Name!="guard" ||
   j.Records[0].State!="intent" ||
   string(j.Records[0].Before)!=`{"absent":true}` {return ErrN4JournalConflict}
 default:
  return ErrN4JournalConflict
 }
 // Even an empty parent WAL must not discard evidence of an independent
 // per-route write-ahead journal: its presence always makes the host
 // network state ambiguous. Do not inspect/delete that sidecar here.
 if _,err:=os.Lstat(path+".routes");err==nil{return ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return err}
 if err:=n4OriginalWintunAbsent(ctx,id);err!=nil{return err}
 // ExclusiveSingleWintun is NOT a caller assertion here: this locally
 // constructed value selects an existing rule PLAN for read-only inspection.
 // It NEVER permits Arm or grants a new adapter owner capability.
 g:=&n4WindowsPersistentWFPGuard{
  InstallationID:id.Installation,WintunLUID:id.WintunLUID,
  ExclusiveSingleWintun:true,
 }
 state,err:=g.ownedState(ctx)
 if err!=nil{return err}
 if state!=N4UnifiedAbsent{return ErrN4NativeGuard}
 if err:=ctx.Err();err!=nil{return err}
 return os.Remove(path)
}
