package faketcp

import (
	"sync"
	"time"
)

const (
	senderMinRTO = time.Second
	senderMaxRTO = 60 * time.Second
)

// Pending is the minimal retransmittable FakeTCP payload state required during
// P2 bootstrap. Steady-state SACK/RACK/repair-budget fields are intentionally
// not present yet.
type Pending struct {
	Seq        uint32
	End        uint32
	Payload    []byte
	FirstSent  time.Time
	LastSent   time.Time
	Retries    uint32
	WasRetried bool
	Bootstrap  bool
}

// SenderStats is deliberately limited to counters exercised by the bootstrap
// path. Later steady-state ARQ extraction can extend it without changing these
// meanings.
type SenderStats struct {
	Enqueued        uint64
	EnqueuedBytes   uint64
	Acked           uint64
	RTOTransmits    uint64
	RetransmitBytes uint64
}

// Sender is the smallest reusable portion of the archived ARQ sender needed to
// make BootstrapStream's ACK-gated writes real: copied retransmission payload,
// cumulative ACK release, a bounded bootstrap RTO, and ACK wait notification.
//
// It is internally synchronized because BootstrapStream.Write may wait for ACK
// while the packet receive loop advances the cumulative ACK concurrently.
type Sender struct {
	mu sync.Mutex

	nextSeq uint32
	lastAck uint32
	pending []*Pending

	rto     time.Duration
	baseRTO time.Duration
	stats   SenderStats

	notify chan struct{}
}

func NewSender(nextSeq uint32, initialRTO time.Duration) *Sender {
	initialRTO = clampSenderRTO(initialRTO)
	return &Sender{
		nextSeq: nextSeq,
		lastAck: nextSeq,
		rto:     initialRTO,
		baseRTO: initialRTO,
		notify:  make(chan struct{}),
	}
}

func (s *Sender) NextSeq() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextSeq
}

func (s *Sender) LastAck() uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastAck
}

func (s *Sender) RTO() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.rto
}

func (s *Sender) Pending() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.pending)
}

func (s *Sender) Stats() SenderStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stats
}

// Enqueue copies payload ownership exactly like the archived sender. The
// synchronous marker installed by BootstrapStream makes bootstrap entries use
// the special retransmission ceiling without leaking that policy to later data.
func (s *Sender) Enqueue(payload []byte, now time.Time) *Pending {
	s.mu.Lock()
	defer s.mu.Unlock()

	buf := append([]byte(nil), payload...)
	p := &Pending{
		Seq:       s.nextSeq,
		End:       s.nextSeq + uint32(len(buf)),
		Payload:   buf,
		FirstSent: now,
		LastSent:  now,
		Bootstrap: isBootstrapPayload(payload),
	}
	s.nextSeq = p.End
	s.pending = append(s.pending, p)
	s.stats.Enqueued++
	s.stats.EnqueuedBytes += uint64(len(buf))
	return p
}

// Ack applies only cumulative ACK semantics needed by bootstrap. SACK/RACK are
// intentionally absent from this P2 subtask.
func (s *Sender) Ack(ack uint32, _ time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !seqLT(s.lastAck, ack) {
		return
	}
	s.lastAck = ack

	n := 0
	for n < len(s.pending) && seqLE(s.pending[n].End, ack) {
		s.stats.Acked++
		n++
	}
	if n != 0 {
		copy(s.pending, s.pending[n:])
		clear(s.pending[len(s.pending)-n:])
		s.pending = s.pending[:len(s.pending)-n]
	}
	s.signalLocked()
}

// WaitAck blocks until the cumulative ACK reaches end. It is suitable directly
// as BootstrapWaitAck. A zero deadline means no local deadline.
func (s *Sender) WaitAck(end uint32, deadline time.Time) error {
	for {
		s.mu.Lock()
		if seqLE(end, s.lastAck) {
			s.mu.Unlock()
			return nil
		}
		ch := s.notify
		s.mu.Unlock()

		if deadline.IsZero() {
			<-ch
			continue
		}
		d := time.Until(deadline)
		if d <= 0 {
			return ErrBootstrapTimeout
		}
		timer := time.NewTimer(d)
		select {
		case <-ch:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		case <-timer.C:
			return ErrBootstrapTimeout
		}
	}
}

// RetransmitDue returns the oldest due payload. Bootstrap retransmissions use a
// hard 2s ceiling and do not back off the shared RTO. A later ordinary payload
// retains normal exponential RTO backoff, preserving the archived invariant.
func (s *Sender) RetransmitDue(now time.Time) *Pending {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.pending) == 0 {
		return nil
	}
	p := s.pending[0]
	rto := s.rto
	if p.Bootstrap && rto > bootstrapRetransmitCeiling {
		rto = bootstrapRetransmitCeiling
	}
	if now.Sub(p.LastSent) < rto {
		return nil
	}

	p.LastSent = now
	p.Retries++
	p.WasRetried = true
	s.stats.RTOTransmits++
	s.stats.RetransmitBytes += uint64(len(p.Payload))

	if !p.Bootstrap {
		s.rto = clampSenderRTO(s.rto * 2)
	}
	return p
}

func (s *Sender) signalLocked() {
	close(s.notify)
	s.notify = make(chan struct{})
}

func clampSenderRTO(v time.Duration) time.Duration {
	if v <= 0 || v < senderMinRTO {
		return senderMinRTO
	}
	if v > senderMaxRTO {
		return senderMaxRTO
	}
	return v
}
