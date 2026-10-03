package runtimeowner

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

// sendACK runs after immediate business delivery. Only gap-free advancing
// records coalesce: first arrival, every second record, SACK/gap/duplicate and
// FIN feedback stay immediate. A single reusable lane timer handles sparse
// traffic without depending on the owner's 100ms maintenance tick.
func (t *laneTransport) sendACK(urgent bool) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	if t.cfg.ACKDelay == 0 || urgent || !t.ackSentOnce || t.recvSACKN != 0 {
		t.ackSentOnce = true
		t.cancelACKLocked()
		seg := t.outboundSegment(t.sendNext, t.recvNext, nil)
		t.mu.Unlock()
		return t.cfg.Emit(seg)
	}
	t.ackCount++
	if t.ackCount >= 2 {
		t.stats.ACKCoalesced++
		t.cancelACKLocked()
		seg := t.outboundSegment(t.sendNext, t.recvNext, nil)
		t.mu.Unlock()
		return t.cfg.Emit(seg)
	}
	t.ackPending = true
	t.stats.ACKDeferred++
	t.ackDue = time.Now().Add(t.cfg.ACKDelay)
	if t.ackTimer == nil {
		t.ackTimer = time.AfterFunc(t.cfg.ACKDelay, t.flushACKTimer)
	} else {
		t.ackTimer.Reset(t.cfg.ACKDelay)
	}
	t.mu.Unlock()
	return nil
}

func (t *laneTransport) cancelACKLocked() {
	t.ackPending = false
	t.ackCount = 0
	t.ackDue = time.Time{}
	if t.ackTimer != nil {
		t.ackTimer.Stop()
	}
}

func (t *laneTransport) noteACKPiggybackLocked(seg faketcp.Segment) {
	// Payload records do not carry SACK. An older ACK must not cancel newer
	// receive progress, and any current gap still requires an ACK-only packet.
	if t.ackPending && t.recvSACKN == 0 && seg.Ack == t.recvNext {
		t.stats.ACKPiggybacked++
		t.cancelACKLocked()
	}
}

func (t *laneTransport) flushACKTimer() {
	t.mu.Lock()
	// A stopped callback may already be running when the timer is reused.
	// Check the latest deadline under the lane lock before selecting feedback.
	if t.closed || !t.ackPending || time.Now().Before(t.ackDue) {
		t.mu.Unlock()
		return
	}
	t.cancelACKLocked()
	seg := t.outboundSegment(t.sendNext, t.recvNext, nil)
	t.stats.ACKTimerSent++
	t.mu.Unlock()
	if err := t.cfg.Emit(seg); err != nil {
		t.mu.Lock()
		t.stats.ACKTimerFailures++
		if t.ackAsyncError == nil {
			t.ackAsyncError = err
		}
		t.mu.Unlock()
	}
}
