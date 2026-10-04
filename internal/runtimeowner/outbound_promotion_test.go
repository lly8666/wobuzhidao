package runtimeowner

import (
	"bytes"
	"errors"
	"fmt"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// Stop emission after encoding a multi-fragment packet. Promotion must wait
// for its local send, then the next packet uses only the fresh incarnation.
// In Game, delivery remains first-arrival once, including the untouched lanes.
func TestSourcePacketPinsGenerationThroughAllFragments(t *testing.T) {
	for _, desired := range []int{1, 4} {
		t.Run(fmt.Sprint(desired), func(t *testing.T) {
			lease := runtimeLease(t)
			addr, _ := lease.Config.LeaseIPv4()
			co, _ := datapath.NewLeasedTunnelOwner(lease, desired, 4)
			so, _ := datapath.NewLeasedTunnelOwner(lease, desired, 4)
			var delivered [][]byte
			cr, _ := New(co, nil)
			sr, _ := New(so, func(packets [][]byte, _ time.Time) error {
				delivered = append(delivered, packets...)
				return nil
			})
			defer cr.Close()
			defer sr.Close()
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			type emitted struct { id uint8; segment faketcp.Segment }
			var wire []emitted
			var cmu sync.Mutex
			var cold, sold datapath.TunnelLaneSnapshot
			serverRefs := make(map[uint8]datapath.TunnelLaneSnapshot)
			for id := 1; id <= desired; id++ {
				laneID := uint8(id)
				cc, sc := transportPair(func(seg faketcp.Segment) error {
					if laneID == 1 { once.Do(func() { close(entered); <-release }) }
					cmu.Lock()
					wire = append(wire, emitted{laneID, seg})
					cmu.Unlock()
					return nil
				}, func(faketcp.Segment) error { return nil }, laneID, uint32(id)*1000)
				cs, err := cr.AttachInitial(laneID, runtimeLane(t, datapath.RoleClient, lease, 0, laneID), cc)
				if err != nil { t.Fatal(err) }
				ss, err := sr.AttachInitial(laneID, runtimeLane(t, datapath.RoleServer, lease, 0, laneID), sc)
				if err != nil { t.Fatal(err) }
				serverRefs[laneID] = ss
				if id == 1 { cold, sold = cs, ss }
			}
			cc2, sc2 := transportPair(func(seg faketcp.Segment) error {
				cmu.Lock(); wire = append(wire, emitted{1, seg}); cmu.Unlock()
				return nil
			}, func(faketcp.Segment) error { return nil }, 1, 900000)
			if err := cr.BeginSameIDReplacement(cold.Ref, runtimeLane(t, datapath.RoleClient, lease, 0, 9), cc2); err != nil { t.Fatal(err) }
			if err := sr.BeginSameIDReplacement(sold.Ref, runtimeLane(t, datapath.RoleServer, lease, 0, 9), sc2); err != nil { t.Fatal(err) }
			now := time.Unix(500, 0)
			packet := runtimeIPv4(addr, netip.MustParseAddr("1.1.1.1"), bytes.Repeat([]byte{0x31}, 9000))
			sent := make(chan error, 1)
			go func() { sent <- cr.SendPacket(packet, now) }()
			select { case <-entered: case <-time.After(3*time.Second): t.Fatal("first fragment not emitted") }
			// Ensure this is an actual generation fence, not a fortunate scheduler.
			if cr.outboundMu.TryLock() {
				cr.outboundMu.Unlock(); close(release)
				t.Fatal("source-to-wire emission did not pin generation")
			}
			promoted := make(chan error, 1)
			go func() { _, err := cr.PromoteSameIDReplacement(cold.Ref); promoted <- err }()
			select {
			case err := <-promoted: close(release); t.Fatalf("promotion crossed blocked packet: %v", err)
			case <-time.After(20*time.Millisecond):
			}
			close(release)
			select { case err := <-sent: if err != nil { t.Fatal(err) }; case <-time.After(3*time.Second): t.Fatal("send deadlocked") }
			select { case err := <-promoted: if err != nil { t.Fatal(err) }; case <-time.After(3*time.Second): t.Fatal("promotion deadlocked") }
			if len(wire) < 2 { t.Fatal("packet did not exercise fragmentation") }
			for _, out := range wire {
				if err := sr.HandleSegment(serverRefs[out.id].Ref, out.segment, now); err != nil { t.Fatal(err) }
			}
			if len(delivered) != 1 || !bytes.Equal(delivered[0], packet) { t.Fatalf("in-flight unique deliveries=%d", len(delivered)) }
			fresh, err := sr.PromoteSameIDReplacement(sold.Ref)
			if err != nil { t.Fatal(err) }
			serverRefs[1] = fresh
			wire = nil
			packet2 := runtimeIPv4(addr, netip.MustParseAddr("8.8.8.8"), bytes.Repeat([]byte{0x32}, 6000))
			if err := cr.SendPacket(packet2, now.Add(time.Second)); err != nil { t.Fatal(err) }
			for _, out := range wire {
				if out.id == 1 && seqLT(out.segment.Seq, cc2.SendNext) { t.Fatal("new packet emitted on old incarnation") }
				if err := sr.HandleSegment(serverRefs[out.id].Ref, out.segment, now.Add(time.Second)); err != nil { t.Fatal(err) }
			}
			if len(delivered) != 2 || !bytes.Equal(delivered[1], packet2) { t.Fatalf("fresh unique deliveries=%d", len(delivered)) }
		})
	}
}

func TestSourcePacketDoesNotRetryAfterPartialEmission(t *testing.T) {
	lease := runtimeLease(t)
	addr, _ := lease.Config.LeaseIPv4()
	owner, _ := datapath.NewLeasedTunnelOwner(lease, 1, 4)
	r, _ := New(owner, nil)
	defer r.Close()
	failed := errors.New("second fragment emission failed")
	count := 0
	cfg, _ := transportPair(func(faketcp.Segment) error {
		count++
		if count == 2 { return failed }
		return nil
	}, func(faketcp.Segment) error { return nil }, 1, 1000)
	if _, err := r.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 1), cfg); err != nil { t.Fatal(err) }
	packet := runtimeIPv4(addr, netip.MustParseAddr("1.1.1.1"), bytes.Repeat([]byte{0x31}, 9000))
	if err := r.SendPacket(packet, time.Now()); !errors.Is(err, failed) { t.Fatalf("send error=%v", err) }
	if count != 2 { t.Fatalf("partial packet retried or continued: emissions=%d", count) }
	if !r.outboundMu.TryLock() { t.Fatal("failed emission retained promotion fence") }
	r.outboundMu.Unlock()
}
