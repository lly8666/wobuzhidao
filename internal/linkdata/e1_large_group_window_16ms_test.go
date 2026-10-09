package linkdata

import (
 "bytes"
 "testing"
 "time"

 "github.com/lly8666/wobuzhidao/internal/fec"
)

// One 32ms partial-parity deadline for EVERY existing size group; never
// delay original systematic datagrams or a fresh independent small packet.
// The former file name is retained to preserve the historical E1 regression
// test location; the old 8ms-small/16ms-large policy is no longer active.
func TestE1Uniform32msAllSizeGroupsAndLargeTailNoHOL(t *testing.T) {
 const window = 32*time.Millisecond
 tx,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:20,FlushAfter:window,MaxBlocks:32})
 if err!=nil{t.Fatal(err)}
 t0:=time.Unix(200,0)
 small:=bytes.Repeat([]byte{0x5a},96)
 large:=bytes.Repeat([]byte{0x78},1372)
 smallWire,err:=tx.Encode(small,t0)
 if err!=nil||len(smallWire)!=1 {t.Fatalf("small systematic=%d err=%v",len(smallWire),err)}
 largeWire,err:=tx.Encode(large,t0)
 if err!=nil||len(largeWire)!=2 {t.Fatalf("large fragments=%d err=%v",len(largeWire),err)}
 firstH,err:=fec.ParseBlockHeader(largeWire[0]);if err!=nil{t.Fatal(err)}
 tailH,err:=fec.ParseBlockHeader(largeWire[1]);if err!=nil{t.Fatal(err)}
 if firstH.BlockID!=tailH.BlockID || firstH.ShardIndex==tailH.ShardIndex {
  t.Fatalf("large datagram's short tail must join original parity block: first=%+v tail=%+v",firstH,tailH)
 }
 if due:=tx.NextFlushDeadline();!due.Equal(t0.Add(window)) {t.Fatalf("first deadline=%v",due)}
 if out,err:=tx.FlushDue(t0.Add(window-time.Nanosecond));err!=nil||len(out)!=0 {t.Fatalf("premature parity=%d err=%v",len(out),err)}
 // Another fragmented large source arrives at 12ms, with the original
 // 32ms expiry unchanged: the extra 2 fragments strengthen the same group.
 more,err:=tx.Encode(bytes.Repeat([]byte{0x79},1372),t0.Add(12*time.Millisecond))
 if err!=nil||len(more)!=2 {t.Fatalf("next systematic=%d err=%v",len(more),err)}
 for _,wire:=range more {
  h,err:=fec.ParseBlockHeader(wire)
  if err!=nil||h.BlockID!=firstH.BlockID {t.Fatalf("subsequent large source escaped group=%+v err=%v",h,err)}
 }
 if due:=tx.NextFlushDeadline();!due.Equal(t0.Add(window)) {t.Fatalf("later source moved expiry=%v",due)}
 parity,err:=tx.FlushDue(t0.Add(window))
 if err!=nil||len(parity)!=5||!tx.NextFlushDeadline().IsZero() {t.Fatalf("all size classes 32ms expiry parity=%d err=%v",len(parity),err)}
 // Exactly one small-group parity and four large-fragment parity.
 var smallParity,largeParity int
 smallH,err:=fec.ParseBlockHeader(smallWire[0]);if err!=nil{t.Fatal(err)}
 for _,wire:=range parity {
  h,err:=fec.ParseBlockHeader(wire);if err!=nil{t.Fatal(err)}
  switch h.BlockID {
  case smallH.BlockID:smallParity++
  case firstH.BlockID:largeParity++
  default:t.Fatalf("unexpected parity block %d",h.BlockID)
  }
 }
 if smallParity!=1||largeParity!=4 {t.Fatalf("parity small=%d large=%d",smallParity,largeParity)}
 rx,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:20,FlushAfter:window,MaxBlocks:32})
 if err!=nil{t.Fatal(err)}
 out,err:=rx.Decode(smallWire[0],t0)
 if err!=nil||len(out)!=1||!bytes.Equal(out[0],small) {t.Fatalf("fresh small HOL=%d err=%v",len(out),err)}
 for i,wire:=range largeWire {
  got,err:=rx.Decode(wire,t0)
  if err!=nil{t.Fatal(err)}
  if (i==0 && len(got)!=0)||(i==1&&(len(got)!=1||!bytes.Equal(got[0],large))) {
   t.Fatalf("fresh large first delivery i=%d got=%d",i,len(got))
  }
 }
}

// All FEC profiles use exactly the configured deadline; no hidden second
// largest-size timer or size-conditioned adaptive delay.
func TestE1Uniform32msAllFECProfilesAndExplicitLegacyWindow(t *testing.T) {
 start:=time.Unix(1000,0)
 for _,p:=range []int{4,8,10,12,16,20} {
  tx,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:p,FlushAfter:32*time.Millisecond,MaxBlocks:32})
  if err!=nil{t.Fatal(err)}
  out,err:=tx.Encode(bytes.Repeat([]byte{0x42},1100),start)
  if err!=nil||len(out)!=1 {t.Fatalf("fec20:%d original systematic immediate=%d err=%v",p,len(out),err)}
  if got:=tx.NextFlushDeadline();!got.Equal(start.Add(32*time.Millisecond)){
   t.Fatalf("profile %d due=%s instead of32ms",p,got)
  }
 }
 for _,window:=range []time.Duration{8*time.Millisecond,24*time.Millisecond,64*time.Millisecond} {
  tx,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:20,FlushAfter:window,MaxBlocks:32})
  if err!=nil{t.Fatal(err)}
  if _,err=tx.Encode(bytes.Repeat([]byte{0x12},1100),start);err!=nil{t.Fatal(err)}
  if got:=tx.NextFlushDeadline();!got.Equal(start.Add(window)) {
   t.Fatalf("explicit window=%s unintentionally multiplied to=%s",window,got)
  }
 }
}
