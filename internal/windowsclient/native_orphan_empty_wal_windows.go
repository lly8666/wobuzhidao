//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "fmt"
 "os"
 "syscall"
 "unsafe"

 "golang.org/x/sys/windows"
)

var n4ConvertGUIDLUID=n4Iphlpapi.NewProc("ConvertInterfaceGuidToLuid")

// n4OriginalWintunAbsent verifies absence of ALL three OS native identifiers
// bound to the old, checksum-journaled created-Wintun. It never opens,
// creates, modifies or adopts a network adapter. Reused ifIndex is FOREIGN.
//
// The conversion APIs may return a platform-specific error for a retired
// interface; only documented not-found codes are treated as evidence.
func n4OriginalWintunAbsent(ctx context.Context,id N4UnifiedIdentity)error{
 if err:=ctx.Err();err!=nil{return err}
 if !id.valid() || id.AdapterAlias=="" || len(id.AdapterAlias)>128 ||
  id.WintunGUID=="" || len(id.WintunGUID)>64{return ErrN4JournalConflict}
 guid,err:=windows.GUIDFromString(id.WintunGUID)
 if err!=nil || guid.String()!=id.WintunGUID{return ErrN4JournalConflict}
 for _,proc:=range []*windows.LazyProc{n4ConvertGUIDLUID,n4ConvertInterfaceLuidIndex,n4ConvertIndexLuid}{
  if err:=proc.Find();err!=nil{return errors.Join(ErrN4NativeGuard,err)}
 }
 absent:=func(rc uintptr)bool{
  e:=syscall.Errno(rc)
  return e==syscall.Errno(2) || e==syscall.Errno(1168)
 }
 var luid uint64
 rc,_,_:=n4ConvertGUIDLUID.Call(uintptr(unsafe.Pointer(&guid)),uintptr(unsafe.Pointer(&luid)))
 if rc==0{return ErrN4ForeignRoute}
 if !absent(rc){return fmt.Errorf("%w: OS GUID lookup status=%d",ErrN4NativeGuard,rc)}
 if err:=ctx.Err();err!=nil{return err}
 var index uint32
 rc,_,_=n4ConvertInterfaceLuidIndex.Call(uintptr(unsafe.Pointer(&id.WintunLUID)),uintptr(unsafe.Pointer(&index)))
 if rc==0{return ErrN4ForeignRoute}
 if !absent(rc){return fmt.Errorf("%w: OS LUID lookup status=%d",ErrN4NativeGuard,rc)}
 if err:=ctx.Err();err!=nil{return err}
 // An index occupied by any NIC (even a different GUID) is not retired:
 // never use old route receipts to mutate an index newly owned elsewhere.
 luid=0
 rc,_,_=n4ConvertIndexLuid.Call(uintptr(id.WintunIndex),uintptr(unsafe.Pointer(&luid)))
 if rc==0{return ErrN4ForeignRoute}
 if !absent(rc){return fmt.Errorf("%w: OS ifIndex lookup status=%d",ErrN4NativeGuard,rc)}
 return ctx.Err()
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
