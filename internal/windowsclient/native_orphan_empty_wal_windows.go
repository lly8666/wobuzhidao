//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "os"
 "unsafe"

 "golang.org/x/sys/windows"
)

// n4OriginalWintunAbsent never interprets conversion error 87 as proof
// of absence: ConvertInterfaceGuidToLuid may report INVALID_PARAMETER after
// removal. A complete read-only GetIfTable2Ex snapshot independently
// enumerates GUID, LUID and index for every live OS network interface.
func n4OriginalWintunAbsent(ctx context.Context,id N4UnifiedIdentity)error{
 if err:=ctx.Err();err!=nil{return err}
 if !id.valid() || id.AdapterAlias=="" || len(id.AdapterAlias)>128 ||
  id.WintunGUID=="" || len(id.WintunGUID)>64{return ErrN4JournalConflict}
 guid,err:=windows.GUIDFromString(id.WintunGUID)
 if err!=nil || guid.String()!=id.WintunGUID{return ErrN4JournalConflict}
 var table *windows.MibIfTable2
 if err:=windows.GetIfTable2Ex(0,&table);err!=nil{return errors.Join(ErrN4NativeGuard,err)}
 if table==nil{return ErrN4NativeGuard}
 defer windows.FreeMibTable(unsafe.Pointer(table))
 // An implausibly large or uninspectable native table is not proof that a
 // crashed NIC vanished. Bound this read-only scan, never truncate it.
 if table.NumEntries>4096{return ErrN4NativeGuard}
 for _,row:=range unsafe.Slice(&table.Table[0],int(table.NumEntries)){
  if err:=ctx.Err();err!=nil{return err}
  if row.InterfaceGuid==guid || row.InterfaceLuid==id.WintunLUID ||
   row.InterfaceIndex==id.WintunIndex {return ErrN4ForeignRoute}
 }
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
