package linkdata

import (
	"testing"
	"time"
)

func TestFECPathNextFlushDeadlineAndFECOff(t *testing.T) {
	start:=time.Unix(100,0)
	off,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:0})
	if err!=nil{t.Fatal(err)}
	if d:=off.NextFlushDeadline();!d.IsZero(){t.Fatalf("FEC off due=%v",d)}
	fecPath,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:20,FlushAfter:8*time.Millisecond,MaxBlocks:8})
	if err!=nil{t.Fatal(err)}
	if d:=fecPath.NextFlushDeadline();!d.IsZero(){t.Fatalf("empty due=%v",d)}
	out,err:=fecPath.Encode([]byte("single source"),start)
	if err!=nil||len(out)!=1{t.Fatalf("source=%d err=%v",len(out),err)}
	want:=start.Add(8*time.Millisecond)
	if d:=fecPath.NextFlushDeadline();!d.Equal(want){t.Fatalf("due=%v want=%v",d,want)}
	if parity,err:=fecPath.FlushDue(want);err!=nil||len(parity)!=1{t.Fatalf("parity=%d err=%v",len(parity),err)}
	if d:=fecPath.NextFlushDeadline();!d.IsZero(){t.Fatalf("due after flush=%v",d)}
}
