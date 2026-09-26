package recovery

import (
	"errors"

	"github.com/lly8666/wobuzhidao/internal/protocol"
)

// ReinjectGapCrossLaneSafe preserves ReinjectGap's strict unknown-gap behavior
// while tolerating the one race that independent carriers legitimately create:
// an ACK can prune a logical subrange before a GAP for that same subrange
// arrives on another lane. Only a gap wholly covered by accumulated logical
// ACK ranges is converted to a stale no-op; every other error is preserved.
func (s *StreamSender) ReinjectGapCrossLaneSafe(gap protocol.GapHintFrame, lanes LaneSender) ([]Reinjection, error) {
	out, err := s.ReinjectGap(gap, lanes)
	if !errors.Is(err, ErrUnknownGap) {
		return out, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.known[gap.FlowID] && rangeCovered(s.acked[gap.FlowID], gap.Start, gap.End) {
		return nil, nil
	}
	return out, err
}
