package datapath

import (
	"testing"
	"time"
)

func TestLaneNextFlushDeadlineCloseAndFECOff(t *testing.T){
	start:=time.Unix(100,0)
	off,_:=NewLane(pairConfig(RoleClient,0))
	defer off.Close()
	if due:=off.NextFlushDeadline();!due.IsZero(){t.Fatalf("off due %v",due)}
	lane,_:=NewLane(pairConfig(RoleClient,20))
	if _,err:=lane.Outbound([]byte("sparse source"),start);err!=nil{t.Fatal(err)}
	want:=start.Add(8*time.Millisecond)
	if due:=lane.NextFlushDeadline();!due.Equal(want){t.Fatalf("due=%v want=%v",due,want)}
	if _,err:=lane.FlushDue(want);err!=nil{t.Fatal(err)}
	if due:=lane.NextFlushDeadline();!due.IsZero(){t.Fatalf("stale due %v",due)}
	if _,err:=lane.Outbound([]byte("new"),want.Add(time.Millisecond));err!=nil{t.Fatal(err)}
	lane.Close()
	if due:=lane.NextFlushDeadline();!due.IsZero(){t.Fatalf("closed due %v",due)}
}

func TestTunnelOwnerActiveOnlyFlushDeadlineAndGeneration(t *testing.T){
	start:=time.Unix(200,0)
	owner,err:=NewTunnelOwner(1,8)
	if err!=nil{t.Fatal(err)}
	defer owner.Close()
	old,_:=NewLane(pairConfig(RoleClient,20))
	snapshot,err:=owner.AttachInitial(1,old)
	if err!=nil{t.Fatal(err)}
	if _,err=old.Outbound([]byte("old"),start);err!=nil{t.Fatal(err)}
	if got:=owner.NextActiveFlushDeadline();!got.Equal(start.Add(8*time.Millisecond)){t.Fatalf("active due=%v",got)}
	candidate,_:=NewLane(pairConfig(RoleClient,20))
	if err=owner.BeginSameIDReplacement(snapshot.Ref,candidate);err!=nil{t.Fatal(err)}
	if _,err=candidate.Outbound([]byte("candidate"),start.Add(time.Millisecond));err!=nil{t.Fatal(err)}
	if got:=owner.NextActiveFlushDeadline();!got.Equal(start.Add(8*time.Millisecond)){t.Fatalf("candidate influenced active deadline=%v",got)}
	if _,err=owner.PromoteSameIDReplacement(snapshot.Ref);err!=nil{t.Fatal(err)}
	if got:=owner.NextActiveFlushDeadline();!got.Equal(start.Add(9*time.Millisecond)){t.Fatalf("retiring deadline influenced current=%v",got)}
	owner.Close()
	if got:=owner.NextActiveFlushDeadline();!got.IsZero(){t.Fatalf("closed owner due=%v",got)}
}
