package faketcp

import "time"

const payloadSlabSize = 2048

const (
	minRTO = time.Second
	maxRTO = 60 * time.Second

	// Shadow repairs are recovery/persona traffic below the real first-arrival
	// path. A finite 128 KiB bucket earns one credit byte per five non-bootstrap
	// fresh bytes. Repeated repairs cost progressively more virtual credit, so an
	// isolated low-loss retry stays TCP-like while a lossy path cannot turn into
	// a retransmission bandwidth storm.
	shadowRepairBudgetDivisor = uint64(5)
	shadowRepairBurstBytes    = uint64(128 * 1024)
	shadowRepairDefer         = 100 * time.Millisecond

	// Compatibility name for the cold-start/emergency bound. The normal soft
	// limit is measured per receiver by repairPressure.softLimit().
	PartialReliabilityReorderSoftLimit = PartialReliabilityEmergencyLimit

	// Keep just enough exact delivery history to suppress normal late repairs.
	// Once an entry ages out, steady-state carrier/DTLS replay protection remains
	// authoritative, so FakeTCP prefers a possible duplicate over a false drop.
	recentDeliveryHistoryLimit = MaxSteadyStateOutstandingDatagrams
)

type RecoveryMode uint8

const (
	RecoveryLegacy RecoveryMode = iota
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

	// Retired means SACK has proved first arrival. The payload and repair-window
	// ownership are released immediately, but the tiny seq/end tombstone remains
	// until cumulative ACK catches up so merged SACK ranges stay walkable.
	Retired bool

	// Local pacing fence used only when repair credit is empty. Fresh data never
	// waits on this timestamp.
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
	RepairBudgetSpent     uint64
	RepairDeferred        uint64
	RepairDeferredBytes   uint64
	LossMarked            uint64
	LossMarkedBytes       uint64
	PeakPending           int
	FreshAdmitted         uint64
	FreshAdmittedBytes    uint64
	FreshBlockedByRepair  uint64
	RepairEvicted         uint64
	RepairEvictedBytes    uint64
	RepairMetadataEvicted uint64
	RepairCreditBytes     uint64
	ShadowRetransmitBytes uint64
}

type Sender struct {
	nextSeq uint32
	pending []*Pending
	bySeq   map[uint32]*Pending
	head    int
	// active counts only records that still own repair payload/state. SACK-proven
	// first arrivals leave it immediately even while their seq tombstones remain.
	active       int
	lastAck      uint32
	dupAcks      int
	fastRetxSeq  uint32
	fastRetxDone bool

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

	rackLatestTx time.Time

	repairBudgetSpent uint64
	repairCredit      uint64
	repairRemainder   uint64
	freeSlabs         [][]byte
	stats             SenderStats
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
		nextSeq:      nextSeq,
		lastAck:      nextSeq,
		rto:          initialRTO,
		baseRTO:      initialRTO,
		bySeq:        make(map[uint32]*Pending),
		recovery:     recovery,
		repairCredit: shadowRepairBurstBytes,
	}
}

func (s *Sender) NextSeq() uint32    { return s.nextSeq }
func (s *Sender) RTO() time.Duration { return s.rto }
func (s *Sender) Pending() int       { return s.active }
func (s *Sender) Stats() SenderStats {
	stats := s.stats
	stats.RepairCreditBytes = s.repairCredit
	return stats
}
func (s *Sender) LastAck() uint32                 { return s.lastAck }
func (s *Sender) RecoveryMode() RecoveryMode      { return s.recovery }
func (s *Sender) SRTT() time.Duration             { return s.srtt }
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
	if !bootstrap {
		s.refillShadowRepair(uint64(len(payload)))
	}
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
			candidate = s.sackLossCandidate(now)
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

	// The current ACK's SACK evidence must remain visible until candidate
	// selection completes. After that, delivered payload has no repair value.
	s.retireSACKedPayloads()
	return repair
}

func (s *Sender) sackLossCandidate(now time.Time) *Pending {
	candidate := s.oldest()
	if candidate == nil || candidate.Seq != s.lastAck || candidate.SACKed || candidate.Retired || candidate.WasRetried {
		return nil
	}
	if !candidate.RepairNotBefore.IsZero() && now.Before(candidate.RepairNotBefore) {
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

// Every repeated fast repair still requires genuinely newer delivery evidence.
// Bandwidth control is deliberately orthogonal: progressive virtual repair cost
// throttles pathological repetition without adding an arbitrary retry-count cliff.
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
	if !p.LastSent.Before(s.rackLatestTx) {
		return nil
	}
	if now.Sub(p.LastSent) < s.rackReorderingWindow() {
		return nil
	}
	return p
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
	// A protected bootstrap entry can pin the head while optional entries after
	// it are evicted. Bound the sparse index too, not just live map entries.
	if len(s.pending) >= 2*MaxSteadyStateTrackedRecords && len(s.bySeq)*2 <= len(s.pending) {
		n := 0
		for _, p := range s.pending {
			if p != nil {
				s.pending[n] = p
				p.slot = n
				n++
			}
		}
		clear(s.pending[n:])
		s.pending = s.pending[:n]
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

func (s *Sender) shadowRepairCost(p *Pending) uint64 {
	if p == nil {
		return 0
	}
	cost := uint64(len(p.Payload))
	if cost == 0 || p.Bootstrap {
		return cost
	}
	// First repair costs 1x credit, then 2x, 4x, 8x. Cap the multiplier so a
	// single pathological hole cannot overflow arithmetic yet remains expensive.
	shift := p.Retries
	if shift > 3 {
		shift = 3
	}
	return cost << shift
}

func (s *Sender) repairBudgetAllows(p *Pending) bool {
	if p == nil || p.Bootstrap {
		return true
	}
	cost := s.shadowRepairCost(p)
	if cost == 0 {
		return false
	}
	return cost <= s.repairCredit
}

// Only fresh bytes earn credit. Unused credit is capped; idle time and
// bootstrap bytes cannot build a bank for a later repair storm. Preserve
// fractional fifths so small datagrams are not systematically under-credited.
func (s *Sender) refillShadowRepair(fresh uint64) {
	credit := fresh / shadowRepairBudgetDivisor
	s.repairRemainder += fresh % shadowRepairBudgetDivisor
	credit += s.repairRemainder / shadowRepairBudgetDivisor
	s.repairRemainder %= shadowRepairBudgetDivisor
	if credit >= shadowRepairBurstBytes-s.repairCredit {
		s.repairCredit = shadowRepairBurstBytes
		s.repairRemainder = 0
	} else {
		s.repairCredit += credit
	}
}

func (s *Sender) tryMarkRetry(p *Pending, now time.Time, fast bool) bool {
	if p == nil || p.SACKed || p.Retired || len(p.Payload) == 0 {
		return false
	}
	cost := s.shadowRepairCost(p)
	if !p.Bootstrap && !s.repairBudgetAllows(p) {
		p.RepairNotBefore = now.Add(shadowRepairDefer)
		s.stats.RepairDeferred++
		s.stats.RepairDeferredBytes += uint64(len(p.Payload))
		return false
	}
	p.RepairNotBefore = time.Time{}
	if !p.Bootstrap {
		s.repairCredit -= cost
		s.repairBudgetSpent += cost
		s.stats.RepairBudgetSpent = s.repairBudgetSpent
		s.stats.ShadowRetransmitBytes += uint64(len(p.Payload))
	}
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
	Delivered             uint64
	Duplicates            uint64
	OutOfOrder            uint64
	LateBelowACK          uint64
	BelowNextDrops        uint64
	BufferedDuplicates    uint64
	ForgivenGaps          uint64
	ForgivenBytes         uint64
	PeakBufferedOO        int
	PressureSoftLimit     int
	PressureRate          float64
	PressureSRTTMillis    int64
	SoftForgivenGaps      uint64
	EmergencyForgivenGaps uint64
}

type Receiver struct {
	next           uint32
	outOfOrder     map[uint32]uint32
	sacksByStart   map[uint32]uint32
	sackStartByEnd map[uint32]uint32
	recentSACK     [4]uint32
	recentSACKN    int
	coverageQueue  []uint32
	coverageAt     int

	// Bootstrap is strict TCP. Steady-state carrier traffic can instead deliver
	// a packet below a forgiven cumulative ACK when FakeTCP has no evidence that
	// exact record was already delivered. A bounded recent-delivery set suppresses
	// ordinary late repairs; older ambiguity is left to carrier/DTLS replay logic.
	steadyStateDelivery bool
	steadyFloor         uint32
	steadyFloorActive   bool
	steadyDelivered     uint64
	deliveredBySeq      map[uint32]uint32
	deliveryOrder       []uint32
	deliveryHead        int

	stats    ReceiverStats
	pressure repairPressure
}

func NewReceiver(nextSeq uint32) *Receiver {
	return &Receiver{
		next:           nextSeq,
		outOfOrder:     make(map[uint32]uint32),
		sacksByStart:   make(map[uint32]uint32),
		sackStartByEnd: make(map[uint32]uint32),
	}
}
func (r *Receiver) Next() uint32 { return r.next }
func (r *Receiver) Stats() ReceiverStats {
	stats := r.stats
	stats.PressureSoftLimit = r.pressure.softLimit()
	stats.PressureRate = r.pressure.rate
	stats.PressureSRTTMillis = r.pressure.rtt.Milliseconds()
	return stats
}

// EnableSteadyStateDelivery is called only after the TLS/bootstrap stream has
// finished. From this point onward payload is datagram carrier/DTLS traffic:
// advancing cumulative ACK may abandon optional FakeTCP repair, but must not
// make a possibly fresh late record undeliverable.
func (r *Receiver) EnableSteadyStateDelivery() {
	if r.steadyStateDelivery {
		return
	}
	r.steadyStateDelivery = true
	r.pressure = repairPressure{}
	r.steadyFloor = r.next
	r.steadyFloorActive = true
	r.deliveredBySeq = make(map[uint32]uint32)
}

func (r *Receiver) Accept(seq uint32, payloadLen int) (deliver, sackNeeded bool) {
	return r.AcceptAt(seq, payloadLen, time.Now(), 0)
}

// AcceptAt uses the caller's receive timestamp and its same-association sender
// SRTT. RTO (including backoff) is deliberately not a substitute for path RTT.
// Callers serialize this with all other receiver mutations.
func (r *Receiver) AcceptAt(seq uint32, payloadLen int, now time.Time, srtt time.Duration) (deliver, sackNeeded bool) {
	if payloadLen <= 0 {
		return false, false
	}
	end := seq + uint32(payloadLen)
	if seqLT(seq, r.next) {
		// Keep pre-steady/bootstrap sequence space strict. During the first bounded
		// steady-state history window this also prevents a straggling TLS repair
		// from being mistaken for a DTLS datagram.
		if !r.steadyStateDelivery || (r.steadyFloorActive && seqLT(seq, r.steadyFloor)) {
			r.stats.Duplicates++
			r.stats.BelowNextDrops++
			return false, len(r.sacksByStart) != 0
		}
		if _, alreadyDelivered := r.deliveredBySeq[seq]; alreadyDelivered {
			r.stats.Duplicates++
			r.stats.BelowNextDrops++
			return false, len(r.sacksByStart) != 0
		}

		// Cumulative ACK has already crossed this sequence, so it cannot recreate
		// sender repair debt or SACK state. Deliver once locally and let the bounded
		// carrier/DTLS replay layer resolve ambiguity older than our exact history.
		r.stats.Delivered++
		r.stats.LateBelowACK++
		r.rememberDelivered(seq, end)
		return true, len(r.sacksByStart) != 0
	}
	if _, exists := r.outOfOrder[seq]; exists {
		r.stats.Duplicates++
		r.stats.BufferedDuplicates++
		return false, len(r.sacksByStart) != 0
	}
	r.stats.Delivered++
	r.rememberDelivered(seq, end)
	if r.steadyStateDelivery {
		r.pressure.observe(now, srtt)
	}
	if seq != r.next {
		r.stats.OutOfOrder++
		r.outOfOrder[seq] = end
		if n := len(r.outOfOrder); n > r.stats.PeakBufferedOO {
			r.stats.PeakBufferedOO = n
		}
		r.insertSACKRange(seq, end)
		r.updatePressureHole(now)
		r.maybeForgiveReorderPressure(now)
		return true, len(r.sacksByStart) != 0
	}

	r.next = end
	r.consumeContiguousSACKs()
	r.updatePressureHole(now)
	return true, len(r.sacksByStart) != 0
}

func (r *Receiver) rememberDelivered(seq, end uint32) {
	if !r.steadyStateDelivery {
		return
	}
	if r.deliveredBySeq == nil {
		r.deliveredBySeq = make(map[uint32]uint32)
	}
	if _, exists := r.deliveredBySeq[seq]; exists {
		return
	}
	r.deliveredBySeq[seq] = end
	r.deliveryOrder = append(r.deliveryOrder, seq)
	r.steadyDelivered++
	if r.steadyDelivered >= recentDeliveryHistoryLimit {
		r.steadyFloorActive = false
	}
	for len(r.deliveredBySeq) > recentDeliveryHistoryLimit {
		for r.deliveryHead < len(r.deliveryOrder) {
			old := r.deliveryOrder[r.deliveryHead]
			r.deliveryHead++
			if _, live := r.deliveredBySeq[old]; live {
				delete(r.deliveredBySeq, old)
				break
			}
		}
	}
	if r.deliveryHead >= recentDeliveryHistoryLimit && r.deliveryHead*2 >= len(r.deliveryOrder) {
		copy(r.deliveryOrder, r.deliveryOrder[r.deliveryHead:])
		r.deliveryOrder = r.deliveryOrder[:len(r.deliveryOrder)-r.deliveryHead]
		r.deliveryHead = 0
	}
}

// maybeForgiveReorderPressure bounds receiver repair debt using delivery-rate
// pressure plus hole age, with a separate emergency bound below the hard limit.
// The receiver has already delivered every out-of-order first arrival to the
// datagram layer, so advancing ACK across the oldest still-missing byte range
// only abandons shadow repair; it never fabricates application payload.
//
// No extra control packet is required. The next ordinary ACK carries the new
// cumulative value, and normal sender ACK processing releases both unresolved
// payload and SACK-retired tombstones below it. In strict/bootstrap mode a late
// packet below r.next is discarded as before. In steady-state datagram mode an
// exact recent duplicate is suppressed, while an unknown late record may still
// be delivered upward without recreating ACK/SACK repair debt.
func (r *Receiver) maybeForgiveReorderPressure(now time.Time) {
	if !r.steadyStateDelivery {
		return
	}
	for r.pressureAllowsForgiveness(now) {
		start, ok := r.oldestLiveSACKStart()
		if !ok || start == r.next {
			return
		}
		gapBytes := uint32(start - r.next)
		if gapBytes == 0 || gapBytes >= 1<<31 {
			return
		}
		r.next = start
		if len(r.outOfOrder) >= PartialReliabilityEmergencyLimit {
			r.stats.EmergencyForgivenGaps++
		} else {
			r.stats.SoftForgivenGaps++
		}
		r.stats.ForgivenGaps++
		r.stats.ForgivenBytes += uint64(gapBytes)
		before := len(r.outOfOrder)
		r.consumeContiguousSACKs()
		r.updatePressureHole(now)
		if len(r.outOfOrder) >= before {
			return
		}
	}
}

func (r *Receiver) oldestLiveSACKStart() (uint32, bool) {
	var best uint32
	var bestDistance uint32
	found := false
	for start := range r.sacksByStart {
		distance := uint32(start - r.next)
		if distance == 0 || distance >= 1<<31 {
			continue
		}
		if !found || distance < bestDistance {
			best = start
			bestDistance = distance
			found = true
		}
	}
	return best, found
}

func (r *Receiver) SACKBlocks(dst *[4]SACKBlock) int {
	if dst == nil {
		return 0
	}
	// Preserve the mature newest-first wire image whenever RFC 2018's four
	// blocks are enough. Rotation is only a severe-reordering fallback.
	if len(r.sacksByStart) <= len(dst) {
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

	// Block zero stays TCP-like: it describes the most recently arrived live
	// range. The remaining three slots rotate across older live ranges. This
	// makes ACK loss unable to strand thousands of already-delivered records in
	// the sender's repair window while keeping the on-wire option standard.
	n := 0
	var primary uint32
	havePrimary := false
	for i := 0; i < r.recentSACKN; i++ {
		start := r.recentSACK[i]
		end, ok := r.sacksByStart[start]
		if !ok || seqLT(start, r.next) {
			continue
		}
		dst[n] = SACKBlock{Start: start, End: end}
		n++
		primary = start
		havePrimary = true
		break
	}

	for n < len(dst) {
		start, end, ok := r.nextCoverageRange(primary, havePrimary, dst[:n])
		if !ok {
			break
		}
		dst[n] = SACKBlock{Start: start, End: end}
		n++
	}
	return n
}

func (r *Receiver) nextCoverageRange(primary uint32, havePrimary bool, selected []SACKBlock) (uint32, uint32, bool) {
	// Every queued entry is consumed at most once before compaction, so stale
	// merge entries are amortized O(1) rather than forcing a sort on every ACK.
	for pass := 0; pass < 2; pass++ {
		for r.coverageAt < len(r.coverageQueue) {
			start := r.coverageQueue[r.coverageAt]
			r.coverageAt++
			end, ok := r.sacksByStart[start]
			if !ok || seqLT(start, r.next) || (havePrimary && start == primary) {
				continue
			}
			duplicate := false
			for _, b := range selected {
				if b.Start == start {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			return start, end, true
		}
		r.compactCoverageQueue()
		if len(r.coverageQueue) == 0 {
			break
		}
	}
	return 0, 0, false
}

func (r *Receiver) compactCoverageQueue() {
	q := r.coverageQueue[:0]
	for start := range r.sacksByStart {
		if !seqLT(start, r.next) {
			q = append(q, start)
		}
	}
	r.coverageQueue = q
	r.coverageAt = 0
}

func (r *Receiver) maybeCompactCoverageQueue() {
	live := len(r.sacksByStart)
	if live == 0 {
		r.coverageQueue = r.coverageQueue[:0]
		r.coverageAt = 0
		return
	}
	remaining := len(r.coverageQueue) - r.coverageAt
	if remaining > live*4+256 {
		r.compactCoverageQueue()
	}
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
	r.coverageQueue = append(r.coverageQueue, start)
	r.maybeCompactCoverageQueue()
}

func (r *Receiver) consumeContiguousSACKs() {
	for {
		start := r.next
		end, ok := r.sacksByStart[start]
		if !ok {
			r.maybeCompactCoverageQueue()
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
