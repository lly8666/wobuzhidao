//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "os"
)

// ErrN4GhostDNSPartialOnly means an exact-owned DNS setting was restored
// on the original, now nonfunctional Wintun PnP instance, but the remaining
// lease/MTU/guard WAL is intentionally NOT retired. It is NOT full Recover.
var ErrN4GhostDNSPartialOnly=errors.New("windowsclient: ghost Wintun DNS-only recovery; MTU/lease/guard WAL retained")

// n4RestoreGhostDNSOnly never reopens Wintun by alias, never edits another
// interface, never deletes a route or a PnP device, and NEVER disarms WFP.
// It is an independently gated, partial recovery for the exact original
// Windows Wintun after child crash: stale GAA GUID/LUID/index, pinned PnP
// instance CR_NO_SUCH_DEVNODE=13 and both IP Helper family rows missing.
// The caller-injected guard is for the hosted disposable-Wintun test. A
// production entry must additionally establish a real persistent guard.
func n4RestoreGhostDNSOnly(ctx context.Context,path string,id N4UnifiedIdentity,g CrashPersistentGuard)error{
 if err:=ctx.Err();err!=nil{return err}
 if path=="" || !id.valid() || !n4ValidPinnedPnPHash(id.PnPInstanceSHA256) ||
  g==nil || !g.Durable(){return ErrN4JournalConflict}
 release,err:=(N4WindowsMachineGate{}).Acquire(ctx)
 if err!=nil{return err}
 defer release()
 j,err:=n4UnifiedRead(path)
 if err!=nil{return err}
 // Exactly the final DNS stage of the original pre-route WAL may be
 // removed. Any route intent, altered generation or incomplete WAL vetoes
 // all writes, including if the checksum has been recalculated.
 if err:=n4ValidatePersistedPreRouteJournal(j,id);err!=nil{return err}
 if len(j.Records)!=4 || j.Records[3].Name!="dns" ||
  j.Records[3].State!="owned" {return ErrN4JournalConflict}
 if _,err:=os.Lstat(path+".routes");err==nil{return ErrN4JournalConflict
 }else if !errors.Is(err,os.ErrNotExist){return err}
 checkGhost:=func()error{
  if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return err}
  p,err:=n4ProbePinnedOriginalWintunPnP(ctx,id)
  if err!=nil{return err}
  if p.State!="EXACT_PNP_DEVNODE_GONE_13"{return ErrN4NativeGuard}
  rows,err:=n4ProbeOriginalIPRows(ctx,id)
  if err!=nil{return err}
  if rows.IPv4!="MISSING_1168" || rows.IPv6!="MISSING_1168"{
   return ErrN4NativeGuard
  }
  return nil
 }
 if err:=checkGhost();err!=nil{return err}
 ok,err:=g.Verify(ctx)
 if err!=nil{return err}
 if !ok{return ErrN4NativeGuard}
 dns,err:=n4PersistedPreRouteStage(id,j.Records[3],g)
 if err!=nil{return err}
 state,err:=dns.Inspect(ctx,id,j.Records[3].Before)
 if err!=nil{return err}
 if state!=N4UnifiedOwned && state!=N4UnifiedAbsent{return ErrN4RouteAmbiguous}
 // The WAL may be left with an 'owned' DNS record after a previous
 // successful Clear but before journal fsync. Re-read and shrink safely.
 if state==N4UnifiedOwned{
  if err:=checkGhost();err!=nil{return err}
  if err:=dns.Undo(ctx,id,j.Records[3].Before);err!=nil{return err}
 }
 state,err=dns.Inspect(ctx,id,j.Records[3].Before)
 if err!=nil{return err}
 if state!=N4UnifiedAbsent{return ErrN4RouteAmbiguous}
 if err:=checkGhost();err!=nil{return err}
 // Only AFTER independently verified DNS state is absent, persist a
 // shrinking cleaning WAL. Lease/MTU/guard intentionally stay durable.
 j.Phase="cleaning"
 j.Records=j.Records[:3]
 if err:=n4UnifiedWrite(path,j,false);err!=nil{return err}
 return ErrN4GhostDNSPartialOnly
}
