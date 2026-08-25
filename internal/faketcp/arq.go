package faketcp

import "time"

const payloadSlabSize = 2048

// RFC 6298 uses a 1 second lower bound for the retransmission timer. The
// product's first-arrival path does not wait on this timer: new/out-of-order
// datagrams continue to be delivered immediately while the TCP-shaped shadow
// reliability state catches up in the background.
const (
	minRTO = time.Second
	maxRTO = 60 * time.Second
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
	// LossMarked counts original data segments that required at least one
	// retransmission. A segment is counted once regardless of how many fast/RTO
	// retries follow. This is the low-overhead sender-side loss sample used by
	// periodic fixed-FEC profile selection; retry-attempt counters must not be
	// treated as a packet-loss probability.
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

	// rackLatestTx is the most recent transmission timestamp among segments
	// newly proven delivered by cumulative ACK or SACK. RFC 8985 RACK compares
	// this timestamp against older outstanding transmissions, including prior
	// retransmissions, so a lost retransmission need not wait for an RTO.
	rackLatestTx time.Time

	freeSlabs [][]byte
	stats     SenderStats
}

func NewSender(nextSeq uint32, initialRTO time.Duration) *Sender {
	if initialRTO <= 0 {
		initialRTO = minRTO
	}
	return &Sender{
		nextSeq: nextSeq,
		lastAck: nextSeq,
		rto:     clampRTO(initialRTO),
		bySeq:   make(map[uint32]*Pending),
	}
}

func (s *Sender) NextSeq() uint32                 { return s.nextSeq }
func (s *Sender) RTO() time.Duration              { return s.rto }
func (s *Sender) Pending() int                    { return s.active }
func (s *Sender) Stats() SenderStats              { return s.stats }
func (s *Sender) LastAck() uint32                 { return s.lastAck }
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

// AckSelective consumes a TCP cumulative ACK plus RFC 2018 SACK information.
// SACK is advisory for retention: payload bytes stay queued until cumulatively
// ACKed. Loss recovery has two shadow-TCP signals and never gates first-arrival
// inner delivery:
//   - an RFC-6675-style three-later-SACK threshold for original holes;
//   - an RFC-8985 RACK-style time ordering check that also detects a lost
//     retransmission when a chronologically later transmission is delivered.
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

	// RACK comes first because it can identify a retransmission that was itself
	// lost. A successful later retransmission/SACK updates rackLatestTx, and an
	// older outstanding transmission becomes eligible after a conservative
	// reordering window. markRetry moves its LastSent forward, so the same ACK
	// cannot repeatedly retransmit it.
	if p := s.rackLossCandidate(now); p != nil {
		if p.Seq == s.lastAck {
			s.fastRetxSeq = p.Seq
			s.fastRetxDone = true
			s.dupAcks = 0
		}
		s.markRetry(p, now, true)
		return p
	}

	// RFC 6675's IsLost rule is richer than this datagram-oriented scoreboard,
	// but the three-later-SACKed-segment threshold preserves the familiar TCP
	// duplicate-ACK evidence while allowing recovery to continue after a
	// cumulative ACK jumps from one repaired hole to the next. Retransmit each
	// inferred original hole once here; a lost retransmission is then RACK/RTO
	// owned rather than being blindly repeated from stale SACK evidence.
	if p := s.sackLossCandidate(); p != nil {
		if p.Seq == s.lastAck {
			s.fastRetxSeq = p.Seq
			s.fastRetxDone = true
			s.dupAcks = 0
		}
		s.markRetry(p, now, true)
		return p
	}

	// RFC 5681 fast retransmit fallback: the third duplicate ACK retransmits
	// SND.UNA when neither RACK nor the SACK scoreboard already classified it.
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

// sackLossCandidate returns the earliest unsacked, never-retried pending
// segment once at least three later segments are known SACKed. Because any
// later candidate with three SACKed segments above it implies the earlier
// candidate has the same evidence, a single forward scan is sufficient.
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

// rackLossCandidate is a compact shadow implementation of RACK's core time
// inference. A not-yet-delivered segment is considered lost when a segment
// transmitted later has been proven delivered and the older transmission has
// aged beyond a reordering window. This applies equally to original sends and
// retransmissions, which is the important property missing from DupAck-only
// recovery under random loss.
func (s *Sender) rackLossCandidate(now time.Time) *Pending {
	if s.rackLatestTx.IsZero() {
		return nil
	}
	reo := s.rackReorderingWindow()
	for i := s.head; i < len(s.pending); i++ {
		p := s.pending[i]
		if p == nil || p.SACKed || p.LastSent.IsZero() {
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
	// RFC 8985 adapts the reordering window using min RTT and reordering history.
	// WBD keeps a deliberately conservative fixed fraction because this is
	// shadow reliability, not the owner of inner delivery or congestion control.
	// Ten milliseconds avoids hypersensitive retries on very low-latency hosts.
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
		// A precise first-time SACK of a never-retransmitted segment is a valid
		// RTT sample. The bytes themselves remain queued until cumulative ACK.
		if !p.WasRetried {
			s.observeRTT(now.Sub(p.FirstSent))
		}
	}
}

func (s *Sender) ackCumulative(oldAck, ack uint32, now time.Time) {
	// Only sample a cumulative ACK if it exactly acknowledges one fresh segment
	// starting at old SND.UNA. If the ACK leaps over a repaired hole and sweeps
	// up older out-of-order data, those packet ages are not RTT samples.
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

// RetransmitDue follows the RFC 6298 timeout behavior relevant to the shadow
// TCP carrier: retransmit the earliest cumulatively-unacknowledged segment and
// exponentially back off the RTO. RACK handles losses for which later ACK/SACK
// timing exists; RTO remains the last resort when feedback disappears entirely.
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
	next uint32

	// outOfOrder retains only exact segment boundaries for duplicate detection
	// and cumulative-ACK advancement. sacksByStart/sackStartByEnd maintain merged
	// contiguous SACK ranges separately, so the inner payload is never buffered.
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

// Accept never buffers payload for ordered delivery. A complete later datagram
// is delivered on its first arrival even while cumulative TCP ACK state retains
// a hole. Retransmitted duplicates are suppressed from the inner path. The
// second result means the ACK should include current SACK blocks; it remains
// true after an in-order repair if another hole still leaves live SACK state.
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

// SACKBlocks returns up to four persistent RFC-2018-style ranges. The first
// range is the one most recently touched by an out-of-order arrival; older live
// ranges follow. This keeps the option useful to a real TCP-style scoreboard
// without sorting or allocating on the hot ACK path.
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
