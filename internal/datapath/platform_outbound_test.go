package datapath

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func TestNormalOwnerOutboundDoesNotCreateBusinessFlow(t *testing.T) {
	owner, err := NewTunnelOwner(1, 4)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	lane := tunnelTestLane(t, RoleServer, 0, 120)
	if _, err := owner.AttachInitial(1, lane); err != nil {
		t.Fatal(err)
	}
	records, err := owner.NormalOutbound([]byte("shared-tun-return"), time.Unix(700, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PN != 0 {
		t.Fatalf("records=%d pn=%d", len(records), firstPN(records))
	}
	stats := owner.Stats()
	if stats.BusinessFlows != 0 || stats.ActiveLogicalLanes != 1 || stats.PhysicalLanes != 1 {
		t.Fatalf("owner stats=%+v", stats)
	}
}

func TestNormalOwnerOutboundPreservesClientLeaseSourceFence(t *testing.T) {
	lease := gameLease(t)
	leaseAddr, err := lease.Config.LeaseIPv4()
	if err != nil {
		t.Fatal(err)
	}
	owner, err := NewLeasedTunnelOwner(lease, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Close()
	lane := leasedTestLane(t, RoleClient, 0, 121, lease)
	if _, err := owner.AttachInitial(1, lane); err != nil {
		t.Fatal(err)
	}

	spoof := businessIPv4Packet(
		netip.MustParseAddr("10.77.0.200"),
		netip.MustParseAddr("1.1.1.1"),
		nil,
	)
	if records, err := owner.NormalOutbound(spoof, time.Unix(701, 0)); !errors.Is(err, logicaltunnel.ErrSourceSpoof) || records != nil {
		t.Fatalf("spoof records=%v err=%v", records, err)
	}
	if stats := lane.Stats(); stats.OutboundDatagrams != 0 || stats.OutboundRecords != 0 {
		t.Fatalf("spoof mutated lane stats=%+v", stats)
	}

	valid := businessIPv4Packet(leaseAddr, netip.MustParseAddr("1.1.1.1"), nil)
	records, err := owner.NormalOutbound(valid, time.Unix(702, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0].PN != 0 {
		t.Fatalf("valid records=%d pn=%d", len(records), firstPN(records))
	}
}
