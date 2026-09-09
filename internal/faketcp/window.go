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
// Sender synchronization exactly as they do for Enqueue/AckSelective.
func (s *Sender) EnqueueSteadyState(payload []byte, now time.Time) (*Pending, error) {
	if s.Pending() >= MaxSteadyStateOutstandingDatagrams {
		return nil, ErrSteadyStateOutstandingFull
	}
	return s.Enqueue(payload, now), nil
}
