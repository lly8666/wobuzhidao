package runtimeowner

import (
	"sort"
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
	for _, seq := range t.pendingOrder {
		p := t.pending[seq]
		if p == nil {
			continue
		}
		if now.Sub(p.firstSent) >= t.cfg.RepairHorizon {
			delete(t.pending, seq)
			if p.retired || p.sacked {
				t.stats.RepairMetadataEvicted++
			} else {
				t.stats.Abandoned++
				t.stats.RepairEvicted++
			}
			continue
		}
		if p.sacked || p.retired || p.repairInFlight ||
			(len(p.payload) == 0 && p.flags&faketcp.FlagFIN == 0) {
			continue
		}
		if !p.repairNotBefore.IsZero() && now.Before(p.repairNotBefore) {
			continue
		}
		if p.lastSent.IsZero() || now.Sub(p.lastSent) >= t.effectiveRepairRTOLocked(p) {
			sel = t.reserveRepairLocked(p, now, false)
			break
		}
	}
	t.compactPendingOrderLocked()

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
	for _, seq := range t.pendingOrder {
		p := t.pending[seq]
		if p == nil || p.flags&faketcp.FlagFIN != 0 || p.repairInFlight {
			continue
		}
		if p.retired || p.sacked {
			delete(t.pending, seq)
			t.stats.RepairMetadataEvicted++
			t.compactPendingOrderLocked()
			return true
		}
	}
	for _, seq := range t.pendingOrder {
		p := t.pending[seq]
		if p == nil || p.flags&faketcp.FlagFIN != 0 || p.repairInFlight {
			continue
		}
		delete(t.pending, seq)
		t.stats.Abandoned++
		t.stats.RepairEvicted++
		t.compactPendingOrderLocked()
		return true
	}
	return false
}

func (t *laneTransport) retireSelectiveACKLocked(ack uint32, now time.Time) {
	oldAck := t.lastAck
	advanced := seqLT(oldAck, ack)
	var sample *pendingRecord
	for _, seq := range t.pendingOrder {
		p := t.pending[seq]
		if p == nil || !seqLE(p.end, ack) {
			continue
		}
		if p.end == ack && !p.wasRetried && !p.rttSampled {
			p.rttSampled = true
			sample = p
		}
		t.noteDeliveredLocked(p)
		delete(t.pending, seq)
		t.stats.Acked++
		if p.flags&faketcp.FlagFIN != 0 && !t.localFINAcked {
			t.localFINAcked = true
			t.stats.FINAcked++
		}
	}
	if advanced {
		t.lastAck = ack
	}
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

func (t *laneTransport) applySACKLocked(blocks []faketcp.SACKBlock, now time.Time) {
	for _, block := range blocks {
		if block.Start == block.End || seqLT(block.End, block.Start) ||
			seqLT(block.Start, t.lastAck) || seqLT(t.sendNext, block.End) {
			continue
		}
		for _, seq := range t.pendingOrder {
			p := t.pending[seq]
			if p == nil || p.flags&faketcp.FlagFIN != 0 ||
				seqLT(p.seq, block.Start) || seqLT(block.End, p.end) {
				continue
			}
			t.markSACKedLocked(p, now)
		}
	}
}

func (t *laneTransport) markSACKedLocked(p *pendingRecord, now time.Time) {
	if p == nil || p.sacked {
		return
	}
	p.sacked = true
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
	var candidate *pendingRecord
	candidateIndex := -1
	for i, seq := range t.pendingOrder {
		p := t.pending[seq]
		if p == nil || p.sacked || p.retired || p.flags&faketcp.FlagFIN != 0 {
			continue
		}
		if p.seq != t.lastAck {
			return nil
		}
		candidate = p
		candidateIndex = i
		break
	}
	if candidate == nil {
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

	sackedAbove := 0
	for i := candidateIndex + 1; i < len(t.pendingOrder); i++ {
		p := t.pending[t.pendingOrder[i]]
		if p == nil || !p.sacked {
			continue
		}
		sackedAbove++
		if sackedAbove >= 3 {
			return t.prepareFastRepairLocked(candidate, now)
		}
	}
	return nil
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

func (t *laneTransport) sackBlocksLocked() []faketcp.SACKBlock {
	if !t.cfg.SACKPermitted || len(t.received) == 0 {
		return nil
	}
	ranges := make([]faketcp.SACKBlock, 0, len(t.received))
	for start, span := range t.received {
		if span.fin || seqLT(start, t.recvNext) {
			continue
		}
		ranges = append(ranges, faketcp.SACKBlock{Start: start, End: span.end})
	}
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool {
		return uint32(ranges[i].Start-t.recvNext) < uint32(ranges[j].Start-t.recvNext)
	})
	merged := ranges[:0]
	for _, block := range ranges {
		n := len(merged)
		if n != 0 && merged[n-1].End == block.Start {
			merged[n-1].End = block.End
			continue
		}
		merged = append(merged, block)
	}

	out := make([]faketcp.SACKBlock, 0, faketcp.MaxSACKBlocks)
	primary := -1
	if t.lastOutOfOrderSet {
		for i, block := range merged {
			if !seqLT(t.lastOutOfOrder, block.Start) && seqLT(t.lastOutOfOrder, block.End) {
				primary = i
				out = append(out, block)
				break
			}
		}
	}
	for i := len(merged) - 1; i >= 0 && len(out) < faketcp.MaxSACKBlocks; i-- {
		if i == primary {
			continue
		}
		out = append(out, merged[i])
	}
	return out
}
