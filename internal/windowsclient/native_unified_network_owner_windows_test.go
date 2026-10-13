//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "testing"
)

func n4WindowsReservationFixture() (N4WindowsCreatedWintunReservation,N4UnifiedIdentity){
 var id N4UnifiedIdentity
 id.Installation[0]=91;id.Generation[0]=73
 id.WintunIndex=15;id.WintunLUID=0x123456
 return N4WindowsCreatedWintunReservation{
  Tunnel:&TUN{name:"WBD",createdByProcess:false},
  Installation:id.Installation,Generation:id.Generation,
 },id
}
func TestN4WindowsWintunReservationNeverPromotesBorrowedName(t *testing.T){
 owner,id:=n4WindowsReservationFixture()
 if err:=owner.Verify(context.Background(),id);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("borrowed same-name adapter granted owner: %v",err)
 }
 release,err:=owner.Hold(context.Background(),id)
 if release!=nil || !errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("borrowed adapter pinned: %v %v",release!=nil,err)
 }
 // Fails before attempting to load Wintun DLL or touching NIC.
}
func TestN4WindowsWintunReservationRejectsStaleGenerationAndClosedHandle(t *testing.T){
 owner,id:=n4WindowsReservationFixture()
 owner.Tunnel.createdByProcess=true
 wrong:=id;wrong.Generation[0]++
 if _,err:=owner.Hold(context.Background(),wrong);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("stale generation was trusted: %v",err)
 }
 wrong=id;wrong.Installation[0]++
 if err:=owner.Verify(context.Background(),wrong);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("foreign installation was trusted: %v",err)
 }
 // No live adapter/session pointer: the beginCall fence can be taken but
 // NativeIdentity must reject BEFORE WintunGetAdapterLuid symbol lookup.
 if release,err:=owner.Hold(context.Background(),id);release!=nil || !errors.Is(err,ErrInvalidBinding){
  t.Fatalf("created marker without handle trusted: %v %v",release!=nil,err)
 }
 owner.Tunnel.stateMu.Lock();owner.Tunnel.closed=true;owner.Tunnel.stateMu.Unlock()
 if release,err:=owner.Hold(context.Background(),id);release!=nil || !errors.Is(err,ErrInvalidBinding){
  t.Fatalf("closed adapter trusted: %v %v",release!=nil,err)
 }
}
func TestN4WindowsWintunReservationCancelBeforeDriver(t *testing.T){
 owner,id:=n4WindowsReservationFixture()
 owner.Tunnel.createdByProcess=true
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if release,err:=owner.Hold(ctx,id);release!=nil || !errors.Is(err,context.Canceled){
  t.Fatalf("cancelled owner pin unexpectedly succeeded: %v %v",release!=nil,err)
 }
}

func TestN4WindowsReservationRejectsPartialJournalIdentityAndForeignAlias(t *testing.T){
 owner,id:=n4WindowsReservationFixture()
 owner.Tunnel.createdByProcess=true
 id.WintunGUID="11111111-2222-3333-4444-555555555555"
 if err:=owner.Verify(context.Background(),id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("GUID-only journal gained owner: %v",err)
 }
 id.AdapterAlias="FOREIGN"
 if err:=owner.Verify(context.Background(),id);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("foreign alias gained owner: %v",err)
 }
 id.AdapterAlias="WBD";id.WintunGUID=""
 if err:=owner.Verify(context.Background(),id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("alias-only journal gained owner: %v",err)
 }
}
