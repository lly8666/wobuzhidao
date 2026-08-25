package faketcp

import "time"

const payloadSlabSize = 2048

const (
	minRTO = time.Second
	maxRTO = 60 * time.Second
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
	slot       int
}

type SenderStats struct {
	Enqueued        uint64
	EnqueuedBytes   uint64
	Acked           uint64
	SACKed          uint64
	FastRetransmits uint64
	RTOTransmits    uint64
	RetransmitBytes uint64
	LossMarked      uint64
	LossMarkedBytes uint64
	PeakPending     int
}

type Sender struct {
	nextSeq      uint32
	pending      []*Pending
	bySeq        map[uint32]*Pending
	head         int
	active       int
	lastAck      uint32
	dupAcks      int
	fastRetxSeq  uint32
	fastRetxDone bool
	rto          time.Duration
	srtt         time.Duration
	rttvar       time.Duration
	recovery     RecoveryMode

	// rackLatestTx is used only in RecoverySACKRACK. It is intentionally absent
	// from the first-send decision and can never gate a new inner datagram.
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
	return &Sender{
		nextSeq: nextSeq,
		lastAck: nextSeq,
		rto:     clampRTO(initialRTO),
		bySeq:   make(map[uint32]*Pending),
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
	buf := s.allocPayload(len(payload))
	copy(buf, payload)
	p := &Pending{
		Seq: s.nextSeq, End: s.nextSeq + uint32(len(payload)),
		Payload: buf, FirstSent: now, LastSent: now, slot: len(s.pending),
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

// AckSelective consumes cumulative ACK plus SACK information. Both recovery
// modes retain packet-preserving, no-HOL inner semantics; the selector changes
// only background TCP-like shadow retransmission behavior so first-arrival can
// be measured A/B without changing any other carrier code.
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

	if s.recovery == RecoverySACKRACK {
		if p := s.rackLossCandidate(now); p != nil {
			if p.Seq == s.lastAck {
				s.fastRetxSeq = p.Seq
				s.fastRetxDone = true
				s.dupAcks = 0
			}
			s.markRetry(p, now, true)
			return p
		}
		if p := s.sackLossCandidate(); p != nil {
			if p.Seq == s.lastAck {
				s.fastRetxSeq = p.Seq
				s.fastRetxDone = true
				s.dupAcks = 0
			}
			s.markRetry(p, now, true)
			return p
		}
	}

	if !advanced && ack == s.lastAck && s.dupAcks >= 3 {
		s.dupAcks = 0
		p := s.oldest()
		if p != nil && p.Seq == ack && !p.WasRetried && (!s.fastRetxDone || s.fastRetxSeq != p.Seq) {
			s.fastRetxSeq = p.Seq
			s.fastRetxDone = true
			s.markRetry(p, now, true)
			return p
		}
	}
	return nil
}

func (s *Sender) sackLossCandidate() *Pending {
	var candidate *Pending
	sackedAbove := 0
	for i := s.head; i < len(s.pending); i++ {
		p := s.pending[i]
		if p == nil {
			continue
		}
		if candidate == nil {
			if !p.SACKed && !p.WasRetried {
				candidate = p
			}
			continue
		}
		if p.SACKed {
			sackedAbove++
			if sackedAbove >= 3 {
				return candidate
			}
		}
	}
	return nil
}

func (s *Sender) rackLossCandidate(now time.Time) *Pending {
	if s.rackLatestTx.IsZero() {
		return nil
	}
	reo := s.rackReorderingWindow()
	for i := s.head; i < len(s.pending); i++ {
		p := s.pending[i]
		// RACK is deliberately restricted to one failed-repair inference. The
		// SACK scoreboard/classic dup-ACK path owns the first fast repair. Once
		// that repair has itself been inferred lost, a second fast repair is
		// allowed; further attempts fall back to the existing backed-off RTO.
		// This bounds duplicate traffic when old SACK ranges fall out of the
		// receiver's four-block advertisement under a large outstanding window.
		if p == nil || p.SACKed || p.LastSent.IsZero() || !p.WasRetried || p.Retries != 1 {
			continue
		}
		if !p.LastSent.Before(s.rackLatestTx) {
			continue
		}
		if now.Sub(p.LastSent) < reo {
			continue
		}
		return p
	}
	return nil
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
	if s.recovery != RecoverySACKRACK || p == nil || p.LastSent.IsZero() {
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
	s.advanceHead()
}

func (s *Sender) ackOne(p *Pending) {
	if p == nil || s.bySeq[p.Seq] != p {
		return
	}
	delete(s.bySeq, p.Seq)
	s.releasePayload(p.Payload)
	p.Payload = nil
	s.active--
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
	p := s.oldest()
	if p == nil || now.Sub(p.LastSent) < s.rto {
		return nil
	}
	s.markRetry(p, now, false)
	s.rto = clampRTO(s.rto * 2)
	return p
}

func (s *Sender) oldest() *Pending {
	s.advanceHead()
	if s.head >= len(s.pending) {
		return nil
	}
	return s.pending[s.head]
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
	s.rto = clampRTO(s.srtt + 4*s.rttvar)
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
