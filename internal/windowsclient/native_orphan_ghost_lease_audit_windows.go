//go:build windows

package windowsclient

import (
 "context"
 "encoding/json"
 "errors"
 "net/netip"
 "os"
)

// n4GhostLeaseAudit is evidence only, NOT a recover/cleanup receipt.
// In particular MIB 1168 is NOT proof that an MTU was restored. The
// persistent guard and all remaining WAL records must remain untouched.
type n4GhostLeaseAudit struct {
 Lease string
 MTU string
 Guard string
 WAL string
}

// n4AuditPinnedGhostLease uses TWO independently sourced native IPv4
// readbacks (all-address table and exact host tuple) after PnP and both
// IP-family interface rows prove the pinned original is a device ghost.
// Never opens a Wintun, changes OS state or shrinks a WAL.
func n4AuditPinnedGhostLease(ctx context.Context,path string,id N4UnifiedIdentity,g CrashPersistentGuard)(n4GhostLeaseAudit,error){
 var zero n4GhostLeaseAudit
 if err:=ctx.Err();err!=nil{return zero,err}
 if path=="" || !id.valid() || !n4ValidPinnedPnPHash(id.PnPInstanceSHA256) ||
  g==nil || !g.Durable(){return zero,ErrN4JournalConflict}
 release,err:=(N4WindowsMachineGate{}).Acquire(ctx)
 if err!=nil{return zero,err}
 defer release()
 j,err:=n4UnifiedRead(path)
 if err!=nil{return zero,err}
 if j.Identity!=id || j.Phase!="cleaning" || len(j.Records)!=3 ||
  j.Records[0].Name!="guard" || j.Records[0].State!="owned" ||
  j.Records[1].Name!="lease" || j.Records[1].State!="owned" ||
  j.Records[2].Name!="mtu" || j.Records[2].State!="owned"{
  return zero,ErrN4JournalConflict
 }
 if err:=n4ValidatePersistedPreRouteJournal(j,id);err!=nil{return zero,err}
 if _,err:=os.Lstat(path+".routes");err==nil{return zero,ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return zero,err}
 inspect:=func()error{
  if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return err}
  pin,err:=n4ProbePinnedOriginalWintunPnP(ctx,id)
  if err!=nil{return err}
  if pin.State!="EXACT_PNP_DEVNODE_GONE_13"{return ErrN4NativeGuard}
  rows,err:=n4ProbeOriginalIPRows(ctx,id)
  if err!=nil{return err}
  if rows.IPv4!="MISSING_1168" || rows.IPv6!="MISSING_1168"{
   return ErrN4NativeGuard
  }
  return nil
 }
 if err:=inspect();err!=nil{return zero,err}
 armed,err:=g.Verify(ctx)
 if err!=nil{return zero,err}
 if !armed{return zero,ErrN4NativeGuard}
 var baseline n4LeaseBaseline
 if err:=json.Unmarshal(j.Records[1].Before,&baseline);err!=nil{
  return zero,ErrN4JournalConflict
 }
 prefix,err:=netip.ParsePrefix(baseline.Address)
 if err!=nil || !baseline.Absent || baseline.Index!=id.WintunIndex ||
  prefix.Bits()!=32 || !prefix.Addr().Is4(){return zero,ErrN4JournalConflict}
 lease:=N4NativeLeaseIntent{InterfaceIndex:id.WintunIndex,Prefix:prefix}
 if err:=n4ValidateLeaseIntent(lease);err!=nil{return zero,ErrN4JournalConflict}
 // A missing IP-interface MTU row NEVER automatically means the unicast
 // address is gone: explicitly enumerate ALL IPv4 tuples for this index.
 all,err:=(N4WindowsAddressSnapshot{}).IPv4(ctx,id.WintunIndex)
 if err!=nil{return zero,err}
 if len(all)!=0{return zero,ErrN4RouteAmbiguous}
 _,present,err:=(N4WindowsLeaseReader{}).Lookup(ctx,lease)
 if err!=nil{return zero,err}
 if present{return zero,ErrN4RouteAmbiguous}
 if err:=inspect();err!=nil{return zero,err}
 armed,err=g.Verify(ctx)
 if err!=nil{return zero,err}
 if !armed{return zero,ErrN4NativeGuard}
 // Verify the same journal is still present and has NOT been advanced.
 again,err:=n4UnifiedRead(path)
 if err!=nil{return zero,err}
 if again.Identity!=id || again.Phase!="cleaning" ||
  len(again.Records)!=3 || again.Records[2].Name!="mtu"{
  return zero,ErrN4JournalConflict
 }
 return n4GhostLeaseAudit{
  Lease:"READONLY_ABSENT_BY_FULL_IPV4_TABLE_AND_EXACT_TUPLE",
  MTU:"UNRECOVERABLE_MIB_1168_NO_SET",
  Guard:"KEPT_ARMED",
  WAL:"UNCHANGED_GUARD_LEASE_MTU",
 },nil
}
