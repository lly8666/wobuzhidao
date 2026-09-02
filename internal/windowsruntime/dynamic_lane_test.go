package windowsruntime

import (
	"errors"
	"reflect"
	"testing"
)

func TestExecutorDynamicTransportRejectsSecondPublicFlow(t *testing.T){
	r:=&recordingRunner{}
	e:=NewExecutor(r)
	pre:=map[int]Process{1:&recordingProcess{runner:r,name:"faketcp-1"}}
	if err:=e.StartMultiLane(testMultiExecutorPlan(1),pre);err!=nil{t.Fatal(err)}
	baseline:=append([]string(nil),r.events...)
	candidate:=testMultiExecutorPlan(1).Lanes[0]
	if err:=e.StartDynamicLane(candidate,&recordingProcess{runner:r,name:"faketcp-1-new"});!errors.Is(err,ErrOverlappingPublicFlow){t.Fatalf("second public flow err=%v",err)}
	if got:=e.DynamicLaneIDs();!reflect.DeepEqual(got,[]int{1}){t.Fatalf("surviving public transports=%v",got)}
	if !reflect.DeepEqual(r.events,baseline){t.Fatalf("rejected second public flow mutated runtime: before=%v after=%v",baseline,r.events)}
}

func TestExecutorDynamicTransportRejectsNonProductID(t *testing.T){
	r:=&recordingRunner{}
	e:=NewExecutor(r)
	pre:=map[int]Process{1:&recordingProcess{runner:r,name:"faketcp-1"}}
	if err:=e.StartMultiLane(testMultiExecutorPlan(1),pre);err!=nil{t.Fatal(err)}
	lane:=LanePlan{ID:2,Slot:2,FakeTCP:Command{Name:"faketcp-2"},DTLS:Command{Name:"dtls-2"},Link:Command{Name:"link-2"}}
	if err:=e.StartDynamicLane(lane,&recordingProcess{runner:r,name:"faketcp-2"});!errors.Is(err,ErrOverlappingPublicFlow){t.Fatalf("non-product dynamic transport err=%v",err)}
}
