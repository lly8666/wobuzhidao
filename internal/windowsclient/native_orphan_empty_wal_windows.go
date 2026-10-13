//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "os"
 "unsafe"

 "golang.org/x/sys/windows"
)

// n4OriginalWintunAbsent enumerates all adapters with the existing
// GetAdaptersAddresses Win32 binding in the pinned x/sys version. The newer
// Go GetIfTable2Ex wrappers are NOT present in pinned x/sys v0.33.0.
// No GUID conversion error (especially 87) is evidence of OS absence.
func n4OriginalWintunAbsent(ctx context.Context,id N4UnifiedIdentity)error{
 if err:=ctx.Err();err!=nil{return err}
 if !id.valid() || id.AdapterAlias=="" || len(id.AdapterAlias)>128 ||
  id.WintunGUID=="" || len(id.WintunGUID)>64{return ErrN4JournalConflict}
 guid,err:=windows.GUIDFromString(id.WintunGUID)
 if err!=nil || guid.String()!=id.WintunGUID{return ErrN4JournalConflict}
 // GAA_FLAG_INCLUDE_ALL_INTERFACES plus skip optional address lists. Do not
 // skip down/unbound interfaces which might be the original Wintun.
 const flags uint32=0x100 | 0x1 | 0x2 | 0x4 | 0x8 | 0x20
 const maxBytes=1024*1024
 buf:=make([]byte,16*1024)
 for attempt:=0;attempt<3;attempt++{
  if err:=ctx.Err();err!=nil{return err}
  size:=uint32(len(buf))
  first:=(*windows.IpAdapterAddresses)(unsafe.Pointer(&buf[0]))
  err:=windows.GetAdaptersAddresses(0,flags,0,first,&size)
  if errors.Is(err,windows.ERROR_BUFFER_OVERFLOW){
   if size>maxBytes || int(size)<=len(buf){return ErrN4NativeGuard}
   buf=make([]byte,int(size))
   continue
  }
  if err!=nil{return errors.Join(ErrN4NativeGuard,err)}
  count:=0
  for row:=first;row!=nil;row=row.Next{
   if err:=ctx.Err();err!=nil{return err}
   count++
   if count>4096{return ErrN4NativeGuard}
   if row.IfIndex==id.WintunIndex || row.Ipv6IfIndex==id.WintunIndex ||
    row.Luid==id.WintunLUID{return ErrN4ForeignRoute}
   if row.AdapterName==nil{return ErrN4NativeGuard}
   name:=""
   foundEnd:=false
   for i:=uintptr(0);i<128;i++{
    ch:=*(*byte)(unsafe.Add(unsafe.Pointer(row.AdapterName),i))
    if ch==0{foundEnd=true;break}
    name+=string(ch)
   }
   if !foundEnd{return ErrN4NativeGuard}
   adapterGUID,parseErr:=windows.GUIDFromString(name)
   if parseErr!=nil{return ErrN4NativeGuard}
   if adapterGUID==guid{return ErrN4ForeignRoute}
  }
  return ctx.Err()
 }
 return ErrN4NativeGuard
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
 if j.Identity!=id || j.Phase!="applying" || len(j.Records)!=0{
  return ErrN4JournalConflict
 }
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
