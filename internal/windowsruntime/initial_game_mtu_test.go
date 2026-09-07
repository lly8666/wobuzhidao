package windowsruntime

import (
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/dataplane"
	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestInitialGameLaneLinkMTUBudgetsDataplaneAndEnvelopeHeaders(t *testing.T) {
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
	want := 1300 + dataplane.HeaderLen + gamelane.HeaderSize
	if len(plan.Lanes) != 1 || !argPair(plan.Lanes[0].Link.Args, "-mtu", "1340") || want != 1340 {
		t.Fatalf("initial Game LINK args=%v want inner+dataplane+game headers=%d", plan.Lanes[0].Link.Args, want)
	}
}
