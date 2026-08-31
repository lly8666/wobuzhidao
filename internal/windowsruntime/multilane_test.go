package windowsruntime

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func authenticatedLane(t *testing.T, profile Profile, laneID int, sourcePort uint16, tunnel logicaltunnel.TunnelConfig) LaneBootstrap {
	t.Helper()
	u:=testUnderlay();u.SourcePort=sourcePort
	b,err:=BuildLaneBootstrap(profile,u,laneID);if err!=nil{t.Fatal(err)}
	b.Ticket=strings.Repeat(fmt.Sprintf("%x",laneID),64)[:64]
	b.TunnelConfig=tunnel
	return b
}

func testAuthenticatedTunnel() logicaltunnel.TunnelConfig {
	return logicaltunnel.TunnelConfig{TunnelID:logicaltunnel.TunnelID("11223344556677889900aabbccddeeff"),Address4:"10.66.0.1/32",Routes4:[]string{"0.0.0.0/0"}}
}

func TestBuildLaneBootstrapUsesIndependentPortsPathsAndStableInstallation(t *testing.T){
	p:=testProfile();p.TunnelIPv4="";p.Lanes=4
	seen:=map[string]bool{}
	for lane:=1;lane<=4;lane++{
		u:=testUnderlay();u.SourcePort=uint16(windowsDynamicPortMin+lane)
		b,err:=BuildLaneBootstrap(p,u,lane);if err!=nil{t.Fatal(err)}
		wantUDP:=fmt.Sprintf("127.0.0.1:%d",defaultFakeTCPLocalPort+lane-1)
		if !argPair(b.FakeTCP.Args,"--local-udp",wantUDP){t.Fatalf("lane %d local UDP args=%v",lane,b.FakeTCP.Args)}
		if !argPair(b.FakeTCP.Args,"--reality-installation-id",p.InstallationID){t.Fatalf("lane %d changed installation identity",lane)}
		if !strings.HasSuffix(b.TicketPath,fmt.Sprintf(".lane%d",lane))||!strings.HasSuffix(b.TunnelConfigPath,fmt.Sprintf(".lane%d",lane)){t.Fatalf("lane %d state paths ticket=%q config=%q",lane,b.TicketPath,b.TunnelConfigPath)}
		for _,path:=range []string{b.TicketPath,b.TunnelConfigPath}{if seen[path]{t.Fatalf("lane state path reused: %s",path)};seen[path]=true}
	}
}

func TestBuildMultiLanePlanUsesTunnelIDAsGameSessionAndOneGamePath(t *testing.T){
	p:=testProfile();p.TunnelIPv4="";p.Lanes=4;tunnel:=testAuthenticatedTunnel()
	lanes:=make([]LaneBootstrap,0,4)
	for lane:=1;lane<=4;lane++{lanes=append(lanes,authenticatedLane(t,p,lane,uint16(windowsDynamicPortMin+lane),tunnel))}
	plan,err:=BuildMultiLanePlan(p,lanes);if err!=nil{t.Fatal(err)}
	if len(plan.Lanes)!=4{t.Fatalf("lanes=%d",len(plan.Lanes))}
	if !argPair(plan.Game.Args,"-session-id",string(tunnel.TunnelID)){t.Fatalf("Game SessionID not bound to authenticated TunnelID: %v",plan.Game.Args)}
	if !argPair(plan.Game.Args,"-lanes","127.0.0.1:47101,127.0.0.1:47102,127.0.0.1:47103,127.0.0.1:47104"){t.Fatalf("Game lane list=%v",plan.Game.Args)}
	if !argPair(plan.TUN.Args,"-transport","127.0.0.1:48101"){t.Fatalf("TUN bypassed Game Lane: %v",plan.TUN.Args)}
	for lane:=1;lane<=4;lane++{
		lp:=plan.Lanes[lane-1]
		if lp.ID!=lane||lp.FakeTCP.Name!=fmt.Sprintf("faketcp-%d",lane)||lp.DTLS.Name!=fmt.Sprintf("dtls-%d",lane)||lp.Link.Name!=fmt.Sprintf("link-%d",lane){t.Fatalf("lane %d plan=%+v",lane,lp)}
		if !slices.Contains(lp.Link.Args,strings.Repeat(fmt.Sprintf("%x",lane),64)[:64]){t.Fatalf("lane %d link lacks lane ticket: %v",lane,lp.Link.Args)}
	}
	if plan.TunnelConfig.Address4!="10.66.0.1/32"{t.Fatalf("authenticated lease=%+v",plan.TunnelConfig)}
}

func TestBuildMultiLanePlanRejectsCrossTunnelCandidate(t *testing.T){
	p:=testProfile();p.TunnelIPv4="";p.Lanes=2;tunnel:=testAuthenticatedTunnel()
	one:=authenticatedLane(t,p,1,windowsDynamicPortMin+1,tunnel)
	other:=tunnel;other.TunnelID=logicaltunnel.TunnelID("ffeeddccbbaa00998877665544332211")
	two:=authenticatedLane(t,p,2,windowsDynamicPortMin+2,other)
	if _,err:=BuildMultiLanePlan(p,[]LaneBootstrap{one,two});err==nil||!strings.Contains(err.Error(),"mismatch"){t.Fatalf("cross-tunnel lane accepted: %v",err)}
}

func TestBuildMultiLanePlanOneLaneStillUsesGameAggregator(t *testing.T){
	p:=testProfile();p.TunnelIPv4="";p.Lanes=1;tunnel:=testAuthenticatedTunnel()
	one:=authenticatedLane(t,p,1,windowsDynamicPortMin+1,tunnel)
	plan,err:=BuildMultiLanePlan(p,[]LaneBootstrap{one});if err!=nil{t.Fatal(err)}
	if !argPair(plan.Game.Args,"-lanes","127.0.0.1:47101"){t.Fatalf("one-lane Game path missing: %v",plan.Game.Args)}
	if !argPair(plan.TUN.Args,"-transport","127.0.0.1:48101"){t.Fatalf("one-lane TUN bypassed Game path: %v",plan.TUN.Args)}
}
