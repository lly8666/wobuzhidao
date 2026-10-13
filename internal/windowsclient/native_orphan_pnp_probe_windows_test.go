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

func TestN4WindowsCMDevnodeStateReadOnlyIsNotDeletionPermission(t *testing.T){
 for _,tt:=range []struct{rc uintptr;status uint32;want string;err bool}{
  {0,0,"EXACT_DEVNODE_NO_PROBLEM",false},
  {0,0x400,"EXACT_DEVNODE_PROBLEM",false},
  {13,0,"EXACT_PNP_DEVNODE_GONE_13",false},
  {13,0x400,"EXACT_PNP_DEVNODE_GONE_13",false},
  {5,0,"",true},
  {19,0,"",true},
 }{
  got,err:=n4ClassifyCMDevNode(tt.rc,tt.status)
  if got!=tt.want || (err!=nil)!=tt.err{
   t.Errorf("ConfigManager rc=%d status=%x classified %q err=%v; want %q err=%v",tt.rc,tt.status,got,err,tt.want,tt.err)
  }
 }
}

func TestN4WindowsProductionPersistedOriginalRollbackNeedsPnPPin(t *testing.T){
 id:=n4GuardOwnedOrphanID()
 // Production must refuse a legacy pre-PnP journal BEFORE reading any
 // OS interface, installing WFP, or mutating the network.
 path:=filepath.Join(t.TempDir(),"legacy-parent.json")
 if err:=N4WindowsRecoverPersistedPreRouteWAL(context.Background(),path,id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("unpinned old-generation journal entered production rollback: %v",err)
 }
 id.PnPInstanceSHA256="UPPERCASE"+strings.Repeat("a",55)
 if err:=N4WindowsRecoverPersistedPreRouteWAL(context.Background(),path,id);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("malformed PnP identity accepted for production rollback: %v",err)
 }
}
