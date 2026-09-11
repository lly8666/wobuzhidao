package faketcp

import "time"

// EnqueueSteadyStateBatch admits all carrier fragments for one original UDP
// datagram or none of them. This keeps the existing fixed 4096 pending-frame
// bound while preventing a locally generated partial carrier datagram when the
// association is already under pressure.
func (s *Sender) EnqueueSteadyStateBatch(payloads [][]byte, now time.Time) ([]*Pending, error) {
	if len(payloads) == 0 {
		return nil, nil
	}
	if s.Pending()+len(payloads) > MaxSteadyStateOutstandingDatagrams {
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
