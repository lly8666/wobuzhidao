package linuxserver

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/platformflow"
)

type fakeServiceHandler struct {
	calls   int
	handled bool
	err     error
	last    []byte
}

func (h *fakeServiceHandler) HandleServicePacket(packet []byte, _ time.Time) (bool, error) {
	h.calls++
	h.last = append([]byte(nil), packet...)
	return h.handled, h.err
}

func TestSharedTUNRouterConsumesReservedPlatformServiceBeforeTUN(t *testing.T) {
	writer := &fakeWriter{}
	r, err := NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 2, writer)
	if err != nil {
		t.Fatal(err)
	}
	owner := &fakeOwner{
		lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.7"),
		stats: datapath.TunnelOwnerStats{DesiredLanes: 1},
	}
	token, err := r.Register(owner)
	if err != nil {
		t.Fatal(err)
	}
	leaseAddr, _ := owner.lease.Config.LeaseIPv4()
	service, err := platformflow.MarshalPacket(leaseAddr, platformflow.Frame{
		Kind: platformflow.KindUDPDatagram, FlowID: 7,
		Peer: netip.MustParseAddrPort("198.51.100.10:53"),
		Payload: []byte("service"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := r.DeliverFromOwnerAt(token, [][]byte{service}, time.Unix(1, 0)); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("missing handler err=%v", err)
	}
	if len(writer.packets) != 0 {
		t.Fatal("reserved service packet leaked to TUN")
	}

	handler := &fakeServiceHandler{handled: true}
	if err := r.SetServiceHandler(token, handler); err != nil {
		t.Fatal(err)
	}
	if err := r.DeliverFromOwnerAt(token, [][]byte{service}, time.Unix(2, 0)); err != nil {
		t.Fatal(err)
	}
	if handler.calls != 1 || len(writer.packets) != 0 {
		t.Fatalf("handler calls=%d tun=%d", handler.calls, len(writer.packets))
	}

	ordinary := ipv4Packet(leaseAddr, netip.MustParseAddr("8.8.8.8"), []byte("ordinary"))
	if err := r.DeliverFromOwnerAt(token, [][]byte{ordinary}, time.Unix(3, 0)); err != nil {
		t.Fatal(err)
	}
	if len(writer.packets) != 1 {
		t.Fatalf("ordinary TUN writes=%d", len(writer.packets))
	}

	bad := append([]byte(nil), service...)
	bad[10] ^= 0xff
	if err := r.DeliverFromOwnerAt(token, [][]byte{bad}, time.Unix(4, 0)); !errors.Is(err, platformflow.ErrMalformed) {
		t.Fatalf("bad service err=%v", err)
	}
	if len(writer.packets) != 1 {
		t.Fatal("malformed service packet leaked to TUN")
	}

	rebound := &fakeOwner{lease: owner.lease.Clone(), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	fresh, err := r.Register(rebound)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.DeliverFromOwnerAt(token, [][]byte{service}, time.Unix(5, 0)); !errors.Is(err, ErrStaleBinding) {
		t.Fatalf("stale token err=%v", err)
	}
	if err := r.DeliverFromOwnerAt(fresh, [][]byte{service}, time.Unix(6, 0)); !errors.Is(err, ErrServiceUnavailable) {
		t.Fatalf("rebind inherited handler err=%v", err)
	}
	if handler.calls != 1 {
		t.Fatalf("stale handler calls=%d", handler.calls)
	}
}
