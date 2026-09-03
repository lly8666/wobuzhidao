package windowsruntime

import (
	"errors"
	"strings"
	"testing"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func TestCandidateLaneUsesPrivateSlotFiveWithSameLogicalID(t *testing.T) {
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 4
	u := testUnderlay()
	u.SourcePort = windowsDynamicPortMin + 99

	b, err := BuildCandidateLaneBootstrap(p, u, 4)
	if err != nil { t.Fatal(err) }
	if b.ID != 4 { t.Fatalf("candidate logical lane id=%d want=4", b.ID) }
	if !strings.HasSuffix(b.TicketPath, ".lane4.candidate.slot5") || !strings.HasSuffix(b.TunnelConfigPath, ".lane4.candidate.slot5") {
		t.Fatalf("candidate state paths ticket=%q config=%q", b.TicketPath, b.TunnelConfigPath)
	}
	if b.FakeTCP.Name != "faketcp-4-candidate-s5" { t.Fatalf("candidate bootstrap name=%q", b.FakeTCP.Name) }
	if !argPair(b.FakeTCP.Args, "--local-udp", "127.0.0.1:45105") { t.Fatalf("candidate FakeTCP args=%v", b.FakeTCP.Args) }
	if !argPair(b.FakeTCP.Args, "--source", "192.0.2.20:49251") { t.Fatalf("candidate source port missing: %v", b.FakeTCP.Args) }

	b.Ticket = strings.Repeat("ab", 32)
	b.TunnelConfig = testAuthenticatedTunnel()
	plan, err := BuildCandidateLanePlan(p, b)
	if err != nil { t.Fatal(err) }
	if plan.ID != 4 || plan.Slot != makeBeforeBreakCandidateSlot { t.Fatalf("candidate id/slot=%d/%d", plan.ID, plan.Slot) }
	got, err := LaneGameTarget(plan)
	if err != nil { t.Fatal(err) }
	if got != "127.0.0.1:47105" { t.Fatalf("candidate Game target=%q", got) }
	if plan.FakeTCP.Name != "faketcp-4-candidate-s5" || plan.DTLS.Name != "dtls-4-candidate-s5" || plan.Link.Name != "link-4-candidate-s5" {
		t.Fatalf("candidate names=%s/%s/%s", plan.FakeTCP.Name, plan.DTLS.Name, plan.Link.Name)
	}
}

func TestCandidateLaneKeepsLogicalRangeAndPrivateSlotBounded(t *testing.T) {
	p := testProfile()
	p.TunnelIPv4 = ""
	p.Lanes = 4
	u := testUnderlay()
	u.SourcePort = windowsDynamicPortMin + 100

	if _, err := BuildCandidateLaneBootstrapSlot(p, u, 5, makeBeforeBreakCandidateSlot); !errors.Is(err, logicaltunnel.ErrTransportLanes) {
		t.Fatalf("fifth logical lane error=%v", err)
	}
	if _, err := BuildCandidateLaneBootstrapSlot(p, u, 4, makeBeforeBreakCandidateSlot+1); err == nil || !strings.Contains(err.Error(), "transport slot must be 1..5") {
		t.Fatalf("sixth transport slot error=%v", err)
	}
}

func TestNextReplacementSlotAlternatesCandidateAndNormalSlot(t *testing.T) {
	if got := NextReplacementSlot(LanePlan{ID: 3, Slot: 3}); got != makeBeforeBreakCandidateSlot {
		t.Fatalf("normal slot next=%d want=%d", got, makeBeforeBreakCandidateSlot)
	}
	if got := NextReplacementSlot(LanePlan{ID: 3, Slot: makeBeforeBreakCandidateSlot}); got != 3 {
		t.Fatalf("candidate slot next=%d want=3", got)
	}
}

func TestNextReplacementSlotForPlansRotatesOneSpareAcrossFourLanes(t *testing.T) {
	plans := map[int]LanePlan{
		1: {ID: 1, Slot: 1},
		2: {ID: 2, Slot: 2},
		3: {ID: 3, Slot: 3},
		4: {ID: 4, Slot: 4},
	}
	sequence := []struct {
		lane int
		want int
	}{
		{lane: 1, want: 5},
		{lane: 2, want: 1},
		{lane: 3, want: 2},
		{lane: 4, want: 3},
		{lane: 1, want: 4},
	}
	for _, step := range sequence {
		current := plans[step.lane]
		got, err := NextReplacementSlotForPlans(current, plans)
		if err != nil { t.Fatal(err) }
		if got != step.want {
			t.Fatalf("lane %d spare=%d want=%d plans=%v", step.lane, got, step.want, plans)
		}
		plans[step.lane] = LanePlan{ID: step.lane, Slot: got}

		used := map[int]int{}
		for id, plan := range plans {
			if owner, exists := used[plan.Slot]; exists {
				t.Fatalf("slot %d shared by authoritative lanes %d and %d", plan.Slot, owner, id)
			}
			used[plan.Slot] = id
		}
		if len(used) != 4 {
			t.Fatalf("authoritative physical slots=%v want=4 unique", used)
		}
	}
}

func TestNextReplacementSlotForPlansRejectsAuthoritativeSlotCollision(t *testing.T) {
	plans := map[int]LanePlan{
		1: {ID: 1, Slot: 5},
		2: {ID: 2, Slot: 5},
	}
	if _, err := NextReplacementSlotForPlans(plans[1], plans); err == nil || !strings.Contains(err.Error(), "authoritative") {
		t.Fatalf("authoritative slot collision err=%v", err)
	}
}
