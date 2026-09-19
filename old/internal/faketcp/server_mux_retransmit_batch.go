package faketcp

import "time"

// SteadyStateRetransmitBatch bounds the number of timer-expired steady-state
// repairs emitted per scheduler tick. It raises recovery capacity above one
// packet per 2ms tick without allowing an unbounded retransmission burst.
const SteadyStateRetransmitBatch = 4

// RetransmitDueBatch is the server-association form of Sender.RetransmitDueBatch.
// Association state and ACK processing share this lock with the sweep snapshot.
func (a *ServerAssociation) RetransmitDueBatch(now time.Time, max int) []*Pending {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationEstablished {
		return nil
	}
	return a.sender.RetransmitDueBatch(now, max)
}
