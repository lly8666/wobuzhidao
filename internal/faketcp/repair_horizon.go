package faketcp

import "time"

// ensureSteadyStateRepairCapacity keeps the fixed 4096-entry bound as a
// shadow-repair debt ceiling, not as a first-arrival admission barrier.
//
// Entries retired here have already entered the normal steady-state send path.
// We give up only future TCP-like repair work: payload memory and retransmit
// indexing are released immediately, while peer ACK/SACK remains authoritative
// on the wire. Fresh data therefore does not wait behind optional shadow repair.
func (s *Sender) ensureSteadyStateRepairCapacity(required int) bool {
	if required <= 0 {
		return true
	}
	if required > MaxSteadyStateOutstandingDatagrams {
		return false
	}
	deficit := s.Pending() + required - MaxSteadyStateOutstandingDatagrams
	if deficit <= 0 {
		return true
	}
	for i := s.head; i < len(s.pending) && deficit > 0; i++ {
		p := s.pending[i]
		if p == nil || p.Bootstrap || p.Retired {
			continue
		}
		s.abandonShadowRepair(p)
		deficit--
	}
	s.advanceHead()
	return deficit == 0
}

func (s *Sender) abandonShadowRepair(p *Pending) {
	if p == nil || p.Bootstrap || p.Retired || s.bySeq[p.Seq] != p {
		return
	}
	delete(s.bySeq, p.Seq)
	s.releasePayload(p.Payload)
	p.Payload = nil
	p.Retired = true
	p.RepairNotBefore = time.Time{}
	if s.active > 0 {
		s.active--
	}
	if p.slot >= 0 && p.slot < len(s.pending) && s.pending[p.slot] == p {
		s.pending[p.slot] = nil
	}
}
