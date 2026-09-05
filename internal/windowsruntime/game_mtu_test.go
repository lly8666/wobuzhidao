package windowsruntime

import (
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
)

func TestDefaultTunnelMTUIsConservativeAndConfigurable(t *testing.T) {
	p := testProfile()
	p.MTU = 0
	if got := p.normalized().MTU; got != DefaultTunnelMTU || DefaultTunnelMTU != 1360 {
		t.Fatalf("default inner MTU=%d want=%d", got, DefaultTunnelMTU)
	}
	p.MTU = 1280
	if got := p.normalized().MTU; got != 1280 {
		t.Fatalf("custom inner MTU=%d want=1280", got)
	}
}

func TestGameLaneLinkMTUBudgetsEnvelopeHeader(t *testing.T) {
	p := testProfile()
	p.MTU = 1280
	p.TunnelIPv4 = ""
	u := testUnderlay()
	u.SourcePort = windowsDynamicPortMin + 77
	b, err := BuildCandidateLaneBootstrap(p, u, 1)
	if err != nil { t.Fatal(err) }
	b.Ticket = strings.Repeat("ab", 32)
	b.TunnelConfig = testAuthenticatedTunnel()
	plan, err := BuildCandidateLanePlan(p, b)
	if err != nil { t.Fatal(err) }
	want := 1280 + gamelane.HeaderSize
	if !argPair(plan.Link.Args, "-mtu", "1312") || want != 1312 {
		t.Fatalf("Game LINK args=%v want inner+header=%d", plan.Link.Args, want)
	}
}