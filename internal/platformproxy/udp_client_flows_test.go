package platformproxy

import (
	"net/netip"
	"testing"
	"time"
)

func TestUDPClientFlowTableStableIdentityAndIsolation(t *testing.T) {
	table := NewUDPClientFlowTable(10 * time.Second)
	now := time.Unix(100, 0)
	clientA := netip.MustParseAddrPort("192.0.2.10:40000")
	clientB := netip.MustParseAddrPort("192.0.2.11:40000")
	peerA := netip.MustParseAddrPort("198.51.100.53:53")
	peerB := netip.MustParseAddrPort("203.0.113.53:53")

	a1, err := table.Forward(clientA, peerA, now)
	if err != nil {
		t.Fatal(err)
	}
	a2, err := table.Forward(clientA, peerA, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if a1.FlowID == 0 || a2.FlowID != a1.FlowID {
		t.Fatalf("stable flow id failed: first=%d second=%d", a1.FlowID, a2.FlowID)
	}

	otherPeer, err := table.Forward(clientA, peerB, now)
	if err != nil {
		t.Fatal(err)
	}
	otherClient, err := table.Forward(clientB, peerA, now)
	if err != nil {
		t.Fatal(err)
	}
	if otherPeer.FlowID == a1.FlowID || otherClient.FlowID == a1.FlowID || otherPeer.FlowID == otherClient.FlowID {
		t.Fatalf("distinct client/peer tuples reused ids: %+v %+v %+v", a1, otherPeer, otherClient)
	}
	if table.Len() != 3 {
		t.Fatalf("len=%d want=3", table.Len())
	}
}

func TestUDPClientFlowTableReversePinsPeer(t *testing.T) {
	table := NewUDPClientFlowTable(time.Minute)
	now := time.Unix(200, 0)
	client := netip.MustParseAddrPort("192.0.2.10:41000")
	peer := netip.MustParseAddrPort("198.51.100.9:5353")
	flow, err := table.Forward(client, peer, now)
	if err != nil {
		t.Fatal(err)
	}
	got, err := table.Reverse(flow.FlowID, peer, now.Add(time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if got != flow {
		t.Fatalf("reverse=%+v want=%+v", got, flow)
	}
	if _, err := table.Reverse(flow.FlowID, netip.MustParseAddrPort("198.51.100.10:5353"), now); err == nil {
		t.Fatal("reverse accepted changed remote peer")
	}
	if _, err := table.Reverse(flow.FlowID+999, peer, now); err == nil {
		t.Fatal("reverse accepted unknown flow id")
	}
}

func TestUDPClientFlowTableExpire(t *testing.T) {
	table := NewUDPClientFlowTable(5 * time.Second)
	start := time.Unix(300, 0)
	client := netip.MustParseAddrPort("192.0.2.10:42000")
	peer := netip.MustParseAddrPort("198.51.100.20:53")
	flow, err := table.Forward(client, peer, start)
	if err != nil {
		t.Fatal(err)
	}
	if expired := table.Expire(start.Add(4 * time.Second)); len(expired) != 0 {
		t.Fatalf("expired early: %+v", expired)
	}
	if expired := table.Expire(start.Add(5 * time.Second)); len(expired) != 1 || expired[0] != flow {
		t.Fatalf("expired=%+v want=%+v", expired, flow)
	}
	if table.Len() != 0 {
		t.Fatalf("len=%d want=0", table.Len())
	}
	if _, err := table.Reverse(flow.FlowID, peer, start.Add(6*time.Second)); err == nil {
		t.Fatal("expired flow still routed in reverse")
	}
	fresh, err := table.Forward(client, peer, start.Add(6*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if fresh.FlowID == 0 || fresh.FlowID == flow.FlowID {
		t.Fatalf("fresh flow id=%d old=%d", fresh.FlowID, flow.FlowID)
	}
}

func TestUDPClientFlowTableRejectsUnspecifiedEndpoints(t *testing.T) {
	table := NewUDPClientFlowTable(time.Minute)
	peer := netip.MustParseAddrPort("198.51.100.20:53")
	if _, err := table.Forward(netip.MustParseAddrPort("0.0.0.0:1234"), peer, time.Now()); err == nil {
		t.Fatal("accepted unspecified client")
	}
	if _, err := table.Forward(netip.MustParseAddrPort("192.0.2.1:1234"), netip.MustParseAddrPort("[::]:53"), time.Now()); err == nil {
		t.Fatal("accepted unspecified peer")
	}
}
