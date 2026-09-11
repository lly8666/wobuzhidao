package faketcp

import "time"

const payloadSlabSize = 2048

const (
	minRTO = time.Second
	maxRTO = 60 * time.Second

	// Shadow repairs are cosmetic/recovery traffic below the real first-arrival
	// data path. Bound their long-run byte cost so a lossy outer path cannot let
	// retransmissions crowd fresh inner traffic out of a fixed-rate bottleneck.
	shadowRepairBudgetDivisor = uint64(5) // 20% of admitted fresh bytes
	shadowRepairBurstBytes    = uint64(128 * 1024)
	shadowRepairDefer         = 100 * time.Millisecond
	shadowHighLossPercent     = uint64(5)
)

type RecoveryMode uint8

const (
	// RecoveryLegacy keeps only classic cumulative ACK / third-duplicate-ACK
	// fast retransmit plus RFC6298-style RTO. It exists as a performance oracle.
	RecoveryLegacy RecoveryMode = iota
	// RecoverySACKRACK adds the persistent SACK scoreboard and compact RACK-style
	// lost-retransmission inference. Neither mode owns inner delivery.
	RecoverySACKRACK
)

type Pending struct {
	Seq        uint32
	End        uint32
	Payload    []byte
	FirstSent  time.Time
	LastSent   time.Time
	Retries    uint32
	WasRetried bool
	SACKed     bool
	Bootstrap  bool

	// Retired means the peer has SACK-proven first arrival. Keep only the tiny
	// sequence tombstone until cumulative ACK catches up, so future merged SACK
	// ranges can still be walked without retaining payload or consuming the
	// steady-state repair/admission window.
	Retired bool

	// RepairNotBefore paces a retry whose global repair-byte budget was empty.
	// Deferral is local scheduling only; it never delays fresh admission.
	RepairNotBefore time.Time
	slot            int
}

type SenderStats struct {
	Enqueued              uint64
	EnqueuedBytes         uint64
	Acked                 uint64
	SACKed                uint64
	RetiredSACKed         uint64
	FastRetransmits       uint64
	RTOTransmits          uint64
	RetransmitBytes       uint64
	RepairDeferred        uint64
	RepairDeferredBytes   uint64
	LossMarked            uint64
	LossMarkedBytes       uint64
	PeakPending           int
}

type Sender struct {
	nextSeq      uint32
	pending      []*Pending
	bySeq        map[uint32]*Pending
	head         int
	// active is repair-bearing shadow state, not every historical sequence
	// tombstone. SACK-proven first arrivals leave this count immediately.
	active       int
	lastAck      uint32
	dupAcks      int
	fastRetxSeq  uint32
	fastRetxDone bool

	// baseRTO is the estimator-derived timeout. rto is the effective timeout for
	// the current loss episode and may be exponentially backed off after RTOs.
	rto      time.Duration
	baseRTO  time.Duration
	srtt     time.Duration
	rttvar   time.Duration
	recovery RecoveryMode

	timeoutEpisode    bool
	timeoutEpisodeEnd uint32

	steadyStateRTOSweep bool
	rtoSweepActive      bool
	rtoSweepStarted     time.Time
	rtoSweepRTO         time.Duration

	// Freshest transmission time for data proven delivered by ACK/SACK.
	rackLatestTx time.Time

	freeSlabs [][]byte
	stats     SenderStats
}

func NewSender(nextSeq uint32, initialRTO time.Duration) *Sender {
	return NewSenderWithRecovery(nextSeq, initialRTO, RecoverySACKRACK)
}

func NewSenderWithRecovery(nextSeq uint32, initialRTO time.Duration, recovery RecoveryMode) *Sender {
	if initialRTO <= 0 {
		initialRTO = minRTO
	}
	if recovery != RecoveryLegacy && recovery != RecoverySACKRACK {
		recovery = RecoverySACKRACK
	}
	initialRTO = clampRTO(initialRTO)
	return &Sender{
		nextSeq:  nextSeq,
		lastAck:  nextSeq,
		rto:      initialRTO,
		baseRTO:  initialRTO,
		bySeq:    make(map[uint32]*Pending),
		recovery: recovery,
	}
}

func (s *Sender) NextSeq() uint32                 { return s.nextSeq }
func (s *Sender) RTO() time.Duration              { return s.rto }
func (s *Sender) Pending() int                    { return s.active }
func (s *Sender) Stats() SenderStats              { return s.stats }
func (s *Sender) LastAck() uint32                 { return s.lastAck }
func (s *Sender) RecoveryMode() RecoveryMode      { return s.recovery }
func (s *Sender) Outstanding(seq uint32) *Pending { return s.bySeq[seq] }

func (s *Sender) Enqueue(payload []byte, now time.Time) *Pending {
	bootstrap := isBootstrapPayload(payload)
	buf := s.allocPayload(len(payload))
	copy(buf, payload)
	p := &Pending{
		Seq: s.nextSeq, End: s.nextSeq + uint32(len(payload)),
		Payload: buf, FirstSent: now, LastSent: now, Bootstrap: bootstrap, slot: len(s.pending),
	}
	s.nextSeq = p.End
	s.pending = append(s.pending, p)
	s.bySeq[p.Seq] = p
	s.active++
	s.stats.Enqueued++
	s.stats.EnqueuedBytes += uint64(len(payload))
	if s.active > s.stats.PeakPending {
		s.stats.PeakPending = s.active
	}
	return p
}

func (s *Sender) allocPayload(n int) []byte {
	if n <= payloadSlabSize {
		last := len(s.freeSlabs) - 1
		if last >= 0 {
			b := s.freeSlabs[last]
			s.freeSlabs = s.freeSlabs[:last]
			return b[:n]
		}
		return make([]byte, n, payloadSlabSize)
	}
	return make([]byte, n)
}

func (s *Sender) releasePayload(b []byte) {
	if cap(b) == payloadSlabSize && len(s.freeSlabs) < 32768 {
		s.freeSlabs = append(s.freeSlabs, b[:payloadSlabSize])
	}
}

func (s *Sender) Ack(ack uint32, now time.Time) *Pending {
	return s.AckSelective(ack, nil, now)
}

// AckSelective consumes cumulative ACK plus SACK information. SACK proves
// first-arrival and therefore releases payload/admission pressure immediately;
// cumulative ACK bookkeeping is retained as a lightweight sequence tombstone.
func (s *Sender) AckSelective(ack uint32, sacks []SACKBlock, now time.Time) *Pending {
	oldAck := s.lastAck
	advanced := seqLT(oldAck, ack)
	if advanced {
		s.lastAck = ack
		s.dupAcks = 0
		s.fastRetxDone = false
		s.ackCumulative(oldAck, ack, now)
	} else if ack == s.lastAck && s.active != 0 {
		s.dupAcks++
	}

	for _, b := range sacks {
		if b.Start == b.End || seqLT(b.End, b.Start) {
			continue
		}
		seq := b.Start
		for {
			p := s.bySeq[seq]
			if p == nil || seqLT(b.End, p.End) {
				break
			}
			next := p.End
			s.markSACK(p, now)
			if next == b.End {
				break
			}
			seq = next
		}
	}

	var repair *Pending
	if s.recovery == RecoverySACKRACK {
		candidate := s.rackLossCandidate(now)
		if candidate == nil {
			candidate = s.sackLossCandidate()
		}
		if candidate != nil && s.tryMarkRetry(candidate, now, true) {
			s.fastRetxSeq = candidate.Seq
			s.fastRetxDone = true
			s.dupAcks = 0
			repair = candidate
		}
	}

	if repair == nil && !advanced && ack == s.lastAck && s.dupAcks >= 3 {
		s.dupAcks = 0
		p := s.oldest()
		if p != nil && p.Seq == ack && !p.WasRetried && (!s.fastRetxDone || s.fastRetxSeq != p.Seq) {
			if s.tryMarkRetry(p, now, true) {
				s.fastRetxSeq = p.Seq
				s.fastRetxDone = true
				repair = p
			}
		}
	}

	// Do this only after the current ACK's loss inference: the just-observed SACK
	// evidence may be needed to select the hole repaired above.
	s.retireSACKedPayloads()
	return repair
}

func (s *Sender) sackLossCandidate() *Pending {
	candidate := s.oldest()
	if candidate == nil || candidate.Seq != s.lastAck || candidate.SACKed || candidate.Retired || candidate.WasRetried {
		return nil
	}
	if !candidate.RepairNotBefore.IsZero() && time.Now().Before(candidate.RepairNotBefore) {
		return nil
	}
	sackedAbove := 0
	for i := candidate.slot + 1; i < len(s.pending); i++ {
		p := s.pending[i]
		if p == nil || !p.SACKed {
			continue
		}
		sackedAbove++
		if sackedAbove >= 3 {
			return candidate
		}
	}
	return nil
}

// rackLossCandidate permits repeated fast repair only while that remains cheap.
// At measured high loss, one fast repair is enough TCP-like evidence; any later
// attempt falls back to paced RTO and the global repair-byte budget.
func (s *Sender) rackLossCandidate(now time.Time) *Pending {
	if s.rackLatestTx.IsZero() {
		return nil
	}
	p := s.oldest()
	if p == nil || p.Seq != s.lastAck || p.SACKed || p.Retired || p.LastSent.IsZero() || !p.WasRetried {
		return nil
	}
	if !p.RepairNotBefore.IsZero() && now.Before(p.RepairNotBefore) {
		return nil
	}
	if !s.fastRepairAllowed(p) {
		return nil
	}
	if !p.LastSent.Before(s.rackLatestTx) {
		return nil
	}
	if now.Sub(p.LastSent) < s.rackReorderingWindow() {
		return nil
	}
	return p
}

func (s *Sender) fastRepairAllowed(p *Pending) bool {
	if p == nil || p.Bootstrap {
		return true
	}
	maxFast := uint32(2)
	if s.highLoss() {
		maxFast = 1
	}
	return p.Retries < maxFast
}

func (s *Sender) highLoss() bool {
	// Avoid classifying from the first handful of packets.
	if s.stats.Enqueued < 128 {
		return false
	}
	return s.stats.LossMarked*100 > s.stats.Enqueued*shadowHighLossPercent
}

func (s *Sender) rackReorderingWindow() time.Duration {
	if s.srtt <= 0 {
		return 10 * time.Millisecond
	}
	v := s.srtt / 4
	if v < 10*time.Millisecond {
		v = 10 * time.Millisecond
	}
	return v
}

func (s *Sender) noteDelivered(p *Pending) {
	if p == nil || p.LastSent.IsZero() {
		return
	}
	if s.rackLatestTx.IsZero() || s.rackLatestTx.Before(p.LastSent) {
		s.rackLatestTx = p.LastSent
	}
}

func (s *Sender) markSACK(p *Pending, now time.Time) {
	if p == nil || s.bySeq[p.Seq] != p {
		return
	}
	if !p.SACKed {
		p.SACKed = true
		s.stats.SACKed++
		s.noteDelivered(p)
		if !p.WasRetried {
			s.observeRTT(now.Sub(p.FirstSent))
		}
	}
}

func (s *Sender) retireSACKedPayloads() {
	for i := s.head; i < len(s.pending); i++ {
		p := s.pending[i]
		if p == nil || !p.SACKed || p.Retired || p.Bootstrap {
			continue
		}
		s.releasePayload(p.Payload)
		p.Payload = nil
		p.Retired = true
		p.RepairNotBefore = time.Time{}
		if s.active > 0 {
			s.active--
		}
		s.stats.RetiredSACKed++
	}
}

func (s *Sender) ackCumulative(oldAck, ack uint32, now time.Time) {
	var sample *Pending
	if p := s.bySeq[oldAck]; p != nil && p.End == ack && !p.WasRetried {
		sample = p
	}
	for i := s.head; i < len(s.pending); i++ {
		p := s.pending[i]
		if p == nil {
			continue
		}
		if !seqLE(p.End, ack) {
			break
		}
		s.noteDelivered(p)
		s.ackOne(p)
	}
	if sample != nil {
		s.observeRTT(now.Sub(sample.FirstSent))
	}
	if s.timeoutEpisode && seqLE(s.timeoutEpisodeEnd, ack) {
		s.timeoutEpisode = false
		s.timeoutEpisodeEnd = 0
		s.rto = s.baseRTO
		s.clearRTOSweep()
	}
	s.advanceHead()
}

func (s *Sender) ackOne(p *Pending) {
	if p == nil || s.bySeq[p.Seq] != p {
		return
	}
	delete(s.bySeq, p.Seq)
	if !p.Retired {
		s.releasePayload(p.Payload)
		p.Payload = nil
		if s.active > 0 {
			s.active--
		}
	}
	s.stats.Acked++
	if p.slot >= 0 && p.slot < len(s.pending) && s.pending[p.slot] == p {
		s.pending[p.slot] = nil
	}
}

func (s *Sender) advanceHead() {
	for s.head < len(s.pending) && s.pending[s.head] == nil {
		s.head++
	}
	if s.head >= 4096 && s.head*2 >= len(s.pending) {
		oldHead := s.head
		copy(s.pending, s.pending[oldHead:])
		s.pending = s.pending[:len(s.pending)-oldHead]
		for i, p := range s.pending {
			if p != nil {
				p.slot = i
			}
		}
		s.head = 0
	}
}

func (s *Sender) RetransmitDue(now time.Time) *Pending {
	if s.recovery == RecoveryLegacy && s.steadyStateRTOSweep {
		batch := s.RetransmitDueBatch(now, 1)
		if len(batch) != 0 {
			return batch[0]
		}
		return nil
	}
	return s.retransmitDueSingle(now)
}

func (s *Sender) RetransmitDueBatch(now time.Time, max int) []*Pending {
	if max <= 0 {
		return nil
	}
	if s.recovery != RecoveryLegacy {
		if p := s.retransmitDueSingle(now); p != nil {
			return []*Pending{p}
		}
		return nil
	}
	if s.rtoSweepActive {
		return s.collectLegacyRTOSweep(now, max)
	}
	p := s.oldest()
	if p == nil {
		return nil
	}
	if p.Bootstrap {
		if p := s.retransmitDueSingle(now); p != nil {
			return []*Pending{p}
		}
		return nil
	}
	rto := s.effectiveRTO(p)
	if now.Sub(p.LastSent) < rto {
		return nil
	}

	s.rtoSweepActive = true
	s.rtoSweepStarted = now
	s.rtoSweepRTO = rto
	if !s.timeoutEpisode {
		s.timeoutEpisode = true
		s.timeoutEpisodeEnd = p.End
	}
	// Backoff is charged only after an actual retransmission leaves the repair
	// scheduler; an empty byte budget must not pretend a packet was sent.
	out := s.collectLegacyRTOSweep(now, max)
	if len(out) != 0 {
		s.rto = clampRTO(s.rto * 2)
	}
	return out
}

func (s *Sender) collectLegacyRTOSweep(now time.Time, max int) []*Pending {
	out := make([]*Pending, 0, max)
	for i := s.head; i < len(s.pending) && len(out) < max; i++ {
		p := s.pending[i]
		if !s.legacySweepEligible(p, now) {
			continue
		}
		if !s.tryMarkRetry(p, now, false) {
			continue
		}
		out = append(out, p)
	}
	if !s.hasLegacySweepEligible(now) {
		s.clearRTOSweep()
	}
	return out
}

func (s *Sender) legacySweepEligible(p *Pending, now time.Time) bool {
	if !s.rtoSweepActive || p == nil || p.Bootstrap || p.SACKed || p.Retired || p.LastSent.IsZero() {
		return false
	}
	if !p.RepairNotBefore.IsZero() && now.Before(p.RepairNotBefore) {
		return false
	}
	if !p.LastSent.Before(s.rtoSweepStarted) {
		return false
	}
	return s.rtoSweepStarted.Sub(p.LastSent) >= s.rtoSweepRTO
}

func (s *Sender) hasLegacySweepEligible(now time.Time) bool {
	for i := s.head; i < len(s.pending); i++ {
		if s.legacySweepEligible(s.pending[i], now) {
			return true
		}
	}
	return false
}

func (s *Sender) clearRTOSweep() {
	s.rtoSweepActive = false
	s.rtoSweepStarted = time.Time{}
	s.rtoSweepRTO = 0
}

func (s *Sender) retransmitDueSingle(now time.Time) *Pending {
	p := s.oldest()
	if p == nil || p.Retired || p.SACKed || len(p.Payload) == 0 {
		return nil
	}
	if !p.RepairNotBefore.IsZero() && now.Before(p.RepairNotBefore) {
		return nil
	}
	rto := s.effectiveRTO(p)
	if now.Sub(p.LastSent) < rto {
		return nil
	}
	if !s.tryMarkRetry(p, now, false) {
		return nil
	}
	if !p.Bootstrap {
		if !s.timeoutEpisode {
			s.timeoutEpisode = true
			s.timeoutEpisodeEnd = p.End
		}
		s.rto = clampRTO(s.rto * 2)
	}
	return p
}

func (s *Sender) effectiveRTO(p *Pending) time.Duration {
	rto := s.rto
	if s.recovery == RecoveryLegacy && p != nil && !p.Bootstrap && !s.rackLatestTx.IsZero() &&
		p.LastSent.Before(s.rackLatestTx) && s.baseRTO < rto {
		rto = s.baseRTO
	}
	if p != nil && p.Bootstrap && rto > bootstrapRetransmitCeiling {
		rto = bootstrapRetransmitCeiling
	}
	return rto
}

func (s *Sender) oldest() *Pending {
	s.advanceHead()
	if s.head >= len(s.pending) {
		return nil
	}
	return s.pending[s.head]
}

func (s *Sender) repairBudgetAllows(p *Pending) bool {
	if p == nil || p.Bootstrap {
		return true
	}
	cost := uint64(len(p.Payload))
	if cost == 0 {
		return false
	}
	budget := s.stats.EnqueuedBytes/shadowRepairBudgetDivisor + shadowRepairBurstBytes
	return s.stats.RetransmitBytes+cost <= budget
}

func (s *Sender) tryMarkRetry(p *Pending, now time.Time, fast bool) bool {
	if p == nil || p.SACKed || p.Retired || len(p.Payload) == 0 {
		return false
	}
	if !p.Bootstrap && !s.repairBudgetAllows(p) {
		p.RepairNotBefore = now.Add(shadowRepairDefer)
		s.stats.RepairDeferred++
		s.stats.RepairDeferredBytes += uint64(len(p.Payload))
		return false
	}
	p.RepairNotBefore = time.Time{}
	s.markRetry(p, now, fast)
	return true
}

func (s *Sender) markRetry(p *Pending, now time.Time, fast bool) {
	firstLossMark := !p.WasRetried
	p.LastSent = now
	p.Retries++
	p.WasRetried = true
	if firstLossMark {
		s.stats.LossMarked++
		s.stats.LossMarkedBytes += uint64(len(p.Payload))
	}
	if fast {
		s.stats.FastRetransmits++
	} else {
		s.stats.RTOTransmits++
	}
	s.stats.RetransmitBytes += uint64(len(p.Payload))
}

func (s *Sender) observeRTT(sample time.Duration) {
	if sample <= 0 {
		return
	}
	if s.srtt == 0 {
		s.srtt = sample
		s.rttvar = sample / 2
	} else {
		d := s.srtt - sample
		if d < 0 {
			d = -d
		}
		s.rttvar = (3*s.rttvar + d) / 4
		s.srtt = (7*s.srtt + sample) / 8
	}
	s.baseRTO = clampRTO(s.srtt + 4*s.rttvar)
	if !s.timeoutEpisode {
		s.rto = s.baseRTO
	}
}

func clampRTO(v time.Duration) time.Duration {
	if v < minRTO {
		return minRTO
	}
	if v > maxRTO {
		return maxRTO
	}
	return v
}

type ReceiverStats struct {
	Delivered  uint64
	Duplicates uint64
	OutOfOrder uint64
}

type Receiver struct {
	next           uint32
	outOfOrder     map[uint32]uint32
	sacksByStart   map[uint32]uint32
	sackStartByEnd map[uint32]uint32
	recentSACK     [4]uint32
	recentSACKN    int
	stats          ReceiverStats
}

func NewReceiver(nextSeq uint32) *Receiver {
	return &Receiver{
		next:           nextSeq,
		outOfOrder:     make(map[uint32]uint32),
		sacksByStart:   make(map[uint32]uint32),
		sackStartByEnd: make(map[uint32]uint32),
	}
}
func (r *Receiver) Next() uint32          { return r.next }
func (r *Receiver) Stats() ReceiverStats { return r.stats }

func (r *Receiver) Accept(seq uint32, payloadLen int) (deliver, sackNeeded bool) {
	if payloadLen <= 0 {
		return false, false
	}
	end := seq + uint32(payloadLen)
	if seqLT(seq, r.next) {
		r.stats.Duplicates++
		return false, len(r.sacksByStart) != 0
	}
	if _, exists := r.outOfOrder[seq]; exists {
		r.stats.Duplicates++
		return false, len(r.sacksByStart) != 0
	}
	r.stats.Delivered++
	if seq != r.next {
		r.stats.OutOfOrder++
		r.outOfOrder[seq] = end
		r.insertSACKRange(seq, end)
		return true, true
	}

	r.next = end
	r.consumeContiguousSACKs()
	return true, len(r.sacksByStart) != 0
}

func (r *Receiver) SACKBlocks(dst *[4]SACKBlock) int {
	if dst == nil {
		return 0
	}
	n := 0
	for i := 0; i < r.recentSACKN && n < len(dst); i++ {
		start := r.recentSACK[i]
		end, ok := r.sacksByStart[start]
		if !ok || seqLT(start, r.next) {
			continue
		}
		dst[n] = SACKBlock{Start: start, End: end}
		n++
	}
	return n
}

func (r *Receiver) insertSACKRange(seq, end uint32) {
	start := seq
	finish := end
	if leftStart, ok := r.sackStartByEnd[seq]; ok {
		start = leftStart
		delete(r.sacksByStart, leftStart)
		delete(r.sackStartByEnd, seq)
		r.removeRecentSACK(leftStart)
	}
	if rightEnd, ok := r.sacksByStart[end]; ok {
		finish = rightEnd
		delete(r.sacksByStart, end)
		delete(r.sackStartByEnd, rightEnd)
		r.removeRecentSACK(end)
	}
	r.sacksByStart[start] = finish
	r.sackStartByEnd[finish] = start
	r.touchRecentSACK(start)
}

func (r *Receiver) consumeContiguousSACKs() {
	for {
		start := r.next
		end, ok := r.sacksByStart[start]
		if !ok {
			return
		}
		delete(r.sacksByStart, start)
		delete(r.sackStartByEnd, end)
		r.removeRecentSACK(start)
		cur := start
		for cur != end {
			nextEnd, exists := r.outOfOrder[cur]
			if !exists {
				break
			}
			delete(r.outOfOrder, cur)
			cur = nextEnd
		}
		r.next = end
	}
}

func (r *Receiver) touchRecentSACK(start uint32) {
	r.removeRecentSACK(start)
	limit := r.recentSACKN
	if limit > 3 {
		limit = 3
	}
	for i := limit; i > 0; i-- {
		r.recentSACK[i] = r.recentSACK[i-1]
	}
	r.recentSACK[0] = start
	if r.recentSACKN < len(r.recentSACK) {
		r.recentSACKN++
	}
}

func (r *Receiver) removeRecentSACK(start uint32) {
	for i := 0; i < r.recentSACKN; i++ {
		if r.recentSACK[i] != start {
			continue
		}
		copy(r.recentSACK[i:], r.recentSACK[i+1:r.recentSACKN])
		r.recentSACKN--
		r.recentSACK[r.recentSACKN] = 0
		return
	}
}

func seqLT(a, b uint32) bool { return int32(a-b) < 0 }
func seqLE(a, b uint32) bool { return a == b || seqLT(a, b) }
