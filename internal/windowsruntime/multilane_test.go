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

func TestProductProfileAcceptsExactlyOnePublicTransport(t *testing.T) {
	p := testProfile(); p.Lanes = 1
	if err := p.Validate(); err != nil { t.Fatalf("single product public transport rejected: %v", err) }
	for _, lanes := range []int{-1,2,3,4,5} {
		p := testProfile(); p.Lanes = lanes
		if err := p.Validate(); !errors.Is(err, logicaltunnel.ErrTransportLanes) { t.Fatalf("non-single product lanes=%d accepted: %v", lanes, err) }
	}
}

func TestProductLaneBootstrapUsesOneSameFlowEndpoint(t *testing.T) {
	p := testProfile(); p.TunnelIPv4=""; p.Lanes=1
	u:=testUnderlay(); u.SourcePort=windowsDynamicPortMin+1
	b,err:=BuildLaneBootstrap(p,u,1); if err!=nil{t.Fatal(err)}
	if !argPair(b.FakeTCP.Args,"--local-udp",fmt.Sprintf("127.0.0.1:%d",defaultFakeTCPLocalPort)){t.Fatalf("local UDP args=%v",b.FakeTCP.Args)}
	if !argPair(b.FakeTCP.Args,"--reality-installation-id",p.InstallationID){t.Fatal("same-flow bootstrap changed installation identity")}
	if !strings.HasSuffix(b.TicketPath,".lane1") || !strings.HasSuffix(b.TunnelConfigPath,".lane1"){t.Fatalf("state paths ticket=%q config=%q",b.TicketPath,b.TunnelConfigPath)}
}

func TestBuildMultiLanePlanShippingPathProducesExactlyOneTransport(t *testing.T) {
	p:=testProfile(); p.TunnelIPv4=""; p.Lanes=1; tunnel:=testAuthenticatedTunnel()
	boot:=authenticatedLane(t,p,1,windowsDynamicPortMin+1,tunnel)
	plan,err:=BuildMultiLanePlan(p,[]LaneBootstrap{boot});if err!=nil{t.Fatal(err)}
	if len(plan.Lanes)!=1{t.Fatalf("plan public transports=%d want=1",len(plan.Lanes))}
	if plan.TunnelConfig.TunnelID!=tunnel.TunnelID || plan.TunnelConfig.Address4!=tunnel.Address4{t.Fatal("plan changed authenticated tunnel config")}
}

func TestBuildMultiLanePlanRejectsProductMultiFlowProfiles(t *testing.T) {
	for _, lanes:=range []int{2,3,4,5}{
		p:=testProfile();p.TunnelIPv4="";p.Lanes=lanes
		if _,err:=BuildMultiLanePlan(p,nil);!errors.Is(err,logicaltunnel.ErrTransportLanes){t.Fatalf("lanes=%d err=%v",lanes,err)}
	}
}
