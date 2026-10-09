package fec

import (
	"testing"
	"time"
)

func TestFastBlockEncoderAbsolutePartialDeadlineAndRetirement(t *testing.T) {
	enc, err := NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(), 1200, 8*time.Millisecond, 1, 20)
	if err != nil { t.Fatal(err) }
	if got := enc.NextFlushDeadline(); !got.IsZero() { t.Fatalf("empty deadline=%v", got) }
	start := time.Unix(100, 0)
	if wire, err := enc.Add([]byte("first"), start); err != nil || len(wire)!=1 { t.Fatalf("first systematic=%d err=%v",len(wire),err) }
	want := start.Add(8*time.Millisecond)
	if got := enc.NextFlushDeadline(); !got.Equal(want) { t.Fatalf("deadline=%v want=%v",got,want) }
	if _,err := enc.Add([]byte("second"),start.Add(5*time.Millisecond));err!=nil { t.Fatal(err) }
	if got := enc.NextFlushDeadline(); !got.Equal(want) { t.Fatal("late source reset first deadline") }
	if parity,err := enc.FlushDue(want.Add(-time.Nanosecond));err!=nil||len(parity)!=0 { t.Fatalf("premature flush parity=%d err=%v",len(parity),err) }
	if parity,err := enc.FlushDue(want);err!=nil||len(parity)!=2 { t.Fatalf("expired partial parity=%d err=%v",len(parity),err) }
	if got := enc.NextFlushDeadline(); !got.IsZero() { t.Fatalf("old deadline survived flush: %v",got) }
	if _,err := enc.Add([]byte("new"),want.Add(time.Millisecond));err!=nil { t.Fatal(err) }
	if got := enc.NextFlushDeadline(); !got.Equal(want.Add(9*time.Millisecond)) { t.Fatalf("new block deadline=%v",got) }
}

func TestSizeClassNearestOfThreeAndNoEmptyScanTrigger(t *testing.T) {
	enc, err := NewSizeClassEncoder(NewFastReedSolomon20x20(), 1200, 8*time.Millisecond, 1, 20)
	if err != nil { t.Fatal(err) }
	start := time.Unix(200, 0)
	if got:=enc.NextFlushDeadline();!got.IsZero(){t.Fatalf("empty=%v",got)}
	for _,c:=range []struct{size int;offset time.Duration}{{900,0},{100,2*time.Millisecond},{350,3*time.Millisecond}} {
		if out,err:=enc.Add(make([]byte,c.size),start.Add(c.offset));err!=nil||len(out)!=1 {t.Fatalf("source size=%d got=%d err=%v",c.size,len(out),err)}
	}
	if got:=enc.NextFlushDeadline();!got.Equal(start.Add(8*time.Millisecond)){t.Fatalf("earliest=%v",got)}
	if out,err:=enc.FlushDue(start.Add(8*time.Millisecond));err!=nil||len(out)!=1 {t.Fatalf("first flush=%d err=%v",len(out),err)}
	if got:=enc.NextFlushDeadline();!got.Equal(start.Add(10*time.Millisecond)){t.Fatalf("next due=%v",got)}
	if out,err:=enc.FlushDue(start.Add(10*time.Millisecond));err!=nil||len(out)!=1 {t.Fatalf("second flush=%d err=%v",len(out),err)}
	if got:=enc.NextFlushDeadline();!got.Equal(start.Add(11*time.Millisecond)){t.Fatalf("last due=%v",got)}
	if out,err:=enc.FlushDue(start.Add(11*time.Millisecond));err!=nil||len(out)!=1 {t.Fatalf("third flush=%d err=%v",len(out),err)}
	if got:=enc.NextFlushDeadline();!got.IsZero(){t.Fatalf("all retired but due=%v",got)}
}

func TestLowerFECProfilesOneDeadline(t *testing.T) {
	start:=time.Unix(300,0)
	for _,parity:=range []int{4,8,10,12,16} {
		enc,err:=NewSizeClassEncoder(NewFastReedSolomon20x20(),1200,8*time.Millisecond,1,parity)
		if err!=nil{t.Fatal(err)}
		if _,err=enc.Add([]byte{1,2,3},start);err!=nil{t.Fatal(err)}
		if got:=enc.NextFlushDeadline();!got.Equal(start.Add(8*time.Millisecond)){t.Fatalf("parity %d due=%v",parity,got)}
	}
}
