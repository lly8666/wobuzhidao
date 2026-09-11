package faketcp

import "time"

const (
	// SteadyStateControlAdmissionMaxDatagramBytes remains part of the regression
	// surface for the older control-admission heuristic. Product admission no
	// longer classifies encrypted traffic by size: fresh data may retire old
	// optional repair debt regardless of datagram size.
	SteadyStateControlAdmissionMaxDatagramBytes = 512
)

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
// datagram or none of them. The 4096 limit is a bound on retained shadow-repair
// state, not a reason to shed a newly generated real datagram. Capacity is made
// atomically by retiring the oldest optional repair candidates before any new
// sequence number is allocated.
func (s *Sender) EnqueueSteadyStateBatch(payloads [][]byte, now time.Time) ([]*Pending, error) {
	if len(payloads) == 0 {
		return nil, nil
	}
	if !s.ensureSteadyStateRepairCapacity(len(payloads)) {
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
