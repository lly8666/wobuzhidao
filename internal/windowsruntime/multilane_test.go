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

func TestProductProfileAcceptsOneThroughFourPublicTransportLanes(t *testing.T) {
	for _, lanes := range []int{1,2,3,4} {
		p := testProfile(); p.Lanes = lanes
		if err := p.Validate(); err != nil { t.Fatalf("product profile lanes=%d rejected: %v", lanes, err) }
	}
	for _, lanes := range []int{-1,5} {
		p := testProfile(); p.Lanes = lanes
		if err := p.Validate(); !errors.Is(err, logicaltunnel.ErrTransportLanes) { t.Fatalf("invalid product lanes=%d accepted: %v", lanes, err) }
	}
}

func TestLaneBootstrapsUseDistinctPortsAndSameInstallation(t *testing.T) {
	p := testProfile(); p.TunnelIPv4=""; p.Lanes=4
	seenUDP := map[string]bool{}; seenSource := map[uint16]bool{}
	for laneID:=1; laneID<=4; laneID++ {
		u:=testUnderlay(); u.SourcePort=uint16(windowsDynamicPortMin+laneID)
		b,err:=BuildLaneBootstrap(p,u,laneID); if err!=nil{t.Fatal(err)}
		wantUDP:=fmt.Sprintf("127.0.0.1:%d",defaultFakeTCPLocalPort+laneID-1)
		if !argPair(b.FakeTCP.Args,"--local-udp",wantUDP){t.Fatalf("lane %d local UDP args=%v",laneID,b.FakeTCP.Args)}
		if seenUDP[wantUDP]{t.Fatalf("lane %d reused UDP %s",laneID,wantUDP)};seenUDP[wantUDP]=true
		if seenSource[u.SourcePort]{t.Fatalf("lane %d reused source port %d",laneID,u.SourcePort)};seenSource[u.SourcePort]=true
		if !argPair(b.FakeTCP.Args,"--reality-installation-id",p.InstallationID){t.Fatalf("lane %d changed installation identity",laneID)}
		if !strings.HasSuffix(b.TicketPath,fmt.Sprintf(".lane%d",laneID)) || !strings.HasSuffix(b.TunnelConfigPath,fmt.Sprintf(".lane%d",laneID)){t.Fatalf("lane %d state paths ticket=%q config=%q",laneID,b.TicketPath,b.TunnelConfigPath)}
	}
}

func TestBuildMultiLanePlanSupportsOneThroughFourAndOneWintun(t *testing.T) {
	for _, lanes := range []int{1,2,3,4} {
		p:=testProfile(); p.TunnelIPv4=""; p.Lanes=lanes; tunnel:=testAuthenticatedTunnel()
		boots:=make([]LaneBootstrap,0,lanes)
		linkAddresses:=make([]string,0,lanes)
		for laneID:=1;laneID<=lanes;laneID++{
			boots=append(boots,authenticatedLane(t,p,laneID,uint16(windowsDynamicPortMin+laneID),tunnel))
			linkAddresses=append(linkAddresses,fmt.Sprintf("127.0.0.1:%d",defaultLinkListenPort+laneID-1))
		}
		plan,err:=BuildMultiLanePlan(p,boots);if err!=nil{t.Fatalf("lanes=%d: %v",lanes,err)}
		if len(plan.Lanes)!=lanes{t.Fatalf("lanes=%d plan lanes=%d",lanes,len(plan.Lanes))}
		if !argPair(plan.Game.Args,"-lanes",strings.Join(linkAddresses,",")){t.Fatalf("lanes=%d Game args=%v",lanes,plan.Game.Args)}
		if !argPair(plan.TUN.Args,"-transport",fmt.Sprintf("127.0.0.1:%d",defaultGameListenPort)){t.Fatalf("lanes=%d TUN does not use single Game/race transport: %v",lanes,plan.TUN.Args)}
		if plan.TunnelConfig.TunnelID!=tunnel.TunnelID || plan.TunnelConfig.Address4!=tunnel.Address4{t.Fatalf("lanes=%d changed tunnel config",lanes)}
	}
}

func TestBuildMultiLanePlanRejectsFifthLane(t *testing.T) {
	p:=testProfile();p.TunnelIPv4="";p.Lanes=5
	if _,err:=BuildMultiLanePlan(p,nil);!errors.Is(err,logicaltunnel.ErrTransportLanes){t.Fatalf("fifth lane not rejected: %v",err)}
}

func TestBuildMultiLanePlanRejectsLaneTunnelMismatch(t *testing.T) {
	p:=testProfile();p.TunnelIPv4="";p.Lanes=2;tunnel:=testAuthenticatedTunnel()
	a:=authenticatedLane(t,p,1,windowsDynamicPortMin+1,tunnel)
	other:=tunnel;other.Address4="10.66.0.2/32"
	b:=authenticatedLane(t,p,2,windowsDynamicPortMin+2,other)
	if _,err:=BuildMultiLanePlan(p,[]LaneBootstrap{a,b});err==nil{t.Fatal("two lanes with different authenticated lease accepted")}
}
