package faketcp

import (
	"errors"
	"time"
)

const (
	// MaxSteadyStateOutstandingDatagrams is the hard per-association bound for
	// DTLS ciphertext datagrams admitted into FakeTCP steady state. Product DTLS
	// emits one application record per LINK datagram, so keeping this bound equal
	// to the pinned wolfSSL replay window prevents a live association from
	// creating a record-reordering span larger than the receiver can authenticate.
	MaxSteadyStateOutstandingDatagrams = 4096

	// PinnedDTLSReplayWindowWords is compiled into the pinned wolfSSL library and
	// wbd_dtls_shim. wolfSSL word32 replay windows have 32 bits per word.
	PinnedDTLSReplayWindowWords   = 128
	PinnedDTLSReplayWindowRecords = PinnedDTLSReplayWindowWords * 32
)

var ErrSteadyStateOutstandingFull = errors.New("faketcp: steady-state outstanding datagram window full")

// EnqueueSteadyState admits one post-bootstrap datagram without allowing the
// sender's retained/retransmittable set to grow without bound. Callers own the
// Sender synchronization exactly as they do for Enqueue/AckSelective. A bounded
// legacy sender also enables pressure-safe RTO sweeping: once a real timer epoch
// expires, the normal paced retransmit loop can service all packets that were
// already expired in that epoch instead of letting the cumulative head monopolize
// recovery indefinitely.
func (s *Sender) EnqueueSteadyState(payload []byte, now time.Time) (*Pending, error) {
	if s.Pending() >= MaxSteadyStateOutstandingDatagrams {
		return nil, ErrSteadyStateOutstandingFull
	}
	s.steadyStateRTOSweep = true
	return s.Enqueue(payload, now), nil
}
