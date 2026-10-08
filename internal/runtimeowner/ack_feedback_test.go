package runtimeowner

import (
	"bytes"
	"errors"
	"net/netip"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

func waitACKFeedback(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatal("ACK worker did not complete")
	}
}

func TestACKFeedbackBlockedWriteKeepsNoHOLAndOnlyLatestACK(t *testing.T) {
	lease := runtimeLease(t)
	addr, _ := lease.Config.LeaseIPv4()
	co, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	so, err := datapath.NewLeasedTunnelOwner(lease, 1, 8)
	if err != nil {
		t.Fatal(err)
	}
	var fresh []faketcp.Segment
	acks := make(chan faketcp.Segment, 8)
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	var calls atomic.Int64
	cc, sc := transportPair(func(seg faketcp.Segment) error { fresh = append(fresh, seg); return nil }, func(seg faketcp.Segment) error {
		if calls.Add(1) == 1 {
			// The callback has been counted and is committed to waiting
			// for release. An ACK channel send alone does not order the
			// independent calls counter observation.
			acks <- seg
			close(entered)
			<-release
			return nil
		}
		acks <- seg
		return nil
	}, 1, 1000)
	sc.AsyncACKFeedback = true
	sc.ACKDelay = DefaultACKDelay
	client, err := New(co, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	delivered := make(chan []byte, 4)
	server, err := New(so, func(p [][]byte, _ time.Time) error {
		for _, v := range p {
			delivered <- append([]byte(nil), v...)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if _, err = client.AttachInitial(1, runtimeLane(t, datapath.RoleClient, lease, 0, 7), cc); err != nil {
		t.Fatal(err)
	}
	snap, err := server.AttachInitial(1, runtimeLane(t, datapath.RoleServer, lease, 0, 7), sc)
	if err != nil {
		t.Fatal(err)
	}
	packets := make([][]byte, 3)
	now := time.Unix(1000, 0)
	for i := range packets {
		packets[i] = runtimeIPv4(addr, netip.MustParseAddr("1.1.1.1"), []byte{byte(i)})
		if err := client.SendPacket(packets[i], now); err != nil {
			t.Fatal(err)
		}
	}
	if len(fresh) != 3 {
		t.Fatal("unexpected source record count")
	}
	done := make(chan error, 1)
	go func() { done <- server.HandleSegment(snap.Ref, fresh[0], now) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("receive waited for native ACK write")
	}
	waitACKFeedback(t, entered)
	select {
	case <-acks:
	case <-time.After(time.Second):
		t.Fatal("worker did not publish the first ACK")
	}
	// Both an out-of-order first arrival and the missing earlier record must
	// deliver while the same native ACK write remains blocked.
	for _, i := range []int{2, 1} {
		if err := server.HandleSegment(snap.Ref, fresh[i], now); err != nil {
			t.Fatal(err)
		}
	}
	for _, i := range []int{0, 2, 1} {
		select {
		case p := <-delivered:
			if !bytes.Equal(p, packets[i]) {
				t.Fatal("delivery order/payload changed")
			}
		default:
			t.Fatal("business waited for ACK or earlier gap")
		}
	}
	for i := 0; i < 100; i++ {
		if err := server.HandleSegment(snap.Ref, fresh[2], now); err != nil {
			t.Fatal(err)
		}
	}
	st, _ := server.TransportStats(snap.Ref)
	if !st.ACKWorkerRunning || !st.ACKWorkerPending || st.ACKWorkerAttempts != 1 || st.ACKWorkerCoalesced < 99 || calls.Load() != 1 {
		t.Fatalf("unbounded or blocking worker: %+v calls=%d", st, calls.Load())
	}
	unblock()
	select {
	case seg := <-acks:
		if seg.Ack != fresh[2].Seq+uint32(len(fresh[2].Payload)) || len(seg.Payload) != 0 || seg.SACKN != 0 {
			t.Fatalf("worker replayed stale ACK: %+v", seg)
		}
	case <-time.After(time.Second):
		t.Fatal("latest ACK not emitted")
	}
	tr := server.lanes[snap.Ref]
	server.Close()
	waitACKFeedback(t, tr.ackWorker.done)
}

func TestACKFeedbackCloseDropsPendingAndFencesOldWorker(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	tr := ackTestTransport(0, func(faketcp.Segment) error { calls.Add(1); close(entered); <-release; return nil })
	tr.cfg.AsyncACKFeedback = true
	if err := tr.sendACK(true); err != nil {
		t.Fatal(err)
	}
	waitACKFeedback(t, entered)
	for i := 0; i < 100; i++ {
		if err := tr.sendACK(true); err != nil {
			t.Fatal(err)
		}
	}
	tr.close()
	tr.close()
	if err := tr.sendACK(true); err != nil {
		t.Fatal(err)
	}
	close(release)
	waitACKFeedback(t, tr.ackWorker.done)
	st := tr.statsSnapshot()
	if calls.Load() != 1 || st.ACKWorkerPending || st.ACKWorkerRunning || !st.Closed {
		t.Fatalf("closed generation emitted queued feedback: %+v", st)
	}
}

func TestACKFeedbackErrorIsLatchedAndReportedByTick(t *testing.T) {
	boom := errors.New("native ACK worker write failed")
	tr := ackTestTransport(0, func(faketcp.Segment) error { return boom })
	tr.cfg.AsyncACKFeedback = true
	defer tr.close()
	if err := tr.sendACK(true); err != nil {
		t.Fatal(err)
	}
	waitACKFeedback(t, tr.ackWorker.done)
	if err := tr.tickRecovery(time.Now()); !errors.Is(err, boom) {
		t.Fatalf("async error swallowed: %v", err)
	}
	if err := tr.sendACK(true); !errors.Is(err, boom) {
		t.Fatalf("failed worker restarted or error lost: %v", err)
	}
	st := tr.statsSnapshot()
	if st.ACKWorkerFailures != 1 || st.ACKWorkerAttempts != 1 || st.ACKWorkerSent != 0 {
		t.Fatalf("failure counters=%+v", st)
	}
}

func TestACKFeedbackSuccessfulPiggybackCancelsOnlyGapFreePending(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int64
	tr := ackTestTransport(0, func(faketcp.Segment) error { calls.Add(1); close(entered); <-release; return nil })
	tr.cfg.AsyncACKFeedback = true
	if err := tr.sendACK(true); err != nil {
		t.Fatal(err)
	}
	waitACKFeedback(t, entered)
	if err := tr.sendACK(true); err != nil {
		t.Fatal(err)
	}
	tr.mu.Lock()
	tr.recvSACKN = 1
	tr.noteACKPiggybackLocked(faketcp.Segment{Ack: tr.recvNext})
	if !tr.ackWorkerPending {
		tr.mu.Unlock()
		t.Fatal("piggyback suppressed SACK")
	}
	tr.recvSACKN = 0
	tr.noteACKPiggybackLocked(faketcp.Segment{Ack: tr.recvNext - 1})
	if !tr.ackWorkerPending {
		tr.mu.Unlock()
		t.Fatal("older piggyback suppressed newer ACK")
	}
	tr.noteACKPiggybackLocked(faketcp.Segment{Ack: tr.recvNext})
	pending := tr.ackWorkerPending
	tr.mu.Unlock()
	if pending {
		t.Fatal("current successful piggyback did not cancel pending ACK")
	}
	tr.close()
	close(release)
	waitACKFeedback(t, tr.ackWorker.done)
	if calls.Load() != 1 || tr.statsSnapshot().ACKWorkerPiggybacked != 1 {
		t.Fatal("pending history replayed")
	}
}

func TestACKFeedbackFINConfirmationRemainsSynchronous(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	returned := make(chan error, 1)
	tr := ackTestTransport(DefaultACKDelay, func(faketcp.Segment) error { close(entered); <-release; return nil })
	tr.cfg.AsyncACKFeedback = true
	defer tr.close()
	go func() {
		returned <- tr.handleSegment(faketcp.Segment{Seq: tr.recvNext, Flags: faketcp.FlagFIN}, time.Now())
	}()
	waitACKFeedback(t, entered)
	select {
	case <-returned:
		t.Fatal("FIN confirmation returned before emit")
	default:
	}
	close(release)
	select {
	case err := <-returned:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIN confirmation did not complete")
	}
	if tr.ackWorker != nil {
		t.Fatal("FIN-only feedback started worker")
	}
}
