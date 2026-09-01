package logicaltunnel

import (
	"errors"
	"testing"
)

func TestPromoteSameIDReplacementAdvancesGenerationWithoutFifthLane(t *testing.T){
	l,err:=NewLaneLifecycle(4);if err!=nil{t.Fatal(err)}
	refs:=make([]LaneRef,4)
	for i:=1;i<=4;i++{refs[i-1],err=l.AttachInitial(uint8(i));if err!=nil{t.Fatal(err)}}
	old:=refs[3]
	fresh,err:=l.PromoteSameIDReplacement(old);if err!=nil{t.Fatal(err)}
	if fresh.ID!=old.ID||fresh.Generation<=old.Generation{t.Fatalf("old=%+v fresh=%+v",old,fresh)}
	if len(l.Snapshot())!=4{t.Fatalf("logical lanes=%d want=4",len(l.Snapshot()))}
	if _,err:=l.current(old);!errors.Is(err,ErrStaleLaneGeneration){t.Fatalf("old generation not fenced: %v",err)}
	if _,err:=l.current(fresh);err!=nil{t.Fatalf("fresh generation missing: %v",err)}
}
