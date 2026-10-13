//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "testing"
)

func TestN4WindowsWintunJournalProofRejectsUnboundLegacyAndAliasBeforeNative(t *testing.T){
 id:=n4TestConcreteNativeIdentity()
 id.AdapterAlias="WBDN4-JOURNAL"
 id.WintunGUID="11111111-2222-3333-4444-555555555555"
 // This negative suite uses no real adapter; it cannot mutate host state.
 if err:=n4VerifyJournalBoundLiveWintun(context.Background(),nil,"unused",id);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("nil borrowed adapter accepted: %v",err)
 }
 borrowed:=&TUN{name:"foreign-name",createdByProcess:false}
 if err:=n4VerifyJournalBoundLiveWintun(context.Background(),borrowed,"unused",id);!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("foreign alias accepted: %v",err)
 }
 legacy:=id;legacy.WintunGUID=""
 if err:=n4VerifyJournalBoundLiveWintun(context.Background(),borrowed,"unused",legacy);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("legacy no GUID accepted: %v",err)
 }
 legacy=id;legacy.AdapterAlias=""
 if err:=n4VerifyJournalBoundLiveWintun(context.Background(),borrowed,"unused",legacy);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("legacy no adapter alias accepted: %v",err)
 }
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if err:=n4VerifyJournalBoundLiveWintun(ctx,borrowed,"unused",id);!errors.Is(err,context.Canceled){
  t.Fatalf("cancelled proof touched native API: %v",err)
 }
}
