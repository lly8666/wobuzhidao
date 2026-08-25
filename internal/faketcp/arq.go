package faketcp

import (
	"time"
)

// Pending is one transmitted datagram occupying TCP sequence space. Payload is
// retained until cumulatively acknowledged so a lost outer segment can be sent
// again without asking DTLS/FEC to regenerate it.
type Pending struct {
	Seq        uint32
	End        uint32
	Payload    []byte
	FirstSent  time.Time
	LastSent   time.Time
	Retries    uint32
	WasRetried bool
}

type Sender struct {
	nextSeq uint32
	pending []*Pending
	head    int
	lastAck uint32
	dupAcks int
	rto     time.Duration
	srtt    time.Duration
	rttvar  time.Duration
}

func NewSender(nextSeq uint32, initialRTO time.Duration) *Sender {
	if initialRTO <= 0 {
		initialRTO = time.Second
	}
	return &Sender{nextSeq: nextSeq, lastAck: nextSeq, rto: clampRTO(initialRTO)}
}

func (s *Sender) NextSeq() uint32 { return s.nextSeq }
func (s *Sender) RTO() time.Duration { return s.rto }
func (s *Sender) Pending() int { return len(s.pending) - s.head }

func (s *Sender) Enqueue(payload []byte, now time.Time) *Pending {
	p := &Pending{
		Seq: s.nextSeq, End: s.nextSeq + uint32(len(payload)),
		Payload: append([]byte(nil), payload...), FirstSent: now, LastSent: now,
	}
	s.nextSeq = p.End
	s.pending = append(s.pending, p)
	return p
}

// Ack consumes a cumulative TCP ACK. It returns a segment for fast retransmit
// after three duplicate ACKs. Receive delivery is intentionally independent of
// this cumulative sequence bookkeeping, so a hole never blocks later datagrams.
func (s *Sender) Ack(ack uint32, now time.Time) *Pending {
	if seqLE(ack, s.lastAck) {
		if ack == s.lastAck && s.Pending() != 0 {
			s.dupAcks++
			if s.dupAcks >= 3 {
				s.dupAcks = 0
				p := s.oldest()
				if p != nil && p.Seq == ack {
					s.markRetry(p, now)
					return p
				}
			}
		}
		return nil
	}

	s.lastAck = ack
	s.dupAcks = 0
	for s.head < len(s.pending) {
		p := s.pending[s.head]
		if !seqLE(p.End, ack) {
			break
		}
		if !p.WasRetried {
			s.observeRTT(now.Sub(p.FirstSent))
		}
		p.Payload = nil
		s.pending[s.head] = nil
		s.head++
	}
	// Compact occasionally without moving payload bytes.
	if s.head >= 4096 && s.head*2 >= len(s.pending) {
		copy(s.pending, s.pending[s.head:])
		s.pending = s.pending[:len(s.pending)-s.head]
		s.head = 0
	}
	return nil
}

func (s *Sender) RetransmitDue(now time.Time) *Pending {
	p := s.oldest()
	if p == nil || now.Sub(p.LastSent) < s.rto {
		return nil
	}
	s.markRetry(p, now)
	// Back off only the timer. There is deliberately no congestion window: this
	// remains a datagram-oriented FakeTCP carrier rather than an ordinary TCP.
	s.rto = clampRTO(s.rto * 2)
	return p
}

func (s *Sender) oldest() *Pending {
	if s.head >= len(s.pending) {
		return nil
	}
	return s.pending[s.head]
}

func (s *Sender) markRetry(p *Pending, now time.Time) {
	p.LastSent = now
	p.Retries++
	p.WasRetried = true
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
		if d < 0 { d = -d }
		s.rttvar = (3*s.rttvar + d) / 4
		s.srtt = (7*s.srtt + sample) / 8
	}
	s.rto = clampRTO(s.srtt + 4*s.rttvar)
}

func clampRTO(v time.Duration) time.Duration {
	if v < 20*time.Millisecond { return 20*time.Millisecond }
	if v > 4*time.Second { return 4*time.Second }
	return v
}

type Receiver struct {
	next uint32
	// outOfOrder only retains sequence ranges, never payload. Payload is emitted
	// immediately on first arrival, which is the key difference from TCP HOL.
	outOfOrder map[uint32]uint32
}

func NewReceiver(nextSeq uint32) *Receiver {
	return &Receiver{next: nextSeq, outOfOrder: make(map[uint32]uint32)}
}

func (r *Receiver) Next() uint32 { return r.next }

// Accept reports whether this datagram is new and should be delivered upward,
// and whether it arrived out of order. Duplicate/retransmitted bytes are ACKed
// but never delivered twice.
func (r *Receiver) Accept(seq uint32, payloadLen int) (deliver, outOfOrder bool) {
	if payloadLen <= 0 {
		return false, false
	}
	end := seq + uint32(payloadLen)
	if seqLT(seq, r.next) {
		return false, false
	}
	if _, exists := r.outOfOrder[seq]; exists {
		return false, seq != r.next
	}
	if seq != r.next {
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

func seqLT(a, b uint32) bool { return int32(a-b) < 0 }
func seqLE(a, b uint32) bool { return a == b || seqLT(a, b) }
