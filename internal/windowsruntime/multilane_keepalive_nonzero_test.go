package windowsruntime

import (
	"fmt"
	"testing"
)

func TestBuildMultiLanePlanPropagatesNonzeroKeepaliveToInitialLinks(t *testing.T) {
	tunnel := testAuthenticatedTunnel()
	for _, tc := range []struct {
		lanes   int
		seconds int
	}{
		{lanes: 1, seconds: 7},
		{lanes: 4, seconds: 30},
	} {
		t.Run(fmt.Sprintf("%dlane-%ds", tc.lanes, tc.seconds), func(t *testing.T) {
			p := testProfile()
			p.TunnelIPv4 = ""
			p.Lanes = tc.lanes
			keepalive := tc.seconds
			p.KeepaliveSeconds = &keepalive

			boots := make([]LaneBootstrap, 0, tc.lanes)
			for id := 1; id <= tc.lanes; id++ {
				boots = append(boots, authenticatedLane(t, p, id, uint16(windowsDynamicPortMin+id), tunnel))
			}
			plan, err := BuildMultiLanePlan(p, boots)
			if err != nil {
				t.Fatal(err)
			}
			want := fmt.Sprintf("%ds", tc.seconds)
			for _, lane := range plan.Lanes {
				if !argPair(lane.Link.Args, "-keepalive", want) {
					t.Fatalf("lane %d link args=%v; want explicit -keepalive %s", lane.ID, lane.Link.Args, want)
				}
			}
		})
	}
}
