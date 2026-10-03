package datapath

import (
	"bytes"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// A missing old systematic must not hold new business; its late parity can
// still repair that one datagram while the explicitly retained old lane drains.
func TestRetiringReceiveRecoversOldFECWithoutHoldingFresh(t *testing.T) {
	lease := gameLease(t)
	addr, _ := lease.Config.LeaseIPv4()
	owner, err := NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	oldTx := leasedTestLane(t, RoleClient, 20, 80, lease)
	defer oldTx.Close()
	oldRx := leasedTestLane(t, RoleServer, 20, 80, lease)
	old, err := owner.AttachInitial(1, oldRx)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(800, 0)
	packet := businessIPv4Packet(addr, netip.MustParseAddr("1.1.1.1"), []byte("old-missing-systematic"))
	systematic, err := oldTx.Outbound(packet, now)
	if err != nil || len(systematic) != 1 {
		t.Fatalf("systematic=%d err=%v", len(systematic), err)
	}
	parity, err := oldTx.FlushDue(now.Add(20 * time.Millisecond))
	if err != nil || len(parity) == 0 {
		t.Fatalf("parity=%d err=%v", len(parity), err)
	}
	freshRx := leasedTestLane(t, RoleServer, 20, 81, lease)
	if err := owner.BeginSameIDReplacement(old.Ref, freshRx); err != nil {
		t.Fatal(err)
	}
	unknown := logicaltunnel.LaneRef{ID: old.Ref.ID, Generation: old.Ref.Generation + 1}
	if _, err := owner.InboundPayload(unknown, parity[0].Wire, now); !errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) {
		t.Fatalf("candidate was accepted before promotion: %v", err)
	}
	fresh, err := owner.PromoteSameIDReplacement(old.Ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.ValidateGeneration(old.Ref); !errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) {
		t.Fatalf("retiring outbound authority revived: %v", err)
	}
	freshTx := leasedTestLane(t, RoleClient, 20, 81, lease)
	defer freshTx.Close()
	freshPacket := businessIPv4Packet(addr, netip.MustParseAddr("1.1.1.1"), []byte("fresh-before-old-repair"))
	freshRecords, err := freshTx.Outbound(freshPacket, now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := owner.InboundPayload(fresh.Ref, freshRecords[0].Wire, now)
	if err != nil || len(got.Datagrams) != 1 || !bytes.Equal(got.Datagrams[0], freshPacket) {
		t.Fatalf("fresh was blocked by old gap: %+v err=%v", got, err)
	}
	got, err = owner.InboundPayload(old.Ref, parity[0].Wire, now.Add(300*time.Millisecond))
	if err != nil || len(got.Datagrams) != 1 || !bytes.Equal(got.Datagrams[0], packet) {
		t.Fatalf("retiring FEC recovery: %+v err=%v", got, err)
	}
	got, err = owner.InboundPayload(old.Ref, systematic[0].Wire, now.Add(400*time.Millisecond))
	if err != nil || len(got.Datagrams) != 0 {
		t.Fatalf("late recovered source duplicated delivery: %+v err=%v", got, err)
	}
	spoof := businessIPv4Packet(netip.MustParseAddr("10.99.0.9"), netip.MustParseAddr("1.1.1.1"), []byte("spoof"))
	spoofRecords, err := oldTx.Outbound(spoof, now.Add(500*time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	got, err = owner.InboundPayload(old.Ref, spoofRecords[0].Wire, now.Add(600*time.Millisecond))
	if err != nil || len(got.Datagrams) != 0 || len(got.PathErrors) != 1 {
		t.Fatalf("retiring bypassed lease source fence: %+v err=%v", got, err)
	}
	if err := owner.RetireIncarnation(old.Ref); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.InboundPayload(old.Ref, parity[0].Wire, now.Add(time.Second)); !errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) {
		t.Fatalf("retired generation revived: %v", err)
	}
}

func TestRetiringGameReceiveSharesFirstArrivalDedupe(t *testing.T) {
	lease := gameLease(t)
	addr, _ := lease.Config.LeaseIPv4()
	pair := newGameOwnerPair(t, lease, 2)
	now := time.Unix(900, 0)
	packet := businessIPv4Packet(addr, netip.MustParseAddr("1.1.1.1"), []byte("in-flight-game"))
	out, err := pair.client.GameOutbound(packet, now)
	if err != nil {
		t.Fatal(err)
	}
	var oldRecords, otherRecords []WireRecord
	for _, lane := range out.Lanes {
		if lane.Ref.ID == 1 {
			oldRecords = lane.Records
		} else {
			otherRecords = lane.Records
		}
	}
	old := pair.serverRef[1]
	if err := pair.server.BeginSameIDReplacement(old, leasedTestLane(t, RoleServer, 0, 90, lease)); err != nil {
		t.Fatal(err)
	}
	if _, err := pair.server.PromoteSameIDReplacement(old); err != nil {
		t.Fatal(err)
	}
	got := deliverGameLane(t, pair.server, old, oldRecords, now.Add(300*time.Millisecond))
	if len(got.Datagrams) != 1 || !bytes.Equal(got.Datagrams[0], packet) {
		t.Fatalf("retiring Game first arrival: %+v", got)
	}
	got = deliverGameLane(t, pair.server, pair.serverRef[2], otherRecords, now.Add(400*time.Millisecond))
	if len(got.Datagrams) != 0 || len(got.PathErrors) != 0 {
		t.Fatalf("active Game duplicate: %+v", got)
	}
	if err := pair.server.RetireIncarnation(old); err != nil {
		t.Fatal(err)
	}
	if _, err := pair.server.GameInboundPayload(old, oldRecords[0].Wire, now.Add(time.Second)); !errors.Is(err, logicaltunnel.ErrStaleLaneGeneration) {
		t.Fatalf("retired Game generation revived: %v", err)
	}
}
