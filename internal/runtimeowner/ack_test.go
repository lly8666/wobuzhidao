package runtimeowner

import (
	"errors"
	"net/netip"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func ackTestTransport(delay time.Duration, emit faketcp.SegmentEmitter) *laneTransport {
	return &laneTransport{cfg: TransportConfig{ACKDelay: delay, Emit: emit, SACKPermitted: true}, recvNext: 100}
}

func TestACKCoalescesTwoAndProtectsGapPiggyback(t *testing.T) {
	var sent []faketcp.Segment
	tr := ackTestTransport(time.Hour, func(seg faketcp.Segment) error {
		sent = append(sent, seg)
		return nil
	})
	defer tr.close()
	for i := 0; i < 3; i++ {
		tr.recvNext++
		if err := tr.sendACK(false); err != nil {
			t.Fatal(err)
		}
		if len(sent) != []int{1, 1, 2}[i] {
			t.Fatalf("i=%d ACK count=%d", i, len(sent))
		}
	}
	if sent[1].Ack != 103 || tr.stats.ACKCoalesced != 1 {
		t.Fatalf("ACK=%+v stats=%+v", sent[1], tr.stats)
	}
	tr.recvNext++
	if err := tr.sendACK(false); err != nil {
		t.Fatal(err)
	}
	tr.noteACKPiggybackLocked(faketcp.Segment{Ack: 103})
	if !tr.ackPending {
		t.Fatal("older piggyback cancelled newer ACK")
	}
	tr.recvSACK[0] = faketcp.SACKBlock{Start: 110, End: 120}
	tr.recvSACKN = 1
	tr.noteACKPiggybackLocked(faketcp.Segment{Ack: 104})
	if !tr.ackPending {
		t.Fatal("payload piggyback suppressed SACK")
	}
	if err := tr.sendACK(false); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 3 || sent[2].SACKN != 1 || tr.ackPending {
		t.Fatalf("gap ACK=%+v pending=%v", sent, tr.ackPending)
	}
	tr.recvSACKN = 0
	if err := tr.sendACK(false); err != nil {
		t.Fatal(err)
	}
	tr.noteACKPiggybackLocked(faketcp.Segment{Ack: 104})
	if tr.ackPending || tr.stats.ACKPiggybacked != 1 {
		t.Fatal("successful covering piggyback did not cancel timer")
	}
	if err := tr.sendACK(false); err != nil {
		t.Fatal(err)
	}
	if err := tr.sendACK(true); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 4 || tr.ackPending {
		t.Fatal("urgent FIN/duplicate feedback deferred")
	}
}

func TestACKSparseTimerAndAsyncFailureAreObservable(t *testing.T) {
	sent := make(chan faketcp.Segment, 4)
	errEmit := errors.New("timer emit failed")
	tr := ackTestTransport(DefaultACKDelay, func(seg faketcp.Segment) error {
		sent <- seg
		return errEmit
	})
	defer tr.close()
	tr.ackSentOnce = true
	if err := tr.sendACK(false); err != nil {
		t.Fatal(err)
	}
	select {
	case seg := <-sent:
		if seg.Ack != 100 {
			t.Fatalf("timer ACK=%d", seg.Ack)
		}
	case <-time.After(time.Second):
		t.Fatal("sparse ACK waited for owner tick or never fired")
	}
	deadline := time.Now().Add(time.Second)
	for {
		tr.mu.Lock()
		ready := tr.ackAsyncError != nil
		tr.mu.Unlock()
		if ready {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("asynchronous error swallowed")
		}
		time.Sleep(time.Millisecond)
	}
	if err := tr.tickRecovery(time.Now()); !errors.Is(err, errEmit) {
		t.Fatalf("tick error=%v", err)
	}
	if tr.statsSnapshot().ACKTimerFailures != 1 {
		t.Fatal("missing timer failure counter")
	}
}

func TestACKStoppedCallbackCannotFlushReusedOrClosedLane(t *testing.T) {
	sent := 0
	tr := ackTestTransport(time.Hour, func(faketcp.Segment) error { sent++; return nil })
	tr.ackSentOnce = true
	if err := tr.sendACK(false); err != nil {
		t.Fatal(err)
	}
	tr.flushACKTimer()
	if sent != 0 || !tr.ackPending {
		t.Fatal("stale callback flushed the new future deadline")
	}
	tr.close()
	tr.flushACKTimer()
	if sent != 0 || tr.ackPending {
		t.Fatal("closed lane timer emitted or remained pending")
	}
}

func TestDeferredACKNeverDefersBusinessDelivery(t *testing.T) {
	lease := runtimeLease(t)
	owner, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	delivered := 0
	rt, err := New(owner, func(packets [][]byte, _ time.Time) error {
		delivered += len(packets)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close()
	acks := 0
	clientCfg, serverCfg := transportPair(nil, func(faketcp.Segment) error { acks++; return nil }, 1, 1000)
	snapshot, err := rt.AttachInitial(1, runtimeLane(t, datapath.RoleServer, lease, 0, 7), serverCfg)
	if err != nil {
		t.Fatal(err)
	}
	// Deterministic count-path test: sparse real timer is covered separately.
	tr := rt.lanes[snapshot.Ref]
	tr.cfg.ACKDelay = time.Hour
	clientLane := runtimeLane(t, datapath.RoleClient, lease, 0, 7)
	defer clientLane.Close()
	addr, _ := lease.Config.LeaseIPv4()
	seq := clientCfg.SendNext
	for i := 0; i < 3; i++ {
		records, err := clientLane.Outbound(runtimeIPv4(addr, netip.MustParseAddr("1.1.1.1"), []byte{byte(i)}), time.Now())
		if err != nil || len(records) != 1 {
			t.Fatalf("records=%d err=%v", len(records), err)
		}
		seg := faketcp.Segment{SrcIP: clientCfg.LocalIP, DstIP: clientCfg.PeerIP,
			SrcPort: clientCfg.LocalPort, DstPort: clientCfg.PeerPort, Seq: seq,
			Ack: serverCfg.SendNext, Flags: faketcp.FlagACK | faketcp.FlagPSH, Payload: records[0].Wire}
		if err := rt.HandleSegment(snapshot.Ref, seg, time.Now()); err != nil {
			t.Fatal(err)
		}
		seq += uint32(len(seg.Payload))
		if delivered != i+1 || acks != []int{1, 1, 2}[i] {
			t.Fatalf("i=%d immediate deliveries=%d ACKs=%d", i, delivered, acks)
		}
	}
}
