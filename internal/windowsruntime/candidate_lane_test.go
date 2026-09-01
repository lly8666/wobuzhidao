package windowsruntime

import (
	"strings"
	"testing"
)

func TestCandidateLaneUsesPrivateSlotFiveWithSameLogicalID(t *testing.T){
	p:=testProfile();p.TunnelIPv4="";p.Lanes=4
	u:=testUnderlay();u.SourcePort=windowsDynamicPortMin+99
	b,err:=BuildCandidateLaneBootstrap(p,u,4);if err!=nil{t.Fatal(err)}
	b.Ticket=strings.Repeat("ab",32);b.TunnelConfig=testAuthenticatedTunnel()
	plan,err:=BuildCandidateLanePlan(p,b);if err!=nil{t.Fatal(err)}
	if plan.ID!=4||plan.Slot!=makeBeforeBreakCandidateSlot{t.Fatalf("candidate id/slot=%d/%d",plan.ID,plan.Slot)}
	wantLink:="127.0.0.1:47105"
	got,err:=LaneGameTarget(plan);if err!=nil{t.Fatal(err)}
	if got!=wantLink{t.Fatalf("candidate Game target=%q want=%q",got,wantLink)}
	if plan.FakeTCP.Name!="faketcp-4-candidate"||plan.DTLS.Name!="dtls-4-candidate"||plan.Link.Name!="link-4-candidate"{t.Fatalf("candidate names=%s/%s/%s",plan.FakeTCP.Name,plan.DTLS.Name,plan.Link.Name)}
	if !argPair(plan.FakeTCP.Args,"--local-udp","127.0.0.1:45105"){t.Fatalf("candidate FakeTCP args=%v",plan.FakeTCP.Args)}
}

func TestCandidateCanCoexistWithFourActiveProcessGroups(t *testing.T){
	r:=&recordingRunner{};e:=NewExecutor(r)
	pre:=map[int]Process{}
	for id:=1;id<=4;id++{pre[id]=&recordingProcess{runner:r,name:"faketcp-"+itoa(id)}}
	if err:=e.StartMultiLane(testMultiExecutorPlan(4),pre);err!=nil{t.Fatal(err)}
	p:=testProfile();p.TunnelIPv4="";p.Lanes=4
	u:=testUnderlay();u.SourcePort=windowsDynamicPortMin+100
	b,err:=BuildCandidateLaneBootstrap(p,u,4);if err!=nil{t.Fatal(err)}
	b.Ticket=strings.Repeat("cd",32);b.TunnelConfig=testAuthenticatedTunnel()
	candidate,err:=BuildCandidateLanePlan(p,b);if err!=nil{t.Fatal(err)}
	if err:=e.StartDynamicLane(candidate,&recordingProcess{runner:r,name:candidate.FakeTCP.Name});err!=nil{t.Fatal(err)}
	if err:=e.StopDynamicLanePlan(candidate);err!=nil{t.Fatal(err)}
	if got:=e.DynamicLaneIDs();len(got)!=4{t.Fatalf("active normal lane ids after candidate cycle=%v",got)}
}
