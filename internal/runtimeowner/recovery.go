package runtimeowner

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

const (
	// These are migrated as bounded compatibility starting values from the
	// frozen old ARQ, not as a production recommendation. P5 qualification
	// decides whether they remain suitable for the active runtime.
	steadyRepairBudgetDivisor = uint64(5)
	steadyRepairBurstBytes    = uint64(128 * 1024)
	steadyRepairDefer         = 100 * time.Millisecond
)

type selectedRepair struct {
	record *pendingRecord
	seq    uint32
	cost   uint64
	fast   bool
	seg    faketcp.Segment
}

func (t *laneTransport) refillRepairCreditLocked(fresh uint64) {
	if fresh == 0 {
		return
	}
	credit := fresh / steadyRepairBudgetDivisor
	t.repairRemainder += fresh % steadyRepairBudgetDivisor
	credit += t.repairRemainder / steadyRepairBudgetDivisor
	t.repairRemainder %= steadyRepairBudgetDivisor
	if credit >= steadyRepairBurstBytes-t.repairCredit {
		t.repairCredit = steadyRepairBurstBytes
		t.repairRemainder = 0
		return
	}
	t.repairCredit += credit
}

func (t *laneTransport) repairCostLocked(p *pendingRecord) uint64 {
	if p == nil || p.flags&faketcp.FlagFIN != 0 {
		return 0
	}
	cost := uint64(len(p.payload))
	if cost == 0 {
		return 0
	}
	shift := p.retries
	if shift > 3 {
		shift = 3
	}
	return cost << shift
}

func (t *laneTransport) reserveRepairLocked(p *pendingRecord, now time.Time, fast bool) *selectedRepair {
	if p == nil || p.sacked || p.retired || p.repairInFlight {
		return nil
	}
	if len(p.payload) == 0 && p.flags&faketcp.FlagFIN == 0 {
		return nil
	}
	if !p.repairNotBefore.IsZero() && now.Before(p.repairNotBefore) {
		return nil
	}
	cost := t.repairCostLocked(p)
	if cost > t.repairCredit {
		p.repairNotBefore = now.Add(steadyRepairDefer)
		t.stats.RepairDeferred++
		return nil
	}
	p.repairNotBefore = time.Time{}
	p.repairInFlight = true
	t.repairCredit -= cost
	t.stats.RepairSelected++
	return &selectedRepair{
		record: p,
		seq:    p.seq,
		cost:   cost,
		fast:   fast,
	}
}

func (t *laneTransport) emitSelectedRepair(sel *selectedRepair, now time.Time) error {
	if sel == nil {
		return nil
	}

	t.mu.Lock()
	current := t.pending[sel.seq]
	if current != sel.record || current.sacked || current.retired {
		if current == sel.record {
			current.repairInFlight = false
		}
		if sel.cost != 0 {
			if sel.cost >= steadyRepairBurstBytes-t.repairCredit {
				t.repairCredit = steadyRepairBurstBytes
			} else {
				t.repairCredit += sel.cost
			}
		}
		t.mu.Unlock()
		return nil
	}
	// Form the actual repair only at the emit boundary so a concurrent receive
	// advancement can contribute the newest ACK/SACK state. Selection itself
	// never mutates retry/RTO accounting.
	sel.seg = t.outboundSegmentFlags(
		current.seq, t.recvNext, current.flags, current.payload,
	)
	t.stats.RepairAttempts++
	t.mu.Unlock()

	err := t.cfg.Emit(sel.seg)

	t.mu.Lock()
	current = t.pending[sel.seq]
	if current == sel.record {
		current.repairInFlight = false
	}
	if err != nil {
		if sel.cost != 0 {
			if sel.cost >= steadyRepairBurstBytes-t.repairCredit {
				t.repairCredit = steadyRepairBurstBytes
			} else {
				t.repairCredit += sel.cost
			}
		}
		t.stats.RepairFailures++
		t.mu.Unlock()
		return err
	}

	t.stats.RepairSucceeded++
	t.stats.Retransmitted++
	t.stats.RepairBudgetSpent += sel.cost
	if sel.fast {
		t.stats.FastRepairs++
	} else {
		t.stats.RTORepairs++
	}
	if sel.record.flags&faketcp.FlagFIN != 0 {
		t.stats.FINTransmits++
	}
	if current == sel.record && !current.sacked && !current.retired {
		current.lastSent = now
		current.retries++
		current.wasRetried = true
	}
	if !sel.fast {
		if !t.timeoutEpisode {
			t.timeoutEpisode = true
			t.timeoutEpisodeEnd = sel.record.end
		}
		t.rto = t.clampRTOLocked(t.rto * 2)
	}
	t.mu.Unlock()
	return nil
}

func (t *laneTransport) tickRecovery(now time.Time) error {
	var (
		sel     *selectedRepair
		ackOnly *faketcp.Segment
	)

	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}

	t.expirePendingHeadLocked(now)

	limit := t.repairCount
	if limit > steadyRepairScanBudget {
		limit = steadyRepairScanBudget
	}
	p := t.repairScan
	if p == nil || !p.repairLinked {
		p = t.repairHead
	}
	for i := 0; p != nil && i < limit; i++ {
		next := p.repairNext
		if next == nil {
			next = t.repairHead
		}
		t.repairScan = next
		if p.sacked || p.retired || p.repairInFlight ||
			(len(p.payload) == 0 && p.flags&faketcp.FlagFIN == 0) {
			p = next
			continue
		}
		if !p.repairNotBefore.IsZero() && now.Before(p.repairNotBefore) {
			p = next
			continue
		}
		if p.lastSent.IsZero() || now.Sub(p.lastSent) >= t.effectiveRepairRTOLocked(p) {
			sel = t.reserveRepairLocked(p, now, false)
			break
		}
		p = next
	}

	forgiven := t.forgiveGapLocked(now, false)
	if sel == nil && forgiven {
		seg := t.outboundSegment(t.sendNext, t.recvNext, nil)
		ackOnly = &seg
	}
	t.mu.Unlock()

	if sel != nil {
		return t.emitSelectedRepair(sel, now)
	}
	if ackOnly != nil {
		return t.cfg.Emit(*ackOnly)
	}
	return nil
}

func (t *laneTransport) evictRepairForFreshLocked() bool {
	t.advancePendingHeadLocked()
	for i := t.pendingHead; i < len(t.pendingOrder); i++ {
		p := t.pending[t.pendingOrder[i]]
		if p == nil || p.flags&faketcp.FlagFIN != 0 || p.repairInFlight {
			continue
		}
		if p.retired || p.sacked {
			t.removePendingLocked(p)
			t.stats.RepairMetadataEvicted++
			t.compactPendingOrderLocked()
			return true
		}
	}
	for i := t.pendingHead; i < len(t.pendingOrder); i++ {
		p := t.pending[t.pendingOrder[i]]
		if p == nil || p.flags&faketcp.FlagFIN != 0 || p.repairInFlight {
			continue
		}
		t.removePendingLocked(p)
		t.stats.Abandoned++
		t.stats.RepairEvicted++
		t.compactPendingOrderLocked()
		return true
	}
	return false
}

func (t *laneTransport) retireSelectiveACKLocked(ack uint32, now time.Time) {
	oldAck := t.lastAck
	if !seqLT(oldAck, ack) {
		return
	}

	var sample *pendingRecord
	for {
		t.advancePendingHeadLocked()
		if t.pendingHead >= len(t.pendingOrder) {
			break
		}
		p := t.pending[t.pendingOrder[t.pendingHead]]
		if p == nil {
			continue
		}
		if !seqLE(p.end, ack) {
			break
		}
		if p.end == ack && !p.wasRetried && !p.rttSampled {
			p.rttSampled = true
			sample = p
		}
		t.noteDeliveredLocked(p)
		t.removePendingLocked(p)
		t.stats.Acked++
		if p.flags&faketcp.FlagFIN != 0 && !t.localFINAcked {
			t.localFINAcked = true
			t.stats.FINAcked++
		}
	}
	t.lastAck = ack
	t.pruneSenderSACKLocked()
	if sample != nil {
		t.observeRTTLocked(now.Sub(sample.firstSent))
	}
	if t.timeoutEpisode && seqLE(t.timeoutEpisodeEnd, ack) {
		t.timeoutEpisode = false
		t.timeoutEpisodeEnd = 0
		t.rto = t.baseRTO
	}
	t.compactPendingOrderLocked()
}

func (t *laneTransport) markSACKedLocked(p *pendingRecord, now time.Time) {
	if p == nil || p.sacked {
		return
	}
	p.sacked = true
	t.sackedOutstanding++
	t.unlinkRepairLocked(p)
	t.stats.SACKed++
	t.noteDeliveredLocked(p)
	if !p.wasRetried && !p.rttSampled {
		p.rttSampled = true
		t.observeRTTLocked(now.Sub(p.firstSent))
	}
	if p.payload != nil {
		p.payload = nil
		p.retired = true
		p.repairNotBefore = time.Time{}
		t.stats.SACKRetired++
	}
}

func (t *laneTransport) selectFastRepairLocked(now time.Time) *selectedRepair {
	candidate := t.pendingAtHeadLocked()
	if candidate == nil || candidate.sacked || candidate.retired ||
		candidate.flags&faketcp.FlagFIN != 0 || candidate.seq != t.lastAck {
		return nil
	}
	if candidate.wasRetried {
		if t.rackLatestTx.IsZero() || candidate.lastSent.IsZero() ||
			!candidate.lastSent.Before(t.rackLatestTx) ||
			now.Sub(candidate.lastSent) < t.rackReorderingWindowLocked() {
			return nil
		}
		return t.prepareFastRepairLocked(candidate, now)
	}
	if t.sackedOutstanding < 3 || candidate.lastSent.IsZero() ||
		now.Before(candidate.lastSent) ||
		now.Sub(candidate.lastSent) < t.rackReorderingWindowLocked() {
		return nil
	}
	return t.prepareFastRepairLocked(candidate, now)
}

func (t *laneTransport) prepareFastRepairLocked(p *pendingRecord, now time.Time) *selectedRepair {
	return t.reserveRepairLocked(p, now, true)
}

func (t *laneTransport) noteDeliveredLocked(p *pendingRecord) {
	if p == nil || p.lastSent.IsZero() {
		return
	}
	if t.rackLatestTx.IsZero() || t.rackLatestTx.Before(p.lastSent) {
		t.rackLatestTx = p.lastSent
	}
}

func (t *laneTransport) effectiveRepairRTOLocked(p *pendingRecord) time.Duration {
	rto := t.rto
	if p == nil {
		return t.clampRTOLocked(rto)
	}
	// The connection-level timeout episode still backs off when there is no
	// evidence that the path is making progress. With the active absolute 3s
	// repair horizon, however, carrying that global backoff onto a record that
	// has already been retried can eliminate its final bounded repair entirely.
	// Likewise, newer delivered/SACKed transmission evidence means an older
	// record should use the clean base estimator rather than inherit a stale
	// cumulative-hole penalty. Progressive repair credit remains the bandwidth
	// backoff for repeated shadow repairs.
	if p.wasRetried ||
		(!t.rackLatestTx.IsZero() && !p.lastSent.IsZero() &&
			p.lastSent.Before(t.rackLatestTx) && t.baseRTO < rto) {
		rto = t.baseRTO
	}
	return t.clampRTOLocked(rto)
}

func (t *laneTransport) rackReorderingWindowLocked() time.Duration {
	if t.srtt <= 0 {
		return 10 * time.Millisecond
	}
	v := t.srtt / 4
	if v < 10*time.Millisecond {
		v = 10 * time.Millisecond
	}
	return v
}

func (t *laneTransport) observeRTTLocked(sample time.Duration) {
	if sample <= 0 {
		return
	}
	if t.srtt == 0 {
		t.srtt = sample
		t.rttvar = sample / 2
	} else {
		d := t.srtt - sample
		if d < 0 {
			d = -d
		}
		t.rttvar = (3*t.rttvar + d) / 4
		t.srtt = (7*t.srtt + sample) / 8
	}
	t.baseRTO = t.clampRTOLocked(t.srtt + 4*t.rttvar)
	if !t.timeoutEpisode {
		t.rto = t.baseRTO
	}
}

func (t *laneTransport) clampRTOLocked(v time.Duration) time.Duration {
	if v < t.cfg.InitialRTO {
		v = t.cfg.InitialRTO
	}
	if v > t.cfg.RepairHorizon {
		v = t.cfg.RepairHorizon
	}
	return v
}
