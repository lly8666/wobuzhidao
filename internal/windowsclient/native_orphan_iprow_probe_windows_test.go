//go:build windows

package windowsclient

import (
 "context"
 "errors"
 "testing"

 "golang.org/x/sys/windows"
)

func TestN4WindowsOriginalIPRowProbeInvalidInputFailClosed(t *testing.T) {
 for _,tt:=range []struct{name string;idx uint32;family uint16}{
  {"zero-index",0,windows.AF_INET},
  {"unsupported-family",1,0},
  {"unsafe-family",1,999},
 }{
  t.Run(tt.name,func(t *testing.T){
   state,err:=n4ReadIPFamilyState(context.Background(),tt.idx,tt.family)
   if !errors.Is(err,ErrN4JournalConflict) || state!=""{
    t.Fatalf("invalid interface family probe: state=%s error=%v",state,err)
   }
  })
 }
 ctx,cancel:=context.WithCancel(context.Background());cancel()
 if state,err:=n4ReadIPFamilyState(ctx,1,windows.AF_INET);!errors.Is(err,context.Canceled)||state!=""{
  t.Fatalf("cancelled native probe touched OS: state=%s err=%v",state,err)
 }
 id:=n4GuardOwnedOrphanID()
 if out,err:=n4ProbeOriginalIPRows(ctx,id);!errors.Is(err,context.Canceled)||out!=(n4OriginalIPRowProbe{}){
  t.Fatalf("cancelled original identity probe touched OS: out=%+v err=%v",out,err)
 }
}
