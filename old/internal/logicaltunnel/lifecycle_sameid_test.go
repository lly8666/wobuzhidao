package logicaltunnel

import (
	"errors"
	"testing"
)

func TestPromoteSameIDReplacementAdvancesGenerationWithOneLogicalTransport(t *testing.T){
	l,err:=NewLaneLifecycle(1);if err!=nil{t.Fatal(err)}
	old,err:=l.AttachInitial(1);if err!=nil{t.Fatal(err)}
	fresh,err:=l.PromoteSameIDReplacement(old);if err!=nil{t.Fatal(err)}
	if fresh.ID!=old.ID||fresh.Generation<=old.Generation{t.Fatalf("old=%+v fresh=%+v",old,fresh)}
	if len(l.Snapshot())!=1{t.Fatalf("logical transports=%d want=1",len(l.Snapshot()))}
	if _,err:=l.current(old);!errors.Is(err,ErrStaleLaneGeneration){t.Fatalf("old generation not fenced: %v",err)}
	if _,err:=l.current(fresh);err!=nil{t.Fatalf("fresh generation missing: %v",err)}
}
