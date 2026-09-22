package datapath

import (
	"bytes"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/gamelane"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

type gameOwnerPair struct {
	client     *TunnelOwner
	server     *TunnelOwner
	clientLane map[uint8]*Lane
	serverLane map[uint8]*Lane
	clientRef  map[uint8]logicaltunnel.LaneRef
	serverRef  map[uint8]logicaltunnel.LaneRef
}

func newGameOwnerPair(t *testing.T, lease logicaltunnel.Lease, desired int) *gameOwnerPair {
	t.Helper()
	client, err := NewLeasedTunnelOwner(lease, desired, 8)
	if err != nil {
		t.Fatal(err)
	}
	server, err := NewLeasedTunnelOwner(lease, desired, 8)
	if err != nil {
		client.Close()
		t.Fatal(err)
	}
	pair := &gameOwnerPair{
		client:     client,
		server:     server,
		clientLane: make(map[uint8]*Lane, desired),
		serverLane: make(map[uint8]*Lane, desired),
		clientRef:  make(map[uint8]logicaltunnel.LaneRef, desired),
		serverRef:  make(map[uint8]logicaltunnel.LaneRef, desired),
	}
	t.Cleanup(func() {
		client.Close()
		server.Close()
	})
	for id := 1; id <= desired; id++ {
		laneID := uint8(id)
		seed := byte(40 + id)
		cl := leasedTestLane(t, RoleClient, 0, seed, lease)
		sl := leasedTestLane(t, RoleServer, 0, seed, lease)
		cs, err := client.AttachInitial(laneID, cl)
		if err != nil {
			t.Fatal(err)
		}
		ss, err := server.AttachInitial(laneID, sl)
		if err != nil {
			t.Fatal(err)
		}
		pair.clientLane[laneID] = cl
		pair.serverLane[laneID] = sl
		pair.clientRef[laneID] = cs.Ref
		pair.serverRef[laneID] = ss.Ref
	}
	return pair
}

func deliverGameLane(t *testing.T, owner *TunnelOwner, ref logicaltunnel.LaneRef, records []WireRecord, now time.Time) InboundResult {
	t.Helper()
	var out InboundResult
	for _, record := range records {
		result, err := owner.GameInboundPayload(ref, record.Wire, now)
		if err != nil {
			t.Fatal(err)
		}
		out.Datagrams = append(out.Datagrams, result.Datagrams...)
		out.RecordErrors = append(out.RecordErrors, result.RecordErrors...)
		out.PathErrors = append(out.PathErrors, result.PathErrors...)
	}
	return out
}

func gameLease(t *testing.T) logicaltunnel.Lease {
	t.Helper()
	manager := datapathLeaseManager(t)
	lease, err := manager.Acquire("game", datapathInstallation(t, "11112222333344445555666677778888"))
	if err != nil {
		t.Fatal(err)
	}
	return lease
}

func TestGameOwnerRacesSamePacketIDAndFirstArrivalWins(t *testing.T) {
	lease := gameLease(t)
	leaseAddr, err := lease.Config.LeaseIPv4()
	if err != nil {
		t.Fatal(err)
	}
	pair := newGameOwnerPair(t, lease, 3)
	now := time.Unix(500, 0)
	packet := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), []byte("game-one"))

	out, err := pair.client.GameOutbound(packet, now)
	if err != nil {
		t.Fatal(err)
	}
	if out.PacketID != 1 || len(out.Lanes) != 3 || len(out.Failures) != 0 {
		t.Fatalf("outbound packetID=%d lanes=%d failures=%v", out.PacketID, len(out.Lanes), out.Failures)
	}
	for id := uint8(1); id <= 3; id++ {
		stats := pair.clientLane[id].Stats()
		if stats.OutboundDatagrams != 1 {
			t.Fatalf("lane=%d outbound datagrams=%d want=1", id, stats.OutboundDatagrams)
		}
	}

	byID := make(map[uint8]GameLaneRecords, len(out.Lanes))
	for _, lane := range out.Lanes {
		byID[lane.Ref.ID] = lane
	}
	first := deliverGameLane(t, pair.server, pair.serverRef[3], byID[3].Records, now)
	if len(first.PathErrors) != 0 || len(first.Datagrams) != 1 || !bytes.Equal(first.Datagrams[0], packet) {
		t.Fatalf("first arrival datagrams=%d pathErrors=%v", len(first.Datagrams), first.PathErrors)
	}
	for _, id := range []uint8{1, 2} {
		dup := deliverGameLane(t, pair.server, pair.serverRef[id], byID[id].Records, now)
		if len(dup.PathErrors) != 0 || len(dup.Datagrams) != 0 {
			t.Fatalf("duplicate lane=%d datagrams=%d pathErrors=%v", id, len(dup.Datagrams), dup.PathErrors)
		}
	}
	stats := pair.server.Stats()
	if stats.GameDelivered != 1 || stats.GameDuplicates != 2 || stats.GameLogicalOutbound != 0 {
		t.Fatalf("server game stats=%+v", stats)
	}
	clientStats := pair.client.Stats()
	wantCopyBytes := uint64(3 * (gamelane.HeaderSize + len(packet)))
	if clientStats.GameLogicalOutbound != 1 || clientStats.GameLogicalOutboundBytes != uint64(len(packet)) ||
		clientStats.GameLaneCopies != 3 || clientStats.GameLaneCopyBytes != wantCopyBytes {
		t.Fatalf("client game stats=%+v want_copy_bytes=%d", clientStats, wantCopyBytes)
	}

	// Unique PacketIDs remain independently deliverable out of order.
	packet2 := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), []byte("game-two"))
	packet3 := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), []byte("game-three"))
	out2, err := pair.client.GameOutbound(packet2, now.Add(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	out3, err := pair.client.GameOutbound(packet3, now.Add(2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	if out2.PacketID != 2 || out3.PacketID != 3 {
		t.Fatalf("packet IDs=%d,%d", out2.PacketID, out3.PacketID)
	}
	pick := func(out GameOutboundResult, id uint8) GameLaneRecords {
		for _, lane := range out.Lanes {
			if lane.Ref.ID == id {
				return lane
			}
		}
		t.Fatalf("lane %d not found", id)
		return GameLaneRecords{}
	}
	got3 := deliverGameLane(t, pair.server, pair.serverRef[1], pick(out3, 1).Records, now.Add(2*time.Millisecond))
	got2 := deliverGameLane(t, pair.server, pair.serverRef[2], pick(out2, 2).Records, now.Add(3*time.Millisecond))
	if len(got3.Datagrams) != 1 || !bytes.Equal(got3.Datagrams[0], packet3) || len(got2.Datagrams) != 1 || !bytes.Equal(got2.Datagrams[0], packet2) {
		t.Fatalf("out-of-order game delivery got3=%d got2=%d", len(got3.Datagrams), len(got2.Datagrams))
	}

	// The reverse direction may legitimately carry an arbitrary internet source.
	reply := businessIPv4Packet(netip.MustParseAddr("8.8.8.8"), leaseAddr, []byte("reply"))
	back, err := pair.server.GameOutbound(reply, now.Add(4*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	clientIn := deliverGameLane(t, pair.client, pair.clientRef[2], pick(back, 2).Records, now.Add(4*time.Millisecond))
	if len(clientIn.PathErrors) != 0 || len(clientIn.Datagrams) != 1 || !bytes.Equal(clientIn.Datagrams[0], reply) {
		t.Fatalf("reverse game delivery datagrams=%d pathErrors=%v", len(clientIn.Datagrams), clientIn.PathErrors)
	}
}

func TestGameInvalidSourceOrLaneCannotPoisonPacketIDDedupe(t *testing.T) {
	lease := gameLease(t)
	leaseAddr, _ := lease.Config.LeaseIPv4()
	pair := newGameOwnerPair(t, lease, 2)
	now := time.Unix(510, 0)

	// Client owner rejects bad source before allocating a Game PacketID.
	spoof := businessIPv4Packet(netip.MustParseAddr("10.77.0.200"), netip.MustParseAddr("1.1.1.1"), []byte("same-len"))
	if out, err := pair.client.GameOutbound(spoof, now); !errors.Is(err, logicaltunnel.ErrSourceSpoof) || out.PacketID != 0 {
		t.Fatalf("client spoof result=%+v err=%v", out, err)
	}
	valid := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), []byte("same-len"))
	healthy, err := pair.client.GameOutbound(valid, now.Add(time.Millisecond))
	if err != nil || healthy.PacketID != 1 {
		t.Fatalf("first valid packetID=%d err=%v", healthy.PacketID, err)
	}

	var session gamelane.SessionID
	copy(session[:], lease.Config.TunnelID.Bytes())

	// Bypass the client owner: a source-spoofed PacketID=100 must be dropped
	// before the server decoder marks it seen. A valid copy with the same ID on
	// another lane is still the first valid arrival.
	badEnc, _ := gamelane.NewEncoder(session, 100)
	_, badCopies, _ := badEnc.WrapCopies(spoof, []uint8{1, 2})
	badWire, err := pair.clientLane[1].Outbound(badCopies[0].Wire, now.Add(2*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	badResult := deliverGameLane(t, pair.server, pair.serverRef[1], badWire, now.Add(2*time.Millisecond))
	if len(badResult.Datagrams) != 0 || len(badResult.PathErrors) != 1 || !errors.Is(badResult.PathErrors[0], logicaltunnel.ErrSourceSpoof) {
		t.Fatalf("spoofed ingress datagrams=%d pathErrors=%v", len(badResult.Datagrams), badResult.PathErrors)
	}

	goodEnc, _ := gamelane.NewEncoder(session, 100)
	_, goodCopies, _ := goodEnc.WrapCopies(valid, []uint8{1, 2})
	goodWire, err := pair.clientLane[2].Outbound(goodCopies[1].Wire, now.Add(3*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	goodResult := deliverGameLane(t, pair.server, pair.serverRef[2], goodWire, now.Add(3*time.Millisecond))
	if len(goodResult.PathErrors) != 0 || len(goodResult.Datagrams) != 1 || !bytes.Equal(goodResult.Datagrams[0], valid) {
		t.Fatalf("valid same-ID ingress datagrams=%d pathErrors=%v", len(goodResult.Datagrams), goodResult.PathErrors)
	}

	// A lane-2 envelope arriving through lane 1 likewise must not consume ID=101.
	mismatchEnc, _ := gamelane.NewEncoder(session, 101)
	_, mismatchCopies, _ := mismatchEnc.WrapCopies(valid, []uint8{1, 2})
	wrongTransport, err := pair.clientLane[1].Outbound(mismatchCopies[1].Wire, now.Add(4*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	mismatch := deliverGameLane(t, pair.server, pair.serverRef[1], wrongTransport, now.Add(4*time.Millisecond))
	if len(mismatch.Datagrams) != 0 || len(mismatch.PathErrors) != 1 || !errors.Is(mismatch.PathErrors[0], ErrGameLaneMismatch) {
		t.Fatalf("lane mismatch datagrams=%d pathErrors=%v", len(mismatch.Datagrams), mismatch.PathErrors)
	}
	correctTransport, err := pair.clientLane[2].Outbound(mismatchCopies[1].Wire, now.Add(5*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	correct := deliverGameLane(t, pair.server, pair.serverRef[2], correctTransport, now.Add(5*time.Millisecond))
	if len(correct.PathErrors) != 0 || len(correct.Datagrams) != 1 {
		t.Fatalf("correct same-ID datagrams=%d pathErrors=%v", len(correct.Datagrams), correct.PathErrors)
	}
	stats := pair.server.Stats()
	if stats.SourceDiscards != 1 || stats.GameLaneMismatches != 1 {
		t.Fatalf("server stats=%+v", stats)
	}
}

func TestGamePacketIDSurvivesReplacementDormantWakeAndFlowGrowth(t *testing.T) {
	lease := gameLease(t)
	leaseAddr, _ := lease.Config.LeaseIPv4()
	owner, err := NewLeasedTunnelOwner(lease, 2, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()

	lane1 := leasedTestLane(t, RoleClient, 0, 60, lease)
	lane2 := leasedTestLane(t, RoleClient, 0, 61, lease)
	snap1, err := owner.AttachInitial(1, lane1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.AttachInitial(2, lane2); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.OpenFlow(); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.OpenFlow(); err != nil {
		t.Fatal(err)
	}
	if stats := owner.Stats(); stats.BusinessFlows != 2 || stats.ActiveLogicalLanes != 2 || stats.PhysicalLanes != 2 {
		t.Fatalf("flow growth changed lanes: %+v", stats)
	}

	packet := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), []byte("one"))
	first, err := owner.GameOutbound(packet, time.Unix(520, 0))
	if err != nil || first.PacketID != 1 {
		t.Fatalf("first packetID=%d err=%v", first.PacketID, err)
	}

	candidate := leasedTestLane(t, RoleClient, 0, 62, lease)
	if err := owner.BeginSameIDReplacement(snap1.Ref, candidate); err != nil {
		t.Fatal(err)
	}
	fresh, err := owner.PromoteSameIDReplacement(snap1.Ref)
	if err != nil {
		t.Fatal(err)
	}
	second, err := owner.GameOutbound(packet, time.Unix(521, 0))
	if err != nil || second.PacketID != 2 {
		t.Fatalf("post-replacement packetID=%d err=%v", second.PacketID, err)
	}
	var sawFresh bool
	for _, lane := range second.Lanes {
		if lane.Ref.ID == 1 {
			sawFresh = lane.Ref == fresh.Ref
		}
	}
	if !sawFresh {
		t.Fatalf("game send did not use fresh lane ref=%+v lanes=%+v", fresh.Ref, second.Lanes)
	}

	if _, err := owner.Dormant(); err != nil {
		t.Fatal(err)
	}
	if out, err := owner.GameOutbound(packet, time.Unix(522, 0)); !errors.Is(err, ErrTunnelDormant) || out.PacketID != 0 {
		t.Fatalf("dormant outbound=%+v err=%v", out, err)
	}

	wake1 := leasedTestLane(t, RoleClient, 0, 63, lease)
	wake2 := leasedTestLane(t, RoleClient, 0, 64, lease)
	if _, err := owner.AttachInitial(1, wake1); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.AttachInitial(2, wake2); err != nil {
		t.Fatal(err)
	}
	third, err := owner.GameOutbound(packet, time.Unix(523, 0))
	if err != nil || third.PacketID != 3 {
		t.Fatalf("wake packetID=%d err=%v", third.PacketID, err)
	}
	if stats := owner.Stats(); stats.BusinessFlows != 2 || stats.ActiveLogicalLanes != 2 || stats.GameLogicalOutbound != 3 {
		t.Fatalf("wake stats=%+v", stats)
	}
}

func TestGameAPIRejectsNormalSingleLaneOwner(t *testing.T) {
	lease := gameLease(t)
	owner, err := NewLeasedTunnelOwner(lease, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	lane := leasedTestLane(t, RoleClient, 0, 70, lease)
	if _, err := owner.AttachInitial(1, lane); err != nil {
		t.Fatal(err)
	}
	leaseAddr, _ := lease.Config.LeaseIPv4()
	packet := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), nil)
	if _, err := owner.GameOutbound(packet, time.Unix(530, 0)); !errors.Is(err, ErrGameLaneMode) {
		t.Fatalf("normal owner GameOutbound err=%v", err)
	}
}
