package session

import (
	"fmt"
	"sort"

	"github.com/lly8666/wobuzhidao/internal/protocol"
)

var ErrUnknownFlow = fmt.Errorf("unknown WBD flow")

// Receipt is a current logical receive snapshot. ACKs may contain multiple
// frames when the normalized range set exceeds protocol.MaxAckRanges. Gap is
// only a hint about the first currently observable hole; it is not a command to
// retransmit and M5 contains no sender recovery policy.
type Receipt struct {
	ACKs []protocol.AckFrame
	Gap  *protocol.GapHintFrame
}

// ReceiptFor returns logical receipt state independent of the lane on which
// the data arrived. The returned frames can be sent on any healthy carrier.
func (r *Receiver) ReceiptFor(flow protocol.FlowID) (Receipt, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	kind, ok := r.kinds[flow]
	if !ok {
		return Receipt{}, fmt.Errorf("%w: %d", ErrUnknownFlow, flow)
	}
	switch kind {
	case flowStream:
		return r.streamReceipt(flow), nil
	case flowDatagram:
		return r.datagramReceipt(flow), nil
	default:
		return Receipt{}, fmt.Errorf("%w: invalid type for flow %d", ErrUnknownFlow, flow)
	}
}

func (r *Receiver) streamReceipt(flow protocol.FlowID) Receipt {
	st := r.streams[flow]
	if st == nil {
		return Receipt{}
	}
	ranges := make([]protocol.Range, 0, len(st.segments)+1)
	if st.next > 0 {
		ranges = append(ranges, protocol.Range{Start: 0, End: st.next})
	}
	for _, seg := range st.segments {
		ranges = append(ranges, protocol.Range{Start: seg.start, End: seg.end()})
	}
	fin := st.final != nil
	out := Receipt{ACKs: splitACKs(flow, protocol.AckStream, fin, ranges)}

	if len(st.segments) > 0 && st.segments[0].start > st.next {
		out.Gap = &protocol.GapHintFrame{FlowID: flow, Kind: protocol.AckStream, Start: st.next, End: st.segments[0].start}
	} else if st.final != nil && st.next < *st.final {
		// A FIN can reveal a hole even if no later payload bytes are buffered.
		out.Gap = &protocol.GapHintFrame{FlowID: flow, Kind: protocol.AckStream, Start: st.next, End: *st.final}
	}
	return out
}

func (r *Receiver) datagramReceipt(flow protocol.FlowID) Receipt {
	st := r.datagrams[flow]
	if st == nil {
		return Receipt{}
	}
	pruneSeen(st, r.clock.Now())
	ids := make([]uint64, 0, len(st.seen))
	for id := range st.seen {
		ids = append(ids, uint64(id))
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	if len(ids) == 0 {
		return Receipt{}
	}

	ranges := make([]protocol.Range, 0, len(ids))
	start, end := ids[0], ids[0]+1
	for _, id := range ids[1:] {
		if id == end {
			end++
			continue
		}
		ranges = append(ranges, protocol.Range{Start: start, End: end})
		start, end = id, id+1
	}
	ranges = append(ranges, protocol.Range{Start: start, End: end})

	out := Receipt{ACKs: splitACKs(flow, protocol.AckDatagram, false, ranges)}
	// Datagram IDs are not assumed to start at zero here. Only an internal hole
	// between two live delivered IDs is observable without a FLOW_OPEN sequence
	// contract. That is enough for later deadline-aware hedging while avoiding a
	// false gap before the first seen ID.
	for i := 1; i < len(ids); i++ {
		if ids[i] > ids[i-1]+1 {
			out.Gap = &protocol.GapHintFrame{FlowID: flow, Kind: protocol.AckDatagram, Start: ids[i-1] + 1, End: ids[i]}
			break
		}
	}
	return out
}

func splitACKs(flow protocol.FlowID, kind protocol.AckKind, fin bool, ranges []protocol.Range) []protocol.AckFrame {
	if len(ranges) == 0 {
		if fin {
			return []protocol.AckFrame{{FlowID: flow, Kind: kind, FIN: true}}
		}
		return nil
	}
	out := make([]protocol.AckFrame, 0, (len(ranges)+protocol.MaxAckRanges-1)/protocol.MaxAckRanges)
	for start := 0; start < len(ranges); start += protocol.MaxAckRanges {
		end := start + protocol.MaxAckRanges
		if end > len(ranges) {
			end = len(ranges)
		}
		chunk := append([]protocol.Range(nil), ranges[start:end]...)
		out = append(out, protocol.AckFrame{FlowID: flow, Kind: kind, FIN: fin, Ranges: chunk})
	}
	return out
}
