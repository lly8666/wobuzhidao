//go:build windows

package windowsclient

import (
 "context"
 "sync"
)

// N4WindowsCreatedWintunReservation pins the CURRENT process-created Wintun
// session across an entire unified native network Apply/Recover. A borrowed
// same-name adapter, a stale LUID/index, a closed session, or a different
// installation/generation must never grant ownership.
//
// This is a low-level owner fence only. The product must still independently
// authenticate its installation/generation at admission and use all native
// stage-specific CAS readers and a cross-process Windows machine gate.
type N4WindowsCreatedWintunReservation struct {
 Tunnel *TUN
 Installation [16]byte
 Generation [16]byte
}

func (o N4WindowsCreatedWintunReservation) checkInputs(ctx context.Context,id N4UnifiedIdentity) error {
 if err:=ctx.Err();err!=nil{return err}
 if !id.valid() || o.Installation!=id.Installation ||
  o.Generation!=id.Generation || o.Tunnel==nil ||
  !o.Tunnel.createdByProcess {return ErrN4ForeignRoute}
 // Half-bound WAL identity never grants native ownership. Older in-memory
 // fixtures omit BOTH fields; production assembler requires both.
 if (id.WintunGUID=="") != (id.AdapterAlias=="") {return ErrN4JournalConflict}
 if id.AdapterAlias!="" {
  if len(id.AdapterAlias)>128 || len(id.WintunGUID)>64 ||
   o.Tunnel.Name()!=id.AdapterAlias {return ErrN4ForeignRoute}
 }
 return nil
}

// Verify does not hold the handle AFTER it returns; the unified transaction
// calls Hold when available, which keeps TUN.Close waiting until release.
func (o N4WindowsCreatedWintunReservation) Verify(ctx context.Context,id N4UnifiedIdentity) error {
 if err:=o.checkInputs(ctx,id);err!=nil{return err}
 actual,err:=o.Tunnel.NativeIdentity(ctx)
 if err!=nil{return err}
 if !actual.CreatedByProcess || actual.Index!=id.WintunIndex ||
  actual.LUID!=id.WintunLUID {return ErrN4ForeignRoute}
 if id.WintunGUID!="" {
  guid,err:=n4NativeInterfaceGUID(actual.Index)
  if err!=nil{return err}
  if guid.String()!=id.WintunGUID{return ErrN4ForeignRoute}
 }
 return nil
}

func (o N4WindowsCreatedWintunReservation) Hold(ctx context.Context,id N4UnifiedIdentity)(func(),error){
 if err:=o.checkInputs(ctx,id);err!=nil{return nil,err}
 if !o.Tunnel.beginCall(){return nil,ErrInvalidBinding}
 if err:=o.Verify(ctx,id);err!=nil{
  o.Tunnel.endCall()
  return nil,err
 }
 var once sync.Once
 return func(){once.Do(o.Tunnel.endCall)},nil
}
