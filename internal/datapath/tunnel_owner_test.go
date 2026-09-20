package datapath

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func tunnelTestLane(t *testing.T, role Role, parity int, seed byte) *Lane {
	t.Helper()
	cfg := pairConfig(role, parity)
	cfg.IncarnationNonce[0] = seed
	cfg.Keys.C2S.AEADKey[0] ^= seed
	cfg.Keys.C2S.HPKey[0] ^= seed
	cfg.Keys.C2S.IV[0] ^= seed
	cfg.Keys.S2C.AEADKey[0] ^= seed
	cfg.Keys.S2C.HPKey[0] ^= seed
	cfg.Keys.S2C.IV[0] ^= seed
	lane, err := NewLane(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return lane
}

func TestNormalBusinessFlowsReuseOneLaneAndFlowCloseIsLocal(t *testing.T) {
	owner, err := NewTunnelOwner(1, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	client := tunnelTestLane(t, RoleClient, 0, 1)
	server := tunnelTestLane(t, RoleServer, 0, 1)
	defer server.Close()
	if _, err := owner.AttachInitial(1, client); err != nil {
		t.Fatal(err)
	}
	a, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	b, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	if st := owner.Stats(); st.BusinessFlows != 2 || st.ActiveLogicalLanes != 1 || st.PhysicalLanes != 1 {
		t.Fatalf("initial owner stats=%+v", st)
	}
	t0 := time.Unix(200, 0)
	aRecords, err := a.Outbound([]byte("flow-a"), t0)
	if err != nil || len(aRecords) != 1 || aRecords[0].PN != 0 {
		t.Fatalf("flow A records=%d pn=%d err=%v", len(aRecords), firstPN(aRecords), err)
	}
	bRecords, err := b.Outbound([]byte("flow-b"), t0.Add(time.Millisecond))
	if err != nil || len(bRecords) != 1 || bRecords[0].PN != 1 {
		t.Fatalf("flow B records=%d pn=%d err=%v", len(bRecords), firstPN(bRecords), err)
	}
	gotA := deliverRecords(t, server, aRecords, t0)
	gotB := deliverRecords(t, server, bRecords, t0.Add(time.Millisecond))
	if len(gotA) != 1 || string(gotA[0]) != "flow-a" || len(gotB) != 1 || string(gotB[0]) != "flow-b" {
		t.Fatalf("sibling deliveries A=%q B=%q", gotA, gotB)
	}
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	bAgain, err := b.Outbound([]byte("flow-b-again"), t0.Add(2*time.Millisecond))
	if err != nil || len(bAgain) != 1 || bAgain[0].PN != 2 {
		t.Fatalf("flow B after sibling close records=%d pn=%d err=%v", len(bAgain), firstPN(bAgain), err)
	}
	if client.Stats().Closed {
		t.Fatal("closing one business flow closed the shared lane")
	}
	if client.Stats().PaddingRequests != 0 || client.Stats().PaddingBytes != 0 {
		t.Fatalf("normal flow unexpectedly enabled padding: %+v", client.Stats())
	}
	if st := owner.Stats(); st.BusinessFlows != 1 || st.PhysicalLanes != 1 {
		t.Fatalf("post-close owner stats=%+v", st)
	}
}

func TestSameIDReplacementFencesLateGenerationAndDoesNotHotSwitchFEC(t *testing.T) {
	owner, err := NewTunnelOwner(1, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	oldLane := tunnelTestLane(t, RoleClient, 4, 2)
	oldSnap, err := owner.AttachInitial(1, oldLane)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	oldBinding, err := owner.normalBinding(flow.ID())
	if err != nil {
		t.Fatal(err)
	}
	if oldBinding.ref != oldSnap.Ref || oldBinding.lane.Config().ParityShards != 4 {
		t.Fatalf("old binding=%+v parity=%d", oldBinding.ref, oldBinding.lane.Config().ParityShards)
	}
	candidate := tunnelTestLane(t, RoleClient, 20, 3)
	if err := owner.BeginSameIDReplacement(oldSnap.Ref, candidate); err != nil {
		t.Fatal(err)
	}
	before, err := owner.normalBinding(flow.ID())
	if err != nil {
		t.Fatal(err)
	}
	if before.ref != oldSnap.Ref || before.lane.Config().ParityShards != 4 {
		t.Fatal("candidate changed authoritative FEC profile before promotion")
	}
	fresh, err := owner.PromoteSameIDReplacement(oldSnap.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Ref.ID != oldSnap.Ref.ID || fresh.Ref.Generation <= oldSnap.Ref.Generation || fresh.ParityShards != 20 {
		t.Fatalf("old=%+v fresh=%+v", oldSnap, fresh)
	}
	if oldLane.Config().ParityShards != 4 || candidate.Config().ParityShards != 20 {
		t.Fatal("replacement mutated an existing lane FEC profile")
	}
	lateWire, err := oldBinding.lane.Outbound([]byte("late-old-generation"), time.Unix(209, 0))
	if err != nil || len(lateWire) == 0 {
		t.Fatalf("late old work records=%d err=%v", len(lateWire), err)
	}
	if accepted, err := owner.FenceOutbound(oldSnap.Ref, lateWire); !errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) || accepted != nil {
		t.Fatalf("late old generation accepted records=%d err=%v", len(accepted), err)
	}
	if err := owner.ValidateGeneration(fresh.Ref); err != nil {
		t.Fatalf("fresh generation rejected: %v", err)
	}
	if st := owner.Stats(); st.BusinessFlows != 1 || st.ActiveLogicalLanes != 1 || st.PhysicalLanes != 2 || st.Retiring != 1 || st.GenerationDiscards != 1 {
		t.Fatalf("replacement stats=%+v", st)
	}
	records, err := flow.Outbound([]byte("new-generation"), time.Unix(210, 0))
	if err != nil || len(records) == 0 || records[0].PN != 0 {
		t.Fatalf("fresh flow records=%d pn=%d err=%v", len(records), firstPN(records), err)
	}
	if err := owner.RetireIncarnation(oldSnap.Ref); err != nil {
		t.Fatal(err)
	}
	if !oldLane.Stats().Closed {
		t.Fatal("retired old lane remained open")
	}
	if st := owner.Stats(); st.PhysicalLanes != 1 || st.Retiring != 0 {
		t.Fatalf("retirement stats=%+v", st)
	}
}

func TestFailedCandidateKeepsHealthyOldGeneration(t *testing.T) {
	owner, err := NewTunnelOwner(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	old := tunnelTestLane(t, RoleClient, 8, 4)
	snap, err := owner.AttachInitial(1, old)
	if err != nil {
		t.Fatal(err)
	}
	flow, _ := owner.OpenFlow()
	candidate := tunnelTestLane(t, RoleClient, 10, 5)
	if err := owner.BeginSameIDReplacement(snap.Ref, candidate); err != nil {
		t.Fatal(err)
	}
	if err := owner.FailSameIDReplacement(snap.Ref); err != nil {
		t.Fatal(err)
	}
	if !candidate.Stats().Closed {
		t.Fatal("failed candidate was not closed")
	}
	if err := owner.ValidateGeneration(snap.Ref); err != nil {
		t.Fatalf("candidate failure disturbed old generation: %v", err)
	}
	if _, err := flow.Outbound([]byte("still-old"), time.Unix(220, 0)); err != nil {
		t.Fatal(err)
	}
	if st := owner.Stats(); st.PhysicalLanes != 1 || st.Candidates != 0 || st.BusinessFlows != 1 {
		t.Fatalf("candidate-failure stats=%+v", st)
	}
}

func TestDormantPreservesFlowRegistryAndWakeUsesFreshGeneration(t *testing.T) {
	owner, err := NewTunnelOwner(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	old := tunnelTestLane(t, RoleClient, 0, 6)
	oldSnap, err := owner.AttachInitial(1, old)
	if err != nil {
		t.Fatal(err)
	}
	flow, err := owner.OpenFlow()
	if err != nil {
		t.Fatal(err)
	}
	refs, err := owner.Dormant()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 || refs[0] != oldSnap.Ref || !old.Stats().Closed {
		t.Fatalf("dormant refs=%+v oldClosed=%v", refs, old.Stats().Closed)
	}
	if st := owner.Stats(); !st.Dormant || st.BusinessFlows != 1 || st.PhysicalLanes != 0 {
		t.Fatalf("dormant stats=%+v", st)
	}
	if _, err := flow.Outbound([]byte("wake-me"), time.Unix(230, 0)); !errors.Is(err, ErrTunnelDormant) {
		t.Fatalf("dormant flow err=%v", err)
	}
	wake := tunnelTestLane(t, RoleClient, 0, 7)
	fresh, err := owner.AttachInitial(1, wake)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Ref.Generation <= oldSnap.Ref.Generation {
		t.Fatalf("wake generation old=%+v fresh=%+v", oldSnap.Ref, fresh.Ref)
	}
	if _, err := flow.Outbound([]byte("awake"), time.Unix(231, 0)); err != nil {
		t.Fatalf("preserved flow did not resume on wake: %v", err)
	}
}

func TestFlowRegistryBoundAndPhysicalLaneCountIndependentOfFlowGrowth(t *testing.T) {
	owner, err := NewTunnelOwner(1, 3)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err := owner.AttachInitial(1, tunnelTestLane(t, RoleClient, 0, 8)); err != nil {
		t.Fatal(err)
	}
	flows := make([]*BusinessFlow, 0, 3)
	for i := 0; i < 3; i++ {
		f, err := owner.OpenFlow()
		if err != nil {
			t.Fatal(err)
		}
		flows = append(flows, f)
	}
	if _, err := owner.OpenFlow(); !errors.Is(err, ErrFlowLimit) {
		t.Fatalf("unbounded flow registry err=%v", err)
	}
	if st := owner.Stats(); st.BusinessFlows != 3 || st.PhysicalLanes != 1 || st.ActiveLogicalLanes != 1 {
		t.Fatalf("flow-growth stats=%+v", st)
	}
	if err := flows[0].Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.OpenFlow(); err != nil {
		t.Fatalf("released flow slot not reusable: %v", err)
	}
	if st := owner.Stats(); st.BusinessFlows != 3 || st.PhysicalLanes != 1 {
		t.Fatalf("flow-slot reuse stats=%+v", st)
	}
}

func TestGameLogicalLanePolicyAndTenPhysicalIncarnationBoundStayUnchanged(t *testing.T) {
	owner, err := NewTunnelOwner(4, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	var lane1 TunnelLaneSnapshot
	for id := uint8(1); id <= 4; id++ {
		snap, err := owner.AttachInitial(id, tunnelTestLane(t, RoleClient, 0, 10+id))
		if err != nil {
			t.Fatalf("attach game lane %d: %v", id, err)
		}
		if id == 1 {
			lane1 = snap
		}
	}
	extra := tunnelTestLane(t, RoleClient, 0, 20)
	if _, err := owner.AttachInitial(5, extra); err == nil {
		extra.Close()
		t.Fatal("fifth logical lane was accepted")
	}
	extra.Close()
	if got := owner.ActiveLanes(); len(got) != 4 {
		t.Fatalf("active game lanes=%d", len(got))
	}
	current := lane1.Ref
	for i := 0; i < logicaltunnel.MaxRetiringPublicTransportIncarnations; i++ {
		candidate := tunnelTestLane(t, RoleClient, 0, byte(30+i))
		if err := owner.BeginSameIDReplacement(current, candidate); err != nil {
			t.Fatalf("begin replacement %d: %v", i, err)
		}
		fresh, err := owner.PromoteSameIDReplacement(current)
		if err != nil {
			t.Fatalf("promote replacement %d: %v", i, err)
		}
		current = fresh.Ref
	}
	if st := owner.Stats(); st.ActiveLogicalLanes != 4 || st.Retiring != 6 || st.PhysicalLanes != logicaltunnel.MaxConcurrentPublicTransportIncarnations {
		t.Fatalf("physical bound stats=%+v", st)
	}
	overflow := tunnelTestLane(t, RoleClient, 0, 50)
	if err := owner.BeginSameIDReplacement(current, overflow); !errors.Is(err, ErrPhysicalIncarnationLimit) {
		overflow.Close()
		t.Fatalf("11th physical incarnation err=%v", err)
	}
	overflow.Close()
	if st := owner.Stats(); st.PhysicalLanes != 10 || st.ActiveLogicalLanes != 4 {
		t.Fatalf("overflow mutated registry=%+v", st)
	}
}

func TestTransitionHeadroomBoundIsSixEvenInNormalMode(t *testing.T) {
	owner, err := NewTunnelOwner(1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	initial := tunnelTestLane(t, RoleClient, 0, 70)
	snap, err := owner.AttachInitial(1, initial)
	if err != nil {
		t.Fatal(err)
	}
	current := snap.Ref
	retired := make([]logicaltunnel.LaneRef, 0, logicaltunnel.MaxRetiringPublicTransportIncarnations)
	for i := 0; i < logicaltunnel.MaxRetiringPublicTransportIncarnations; i++ {
		candidate := tunnelTestLane(t, RoleClient, 0, byte(71+i))
		if err := owner.BeginSameIDReplacement(current, candidate); err != nil {
			t.Fatalf("begin replacement %d: %v", i, err)
		}
		retired = append(retired, current)
		fresh, err := owner.PromoteSameIDReplacement(current)
		if err != nil {
			t.Fatalf("promote replacement %d: %v", i, err)
		}
		current = fresh.Ref
	}
	if st := owner.Stats(); st.ActiveLogicalLanes != 1 || st.Retiring != 6 || st.PhysicalLanes != 7 {
		t.Fatalf("normal transition headroom stats=%+v", st)
	}
	overflow := tunnelTestLane(t, RoleClient, 0, 90)
	if err := owner.BeginSameIDReplacement(current, overflow); !errors.Is(err, ErrTransitionIncarnationLimit) {
		overflow.Close()
		t.Fatalf("seventh transition incarnation err=%v", err)
	}
	overflow.Close()
	if err := owner.RetireIncarnation(retired[0]); err != nil {
		t.Fatal(err)
	}
	afterRetire := tunnelTestLane(t, RoleClient, 0, 91)
	if err := owner.BeginSameIDReplacement(current, afterRetire); err != nil {
		afterRetire.Close()
		t.Fatalf("released transition slot not reusable: %v", err)
	}
}

func TestConcurrentNormalFlowsShareLaneUnderRace(t *testing.T) {
	const n = 32
	owner, err := NewTunnelOwner(1, n)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	if _, err := owner.AttachInitial(1, tunnelTestLane(t, RoleClient, 0, 60)); err != nil {
		t.Fatal(err)
	}
	flows := make([]*BusinessFlow, n)
	for i := range flows {
		flows[i], err = owner.OpenFlow()
		if err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	pnCh := make(chan uint64, n)
	errCh := make(chan error, n)
	for i, flow := range flows {
		i, flow := i, flow
		wg.Add(1)
		go func() {
			defer wg.Done()
			records, err := flow.Outbound([]byte{byte(i)}, time.Unix(240, int64(i)))
			if err != nil {
				errCh <- err
				return
			}
			if len(records) != 1 {
				errCh <- errors.New("unexpected record count")
				return
			}
			pnCh <- records[0].PN
			errCh <- flow.Close()
		}()
	}
	wg.Wait()
	close(pnCh)
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
	seen := make(map[uint64]bool, n)
	for pn := range pnCh {
		if seen[pn] {
			t.Fatalf("duplicate PN=%d", pn)
		}
		seen[pn] = true
	}
	if len(seen) != n {
		t.Fatalf("unique PN count=%d want=%d", len(seen), n)
	}
	if st := owner.Stats(); st.BusinessFlows != 0 || st.PhysicalLanes != 1 {
		t.Fatalf("concurrent flow stats=%+v", st)
	}
}
