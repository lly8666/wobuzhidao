package runtimeowner

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/faketcp"
)

const (
	steadyIndexCompactThreshold = 1024
	steadyRepairScanBudget       = 256
	steadyRepairExpiryBudget     = 64
	steadyGapForgiveBudget       = 64
	steadySenderSACKHistory      = 8
)

func (t *laneTransport) linkRepairLocked(p *pendingRecord) {
	if p == nil || p.repairLinked {
		return
	}
	p.repairLinked = true
	p.repairPrev = t.repairTail
	p.repairNext = nil
	if t.repairTail != nil {
		t.repairTail.repairNext = p
	} else {
		t.repairHead = p
	}
	t.repairTail = p
	t.repairCount++
	if t.repairScan == nil {
		t.repairScan = p
	}
}

func (t *laneTransport) unlinkRepairLocked(p *pendingRecord) {
	if p == nil || !p.repairLinked {
		return
	}
	prev := p.repairPrev
	next := p.repairNext
	if prev != nil {
		prev.repairNext = next
	} else {
		t.repairHead = next
	}
	if next != nil {
		next.repairPrev = prev
	} else {
		t.repairTail = prev
	}
	if t.repairScan == p {
		if next != nil {
			t.repairScan = next
		} else {
			t.repairScan = t.repairHead
		}
	}
	p.repairPrev = nil
	p.repairNext = nil
	p.repairLinked = false
	if t.repairCount > 0 {
		t.repairCount--
	}
	if t.repairCount == 0 {
		t.repairHead = nil
		t.repairTail = nil
		t.repairScan = nil
	}
}

func (t *laneTransport) linkEvictLocked(p *pendingRecord) {
	if p == nil || p.evictLinked || p.control || p.flags&faketcp.FlagFIN != 0 ||
		p.sacked || p.retired || p.repairInFlight || p.fastRepairArmed ||
		p.seq == t.lastAck {
		return
	}
	p.evictLinked = true
	p.evictPrev = t.evictTail
	if t.evictTail != nil {
		t.evictTail.evictNext = p
	} else {
		t.evictHead = p
	}
	t.evictTail = p
}

func (t *laneTransport) linkEvictFrontLocked(p *pendingRecord) {
	if p == nil || p.evictLinked || p.control || p.flags&faketcp.FlagFIN != 0 ||
		p.sacked || p.retired || p.repairInFlight || p.fastRepairArmed ||
		p.seq == t.lastAck {
		return
	}
	p.evictLinked = true
	p.evictNext = t.evictHead
	if t.evictHead != nil {
		t.evictHead.evictPrev = p
	} else {
		t.evictTail = p
	}
	t.evictHead = p
}

func (t *laneTransport) unlinkEvictLocked(p *pendingRecord) {
	if p == nil || !p.evictLinked {
		return
	}
	if p.evictPrev != nil {
		p.evictPrev.evictNext = p.evictNext
	} else {
		t.evictHead = p.evictNext
	}
	if p.evictNext != nil {
		p.evictNext.evictPrev = p.evictPrev
	} else {
		t.evictTail = p.evictPrev
	}
	p.evictPrev, p.evictNext, p.evictLinked = nil, nil, false
}

func (t *laneTransport) linkRetiredLocked(p *pendingRecord) {
	if p == nil || p.retiredLinked || p.repairInFlight || !(p.sacked || p.retired) ||
		p.flags&faketcp.FlagFIN != 0 {
		return
	}
	p.retiredLinked = true
	p.retiredPrev = t.retiredTail
	if t.retiredTail != nil {
		t.retiredTail.retiredNext = p
	} else {
		t.retiredHead = p
	}
	t.retiredTail = p
}

func (t *laneTransport) unlinkRetiredLocked(p *pendingRecord) {
	if p == nil || !p.retiredLinked {
		return
	}
	if p.retiredPrev != nil {
		p.retiredPrev.retiredNext = p.retiredNext
	} else {
		t.retiredHead = p.retiredNext
	}
	if p.retiredNext != nil {
		p.retiredNext.retiredPrev = p.retiredPrev
	} else {
		t.retiredTail = p.retiredPrev
	}
	p.retiredPrev, p.retiredNext, p.retiredLinked = nil, nil, false
}

func (t *laneTransport) restorePendingEvictionLocked(p *pendingRecord) {
	if p == nil || t.pending[p.seq] != p || p.repairInFlight || p.fastRepairArmed {
		return
	}
	if p.sacked || p.retired {
		t.linkRetiredLocked(p)
		return
	}
	t.linkEvictFrontLocked(p)
}

func (t *laneTransport) protectCurrentHeadRepairLocked() {
	p := t.pendingAtHeadLocked()
	if p == nil || p.seq != t.lastAck {
		return
	}
	// Keep exactly the current cumulative head out of the optional fresh-eviction
	// chain before SACK feedback arrives. All later business shadows remain
	// evictable, so this consumes one bounded slot without turning 4096 into a
	// fresh admission window.
	t.unlinkEvictLocked(p)
}

func (t *laneTransport) linkExpiryLocked(p *pendingRecord) {
	if p == nil || p.expiryLinked || p.control || p.flags&faketcp.FlagFIN != 0 {
		return
	}
	p.expiryLinked = true
	p.expiryPrev = t.expiryTail
	if t.expiryTail != nil {
		t.expiryTail.expiryNext = p
	} else {
		t.expiryHead = p
	}
	t.expiryTail = p
}

func (t *laneTransport) unlinkExpiryLocked(p *pendingRecord) {
	if p == nil || !p.expiryLinked {
		return
	}
	if p.expiryPrev != nil {
		p.expiryPrev.expiryNext = p.expiryNext
	} else {
		t.expiryHead = p.expiryNext
	}
	if p.expiryNext != nil {
		p.expiryNext.expiryPrev = p.expiryPrev
	} else {
		t.expiryTail = p.expiryPrev
	}
	p.expiryPrev, p.expiryNext, p.expiryLinked = nil, nil, false
}

func (t *laneTransport) removePendingLocked(p *pendingRecord) bool {
	if p == nil || t.pending[p.seq] != p {
		return false
	}
	if p.fastRepairArmed {
		p.fastRepairArmed = false
		p.fastRepairReadyTick = 0
		t.stats.FastRepairArmCanceled++
	}
	delete(t.pending, p.seq)
	t.unlinkRepairLocked(p)
	t.unlinkEvictLocked(p)
	t.unlinkRetiredLocked(p)
	t.unlinkExpiryLocked(p)
	if p.sacked && t.sackedOutstanding > 0 {
		t.sackedOutstanding--
	}
	return true
}

func (t *laneTransport) advancePendingHeadLocked() {
	for t.pendingHead < len(t.pendingOrder) {
		if t.pending[t.pendingOrder[t.pendingHead]] != nil {
			return
		}
		t.pendingHead++
	}
}

func (t *laneTransport) pendingAtHeadLocked() *pendingRecord {
	t.advancePendingHeadLocked()
	if t.pendingHead >= len(t.pendingOrder) {
		return nil
	}
	return t.pending[t.pendingOrder[t.pendingHead]]
}

func (t *laneTransport) compactPendingOrderLocked() {
	t.advancePendingHeadLocked()
	if len(t.pending) == 0 || t.pendingHead >= len(t.pendingOrder) {
		t.pendingOrder = nil
		t.pendingHead = 0
		return
	}
	tombstones := len(t.pendingOrder) - len(t.pending)
	if t.pendingHead < steadyIndexCompactThreshold && tombstones < steadyIndexCompactThreshold {
		return
	}
	out := t.pendingOrder[:0]
	for i := t.pendingHead; i < len(t.pendingOrder); i++ {
		seq := t.pendingOrder[i]
		if t.pending[seq] != nil {
			out = append(out, seq)
		}
	}
	t.pendingOrder = out
	t.pendingHead = 0
}

func (t *laneTransport) expirePendingHeadLocked(now time.Time) {
	for i := 0; i < steadyRepairExpiryBudget; i++ {
		p := t.expiryHead
		if p == nil {
			break
		}
		if p.repairInFlight {
			// Oldest business metadata is temporarily borrowed by Emit. Do
			// not free it or scan around it; the next tick resumes in order.
			break
		}
		if now.Before(p.firstSent) || now.Sub(p.firstSent) < t.cfg.RepairHorizon {
			break
		}
		retired := p.retired || p.sacked
		t.removePendingLocked(p)
		if retired {
			t.stats.RepairMetadataEvicted++
		} else {
			t.stats.Abandoned++
			t.stats.RepairEvicted++
		}
	}
	t.compactPendingOrderLocked()
}

func (t *laneTransport) pendingLowerBoundLocked(seq uint32) int {
	t.advancePendingHeadLocked()
	if t.pendingHead >= len(t.pendingOrder) {
		return len(t.pendingOrder)
	}
	base := t.pendingOrder[t.pendingHead]
	target := uint32(seq - base)
	lo, hi := t.pendingHead, len(t.pendingOrder)
	for lo < hi {
		mid := lo + (hi-lo)/2
		if uint32(t.pendingOrder[mid]-base) < target {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo
}

func (t *laneTransport) clearSteadyIndexesLocked() {
	t.pendingHead = 0
	t.repairHead = nil
	t.repairTail = nil
	t.repairScan = nil
	t.repairCount = 0
	t.evictHead = nil
	t.evictTail = nil
	t.retiredHead = nil
	t.retiredTail = nil
	t.expiryHead = nil
	t.expiryTail = nil
	t.recvHeap = nil
	t.sackedOutstanding = 0
	t.sackSeenN = 0
	t.firstRepairEvidenceAck = 0
	t.firstRepairEvidence = 0
	t.recvSACKN = 0
}

func (t *laneTransport) noteRecvSACKLocked(start, end uint32) {
	if start == end || seqLT(end, start) || !seqLT(t.recvNext, start) {
		return
	}
	merged := faketcp.SACKBlock{Start: start, End: end}
	var keep [faketcp.MaxSACKBlocks]faketcp.SACKBlock
	keepN := 0
	for i := 0; i < t.recvSACKN; i++ {
		block := t.recvSACK[i]
		if sackRangesTouch(t.recvNext, merged, block) {
			merged = mergeSACKRanges(t.recvNext, merged, block)
			continue
		}
		if keepN < faketcp.MaxSACKBlocks-1 {
			keep[keepN] = block
			keepN++
		}
	}
	t.recvSACK[0] = merged
	for i := 0; i < keepN; i++ {
		t.recvSACK[i+1] = keep[i]
	}
	t.recvSACKN = 1 + keepN
	for i := t.recvSACKN; i < faketcp.MaxSACKBlocks; i++ {
		t.recvSACK[i] = faketcp.SACKBlock{}
	}
}

func (t *laneTransport) pruneRecvSACKLocked() {
	n := 0
	for i := 0; i < t.recvSACKN; i++ {
		block := t.recvSACK[i]
		if !seqLT(t.recvNext, block.End) {
			continue
		}
		if seqLT(block.Start, t.recvNext) {
			block.Start = t.recvNext
		}
		if block.Start == block.End {
			continue
		}
		t.recvSACK[n] = block
		n++
	}
	for i := n; i < faketcp.MaxSACKBlocks; i++ {
		t.recvSACK[i] = faketcp.SACKBlock{}
	}
	t.recvSACKN = n
}

func (t *laneTransport) sackBlocksLocked() ([faketcp.MaxSACKBlocks]faketcp.SACKBlock, int) {
	t.pruneRecvSACKLocked()
	return t.recvSACK, t.recvSACKN
}

func sackRangesTouch(base uint32, a, b faketcp.SACKBlock) bool {
	as, ae := uint32(a.Start-base), uint32(a.End-base)
	bs, be := uint32(b.Start-base), uint32(b.End-base)
	return as <= be && bs <= ae
}

func mergeSACKRanges(base uint32, a, b faketcp.SACKBlock) faketcp.SACKBlock {
	as, ae := uint32(a.Start-base), uint32(a.End-base)
	bs, be := uint32(b.Start-base), uint32(b.End-base)
	if bs < as {
		as = bs
	}
	if be > ae {
		ae = be
	}
	return faketcp.SACKBlock{Start: base + as, End: base + ae}
}

func (t *laneTransport) applySACKLocked(blocks []faketcp.SACKBlock, now time.Time) {
	t.pruneSenderSACKLocked()
	progressed := false
	for _, block := range blocks {
		if block.Start == block.End || seqLT(block.End, block.Start) ||
			seqLT(block.Start, t.lastAck) || seqLT(t.sendNext, block.End) {
			continue
		}
		novel, n := t.unseenSenderSACKLocked(block)
		if n != 0 {
			progressed = true
		}
		for i := 0; i < n; i++ {
			t.markSACKRangeLocked(novel[i], now)
		}
		t.rememberSenderSACKLocked(block)
	}
	if progressed {
		t.noteFirstRepairSACKEvidenceLocked()
	}
}

func (t *laneTransport) noteFirstRepairSACKEvidenceLocked() {
	p := t.pendingAtHeadLocked()
	if p == nil || p.seq != t.lastAck || p.wasRetried || p.sacked || p.retired ||
		p.control || p.flags&faketcp.FlagFIN != 0 || p.repairInFlight {
		return
	}
	if t.firstRepairEvidenceAck != t.lastAck {
		t.firstRepairEvidenceAck = t.lastAck
		t.firstRepairEvidence = 0
	}
	if t.firstRepairEvidence < 3 {
		t.firstRepairEvidence++
		t.stats.FastRepairEvidence++
	}
}

func (t *laneTransport) markSACKRangeLocked(block faketcp.SACKBlock, now time.Time) {
	for i := t.pendingLowerBoundLocked(block.Start); i < len(t.pendingOrder); i++ {
		seq := t.pendingOrder[i]
		if !seqLT(seq, block.End) {
			break
		}
		p := t.pending[seq]
		if p == nil || p.flags&faketcp.FlagFIN != 0 ||
			seqLT(p.seq, block.Start) || !seqLE(p.end, block.End) {
			continue
		}
		t.markSACKedLocked(p, now)
	}
}

func (t *laneTransport) unseenSenderSACKLocked(block faketcp.SACKBlock) ([steadySenderSACKHistory + 1]faketcp.SACKBlock, int) {
	var current [steadySenderSACKHistory + 1]faketcp.SACKBlock
	current[0] = block
	n := 1
	for i := 0; i < t.sackSeenN && n != 0; i++ {
		var next [steadySenderSACKHistory + 1]faketcp.SACKBlock
		nextN := 0
		for j := 0; j < n; j++ {
			pieces, pieceN := subtractSACKRange(t.lastAck, current[j], t.sackSeen[i])
			for k := 0; k < pieceN && nextN < len(next); k++ {
				next[nextN] = pieces[k]
				nextN++
			}
		}
		current = next
		n = nextN
	}
	return current, n
}

func subtractSACKRange(base uint32, whole, seen faketcp.SACKBlock) ([2]faketcp.SACKBlock, int) {
	var out [2]faketcp.SACKBlock
	ws, we := uint32(whole.Start-base), uint32(whole.End-base)
	ss, se := uint32(seen.Start-base), uint32(seen.End-base)
	if se <= ws || we <= ss {
		out[0] = whole
		return out, 1
	}
	if ss <= ws && we <= se {
		return out, 0
	}
	if ss <= ws {
		out[0] = faketcp.SACKBlock{Start: base + se, End: whole.End}
		return out, 1
	}
	if we <= se {
		out[0] = faketcp.SACKBlock{Start: whole.Start, End: base + ss}
		return out, 1
	}
	out[0] = faketcp.SACKBlock{Start: whole.Start, End: base + ss}
	out[1] = faketcp.SACKBlock{Start: base + se, End: whole.End}
	return out, 2
}

func (t *laneTransport) rememberSenderSACKLocked(block faketcp.SACKBlock) {
	var ranges [steadySenderSACKHistory + 1]faketcp.SACKBlock
	n := 0
	for i := 0; i < t.sackSeenN; i++ {
		current, ok := t.normalizeSenderSACKLocked(t.sackSeen[i])
		if !ok {
			continue
		}
		ranges[n] = current
		n++
	}
	if current, ok := t.normalizeSenderSACKLocked(block); ok {
		ranges[n] = current
		n++
	}
	for i := 1; i < n; i++ {
		v := ranges[i]
		j := i - 1
		for ; j >= 0 && uint32(ranges[j].Start-t.lastAck) > uint32(v.Start-t.lastAck); j-- {
			ranges[j+1] = ranges[j]
		}
		ranges[j+1] = v
	}
	var merged [steadySenderSACKHistory + 1]faketcp.SACKBlock
	m := 0
	for i := 0; i < n; i++ {
		if m == 0 {
			merged[0] = ranges[i]
			m = 1
			continue
		}
		last := &merged[m-1]
		lastEnd := uint32(last.End - t.lastAck)
		start := uint32(ranges[i].Start - t.lastAck)
		end := uint32(ranges[i].End - t.lastAck)
		if start <= lastEnd {
			if end > lastEnd {
				last.End = t.lastAck + end
			}
			continue
		}
		merged[m] = ranges[i]
		m++
	}
	if m > steadySenderSACKHistory {
		m = steadySenderSACKHistory
	}
	copy(t.sackSeen[:], merged[:m])
	for i := m; i < steadySenderSACKHistory; i++ {
		t.sackSeen[i] = faketcp.SACKBlock{}
	}
	t.sackSeenN = m
}

func (t *laneTransport) normalizeSenderSACKLocked(block faketcp.SACKBlock) (faketcp.SACKBlock, bool) {
	if block.Start == block.End || !seqLT(t.lastAck, block.End) {
		return faketcp.SACKBlock{}, false
	}
	if seqLT(block.Start, t.lastAck) {
		block.Start = t.lastAck
	}
	if block.Start == block.End {
		return faketcp.SACKBlock{}, false
	}
	return block, true
}

func (t *laneTransport) pruneSenderSACKLocked() {
	var kept [steadySenderSACKHistory]faketcp.SACKBlock
	n := 0
	for i := 0; i < t.sackSeenN; i++ {
		block, ok := t.normalizeSenderSACKLocked(t.sackSeen[i])
		if !ok {
			continue
		}
		kept[n] = block
		n++
	}
	t.sackSeen = kept
	t.sackSeenN = n
}
