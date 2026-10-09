package datapath

import (
 "testing"
 "time"
)

// The runtime CLI on Linux, Windows and server uses these fixed defaults.
// No new CLI knob or per-packet size-specific timer may be introduced.
func TestE1UniformFECDefault32msAllFixedProfiles(t *testing.T) {
 for _,p:=range []int{4,8,10,12,16,20} {
  window,maxBlocks,err:=FixedFECRuntimeDefaults(p)
  if err!=nil {t.Fatal(err)}
  if window!=32*time.Millisecond || maxBlocks!=8 {
   t.Fatalf("fec20:%d flush=%v maxBlocks=%d want32ms and8",p,window,maxBlocks)
  }
 }
 window,blocks,err:=FixedFECRuntimeDefaults(0)
 if err!=nil || window!=0 || blocks!=0 {
  t.Fatalf("FEC off changed: flush=%v blocks=%d err=%v",window,blocks,err)
 }
 if _,_,err:=FixedFECRuntimeDefaults(9);err==nil{
  t.Fatal("unsupported fixed FEC profile should still reject")
 }
}
