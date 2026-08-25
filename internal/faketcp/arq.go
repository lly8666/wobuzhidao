package faketcp

import "time"

const payloadSlabSize = 2048

type Pending struct {
	Seq        uint32
	End        uint32
	Payload    []byte
	FirstSent  time.Time
	LastSent   time.Time
	Retries    uint32
	WasRetried bool
	slot       int
}

type SenderStats struct {
	Enqueued        uint64
	Acked           uint64
	SACKed          uint64
	FastRetransmits uint64
	RTOTransmits    uint64
	RetransmitBytes uint64
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
	freeSlabs    [][]byte
	stats        SenderStats
}

func NewSender(nextSeq uint32, initialRTO time.Duration) *Sender {
	if initialRTO <= 0 { initialRTO = time.Second }
	return &Sender{nextSeq:nextSeq, lastAck:nextSeq, rto:clampRTO(initialRTO), bySeq:make(map[uint32]*Pending)}
}

func (s *Sender) NextSeq() uint32 { return s.nextSeq }
func (s *Sender) RTO() time.Duration { return s.rto }
func (s *Sender) Pending() int { return s.active }
func (s *Sender) Stats() SenderStats { return s.stats }

func (s *Sender) Enqueue(payload []byte, now time.Time) *Pending {
	buf := s.allocPayload(len(payload))
	copy(buf, payload)
	p := &Pending{Seq:s.nextSeq, End:s.nextSeq+uint32(len(payload)), Payload:buf, FirstSent:now, LastSent:now, slot:len(s.pending)}
	s.nextSeq = p.End
	s.pending = append(s.pending, p)
	s.bySeq[p.Seq] = p
	s.active++
	s.stats.Enqueued++
	if s.active > s.stats.PeakPending { s.stats.PeakPending = s.active }
	return p
}

func (s *Sender) allocPayload(n int) []byte {
	if n <= payloadSlabSize {
		last := len(s.freeSlabs)-1
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

// AckSelective consumes cumulative ACK + RFC 2018 SACK. Exact WBD datagram
// SACKs are O(1) through bySeq; cumulative ACK remains a sequential head walk.
// A given cumulative-ACK hole gets at most one fast retransmit until ACK moves.
func (s *Sender) AckSelective(ack uint32, sacks []SACKBlock, now time.Time) *Pending {
	advanced := seqLT(s.lastAck, ack)
	if advanced {
		s.lastAck = ack
		s.dupAcks = 0
		s.fastRetxDone = false
		s.ackCumulative(ack, now)
	} else if ack == s.lastAck && s.active != 0 {
		s.dupAcks++
	}

	for _, b := range sacks {
		if b.Start == b.End || seqLT(b.End, b.Start) { continue }
		seq := b.Start
		for {
			p := s.bySeq[seq]
			if p == nil || seqLT(b.End, p.End) { break }
			next := p.End
			s.ackOne(p, true, now)
			if next == b.End { break }
			seq = next
		}
	}
	s.advanceHead()

	if !advanced && ack == s.lastAck && s.dupAcks >= 3 {
		s.dupAcks = 0
		p := s.oldest()
		if p != nil && p.Seq == ack && (!s.fastRetxDone || s.fastRetxSeq != p.Seq) {
			s.fastRetxSeq = p.Seq
			s.fastRetxDone = true
			s.markRetry(p, now, true)
			return p
		}
	}
	return nil
}

func (s *Sender) ackCumulative(ack uint32, now time.Time) {
	for i := s.head; i < len(s.pending); i++ {
		p := s.pending[i]
		if p == nil { continue }
		if !seqLE(p.End, ack) { break }
		s.ackOne(p, false, now)
	}
	s.advanceHead()
}

func (s *Sender) ackOne(p *Pending, sack bool, now time.Time) {
	if p == nil || s.bySeq[p.Seq] != p { return }
	if !p.WasRetried { s.observeRTT(now.Sub(p.FirstSent)) }
	delete(s.bySeq, p.Seq)
	s.releasePayload(p.Payload)
	p.Payload = nil
	s.active--
	s.stats.Acked++
	if sack { s.stats.SACKed++ }
	if p.slot >= 0 && p.slot < len(s.pending) && s.pending[p.slot] == p {
		s.pending[p.slot] = nil
	}
}

func (s *Sender) advanceHead() {
	for s.head < len(s.pending) && s.pending[s.head] == nil { s.head++ }
	if s.head >= 4096 && s.head*2 >= len(s.pending) {
		oldHead := s.head
		copy(s.pending, s.pending[oldHead:])
		s.pending = s.pending[:len(s.pending)-oldHead]
		for i, p := range s.pending { if p != nil { p.slot = i } }
		s.head = 0
	}
}

// RetransmitDue deliberately does not apply TCP-style global exponential RTO
// backoff. Packet loss on this carrier is not treated as congestion and a slow
// hole must not inflate recovery time for all later independent datagrams.
func (s *Sender) RetransmitDue(now time.Time) *Pending {
	p := s.oldest()
	if p == nil || now.Sub(p.LastSent) < s.rto { return nil }
	s.markRetry(p, now, false)
	return p
}

func (s *Sender) oldest() *Pending {
	s.advanceHead()
	if s.head >= len(s.pending) { return nil }
	return s.pending[s.head]
}

func (s *Sender) markRetry(p *Pending, now time.Time, fast bool) {
	p.LastSent = now
	p.Retries++
	p.WasRetried = true
	if fast { s.stats.FastRetransmits++ } else { s.stats.RTOTransmits++ }
	s.stats.RetransmitBytes += uint64(len(p.Payload))
}

func (s *Sender) observeRTT(sample time.Duration) {
	if sample <= 0 { return }
	if s.srtt == 0 {
		s.srtt = sample
		s.rttvar = sample/2
	} else {
		d := s.srtt-sample
		if d < 0 { d = -d }
		s.rttvar = (3*s.rttvar+d)/4
		s.srtt = (7*s.srtt+sample)/8
	}
	s.rto = clampRTO(s.srtt+4*s.rttvar)
}

func clampRTO(v time.Duration) time.Duration {
	if v < 20*time.Millisecond { return 20*time.Millisecond }
	if v > 2*time.Second { return 2*time.Second }
	return v
}

type ReceiverStats struct {
	Delivered  uint64
	Duplicates uint64
	OutOfOrder uint64
}

type Receiver struct {
	next       uint32
	outOfOrder map[uint32]uint32
	stats      ReceiverStats
}

func NewReceiver(nextSeq uint32) *Receiver {
	return &Receiver{next:nextSeq, outOfOrder:make(map[uint32]uint32)}
}
func (r *Receiver) Next() uint32 { return r.next }
func (r *Receiver) Stats() ReceiverStats { return r.stats }

// Accept never buffers payload. Later datagrams are delivered immediately while
// only sequence ranges are retained to advance cumulative ACK once holes close.
func (r *Receiver) Accept(seq uint32, payloadLen int) (deliver, outOfOrder bool) {
	if payloadLen <= 0 { return false, false }
	end := seq+uint32(payloadLen)
	if seqLT(seq, r.next) {
		r.stats.Duplicates++
		return false, false
	}
	if _, exists := r.outOfOrder[seq]; exists {
		r.stats.Duplicates++
		return false, seq != r.next
	}
	r.stats.Delivered++
	if seq != r.next {
		r.stats.OutOfOrder++
		r.outOfOrder[seq] = end
		return true, true
	}
	r.next = end
	for {
		nextEnd, ok := r.outOfOrder[r.next]
		if !ok { break }
		delete(r.outOfOrder, r.next)
		r.next = nextEnd
	}
	return true, false
}

func seqLT(a,b uint32) bool { return int32(a-b)<0 }
func seqLE(a,b uint32) bool { return a==b || seqLT(a,b) }
