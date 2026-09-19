package windowsruntime

import (
	"fmt"
	"testing"
)

func TestSingleTransportPlanExposesLoopbackGameControl(t *testing.T) {
	p:=testProfile();p.TunnelIPv4="";p.Lanes=1;tunnel:=testAuthenticatedTunnel()
	boots:=[]LaneBootstrap{authenticatedLane(t,p,1,windowsDynamicPortMin+1,tunnel)}
	plan,err:=BuildMultiLanePlan(p,boots);if err!=nil{t.Fatal(err)}
	if len(plan.Lanes)!=1{t.Fatalf("public transports=%d want=1",len(plan.Lanes))}
	want:=fmt.Sprintf("127.0.0.1:%d",defaultGameControlPort)
	if plan.GameControl!=want{t.Fatalf("GameControl=%q want=%q",plan.GameControl,want)}
	if !argPair(plan.Game.Args,"-control",want){t.Fatalf("local Game args missing control endpoint: %v",plan.Game.Args)}
}
