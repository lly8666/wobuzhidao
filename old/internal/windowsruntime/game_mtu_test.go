package windowsruntime

import (
	"strings"
	"testing"
)

func TestDefaultConnectionMTUIsConfigurableCeiling(t *testing.T) {
	p := testProfile()
	p.MTU = 0
	if got := p.normalized().MTU; got != DefaultConnectionMTU || DefaultConnectionMTU != 1500 {
		t.Fatalf("default connection MTU=%d want=%d", got, DefaultConnectionMTU)
	}
	p.MTU = 1280
	if got := p.normalized().MTU; got != 1280 {
		t.Fatalf("custom connection MTU=%d want=1280", got)
	}
}

func TestGameConnectionMTUBudgetTracksConfiguredValueAndFEC(t *testing.T) {
	// These are representative inputs, not product constants. The contract is
	// that every derived MTU follows the configured connection MTU and feature
	// overhead instead of being frozen to one qualification value.
	for _, fecMode := range []string{"off", "20:4", "20:20"} {
		low, err := gameConnectionMTUBudget(1300, fecMode)
		if err != nil {
			t.Fatal(err)
		}
		high, err := gameConnectionMTUBudget(1500, fecMode)
		if err != nil {
			t.Fatal(err)
		}
		if high.ConnectionMTU-low.ConnectionMTU != 200 || high.LinkPlaintextMTU-low.LinkPlaintextMTU != 200 || high.InnerMTU-low.InnerMTU != 200 {
			t.Fatalf("fec=%s budget did not track configured MTU: low=%+v high=%+v", fecMode, low, high)
		}
	}

	off, err := gameConnectionMTUBudget(1400, "off")
	if err != nil {
		t.Fatal(err)
	}
	fec20, err := gameConnectionMTUBudget(1400, "20:20")
	if err != nil {
		t.Fatal(err)
	}
	if fec20.LinkPlaintextMTU >= off.LinkPlaintextMTU || fec20.InnerMTU >= off.InnerMTU {
		t.Fatalf("FEC overhead was not reflected dynamically: off=%+v fec20=%+v", off, fec20)
	}
}

func TestGameLaneDerivesLinkMTUFromConnectionCeiling(t *testing.T) {
	p := testProfile()
	p.MTU = 1280
	p.TunnelIPv4 = ""
	u := testUnderlay()
	u.SourcePort = windowsDynamicPortMin + 77
	b, err := BuildCandidateLaneBootstrap(p, u, 1)
	if err != nil {
		t.Fatal(err)
	}
	b.Ticket = strings.Repeat("ab", 32)
	b.TunnelConfig = testAuthenticatedTunnel()
	plan, err := BuildCandidateLanePlan(p, b)
	if err != nil {
		t.Fatal(err)
	}
	// 1280 connection - 40 IPv4/TCP - 32 DTLS reserve - 56 FEC = 1152 LINK.
	if !argPair(plan.Link.Args, "-mtu", "1152") {
		t.Fatalf("Game LINK args=%v want derived MTU 1152", plan.Link.Args)
	}
}

// BuildPlan remains a public runtime planning primitive even though current
// product Connect wraps it with BuildMultiLanePlan. Freeze the new contract:
// Profile.MTU is the connection ceiling, and neither TUN nor LINK receives it
// directly.
func TestBuildPlanKeepsConnectionLinkAndInnerMTUSeparate(t *testing.T) {
	p := testProfile()
	p.MTU = 1280
	p.TunnelIPv4 = testAuthenticatedTunnel().Address4
	u := testUnderlay()
	plan, err := BuildPlan(p, u, strings.Repeat("ab", 32))
	if err != nil {
		t.Fatal(err)
	}
	// Game consumes its 40-byte private envelope after LINK, leaving 1112 inner IP.
	if !argPair(plan.TUN.Args, "-mtu", "1112") {
		t.Fatalf("TUN args=%v want derived inner MTU 1112", plan.TUN.Args)
	}
	if !argPair(plan.Link.Args, "-mtu", "1152") {
		t.Fatalf("LINK args=%v want derived LINK plaintext MTU 1152", plan.Link.Args)
	}
	if argPair(plan.TUN.Args, "-mtu", "1280") || argPair(plan.Link.Args, "-mtu", "1280") {
		t.Fatalf("connection MTU leaked directly into inner layers: tun=%v link=%v", plan.TUN.Args, plan.Link.Args)
	}
}
