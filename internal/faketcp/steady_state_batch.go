package faketcp

import "time"

const (
	// SteadyStateControlAdmissionMaxDatagramBytes bounds the small-datagram
	// class that is allowed to wait for capacity while a steady-state sender is
	// at its hard outstanding limit. FakeTCP cannot inspect encrypted LINK/DTLS
	// semantics here, so admission uses only the already-bounded carrier batch
	// size. Larger bulk datagrams are shed before sequence allocation, allowing
	// the ingress loop to keep reading and preserving liveness for later small
	// control-plane datagrams without growing pending state.
	SteadyStateControlAdmissionMaxDatagramBytes = 512
)

// steadyStateBatchBytes returns the bounded byte size of one original UDP
// datagram after carrier fragmentation. Callers only use it before enqueue, so
// a shed batch cannot leave sequence or retransmit bookkeeping behind.
func steadyStateBatchBytes(payloads [][]byte) int {
	total := 0
	for _, payload := range payloads {
		total += len(payload)
		if total > SteadyStateControlAdmissionMaxDatagramBytes {
			return total
		}
	}
	return total
}

// EnqueueSteadyStateBatch admits all carrier fragments for one original UDP
// datagram or none of them. This keeps the existing fixed 4096 pending-frame
// bound while preventing a locally generated partial carrier datagram when the
// association is already under pressure.
func (s *Sender) EnqueueSteadyStateBatch(payloads [][]byte, now time.Time) ([]*Pending, error) {
	if len(payloads) == 0 {
		return nil, nil
	}
	if s.Pending()+len(payloads) > MaxSteadyStateOutstandingDatagrams {
		// Do not let a bulk datagram become an ingress head-of-line blocker when
		// the outstanding window is full. Returning a successful empty batch is
		// an explicit bounded shed: no sequence number has been allocated and no
		// Pending entry exists to become a zombie. Small datagrams retain the
		// existing backpressure signal so they are admitted as soon as ACKs free
		// capacity.
		if steadyStateBatchBytes(payloads) > SteadyStateControlAdmissionMaxDatagramBytes {
			return nil, nil
		}
		return nil, ErrSteadyStateOutstandingFull
	}
	out := make([]*Pending, 0, len(payloads))
	for _, payload := range payloads {
		p, err := s.EnqueueSteadyState(payload, now)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// EnqueueSteadyStateBatch is the server-association form of the same atomic
// admission contract. ACK processing and the full batch share one association
// lock, so capacity cannot change between the preflight and fragment enqueues.
func (a *ServerAssociation) EnqueueSteadyStateBatch(payloads [][]byte, now time.Time) ([]*Pending, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationEstablished {
		return nil, ErrHandshakeState
	}
	return a.sender.EnqueueSteadyStateBatch(payloads, now)
}
