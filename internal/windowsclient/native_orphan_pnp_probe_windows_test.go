//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "testing"
 "strings"
 "path/filepath"
 "os"
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

func TestN4WindowsPinnedOriginalPnPGenerationBindingRejectsForeign(t *testing.T){
 id:=n4GuardOwnedOrphanID()
 original:=strings.Repeat("a",64)
 observed:=n4OriginalPnPProbe{State:"EXACT_DEVNODE_NO_PROBLEM",InstanceSHA256:original}
 if err:=n4RequireOriginalPnPBinding(id,observed);!errors.Is(err,ErrN4JournalConflict){
  t.Fatalf("legacy WAL acquired PnP orphan authority: %v",err)
 }
 id.PnPInstanceSHA256=original
 for _,state:=range []string{"EXACT_DEVNODE_NO_PROBLEM","EXACT_DEVNODE_PROBLEM","EXACT_PNP_DEVNODE_GONE_13"}{
  observed.State=state
  if err:=n4RequireOriginalPnPBinding(id,observed);err!=nil{
   t.Fatalf("same pinned original PnP identity state=%s rejected: %v",state,err)
  }
 }
 for _,other:=range []string{strings.Repeat("b",64),strings.Repeat("A",64),"",strings.Repeat("g",64)}{
  observed.InstanceSHA256=other
  if err:=n4RequireOriginalPnPBinding(id,observed);err==nil{
   t.Fatal("foreign or malformed instance fingerprint was accepted")
  }
 }
 observed.InstanceSHA256=original
 observed.State="UNKNOWN"
 if err:=n4RequireOriginalPnPBinding(id,observed);!errors.Is(err,ErrN4NativeGuard){
  t.Fatalf("unrecognized SetupAPI state became deletion authority: %v",err)
 }
}

func TestN4WindowsPinnedPnPBindingDurableInOriginalWAL(t *testing.T){
 id:=n4GuardOwnedOrphanID()
 id.PnPInstanceSHA256=strings.Repeat("c",64)
 path:=filepath.Join(t.TempDir(),"original-pnp-bound.json")
 j:=n4UnifiedJournal{Schema:n4UnifiedSchema,Identity:id,Phase:"applying"}
 if err:=n4UnifiedWrite(path,j,true);err!=nil{t.Fatal(err)}
 read,err:=n4UnifiedRead(path)
 if err!=nil || read.Identity!=id{
  t.Fatalf("original instance SHA not sealed in same generation WAL: %v",err)
 }
 foreign:=read
 foreign.Identity.PnPInstanceSHA256=strings.Repeat("d",64)
 if err:=n4UnifiedWrite(path,foreign,false);err!=nil{t.Fatal(err)}
 updated,err:=n4UnifiedRead(path)
 if err!=nil{t.Fatal(err)}
 if updated.Identity==id {t.Fatal("test failed to modify the identity")}
 if err:=n4RequireOriginalPnPBinding(updated.Identity,n4OriginalPnPProbe{
  State:"EXACT_PNP_DEVNODE_GONE_13",InstanceSHA256:id.PnPInstanceSHA256,
 });!errors.Is(err,ErrN4ForeignRoute){
  t.Fatalf("checksum-valid foreign fingerprint was accepted: %v",err)
 }
 if _,err:=os.Stat(path);err!=nil{t.Fatal(err)}
 for _,bad:=range []string{"","A"+strings.Repeat("a",63),strings.Repeat("f",63),strings.Repeat("z",64)}{
  modified:=id
  modified.PnPInstanceSHA256=bad
  if bad==""{
   if n4ValidPinnedPnPHash(bad){t.Fatal("legacy missing pin accepted as new authority")}
   continue
  }
  if modified.valid(){t.Fatalf("malformed original PnP fingerprint accepted: %s",bad)}
 }
}

func TestN4WindowsUnkeyedGhostSkipOnlyOnUnboundCMNoSuchDevnode(t *testing.T){
 for _,tt:=range []struct{rc uintptr;pinned bool;skip bool}{
  {13,false,true},
  {13,true,false},
  {0,false,false},
  {0,true,false},
  {19,false,false},
  {5,false,false},
  {259,false,false},
 }{
  got:=n4MaySkipUnkeyedGhost(tt.rc,tt.pinned)
  if got!=tt.skip{t.Fatalf("unkeyed ghost rc=%d pinned=%t skip=%t; want %t",tt.rc,tt.pinned,got,tt.skip)}
 }
}
