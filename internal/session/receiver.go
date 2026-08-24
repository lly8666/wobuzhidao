package session

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/protocol"
)

const DefaultMaxStreamBuffered = 8 << 20 // 8 MiB of unique out-of-order bytes per flow.

var (
	ErrFlowTypeMismatch = errors.New("WBD flow type mismatch")
	ErrStreamConflict   = errors.New("conflicting WBD stream bytes")
	ErrFinalOffset      = errors.New("inconsistent WBD stream final offset")
	ErrReorderLimit     = errors.New("WBD stream reorder buffer limit exceeded")
	ErrDatagramConflict = errors.New("conflicting WBD datagram duplicate")
	ErrDeadlineConflict = errors.New("inconsistent WBD datagram deadline")
	ErrInvalidFrame     = errors.New("invalid WBD logical frame")
)

type flowKind uint8

const (
	flowStream flowKind = 1 + iota
	flowDatagram
)

// Receiver owns in-memory logical receive state. It deliberately has no lane,
// socket, FEC, Xray or TUN dependency.
type Receiver struct {
	mu                sync.Mutex
	clock             Clock
	maxStreamBuffered int
	kinds             map[protocol.FlowID]flowKind
	streams           map[protocol.FlowID]*streamState
	datagrams         map[protocol.FlowID]*datagramState
}

func NewReceiver(clock Clock, maxStreamBuffered int) *Receiver {
	if clock == nil {
		clock = SystemClock{}
	}
	if maxStreamBuffered <= 0 {
		maxStreamBuffered = DefaultMaxStreamBuffered
	}
	return &Receiver{
		clock:             clock,
		maxStreamBuffered: maxStreamBuffered,
		kinds:             make(map[protocol.FlowID]flowKind),
		streams:           make(map[protocol.FlowID]*streamState),
		datagrams:         make(map[protocol.FlowID]*datagramState),
	}
}

type StreamDelivery struct {
	FlowID         protocol.FlowID
	Data           []byte
	NextOffset     protocol.StreamOffset
	Complete       bool
	Duplicate      bool
	NewUniqueBytes int
	BufferedBytes  int
}

type DatagramDelivery struct {
	FlowID     protocol.FlowID
	DatagramID protocol.DatagramID
	Payload    []byte
	Delivered  bool
	Duplicate  bool
	Expired    bool
}

type segment struct {
	start uint64
	data  []byte
}

func (s segment) end() uint64 { return s.start + uint64(len(s.data)) }

type streamState struct {
	next     uint64
	final    *uint64
	segments []segment // sorted, non-overlapping, non-adjacent
	buffered int
}

type datagramSeen struct {
	hash      [32]byte
	expiresAt time.Time
}

type datagramState struct {
	seen map[protocol.DatagramID]datagramSeen
}

// AcceptData applies one logical DATA transmission. TransmissionID is not part
// of stream identity: reinjected copies with a new TransmissionID are deduped
// by FlowID/Offset/content.
func (r *Receiver) AcceptData(f protocol.DataFrame) (StreamDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := validateDataFrame(f); err != nil {
		return StreamDelivery{}, err
	}
	if got, ok := r.kinds[f.FlowID]; ok && got != flowStream {
		return StreamDelivery{}, fmt.Errorf("%w: flow=%d", ErrFlowTypeMismatch, f.FlowID)
	}

	base := r.streams[f.FlowID]
	if base == nil {
		base = &streamState{}
	}
	// Apply the frame to a clone and publish the state only after every
	// validation succeeds. A rejected FIN/conflicting/reorder-limit frame must
	// not leave final-offset or buffered-byte side effects behind.
	st := cloneStreamState(base)

	start := uint64(f.Offset)
	if uint64(len(f.Payload)) > protocol.MaxValue-start {
		return StreamDelivery{}, fmt.Errorf("%w: stream extent overflows 62-bit space", ErrInvalidFrame)
	}
	end := start + uint64(len(f.Payload))

	if st.final != nil && end > *st.final {
		return StreamDelivery{}, fmt.Errorf("%w: data end=%d final=%d", ErrFinalOffset, end, *st.final)
	}

	finChanged := false
	if f.FIN {
		if st.final != nil && *st.final != end {
			return StreamDelivery{}, fmt.Errorf("%w: existing=%d new=%d", ErrFinalOffset, *st.final, end)
		}
		if st.final == nil {
			if st.next > end {
				return StreamDelivery{}, fmt.Errorf("%w: delivered=%d new=%d", ErrFinalOffset, st.next, end)
			}
			for _, seg := range st.segments {
				if seg.end() > end {
					return StreamDelivery{}, fmt.Errorf("%w: buffered end=%d new=%d", ErrFinalOffset, seg.end(), end)
				}
			}
			v := end
			st.final = &v
			finChanged = true
		}
	}

	payload := f.Payload
	if end <= st.next {
		r.kinds[f.FlowID] = flowStream
		r.streams[f.FlowID] = st
		return r.streamResult(f.FlowID, st, nil, !finChanged, 0), nil
	}
	if start < st.next {
		trim := st.next - start
		payload = payload[trim:]
		start = st.next
	}

	candidate, unique, err := insertSegment(st.segments, start, payload)
	if err != nil {
		return StreamDelivery{}, err
	}
	if st.buffered+unique > r.maxStreamBuffered {
		return StreamDelivery{}, fmt.Errorf("%w: have=%d add=%d max=%d", ErrReorderLimit, st.buffered, unique, r.maxStreamBuffered)
	}
	st.segments = candidate
	st.buffered += unique

	delivered := make([]byte, 0)
	for len(st.segments) > 0 && st.segments[0].start == st.next {
		seg := st.segments[0]
		delivered = append(delivered, seg.data...)
		st.next += uint64(len(seg.data))
		st.buffered -= len(seg.data)
		st.segments = st.segments[1:]
	}

	r.kinds[f.FlowID] = flowStream
	r.streams[f.FlowID] = st
	return r.streamResult(f.FlowID, st, delivered, unique == 0 && len(delivered) == 0 && !finChanged, unique), nil
}

func cloneStreamState(in *streamState) *streamState {
	out := &streamState{next: in.next, buffered: in.buffered}
	if in.final != nil {
		v := *in.final
		out.final = &v
	}
	out.segments = make([]segment, len(in.segments))
	for i, seg := range in.segments {
		out.segments[i] = segment{start: seg.start, data: append([]byte(nil), seg.data...)}
	}
	return out
}

func (r *Receiver) streamResult(flow protocol.FlowID, st *streamState, data []byte, duplicate bool, unique int) StreamDelivery {
	complete := st.final != nil && st.next == *st.final
	out := append([]byte(nil), data...)
	return StreamDelivery{
		FlowID:         flow,
		Data:           out,
		NextOffset:     protocol.StreamOffset(st.next),
		Complete:       complete,
		Duplicate:      duplicate,
		NewUniqueBytes: unique,
		BufferedBytes:  st.buffered,
	}
}

// AcceptDatagram applies one unreliable/expiring logical datagram. expiresAt
// is M2 receive metadata, not yet a wire field. A datagram arriving at or after
// its hard deadline is expired and is never delivered. TransmissionID is not
// part of dedup identity.
func (r *Receiver) AcceptDatagram(f protocol.DatagramFrame, expiresAt time.Time) (DatagramDelivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := validateDatagramFrame(f); err != nil {
		return DatagramDelivery{}, err
	}
	if expiresAt.IsZero() {
		return DatagramDelivery{}, fmt.Errorf("%w: zero datagram deadline", ErrInvalidFrame)
	}
	if err := r.ensureKind(f.FlowID, flowDatagram); err != nil {
		return DatagramDelivery{}, err
	}
	st := r.datagrams[f.FlowID]
	if st == nil {
		st = &datagramState{seen: make(map[protocol.DatagramID]datagramSeen)}
		r.datagrams[f.FlowID] = st
	}

	now := r.clock.Now()
	pruneSeen(st, now)
	if !now.Before(expiresAt) {
		return DatagramDelivery{FlowID: f.FlowID, DatagramID: f.DatagramID, Expired: true}, nil
	}

	hash := sha256.Sum256(f.Payload)
	if prev, ok := st.seen[f.DatagramID]; ok {
		if !prev.expiresAt.Equal(expiresAt) {
			return DatagramDelivery{}, fmt.Errorf("%w: datagram=%d", ErrDeadlineConflict, f.DatagramID)
		}
		if prev.hash != hash {
			return DatagramDelivery{}, fmt.Errorf("%w: datagram=%d", ErrDatagramConflict, f.DatagramID)
		}
		return DatagramDelivery{FlowID: f.FlowID, DatagramID: f.DatagramID, Duplicate: true}, nil
	}

	st.seen[f.DatagramID] = datagramSeen{hash: hash, expiresAt: expiresAt}
	payload := append([]byte(nil), f.Payload...)
	return DatagramDelivery{FlowID: f.FlowID, DatagramID: f.DatagramID, Payload: payload, Delivered: true}, nil
}

// PruneExpiredDatagrams bounds dedup state using the same deterministic clock.
// It returns the number of removed logical datagram identities.
func (r *Receiver) PruneExpiredDatagrams() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.clock.Now()
	removed := 0
	for flow, st := range r.datagrams {
		before := len(st.seen)
		pruneSeen(st, now)
		removed += before - len(st.seen)
		if len(st.seen) == 0 {
			delete(r.datagrams, flow)
		}
	}
	return removed
}

func (r *Receiver) ensureKind(flow protocol.FlowID, kind flowKind) error {
	if got, ok := r.kinds[flow]; ok && got != kind {
		return fmt.Errorf("%w: flow=%d", ErrFlowTypeMismatch, flow)
	}
	r.kinds[flow] = kind
	return nil
}

func validateDataFrame(f protocol.DataFrame) error {
	if uint64(f.FlowID) > protocol.MaxValue || uint64(f.Offset) > protocol.MaxValue || uint64(f.TransmissionID) > protocol.MaxValue {
		return fmt.Errorf("%w: DATA identity exceeds 62-bit limit", ErrInvalidFrame)
	}
	if len(f.Payload) > protocol.MaxPayload {
		return fmt.Errorf("%w: DATA payload=%d", ErrInvalidFrame, len(f.Payload))
	}
	return nil
}

func validateDatagramFrame(f protocol.DatagramFrame) error {
	if uint64(f.FlowID) > protocol.MaxValue || uint64(f.DatagramID) > protocol.MaxValue || uint64(f.TransmissionID) > protocol.MaxValue {
		return fmt.Errorf("%w: DATAGRAM identity exceeds 62-bit limit", ErrInvalidFrame)
	}
	if len(f.Payload) > protocol.MaxPayload {
		return fmt.Errorf("%w: DATAGRAM payload=%d", ErrInvalidFrame, len(f.Payload))
	}
	return nil
}

func pruneSeen(st *datagramState, now time.Time) {
	for id, item := range st.seen {
		if !now.Before(item.expiresAt) {
			delete(st.seen, id)
		}
	}
}

// insertSegment transactionally validates overlaps and returns normalized
// sorted segments plus the number of newly learned logical bytes.
func insertSegment(existing []segment, start uint64, data []byte) ([]segment, int, error) {
	if len(data) == 0 {
		return append([]segment(nil), existing...), 0, nil
	}
	end := start + uint64(len(data))
	cursor := start
	pieces := make([]segment, 0, 2)
	unique := 0

	for _, seg := range existing {
		if seg.end() <= cursor {
			continue
		}
		if seg.start >= end {
			break
		}
		if seg.start > cursor {
			pieceEnd := seg.start
			if pieceEnd > end {
				pieceEnd = end
			}
			lo := cursor - start
			hi := pieceEnd - start
			pieceData := append([]byte(nil), data[lo:hi]...)
			pieces = append(pieces, segment{start: cursor, data: pieceData})
			unique += len(pieceData)
			cursor = pieceEnd
		}
		if cursor >= end {
			break
		}
		overlapStart := cursor
		if overlapStart < seg.start {
			overlapStart = seg.start
		}
		overlapEnd := end
		if seg.end() < overlapEnd {
			overlapEnd = seg.end()
		}
		if overlapEnd > overlapStart {
			incoming := data[overlapStart-start : overlapEnd-start]
			known := seg.data[overlapStart-seg.start : overlapEnd-seg.start]
			if !bytes.Equal(incoming, known) {
				return nil, 0, fmt.Errorf("%w: overlap [%d,%d)", ErrStreamConflict, overlapStart, overlapEnd)
			}
			cursor = overlapEnd
		}
	}
	if cursor < end {
		lo := cursor - start
		pieceData := append([]byte(nil), data[lo:]...)
		pieces = append(pieces, segment{start: cursor, data: pieceData})
		unique += len(pieceData)
	}

	all := make([]segment, 0, len(existing)+len(pieces))
	for _, seg := range existing {
		all = append(all, segment{start: seg.start, data: append([]byte(nil), seg.data...)})
	}
	all = append(all, pieces...)
	sort.Slice(all, func(i, j int) bool { return all[i].start < all[j].start })
	if len(all) == 0 {
		return all, unique, nil
	}

	merged := make([]segment, 0, len(all))
	for _, seg := range all {
		if len(merged) == 0 {
			merged = append(merged, seg)
			continue
		}
		last := &merged[len(merged)-1]
		if last.end() == seg.start {
			last.data = append(last.data, seg.data...)
			continue
		}
		if last.end() > seg.start {
			return nil, 0, fmt.Errorf("%w: normalized overlap", ErrStreamConflict)
		}
		merged = append(merged, seg)
	}
	return merged, unique, nil
}
