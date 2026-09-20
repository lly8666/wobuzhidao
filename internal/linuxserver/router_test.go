package linuxserver

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

type fakeWriter struct {
	packets [][]byte
	short   bool
	err     error
}

func (w *fakeWriter) WritePacket(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.packets = append(w.packets, append([]byte(nil), p...))
	if w.short {
		return len(p) - 1, nil
	}
	return len(p), nil
}

type fakeOwner struct {
	lease logicaltunnel.Lease
	stats datapath.TunnelOwnerStats

	normalCalls int
	gameCalls   int
	lastPacket  []byte
}

func (o *fakeOwner) Lease() (logicaltunnel.Lease, bool) { return o.lease.Clone(), true }
func (o *fakeOwner) Stats() datapath.TunnelOwnerStats   { return o.stats }
func (o *fakeOwner) NormalOutbound(p []byte, _ time.Time) ([]datapath.WireRecord, error) {
	o.normalCalls++
	o.lastPacket = append([]byte(nil), p...)
	return []datapath.WireRecord{{PN: 1, Wire: []byte{1}}}, nil
}
func (o *fakeOwner) GameOutbound(p []byte, _ time.Time) (datapath.GameOutboundResult, error) {
	o.gameCalls++
	o.lastPacket = append([]byte(nil), p...)
	return datapath.GameOutboundResult{PacketID: 7}, nil
}

func testLease(t *testing.T, idHex, addr string) logicaltunnel.Lease {
	t.Helper()
	id, err := logicaltunnel.ParseTunnelID(idHex)
	if err != nil {
		t.Fatal(err)
	}
	return logicaltunnel.Lease{
		Account: "acct",
		Config: logicaltunnel.TunnelConfig{
			TunnelID: id,
			Address4: addr + "/32",
		},
	}
}

func ipv4Packet(src, dst netip.Addr, payload []byte) []byte {
	p := make([]byte, 20+len(payload))
	p[0] = 0x45
	binary.BigEndian.PutUint16(p[2:4], uint16(len(p)))
	copy(p[12:16], src.AsSlice())
	copy(p[16:20], dst.AsSlice())
	copy(p[20:], payload)
	return p
}

func TestSharedTUNRouterDemuxesTwoLeasesAndModes(t *testing.T) {
	writer := &fakeWriter{}
	r, err := NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 4, writer)
	if err != nil {
		t.Fatal(err)
	}
	a := &fakeOwner{
		lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.1"),
		stats: datapath.TunnelOwnerStats{DesiredLanes: 1},
	}
	b := &fakeOwner{
		lease: testLease(t, "ffeeddccbbaa99887766554433221100", "10.66.0.2"),
		stats: datapath.TunnelOwnerStats{DesiredLanes: 3},
	}
	if _, err := r.Register(a); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register(b); err != nil {
		t.Fatal(err)
	}
	if r.ActiveTunnels() != 2 {
		t.Fatalf("active=%d", r.ActiveTunnels())
	}

	toA := ipv4Packet(netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.66.0.1"), []byte("a"))
	outA, err := r.RouteFromTUN(toA, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if outA.IsGame || len(outA.Normal) != 1 || a.normalCalls != 1 || b.gameCalls != 0 || !bytes.Equal(a.lastPacket, toA) {
		t.Fatalf("normal route out=%+v a=%d b=%d", outA, a.normalCalls, b.gameCalls)
	}

	toB := ipv4Packet(netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("10.66.0.2"), []byte("b"))
	outB, err := r.RouteFromTUN(toB, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if !outB.IsGame || outB.Game.PacketID != 7 || b.gameCalls != 1 || !bytes.Equal(b.lastPacket, toB) {
		t.Fatalf("game route out=%+v b=%d", outB, b.gameCalls)
	}

	miss := ipv4Packet(netip.MustParseAddr("9.9.9.9"), netip.MustParseAddr("10.66.0.99"), nil)
	if _, err := r.RouteFromTUN(miss, time.Unix(3, 0)); !errors.Is(err, ErrNoLeaseRoute) {
		t.Fatalf("missing lease err=%v", err)
	}
}

func TestRegisterRejectsLeaseTunnelConflictsAndAllowsExactRebind(t *testing.T) {
	r, err := NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 2, &fakeWriter{})
	if err != nil {
		t.Fatal(err)
	}
	a := &fakeOwner{lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.1"), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	tokenA, err := r.Register(a)
	if err != nil {
		t.Fatal(err)
	}
	dupLease := &fakeOwner{lease: testLease(t, "ffeeddccbbaa99887766554433221100", "10.66.0.1"), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	if _, err := r.Register(dupLease); !errors.Is(err, ErrLeaseInUse) {
		t.Fatalf("duplicate lease err=%v", err)
	}
	dupTunnel := &fakeOwner{lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.2"), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	if _, err := r.Register(dupTunnel); !errors.Is(err, ErrTunnelInUse) {
		t.Fatalf("duplicate tunnel err=%v", err)
	}

	rebound := &fakeOwner{lease: a.lease.Clone(), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	tokenB, err := r.Register(rebound)
	if err != nil {
		t.Fatal(err)
	}
	if tokenA == tokenB || r.Unregister(tokenA) {
		t.Fatal("stale binding token remained authoritative after exact rebind")
	}
	if !r.Unregister(tokenB) || r.ActiveTunnels() != 0 {
		t.Fatalf("fresh unregister active=%d", r.ActiveTunnels())
	}
}

func TestDeliverFromOwnerFencesSourceAndStaleToken(t *testing.T) {
	writer := &fakeWriter{}
	r, _ := NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 2, writer)
	owner := &fakeOwner{lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.1"), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	token, err := r.Register(owner)
	if err != nil {
		t.Fatal(err)
	}
	valid := ipv4Packet(netip.MustParseAddr("10.66.0.1"), netip.MustParseAddr("8.8.8.8"), []byte("query"))
	if err := r.DeliverFromOwner(token, [][]byte{valid}); err != nil {
		t.Fatal(err)
	}
	if len(writer.packets) != 1 || !bytes.Equal(writer.packets[0], valid) {
		t.Fatalf("writer packets=%x", writer.packets)
	}

	spoof := ipv4Packet(netip.MustParseAddr("10.66.0.2"), netip.MustParseAddr("8.8.8.8"), nil)
	if err := r.DeliverFromOwner(token, [][]byte{spoof}); !errors.Is(err, logicaltunnel.ErrSourceSpoof) {
		t.Fatalf("spoof err=%v", err)
	}
	if len(writer.packets) != 1 {
		t.Fatal("spoofed packet reached shared TUN")
	}

	rebound := &fakeOwner{lease: owner.lease.Clone(), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	fresh, err := r.Register(rebound)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.DeliverFromOwner(token, [][]byte{valid}); !errors.Is(err, ErrStaleBinding) {
		t.Fatalf("old token err=%v", err)
	}
	if err := r.DeliverFromOwner(fresh, [][]byte{valid}); err != nil {
		t.Fatal(err)
	}
	if len(writer.packets) != 2 {
		t.Fatalf("fresh write count=%d", len(writer.packets))
	}
}

func TestRouterRejectsMalformedIPv4AndShortTUNWrite(t *testing.T) {
	writer := &fakeWriter{short: true}
	r, _ := NewSharedTUNRouter(netip.MustParsePrefix("10.66.0.0/16"), 1, writer)
	owner := &fakeOwner{lease: testLease(t, "00112233445566778899aabbccddeeff", "10.66.0.1"), stats: datapath.TunnelOwnerStats{DesiredLanes: 1}}
	token, _ := r.Register(owner)
	if _, err := r.RouteFromTUN([]byte{0x45}, time.Now()); !errors.Is(err, ErrInvalidIPv4) {
		t.Fatalf("short route err=%v", err)
	}
	valid := ipv4Packet(netip.MustParseAddr("10.66.0.1"), netip.MustParseAddr("8.8.8.8"), nil)
	if err := r.DeliverFromOwner(token, [][]byte{valid}); !errors.Is(err, ErrShortTUNWrite) {
		t.Fatalf("short write err=%v", err)
	}
}
