package faketcp

import (
	"errors"
	"sync"
	"time"
)

const (
	senderMinRTO = time.Second
	senderMaxRTO = 60 * time.Second
)

var ErrInvalidACK = errors.New("faketcp: ACK exceeds sent sequence space")

// Pending is the minimal retransmittable FakeTCP payload/control state required
// during bootstrap. Payload is copied and Flags/sequence are immutable so a
// retransmission of the same TCP sequence always emits the same bytes/control.
type Pending struct {
	Seq        uint32
	End        uint32
	Payload    []byte
	Flags      uint8
	FirstSent  time.Time
	LastSent   time.Time
	Retries    uint32
	WasRetried bool
	Bootstrap  bool
}

type SenderStats struct {
	Enqueued        uint64
	EnqueuedBytes   uint64
	Acked           uint64
	RTOTransmits    uint64
	RetransmitBytes uint64
}

// Sender owns the reliable bootstrap send sequence. It is intentionally not the
// steady-state finite-repair sender. It additionally tracks the peer's current
// TCP receive window so bootstrap can have a small bounded flight instead of
// stop-and-wait without overrunning a zero/small window.
type Sender struct {
	mu sync.Mutex

	nextSeq uint32
	lastAck uint32
	pending []*Pending

	rto     time.Duration
	baseRTO time.Duration
	stats   SenderStats

	peerWindow      uint32
	peerWindowKnown bool
	fail            error
	notify          chan struct{}
}

func NewSender(nextSeq uint32, initialRTO time.Duration) *Sender {
	initialRTO = clampSenderRTO(initialRTO)
	return &Sender{
		nextSeq: nextSeq,
		lastAck: nextSeq,
		rto:     initialRTO,
		baseRTO: initialRTO,
		peerWindow: 65535,
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

func (s *Sender) Enqueue(payload []byte, now time.Time) *Pending {
	s.mu.Lock()
	defer s.mu.Unlock()

	buf := append([]byte(nil), payload...)
	p := &Pending{
		Seq:       s.nextSeq,
		End:       s.nextSeq + uint32(len(buf)),
		Payload:   buf,
		Flags:     FlagACK | FlagPSH,
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

// EnqueueFIN allocates exactly one sequence number and retains the control flag
// for retransmission. It is used only by the temporary TCP lifecycle/fallback
// path, not by the steady-state data plane.
func (s *Sender) EnqueueFIN(now time.Time) *Pending {
	s.mu.Lock()
	defer s.mu.Unlock()

	p := &Pending{
		Seq:       s.nextSeq,
		End:       s.nextSeq + 1,
		Flags:     FlagACK | FlagFIN,
		FirstSent: now,
		LastSent:  now,
		Bootstrap: true,
	}
	s.nextSeq = p.End
	s.pending = append(s.pending, p)
	s.stats.Enqueued++
	return p
}

// Ack keeps the legacy void helper for existing focused tests. New association
// code uses AckChecked so an ACK beyond nextSeq cannot advance state.
func (s *Sender) Ack(ack uint32, now time.Time) {
	_ = s.AckChecked(ack, now)
}

func (s *Sender) AckChecked(ack uint32, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if seqLT(s.nextSeq, ack) {
		return ErrInvalidACK
	}
	if !seqLT(s.lastAck, ack) {
		return nil
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
	return nil
}

// UpdatePeerWindow records the receive window advertised by the peer. Window
// scaling applies only after the SYN exchange, which is exactly where callers
// use this method.
func (s *Sender) UpdatePeerWindow(raw uint16, scale uint8, scaleSet bool) {
	s.mu.Lock()
	if scaleSet && scale <= MaxWindowScale {
		s.peerWindow = uint32(raw) << scale
	} else {
		s.peerWindow = uint32(raw)
	}
	s.peerWindowKnown = true
	s.signalLocked()
	s.mu.Unlock()
}

// WaitWindow waits until both the peer window and the bootstrap-local flight cap
// can admit need bytes. A zero peer window therefore blocks new sends but ACK or
// window-update segments wake the waiter. No artificial pacing sleep is used.
func (s *Sender) WaitWindow(maxNeed, localCap int, deadline time.Time) (int, error) {
	if maxNeed <= 0 || localCap <= 0 {
		return 0, ErrBootstrapOverflow
	}
	if maxNeed > localCap {
		maxNeed = localCap
	}
	for {
		s.mu.Lock()
		if s.fail != nil {
			err := s.fail
			s.mu.Unlock()
			return 0, err
		}
		limit := uint32(localCap)
		if s.peerWindowKnown && s.peerWindow < limit {
			limit = s.peerWindow
		}
		outstanding := s.nextSeq - s.lastAck
		if outstanding < limit {
			available := int(limit - outstanding)
			if available > maxNeed {
				available = maxNeed
			}
			if available > 0 {
				s.mu.Unlock()
				return available, nil
			}
		}
		ch := s.notify
		s.mu.Unlock()

		if err := waitSenderNotify(ch, deadline); err != nil {
			return 0, err
		}
	}
}

func (s *Sender) WaitAck(end uint32, deadline time.Time) error {
	for {
		s.mu.Lock()
		if s.fail != nil {
			err := s.fail
			s.mu.Unlock()
			return err
		}
		if seqLE(end, s.lastAck) {
			s.mu.Unlock()
			return nil
		}
		ch := s.notify
		s.mu.Unlock()

		if err := waitSenderNotify(ch, deadline); err != nil {
			return err
		}
	}
}

// Abort wakes all window/ACK waiters. It does not manufacture ACK progress.
func (s *Sender) Abort(err error) {
	if err == nil {
		err = ErrBootstrapClosed
	}
	s.mu.Lock()
	if s.fail == nil {
		s.fail = err
		s.signalLocked()
	}
	s.mu.Unlock()
}

// RetransmitDue returns the oldest due item. Bootstrap data and FIN use a hard
// 2s ceiling and keep immutable sequence/payload/control state.
func (s *Sender) RetransmitDue(now time.Time) *Pending {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.fail != nil || len(s.pending) == 0 {
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

func waitSenderNotify(ch <-chan struct{}, deadline time.Time) error {
	if deadline.IsZero() {
		<-ch
		return nil
	}
	d := time.Until(deadline)
	if d <= 0 {
		return ErrBootstrapTimeout
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ch:
		return nil
	case <-timer.C:
		return ErrBootstrapTimeout
	}
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
