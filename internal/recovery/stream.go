package recovery

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/lly8666/wobuzhidao/internal/protocol"
)

var (
	ErrNotStream             = errors.New("WBD recovery requires STREAM control")
	ErrTrackedOverlap        = errors.New("overlapping WBD sender stream data")
	ErrUnknownGap            = errors.New("WBD gap is outside tracked outstanding data")
	ErrUnknownACK            = errors.New("WBD ACK is for an unknown sender flow")
	ErrNoAlternateLane       = errors.New("no alternate WBD lane available")
	ErrTransmissionExhausted = errors.New("WBD transmission id exhausted")
)

// LaneSender is intentionally narrower than lane.Pool so the recovery engine
// remains independent of concrete socket/pool implementation.
type LaneSender interface {
	ActiveLaneIDs() []protocol.LaneID
	SendOn(protocol.LaneID, any) error
}

// Reinjection describes one successful cross-lane transmission of existing
// logical data. Logical FlowID/Offset/Payload remain unchanged; only the
// transmission attempt and carrier change.
type Reinjection struct {
	LaneID protocol.LaneID
	Frame  protocol.DataFrame
}

type tracked struct {
	frame    protocol.DataFrame
	lastLane protocol.LaneID
}

func (t tracked) start() uint64 { return uint64(t.frame.Offset) }
func (t tracked) end() uint64   { return t.start() + uint64(len(t.frame.Payload)) }

// StreamSender is the M6 sender-side logical outstanding scoreboard. It stores
// immutable copies of source bytes until logically ACKed. M7 will add bounded
// flight/backpressure; M6 deliberately focuses only on correctness of ACK
// pruning and GAP-driven cross-lane reinjection.
type StreamSender struct {
	mu        sync.Mutex
	flows     map[protocol.FlowID][]tracked
	acked     map[protocol.FlowID][]protocol.Range
	finAck    map[protocol.FlowID]bool
	known     map[protocol.FlowID]bool
	nextTx    protocol.TransmissionID
	exhausted bool
}

func NewStreamSender(firstTransmissionID protocol.TransmissionID) *StreamSender {
	if firstTransmissionID == 0 {
		firstTransmissionID = 1
	}
	return &StreamSender{
		flows:  make(map[protocol.FlowID][]tracked),
		acked:  make(map[protocol.FlowID][]protocol.Range),
		finAck: make(map[protocol.FlowID]bool),
		known:  make(map[protocol.FlowID]bool),
		nextTx: firstTransmissionID,
	}
}

// Track records a source DATA frame after the caller has scheduled/sent it.
// Payload bytes are copied. Source stream ranges must not overlap; a later
// reinjection is not tracked as a new source record.
func (s *StreamSender) Track(frame protocol.DataFrame, lane protocol.LaneID) error {
	if uint64(frame.FlowID) > protocol.MaxValue || uint64(frame.Offset) > protocol.MaxValue || uint64(frame.TransmissionID) > protocol.MaxValue {
		return fmt.Errorf("%w: invalid DATA identity", protocol.ErrLimit)
	}
	if len(frame.Payload) > protocol.MaxPayload || uint64(len(frame.Payload)) > protocol.MaxValue-uint64(frame.Offset) {
		return fmt.Errorf("%w: invalid DATA extent", protocol.ErrLimit)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	copyFrame := frame
	copyFrame.Payload = append([]byte(nil), frame.Payload...)
	rec := tracked{frame: copyFrame, lastLane: lane}
	list := s.flows[frame.FlowID]
	for _, old := range list {
		if rangesOverlap(rec.start(), rec.end(), old.start(), old.end()) {
			return fmt.Errorf("%w: flow=%d [%d,%d) with [%d,%d)", ErrTrackedOverlap, frame.FlowID, rec.start(), rec.end(), old.start(), old.end())
		}
		// Zero-length FIN has no byte range; only one source FIN at one offset is
		// useful and conflicting final semantics belong to the sender caller.
		if len(frame.Payload) == 0 && frame.FIN && len(old.frame.Payload) == 0 && old.frame.FIN && rec.start() == old.start() {
			return fmt.Errorf("%w: duplicate FIN at %d", ErrTrackedOverlap, rec.start())
		}
	}
	list = append(list, rec)
	sort.Slice(list, func(i, j int) bool {
		if list[i].start() == list[j].start() {
			return list[i].end() < list[j].end()
		}
		return list[i].start() < list[j].start()
	})
	s.flows[frame.FlowID] = list
	s.known[frame.FlowID] = true
	if !s.exhausted && frame.TransmissionID >= s.nextTx {
		if frame.TransmissionID == protocol.TransmissionID(protocol.MaxValue) {
			s.exhausted = true
		} else {
			s.nextTx = frame.TransmissionID + 1
		}
	}
	return nil
}

// ApplyACK merges logical receipt ranges and prunes source records that no
// longer need recovery. FIN-bearing records are retained until FIN itself has
// been observed, even if their payload bytes are acknowledged.
func (s *StreamSender) ApplyACK(ack protocol.AckFrame) error {
	if ack.Kind != protocol.AckStream {
		return ErrNotStream
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.known[ack.FlowID] {
		return fmt.Errorf("%w: flow=%d", ErrUnknownACK, ack.FlowID)
	}

	merged, err := mergeRanges(append(append([]protocol.Range(nil), s.acked[ack.FlowID]...), ack.Ranges...))
	if err != nil {
		return err
	}
	s.acked[ack.FlowID] = merged
	if ack.FIN {
		s.finAck[ack.FlowID] = true
	}
	list := s.flows[ack.FlowID]
	kept := list[:0]
	for _, rec := range list {
		bytesDone := rec.start() == rec.end() || rangeCovered(merged, rec.start(), rec.end())
		finDone := !rec.frame.FIN || s.finAck[ack.FlowID]
		if bytesDone && finDone {
			continue
		}
		kept = append(kept, rec)
	}
	if len(kept) == 0 {
		delete(s.flows, ack.FlowID)
	} else {
		s.flows[ack.FlowID] = kept
	}
	return nil
}

// ReinjectGap sends every still-unacknowledged source byte intersecting gap on
// an active lane different from the most recent lane used for that source
// record. Each send invocation receives a fresh TransmissionID. A local send
// failure consumes that ID and the engine tries another eligible lane.
func (s *StreamSender) ReinjectGap(gap protocol.GapHintFrame, lanes LaneSender) ([]Reinjection, error) {
	if gap.Kind != protocol.AckStream {
		return nil, ErrNotStream
	}
	if gap.End <= gap.Start || gap.End > protocol.MaxValue {
		return nil, fmt.Errorf("%w: invalid gap [%d,%d)", protocol.ErrMalformed, gap.Start, gap.End)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	list := s.flows[gap.FlowID]
	if len(list) == 0 {
		if s.known[gap.FlowID] {
			// ACK and GAP frames may cross on different carriers. A stale gap for
			// an already-pruned flow is a normal no-op, not a session error.
			return nil, nil
		}
		return nil, fmt.Errorf("%w: flow=%d", ErrUnknownGap, gap.FlowID)
	}
	active := append([]protocol.LaneID(nil), lanes.ActiveLaneIDs()...)
	sort.Slice(active, func(i, j int) bool { return active[i] < active[j] })
	if len(active) == 0 {
		return nil, ErrNoAlternateLane
	}

	acked := s.acked[gap.FlowID]
	var out []Reinjection
	overlappedSource := false
	for i := range list {
		rec := &list[i]
		start := max64(gap.Start, rec.start())
		end := min64(gap.End, rec.end())
		if end <= start {
			continue
		}
		overlappedSource = true
		pieces := subtractRanges(start, end, acked)
		for _, piece := range pieces {
			frame := rec.frame
			frame.Offset = protocol.StreamOffset(piece.Start)
			lo := piece.Start - rec.start()
			hi := piece.End - rec.start()
			frame.Payload = append([]byte(nil), rec.frame.Payload[lo:hi]...)
			frame.FIN = rec.frame.FIN && piece.End == rec.end()

			lane, tx, err := s.sendAlternate(frame, rec.lastLane, active, lanes)
			if err != nil {
				return out, err
			}
			frame.TransmissionID = tx
			if piece.Start == rec.start() && piece.End == rec.end() {
				rec.lastLane = lane
			}
			out = append(out, Reinjection{LaneID: lane, Frame: frame})
		}
	}
	if !overlappedSource {
		return nil, fmt.Errorf("%w: flow=%d gap=[%d,%d)", ErrUnknownGap, gap.FlowID, gap.Start, gap.End)
	}
	// If every intersecting byte was already ACKed, the gap was stale.
	return out, nil
}

func (s *StreamSender) sendAlternate(frame protocol.DataFrame, last protocol.LaneID, active []protocol.LaneID, lanes LaneSender) (protocol.LaneID, protocol.TransmissionID, error) {
	attempted := false
	var lastErr error
	for _, lane := range active {
		if lane == last {
			continue
		}
		attempted = true
		tx, err := s.allocateTx()
		if err != nil {
			return 0, 0, err
		}
		candidate := frame
		candidate.TransmissionID = tx
		if err := lanes.SendOn(lane, candidate); err != nil {
			lastErr = err
			continue
		}
		return lane, tx, nil
	}
	if !attempted {
		return 0, 0, ErrNoAlternateLane
	}
	if lastErr != nil {
		return 0, 0, fmt.Errorf("%w: %v", ErrNoAlternateLane, lastErr)
	}
	return 0, 0, ErrNoAlternateLane
}

func (s *StreamSender) allocateTx() (protocol.TransmissionID, error) {
	if s.exhausted || uint64(s.nextTx) > protocol.MaxValue {
		return 0, ErrTransmissionExhausted
	}
	tx := s.nextTx
	if uint64(tx) == protocol.MaxValue {
		s.exhausted = true
	} else {
		s.nextTx++
	}
	return tx, nil
}

// Outstanding returns source-record and byte counts for tests/telemetry.
func (s *StreamSender) Outstanding(flow protocol.FlowID) (records int, bytes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, rec := range s.flows[flow] {
		records++
		bytes += len(rec.frame.Payload)
	}
	return records, bytes
}

func mergeRanges(in []protocol.Range) ([]protocol.Range, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := append([]protocol.Range(nil), in...)
	for _, r := range out {
		if r.End <= r.Start || r.End > protocol.MaxValue {
			return nil, fmt.Errorf("%w: invalid ACK range [%d,%d)", protocol.ErrMalformed, r.Start, r.End)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start == out[j].Start {
			return out[i].End < out[j].End
		}
		return out[i].Start < out[j].Start
	})
	merged := out[:0]
	for _, r := range out {
		if len(merged) == 0 || r.Start > merged[len(merged)-1].End {
			merged = append(merged, r)
			continue
		}
		if r.End > merged[len(merged)-1].End {
			merged[len(merged)-1].End = r.End
		}
	}
	return merged, nil
}

func rangeCovered(ranges []protocol.Range, start, end uint64) bool {
	if start == end {
		return true
	}
	for _, r := range ranges {
		if r.Start <= start && r.End >= end {
			return true
		}
		if r.Start > start {
			return false
		}
	}
	return false
}

func subtractRanges(start, end uint64, acked []protocol.Range) []protocol.Range {
	cursor := start
	var out []protocol.Range
	for _, r := range acked {
		if r.End <= cursor {
			continue
		}
		if r.Start >= end {
			break
		}
		if r.Start > cursor {
			out = append(out, protocol.Range{Start: cursor, End: min64(r.Start, end)})
		}
		if r.End > cursor {
			cursor = r.End
			if cursor >= end {
				break
			}
		}
	}
	if cursor < end {
		out = append(out, protocol.Range{Start: cursor, End: end})
	}
	return out
}

func rangesOverlap(aStart, aEnd, bStart, bEnd uint64) bool {
	if aStart == aEnd || bStart == bEnd {
		return false
	}
	return aStart < bEnd && bStart < aEnd
}

func max64(a, b uint64) uint64 {
	if a > b {
		return a
	}
	return b
}
func min64(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}

// EqualLogicalData is kept tiny and exported for recovery tests/telemetry: it
// intentionally ignores TransmissionID.
func EqualLogicalData(a, b protocol.DataFrame) bool {
	return a.FlowID == b.FlowID && a.Offset == b.Offset && a.FIN == b.FIN && bytes.Equal(a.Payload, b.Payload)
}
