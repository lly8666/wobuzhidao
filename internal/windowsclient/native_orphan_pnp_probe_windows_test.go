//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "testing"
)

func TestN4WindowsOriginalPnPProbeRejectsMissingOwnerAndCancel(t *testing.T){
 id:=n4GuardOwnedOrphanID()
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if out,err:=n4ProbeOriginalWintunPnP(ctx,id);!errors.Is(err,context.Canceled)||out!=(n4OriginalPnPProbe{}){
  t.Fatalf("cancelled PnP probe queried OS: %+v %v",out,err)
 }
 bad:=id
 bad.WintunGUID=""
 if out,err:=n4ProbeOriginalWintunPnP(context.Background(),bad);!errors.Is(err,ErrN4JournalConflict)||out!=(n4OriginalPnPProbe{}){
  t.Fatalf("missing original GUID accepted: %+v %v",out,err)
 }
 // Synthetic original cannot be matched to a real Wintun by alias; this
 // must never enumerate unrelated devnodes or acquire a new adapter.
 if out,err:=n4ProbeOriginalWintunPnP(context.Background(),id);err==nil || out!=(n4OriginalPnPProbe{}){
  t.Fatalf("synthetic original unexpectedly authorized PnP node: %+v %v",out,err)
 }
}
