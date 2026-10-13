//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "testing"
)
func TestN4WintunHandleOwnershipNeverBasedOnAlias(t *testing.T){
 borrowed:=&TUN{name:"WBD",createdByProcess:false}
 for _,idx:=range []uint32{1,12,5000}{
  ok,err:=borrowed.IsCurrentOwnedWintun(context.Background(),idx)
  if err!=nil || ok {t.Fatalf("opened foreign Wintun alias promoted to owner: %v/%v",ok,err)}
 }
 if _,err:=borrowed.IsCurrentOwnedWintun(context.Background(),0);!errors.Is(err,ErrInvalidBinding){
  t.Fatalf("zero interface must fail closed: %v",err)
 }
 createdButNoHandle:=&TUN{name:"WBD",createdByProcess:true}
 if _,err:=createdButNoHandle.IsCurrentOwnedWintun(context.Background(),7);!errors.Is(err,ErrInvalidBinding){
  t.Fatalf("process-created marker without live adapter handle accepted: %v",err)
 }
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if _,err:=createdButNoHandle.IsCurrentOwnedWintun(ctx,7);!errors.Is(err,context.Canceled){
  t.Fatalf("cancel not enforced: %v",err)
 }
 if _,err:=borrowed.NativeIdentity(context.Background());!errors.Is(err,ErrInvalidBinding){
  t.Fatalf("no native Wintun handle unexpectedly accepted: %v",err)
 }
}
func TestN4WintunAdapterClaimDoesNotSurviveClosedSession(t *testing.T){
 tnl:=&TUN{name:"WBD",createdByProcess:true,adapter:17,session:18}
 // No real handle calls: mark closed first to refuse before Win DLL lookup.
 tnl.stateMu.Lock();tnl.closed=true;tnl.stateMu.Unlock()
 if _,err:=tnl.IsCurrentOwnedWintun(context.Background(),17);!errors.Is(err,ErrInvalidBinding){
  t.Fatalf("closed adapter still considered owned: %v",err)
 }
 // WintunGetAdapterLuid is not executed by this hosted test.
}
