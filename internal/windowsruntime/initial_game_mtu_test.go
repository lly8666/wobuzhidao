package windowsruntime

import (
	"strings"
	"testing"
)

func TestInitialGameLaneDerivesMTUsFromConnectionCeiling(t *testing.T) {
	p := testProfile()
	p.Lanes = 1
	p.MTU = 1300
	u := testUnderlay()
	u.SourcePort = windowsDynamicPortMin + 91
	b, err := BuildLaneBootstrap(p, u, 1)
	if err != nil {
		t.Fatal(err)
	}
	b.Ticket = strings.Repeat("ab", 32)
	b.TunnelConfig = testAuthenticatedTunnel()
	plan, err := BuildMultiLanePlan(p, []LaneBootstrap{b})
	if err != nil {
		t.Fatal(err)
	}
	// 1300 connection - 40 IPv4/TCP - 32 DTLS reserve - 56 FEC = 1172 LINK;
	// Game consumes another 40 bytes, leaving 1132 inner IP for Wintun.
	if len(plan.Lanes) != 1 || !argPair(plan.Lanes[0].Link.Args, "-mtu", "1172") {
		t.Fatalf("initial Game LINK args=%v want derived MTU 1172", plan.Lanes[0].Link.Args)
	}
	if !argPair(plan.TUN.Args, "-mtu", "1132") {
		t.Fatalf("initial Game TUN args=%v want derived inner MTU 1132", plan.TUN.Args)
	}
}
