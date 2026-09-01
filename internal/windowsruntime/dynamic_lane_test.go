package windowsruntime

import (
	"reflect"
	"testing"
)

func TestExecutorDynamicLaneFailureKeepsSharedRuntimeAndOldLane(t *testing.T){
	r:=&recordingRunner{}
	e:=NewExecutor(r)
	pre:=map[int]Process{1:&recordingProcess{runner:r,name:"faketcp-1"}}
	if err:=e.StartMultiLane(testMultiExecutorPlan(1),pre);err!=nil{t.Fatal(err)}
	baseline:=append([]string(nil),r.events...)
	r.failReady="dtls-2"
	candidate:=testMultiExecutorPlan(2).Lanes[1]
	if err:=e.StartDynamicLane(candidate,&recordingProcess{runner:r,name:"faketcp-2"});err==nil{t.Fatal("expected candidate failure")}
	if got:=e.DynamicLaneIDs();!reflect.DeepEqual(got,[]int{1}){t.Fatalf("surviving lanes=%v",got)}
	newEvents:=r.events[len(baseline):]
	want:=[]string{"ready:faketcp-2","start:dtls-2","ready:dtls-2","stop:dtls-2","stop:faketcp-2"}
	if !reflect.DeepEqual(newEvents,want){t.Fatalf("candidate rollback=%v want=%v",newEvents,want)}
	for _,ev:=range newEvents{if ev=="stop:game"||ev=="stop:tun"||ev=="run:route-cleanup"{t.Fatalf("candidate failure disturbed shared runtime: %v",newEvents)}}
}

func TestExecutorDynamicLaneStartStopLeavesGameTunAndNetworkAlive(t *testing.T){
	r:=&recordingRunner{}
	e:=NewExecutor(r)
	pre:=map[int]Process{1:&recordingProcess{runner:r,name:"faketcp-1"}}
	if err:=e.StartMultiLane(testMultiExecutorPlan(1),pre);err!=nil{t.Fatal(err)}
	candidate:=testMultiExecutorPlan(2).Lanes[1]
	if err:=e.StartDynamicLane(candidate,&recordingProcess{runner:r,name:"faketcp-2"});err!=nil{t.Fatal(err)}
	if got:=e.DynamicLaneIDs();!reflect.DeepEqual(got,[]int{1,2}){t.Fatalf("lanes after start=%v",got)}
	cut:=len(r.events)
	if err:=e.StopDynamicLane(1);err!=nil{t.Fatal(err)}
	if got:=e.DynamicLaneIDs();!reflect.DeepEqual(got,[]int{2}){t.Fatalf("lanes after retire=%v",got)}
	want:=[]string{"stop:link-1","stop:dtls-1","stop:faketcp-1"}
	if !reflect.DeepEqual(r.events[cut:],want){t.Fatalf("retire events=%v want=%v",r.events[cut:],want)}
}
