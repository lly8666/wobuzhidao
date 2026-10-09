package linkdata

import (
 "bytes"
 "testing"
 "time"
 "github.com/lly8666/wobuzhidao/internal/fec"
)

// This intentionally changes only the 20:20 >512B parity assembly window.
// First-arriving systematic LINK datagrams never wait for either deadline;
// low-RTT small packets must retain exactly their existing 8ms parity deadline.
func TestE1LargestSizeClass16msSmallClasses8msRealFECPath(t *testing.T) {
 path,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:20,FlushAfter:8*time.Millisecond,MaxBlocks:32})
 if err!=nil{t.Fatal(err)}
 t0:=time.Unix(200,0)
 small:=bytes.Repeat([]byte{0x5a},96)
 large:=bytes.Repeat([]byte{0x78},1372)
 smallWire,err:=path.Encode(small,t0)
 if err!=nil||len(smallWire)!=1 {t.Fatalf("small systematic=%d err=%v",len(smallWire),err)}
 largeWire,err:=path.Encode(large,t0)
 if err!=nil||len(largeWire)!=2 {t.Fatalf("large link fragments=%d err=%v",len(largeWire),err)}
 if due:=path.NextFlushDeadline();!due.Equal(t0.Add(8*time.Millisecond)) {t.Fatalf("first small deadline=%v",due)}
 if wire,err:=path.FlushDue(t0.Add(8*time.Millisecond-time.Nanosecond));err!=nil||len(wire)!=0 {t.Fatalf("early small flush %d %v",len(wire),err)}
 smallParity,err:=path.FlushDue(t0.Add(8*time.Millisecond))
 if err!=nil||len(smallParity)!=1 {t.Fatalf("small parity at 8ms=%d err=%v",len(smallParity),err)}
 if due:=path.NextFlushDeadline();!due.Equal(t0.Add(16*time.Millisecond)) {t.Fatalf("large deadline=%v",due)}
 if wire,err:=path.FlushDue(t0.Add(15*time.Millisecond));err!=nil||len(wire)!=0 {t.Fatalf("unexpected large early parity %d %v",len(wire),err)}
 // Another large datagram arrives 12ms after the first: more recovery
 // equations for the same largest-size block, but it MUST NOT reset the due.
 more,err:=path.Encode(bytes.Repeat([]byte{0x79},1372),t0.Add(12*time.Millisecond))
 if err!=nil||len(more)!=2 {t.Fatalf("more systematic=%d err=%v",len(more),err)}
 if due:=path.NextFlushDeadline();!due.Equal(t0.Add(16*time.Millisecond)) {t.Fatalf("second source reset deadline=%v",due)}
 parity,err:=path.FlushDue(t0.Add(16*time.Millisecond))
 if err!=nil||len(parity)!=4 || !path.NextFlushDeadline().IsZero(){t.Fatalf("largest parity at16ms=%d err=%v",len(parity),err)}
 // The independent 96B source must deliver immediately even if the
 // larger block remains unresolved at the time of small delivery.
 rx,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:20,FlushAfter:8*time.Millisecond,MaxBlocks:32})
 if err!=nil{t.Fatal(err)}
 out,err:=rx.Decode(smallWire[0],t0)
 if err!=nil||len(out)!=1||!bytes.Equal(out[0],small){t.Fatalf("small fresh HOL regression: %d %v",len(out),err)}
 // Large source datagrams are also complete and arrive without waiting for
 // parity in a zero-loss transport, even though parity is still outstanding.
 for i,wire:=range [][]byte{largeWire[0],largeWire[1]} {
  got,err:=rx.Decode(wire,t0)
  if err!=nil{t.Fatal(err)}
  if (i==0 && len(got)!=0) || (i==1 && (len(got)!=1||!bytes.Equal(got[0],large))) {
   t.Fatalf("large first delivery i=%d out=%d",i,len(got))
  }
 }
 _=smallParity
}

func TestE1LargestGroupExtensionLimitedTo20x20Default8ms(t *testing.T) {
 start:=time.Unix(1000,0)
 for _,p:=range []int{4,8,10,12,16,20} {
  path,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:p,FlushAfter:8*time.Millisecond,MaxBlocks:32})
  if err!=nil{t.Fatal(err)}
  out,err:=path.Encode(bytes.Repeat([]byte{0x42},1100),start)
  if err!=nil||len(out)!=1{t.Fatalf("fec20:%d immediate=%d err=%v",p,len(out),err)}
  want:=start.Add(8*time.Millisecond)
  if p==20 {want=start.Add(16*time.Millisecond)}
  if got:=path.NextFlushDeadline();!got.Equal(want){t.Fatalf("fec20:%d deadline=%s want=%s",p,got,want)}
 }
 // A nondefault configured deadline is not silently multiplied.
 path,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:20,FlushAfter:24*time.Millisecond,MaxBlocks:32})
 if err!=nil{t.Fatal(err)}
 if _,err=path.Encode(bytes.Repeat([]byte{0x12},1100),start);err!=nil{t.Fatal(err)}
 if got:=path.NextFlushDeadline();!got.Equal(start.Add(24*time.Millisecond)){t.Fatalf("nondefault widened unexpectedly: %s",got)}
}

func TestE1LargestWindowConstructorFailsClosedForEarlierDeadline(t *testing.T) {
 _,err:=fec.NewSizeClassEncoderWithLargestWindow(fec.NewFastReedSolomon20x20(),1200,8*time.Millisecond,4*time.Millisecond,1,20)
 if err==nil{t.Fatal("must reject shrinking largest below normal deadline")}
}
