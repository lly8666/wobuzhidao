package faketcp

import (
	"errors"
	"time"
)

const (
	// MaxSteadyStateOutstandingDatagrams is the hard per-association bound for
	// retransmittable DTLS ciphertext state retained by FakeTCP steady state.
	// Fresh first-arrival traffic is allowed to displace old optional shadow
	// repair debt instead of waiting behind it.
	MaxSteadyStateOutstandingDatagrams = 4096

	// PinnedDTLSReplayWindowWords is compiled into the pinned wolfSSL library and
	// wbd_dtls_shim. wolfSSL word32 replay windows have 32 bits per word.
	PinnedDTLSReplayWindowWords   = 128
	PinnedDTLSReplayWindowRecords = PinnedDTLSReplayWindowWords * 32
)

var ErrSteadyStateOutstandingFull = errors.New("faketcp: steady-state outstanding datagram window full")

// EnqueueSteadyState admits one post-bootstrap datagram. The fixed 4096 bound
// caps retained TCP-like repair state rather than fresh-data latency: when the
// repair horizon is full, the oldest non-bootstrap repair candidate gives up
// its future retransmission state so this new first-arrival can proceed.
// Bootstrap remains separately ACK-gated stop-and-wait and is never evicted.
func (s *Sender) EnqueueSteadyState(payload []byte, now time.Time) (*Pending, error) {
	if !s.ensureSteadyStateRepairCapacity(1) {
		s.stats.FreshBlockedByRepair++
		return nil, ErrSteadyStateOutstandingFull
	}
	s.steadyStateRTOSweep = true
	p := s.Enqueue(payload, now)
	s.stats.FreshAdmitted++
	s.stats.FreshAdmittedBytes += uint64(len(payload))
	return p, nil
}
