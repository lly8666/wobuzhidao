//go:build windows

package windowsclient

import (
 "context"
)

// n4VerifyJournalBoundLiveWintun is strictly READ-ONLY corroboration of a
// Windows adapter against ONE bounded, checksum-validated native unified WAL.
// It does NOT grant native Apply/Recover ownership: a fully qualified owner
// reservation, generation fencing, host-exclusive gate and CAS are still
// necessary. A caller-provided GUID, alias or generation alone is never
// sufficient, including when an adapter is reopened after a process crash.
//
// v1 journals without an explicit OS-observed GUID+alias MUST fail closed,
// not be silently promoted to safe recovered ownership.
func n4VerifyJournalBoundLiveWintun(ctx context.Context, tun *TUN, walPath string, expected N4UnifiedIdentity)error{
 if err:=ctx.Err();err!=nil{return err}
 if !expected.valid() || walPath=="" ||
  expected.AdapterAlias=="" || len(expected.AdapterAlias)>128 ||
  expected.WintunGUID=="" || len(expected.WintunGUID)>64{
  return ErrN4JournalConflict
 }
 if tun==nil || tun.Name()!=expected.AdapterAlias{return ErrN4ForeignRoute}
 journal,err:=n4UnifiedRead(walPath)
 if err!=nil{return err}
 if journal.Identity!=expected{return ErrN4JournalConflict}
 live,err:=tun.NativeIdentity(ctx)
 if err!=nil{return err}
 if live.Index!=expected.WintunIndex || live.LUID!=expected.WintunLUID{
  return ErrN4ForeignRoute
 }
 // Win32 independently resolves live interface index through IP Helper; it
 // cannot be satisfied by a borrowed same-name Wintun with a different GUID.
 actual,err:=n4NativeInterfaceGUID(live.Index)
 if err!=nil{return err}
 if actual.String()!=expected.WintunGUID{return ErrN4ForeignRoute}
 return nil
}
