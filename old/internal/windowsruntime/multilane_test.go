package windowsruntime

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func authenticatedLane(t *testing.T, profile Profile, laneID int, sourcePort uint16, tunnel logicaltunnel.TunnelConfig) LaneBootstrap {
	t.Helper()
	u := testUnderlay(); u.SourcePort = sourcePort
	b, err := BuildLaneBootstrap(profile, u, laneID); if err != nil { t.Fatal(err) }
	b.Ticket = strings.Repeat(fmt.Sprintf("%x", laneID), 64)[:64]
	b.TunnelConfig = tunnel
	return b
}

func testAuthenticatedTunnel() logicaltunnel.TunnelConfig {
	return logicaltunnel.TunnelConfig{TunnelID: logicaltunnel.TunnelID("11223344556677889900aabbccddeeff"), Address4:"10.66.0.1/32", Routes4:[]string{"0.0.0.0/0"}}
}

func TestProductProfileAcceptsOneToFourPublicTransports(t *testing.T) {
	for _, lanes := range []int{0, 1, 2, 3, 4} {
		p := testProfile(); p.Lanes = lanes
		if err := p.Validate(); err != nil { t.Fatalf("product lanes=%d rejected: %v", lanes, err) }
	}
	for _, lanes := range []int{-1, 5} {
		p := testProfile(); p.Lanes = lanes
		if err := p.Validate(); !errors.Is(err, logicaltunnel.ErrTransportLanes) { t.Fatalf("invalid product lanes=%d err=%v", lanes, err) }
	}
}

func TestProductLaneBootstrapUsesOneSameFlowEndpointPerLane(t *testing.T) {
	p := testProfile(); p.TunnelIPv4=""; p.Lanes=4; p.MTU=1300
	seenSource := map[string]bool{}
	for laneID := 1; laneID <= 4; laneID++ {
		u:=testUnderlay(); u.SourcePort=uint16(windowsDynamicPortMin+laneID)
		b,err:=BuildLaneBootstrap(p,u,laneID); if err!=nil{t.Fatal(err)}
		wantLocal := fmt.Sprintf("127.0.0.1:%d", defaultFakeTCPLocalPort+laneID-1)
		if !argPair(b.FakeTCP.Args,"--local-udp",wantLocal){t.Fatalf("lane %d local UDP args=%v",laneID,b.FakeTCP.Args)}
		if !argPair(b.FakeTCP.Args,"--connection-mtu","1300"){t.Fatalf("lane %d connection MTU args=%v",laneID,b.FakeTCP.Args)}
		if !argPair(b.FakeTCP.Args,"--reality-installation-id",p.InstallationID){t.Fatalf("lane %d changed installation identity", laneID)}
		if !strings.HasSuffix(b.TicketPath,fmt.Sprintf(".lane%d",laneID)) || !strings.HasSuffix(b.TunnelConfigPath,fmt.Sprintf(".lane%d",laneID)){t.Fatalf("lane %d state paths ticket=%q config=%q",laneID,b.TicketPath,b.TunnelConfigPath)}
		for i, arg := range b.FakeTCP.Args {
			if arg == "--source" && i+1 < len(b.FakeTCP.Args) {
				if seenSource[b.FakeTCP.Args[i+1]] { t.Fatalf("lane %d reused source tuple %q", laneID, b.FakeTCP.Args[i+1]) }
				seenSource[b.FakeTCP.Args[i+1]] = true
			}
		}
	}
	if len(seenSource) != 4 { t.Fatalf("distinct source tuples=%d want=4", len(seenSource)) }
}

func TestBuildMultiLanePlanProducesAuthorizedTransportCounts(t *testing.T) {
	tunnel:=testAuthenticatedTunnel()
	for lanes := 1; lanes <= 4; lanes++ {
		p:=testProfile(); p.TunnelIPv4=""; p.Lanes=lanes
		boots := make([]LaneBootstrap, 0, lanes)
		for id := 1; id <= lanes; id++ {
			boots = append(boots, authenticatedLane(t,p,id,uint16(windowsDynamicPortMin+id),tunnel))
		}
		plan,err:=BuildMultiLanePlan(p,boots);if err!=nil{t.Fatalf("lanes=%d: %v",lanes,err)}
		if len(plan.Lanes)!=lanes{t.Fatalf("plan public transports=%d want=%d",len(plan.Lanes),lanes)}
		if plan.TunnelConfig.TunnelID!=tunnel.TunnelID || plan.TunnelConfig.Address4!=tunnel.Address4{t.Fatal("plan changed authenticated tunnel config")}
	}
}

func TestBuildMultiLanePlanPropagatesKeepaliveToInitialLinks(t *testing.T) {
	tunnel:=testAuthenticatedTunnel()
	zero:=0
	for _,tc:=range []struct{
		name string
		lanes int
		keepalive *int
		wantPair bool
	}{
		{name:"1lane-explicit-zero",lanes:1,keepalive:&zero,wantPair:true},
		{name:"2lane-explicit-zero",lanes:2,keepalive:&zero,wantPair:true},
		{name:"3lane-explicit-zero",lanes:3,keepalive:&zero,wantPair:true},
		{name:"4lane-explicit-zero",lanes:4,keepalive:&zero,wantPair:true},
		{name:"1lane-default",lanes:1,keepalive:nil,wantPair:false},
		{name:"4lane-default",lanes:4,keepalive:nil,wantPair:false},
	}{
		t.Run(tc.name,func(t *testing.T){
			p:=testProfile();p.TunnelIPv4="";p.Lanes=tc.lanes;p.KeepaliveSeconds=tc.keepalive
			boots:=make([]LaneBootstrap,0,tc.lanes)
			for id:=1;id<=tc.lanes;id++{boots=append(boots,authenticatedLane(t,p,id,uint16(windowsDynamicPortMin+id),tunnel))}
			plan,err:=BuildMultiLanePlan(p,boots);if err!=nil{t.Fatal(err)}
			for _,lane:=range plan.Lanes{
				hasFlag:=false
				for _,arg:=range lane.Link.Args{if arg=="-keepalive"{hasFlag=true;break}}
				if tc.wantPair{
					if !argPair(lane.Link.Args,"-keepalive","0s"){t.Fatalf("lane %d link args=%v; want explicit -keepalive 0s",lane.ID,lane.Link.Args)}
				}else if hasFlag{
					t.Fatalf("lane %d link args=%v; omitted keepalive must preserve proxy default",lane.ID,lane.Link.Args)
				}
			}
		})
	}
}

func TestBuildMultiLanePlanRejectsFifthProductLane(t *testing.T) {
	p:=testProfile();p.TunnelIPv4="";p.Lanes=5
	if _,err:=BuildMultiLanePlan(p,nil);!errors.Is(err,logicaltunnel.ErrTransportLanes){t.Fatalf("lanes=5 err=%v",err)}
}
