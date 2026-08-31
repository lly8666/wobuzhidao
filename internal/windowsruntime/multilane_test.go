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
	u := testUnderlay()
	u.SourcePort = sourcePort
	b, err := BuildLaneBootstrap(profile, u, laneID)
	if err != nil {
		t.Fatal(err)
	}
	b.Ticket = strings.Repeat(fmt.Sprintf("%x", laneID), 64)[:64]
	b.TunnelConfig = tunnel
	return b
}

func testAuthenticatedTunnel() logicaltunnel.TunnelConfig {
	return logicaltunnel.TunnelConfig{
		TunnelID: logicaltunnel.TunnelID("11223344556677889900aabbccddeeff"),
		Address4: "10.66.0.1/32",
		Routes4:  []string{"0.0.0.0/0"},
	}
}

func TestProductProfileRejectsMultiplePublicTransportLanes(t *testing.T) {
	for _, lanes := range []int{2, 3, 4} {
		p := testProfile()
		p.Lanes = lanes
		if err := p.Validate(); !errors.Is(err, logicaltunnel.ErrTransportLanes) {
			t.Fatalf("product profile lanes=%d not rejected by ADR-0014: %v", lanes, err)
		}
	}
}

func TestSingleProductLaneBootstrapUsesSameFlowStatePaths(t *testing.T) {
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 1
	u := testUnderlay()
	u.SourcePort = windowsDynamicPortMin + 1
	b, err := BuildLaneBootstrap(p, u, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !argPair(b.FakeTCP.Args, "--local-udp", "127.0.0.1:45101") {
		t.Fatalf("single lane local UDP args=%v", b.FakeTCP.Args)
	}
	if !argPair(b.FakeTCP.Args, "--reality-installation-id", p.InstallationID) {
		t.Fatal("single lane changed installation identity")
	}
	if !strings.HasSuffix(b.TicketPath, ".lane1") || !strings.HasSuffix(b.TunnelConfigPath, ".lane1") {
		t.Fatalf("single lane state paths ticket=%q config=%q", b.TicketPath, b.TunnelConfigPath)
	}
}

// The builder remains available to preserve research evidence, but product
// configuration cannot reach it with more than one public transport because
// Profile.Validate is governed by logicaltunnel.MaxProductPublicTransportLanes=1.
func TestResearchMultiLaneBuilderIsFencedByProductProfile(t *testing.T) {
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 2
	if _, err := BuildMultiLanePlan(p, nil); !errors.Is(err, logicaltunnel.ErrTransportLanes) {
		t.Fatalf("research multi-lane builder accepted product lanes=2: %v", err)
	}
}

func TestResearchOneLaneGameAggregatorStillBuildsInIsolation(t *testing.T) {
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 1
	tunnel := testAuthenticatedTunnel()
	one := authenticatedLane(t, p, 1, windowsDynamicPortMin+1, tunnel)
	plan, err := BuildMultiLanePlan(p, []LaneBootstrap{one})
	if err != nil {
		t.Fatal(err)
	}
	if !argPair(plan.Game.Args, "-lanes", "127.0.0.1:47101") {
		t.Fatalf("research one-lane Game path missing: %v", plan.Game.Args)
	}
	if !argPair(plan.TUN.Args, "-transport", "127.0.0.1:48101") {
		t.Fatalf("research one-lane TUN path missing: %v", plan.TUN.Args)
	}
}
