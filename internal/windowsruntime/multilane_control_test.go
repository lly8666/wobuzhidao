package windowsruntime

import (
	"fmt"
	"testing"
)

func TestMultiLanePlanExposesLoopbackGameControl(t *testing.T) {
	p:=testProfile();p.TunnelIPv4="";p.Lanes=2;tunnel:=testAuthenticatedTunnel()
	boots:=[]LaneBootstrap{authenticatedLane(t,p,1,windowsDynamicPortMin+1,tunnel),authenticatedLane(t,p,2,windowsDynamicPortMin+2,tunnel)}
	plan,err:=BuildMultiLanePlan(p,boots);if err!=nil{t.Fatal(err)}
	want:=fmt.Sprintf("127.0.0.1:%d",defaultGameControlPort)
	if plan.GameControl!=want{t.Fatalf("GameControl=%q want=%q",plan.GameControl,want)}
	if !argPair(plan.Game.Args,"-control",want){t.Fatalf("Game args missing control endpoint: %v",plan.Game.Args)}
}
