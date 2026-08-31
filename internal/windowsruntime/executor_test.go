package windowsruntime

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type recordingRunner struct { events []string; fail string; failOnce string; failReady string; failMarker string }
func(r *recordingRunner)shouldFail(name string)bool{if r.fail==name{return true};if r.failOnce==name{r.failOnce="";return true};return false}
func(r *recordingRunner)Run(command Command)error{r.events=append(r.events,"run:"+command.Name);if r.shouldFail(command.Name){return errors.New("injected failure")};return nil}
func(r *recordingRunner)Start(command Command)(Process,error){r.events=append(r.events,"start:"+command.Name);if r.shouldFail(command.Name){return nil,errors.New("injected failure")};return &recordingProcess{runner:r,name:command.Name},nil}
type recordingProcess struct{runner *recordingRunner;name string}
func(p *recordingProcess)Stop()error{p.runner.events=append(p.runner.events,"stop:"+p.name);return nil}
func(p *recordingProcess)WaitReady(marker string,timeout time.Duration)error{event:="ready:"+p.name;if strings.Contains(marker,singleFlowBootstrapReadyMarker){event+=":bootstrap"};p.runner.events=append(p.runner.events,event);if p.runner.failReady==p.name||(p.runner.failMarker!=""&&strings.Contains(marker,p.runner.failMarker)){return errors.New("injected readiness failure")};if marker==""||timeout<=0{return errors.New("invalid readiness contract")};return nil}

func testExecutorPlan()Plan{return Plan{FakeTCP:Command{Name:"faketcp"},DTLS:Command{Name:"dtls"},Link:Command{Name:"link"},TUN:Command{Name:"tun"},IPv6Apply:Command{Name:"ipv6-apply"},RouteApply:Command{Name:"route-apply"},RouteCleanup:Command{Name:"route-cleanup"},IPv6Cleanup:Command{Name:"ipv6-cleanup"}}}
func testMultiExecutorPlan(lanes int)MultiLanePlan{p:=MultiLanePlan{Game:Command{Name:"game"},TUN:Command{Name:"tun"},IPv6Apply:Command{Name:"ipv6-apply"},RouteApply:Command{Name:"route-apply"},RouteCleanup:Command{Name:"route-cleanup"},IPv6Cleanup:Command{Name:"ipv6-cleanup"}};for i:=1;i<=lanes;i++{p.Lanes=append(p.Lanes,LanePlan{ID:i,FakeTCP:Command{Name:"faketcp-"+itoa(i)},DTLS:Command{Name:"dtls-"+itoa(i)},Link:Command{Name:"link-"+itoa(i)}})};return p}
func itoa(i int)string{if i<10{return string(rune('0'+i))};panic("test itoa only supports <10")}

func TestExecutorStartStopPreservesSinglePlanCompatibilityOrder(t *testing.T){r:=&recordingRunner{};e:=NewExecutor(r);if err:=e.Start(testExecutorPlan());err!=nil{t.Fatal(err)};if err:=e.Stop();err!=nil{t.Fatal(err)};want:=[]string{"start:faketcp","ready:faketcp","start:dtls","ready:dtls","start:link","ready:link","start:tun","ready:tun","run:ipv6-apply","run:route-apply","run:route-cleanup","run:ipv6-cleanup","stop:tun","stop:link","stop:dtls","stop:faketcp"};if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}}

func TestExecutorStartMultiLaneWaitsEveryLaneBeforeGameAndNetwork(t *testing.T){
	r:=&recordingRunner{};e:=NewExecutor(r);pre:=map[int]Process{1:&recordingProcess{runner:r,name:"faketcp-1"},2:&recordingProcess{runner:r,name:"faketcp-2"}}
	if err:=e.StartMultiLane(testMultiExecutorPlan(2),pre);err!=nil{t.Fatal(err)};if err:=e.Stop();err!=nil{t.Fatal(err)}
	want:=[]string{"ready:faketcp-1","start:dtls-1","ready:dtls-1","start:link-1","ready:link-1","ready:faketcp-2","start:dtls-2","ready:dtls-2","start:link-2","ready:link-2","start:game","ready:game","start:tun","ready:tun","run:ipv6-apply","run:route-apply","run:route-cleanup","run:ipv6-cleanup","stop:tun","stop:game","stop:link-2","stop:dtls-2","stop:faketcp-2","stop:link-1","stop:dtls-1","stop:faketcp-1"}
	if !reflect.DeepEqual(r.events,want){t.Fatalf("multi lifecycle=%v want=%v",r.events,want)}
}

func TestExecutorMultiLaneFailureRollsBackAllBeforeNetworkMutation(t *testing.T){
	r:=&recordingRunner{failReady:"dtls-2"};e:=NewExecutor(r);pre:=map[int]Process{1:&recordingProcess{runner:r,name:"faketcp-1"},2:&recordingProcess{runner:r,name:"faketcp-2"}}
	if err:=e.StartMultiLane(testMultiExecutorPlan(2),pre);err==nil{t.Fatal("expected lane2 readiness failure")}
	want:=[]string{"ready:faketcp-1","start:dtls-1","ready:dtls-1","start:link-1","ready:link-1","ready:faketcp-2","start:dtls-2","ready:dtls-2","stop:dtls-2","stop:faketcp-2","stop:link-1","stop:dtls-1","stop:faketcp-1"}
	if !reflect.DeepEqual(r.events,want){t.Fatalf("rollback=%v want=%v",r.events,want)}
	for _,ev:=range r.events{if ev=="run:ipv6-apply"||ev=="run:route-apply"{t.Fatalf("network mutated before all lanes ready: %v",r.events)}}
}

func TestExecutorMultiLaneRouteFailureCleansNetworkAndAllProcesses(t *testing.T){
	r:=&recordingRunner{fail:"route-apply"};e:=NewExecutor(r);pre:=map[int]Process{1:&recordingProcess{runner:r,name:"faketcp-1"}}
	if err:=e.StartMultiLane(testMultiExecutorPlan(1),pre);err==nil{t.Fatal("expected route failure")}
	wantTail:=[]string{"run:route-apply","run:route-cleanup","run:ipv6-cleanup","stop:tun","stop:game","stop:link-1","stop:dtls-1","stop:faketcp-1"}
	if len(r.events)<len(wantTail)||!reflect.DeepEqual(r.events[len(r.events)-len(wantTail):],wantTail){t.Fatalf("route rollback=%v want tail=%v",r.events,wantTail)}
}

func TestExecutorStartAfterFakeTCPCompatibilityStillWaitsBeforeDTLS(t *testing.T){r:=&recordingRunner{};fake:=&recordingProcess{runner:r,name:"faketcp"};e:=NewExecutor(r);if err:=e.StartAfterFakeTCP(testExecutorPlan(),fake);err!=nil{t.Fatal(err)};if err:=e.Stop();err!=nil{t.Fatal(err)};for _,event:=range r.events{if event=="start:faketcp"{t.Fatalf("prestarted flow started twice: %v",r.events)}}}

func TestExecutorReadinessFailureRollsBackBeforeNetworkMutation(t *testing.T){r:=&recordingRunner{failReady:"dtls"};e:=NewExecutor(r);if err:=e.Start(testExecutorPlan());err==nil{t.Fatal("expected failure")};want:=[]string{"start:faketcp","ready:faketcp","start:dtls","ready:dtls","stop:dtls","stop:faketcp"};if !reflect.DeepEqual(r.events,want){t.Fatalf("events=%v want=%v",r.events,want)}}

func TestExecutorRetriesFailedCleanupBeforeAllowingRestart(t *testing.T){r:=&recordingRunner{};e:=NewExecutor(r);if err:=e.Start(testExecutorPlan());err!=nil{t.Fatal(err)};r.failOnce="route-cleanup";if err:=e.Stop();err==nil{t.Fatal("expected cleanup failure")};if !e.CleanupPending(){t.Fatal("cleanup must remain pending")};if err:=e.Start(testExecutorPlan());err==nil{t.Fatal("restart must be blocked")};if err:=e.Stop();err!=nil{t.Fatalf("cleanup retry: %v",err)};if e.CleanupPending(){t.Fatal("cleanup retry did not clear state")}}
