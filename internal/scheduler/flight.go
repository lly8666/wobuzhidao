package scheduler

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/lly8666/wobuzhidao/internal/lane"
	"github.com/lly8666/wobuzhidao/internal/protocol"
)

var (
	ErrInvalidConfig   = errors.New("invalid WBD flight scheduler config")
	ErrNoBulkCredit    = errors.New("no WBD bulk lane credit available")
	ErrNoRescueCredit  = errors.New("no WBD rescue lane credit available")
	ErrNotStreamACK    = errors.New("WBD flight accounting requires STREAM ACK")
	ErrUnsupportedData = errors.New("unsupported WBD scheduled DATA frame")
)

// Config deliberately separates bulk and rescue capacity. Limits are logical
// bytes admitted to each carrier before logical ACK, not kernel TCP send-buffer
// occupancy. Platform socket telemetry/limits may tighten that approximation
// later without changing scheduler semantics.
type Config struct {
	BulkLaneIDs          []protocol.LaneID
	RescueLaneID         protocol.LaneID
	MaxBulkFlightBytes   int
	MaxRescueFlightBytes int
}

type flightRecord struct {
	token        uint64
	flow         protocol.FlowID
	remaining    []protocol.Range
	fin          bool
	cost         int  // payload bytes; zero-byte FIN costs one unit until FIN ACK
	writePending bool // reserved before Write; ACK cannot release this cost yet
}

type CarrierPool interface {
	ActiveLaneIDs() []protocol.LaneID
	SendOn(protocol.LaneID, any) error
}

type Scheduler struct {
	pool CarrierPool
	cfg  Config

	mu        sync.Mutex
	rr        int
	nextToken uint64
	flight    map[protocol.LaneID][]flightRecord
	bytes     map[protocol.LaneID]int
}

func New(pool CarrierPool, cfg Config) (*Scheduler, error) {
	if pool == nil || len(cfg.BulkLaneIDs) == 0 || cfg.MaxBulkFlightBytes <= 0 || cfg.MaxRescueFlightBytes <= 0 {
		return nil, ErrInvalidConfig
	}
	bulk := append([]protocol.LaneID(nil), cfg.BulkLaneIDs...)
	sort.Slice(bulk, func(i, j int) bool { return bulk[i] < bulk[j] })
	seen := make(map[protocol.LaneID]bool, len(bulk)+1)
	for _, id := range bulk {
		if seen[id] || id == cfg.RescueLaneID {
			return nil, fmt.Errorf("%w: duplicate/overlapping lane %d", ErrInvalidConfig, id)
		}
		seen[id] = true
	}
	if seen[cfg.RescueLaneID] {
		return nil, fmt.Errorf("%w: rescue lane overlaps bulk", ErrInvalidConfig)
	}
	cfg.BulkLaneIDs = bulk
	return &Scheduler{
		pool:      pool,
		cfg:       cfg,
		nextToken: 1,
		flight:    make(map[protocol.LaneID][]flightRecord),
		bytes:     make(map[protocol.LaneID]int),
	}, nil
}

// SendBulk picks the next active bulk lane with enough logical credit. Credit
// is reserved before the socket write and rolled back if the write fails, so
// concurrent writers cannot over-admit a carrier.
func (s *Scheduler) SendBulk(frame protocol.DataFrame) (protocol.LaneID, error) {
	if err := validateData(frame); err != nil {
		return 0, err
	}
	active := activeSet(s.pool.ActiveLaneIDs())

	s.mu.Lock()
	var id protocol.LaneID
	var token uint64
	found := false
	for tried := 0; tried < len(s.cfg.BulkLaneIDs); tried++ {
		idx := s.rr % len(s.cfg.BulkLaneIDs)
		candidate := s.cfg.BulkLaneIDs[idx]
		s.rr = (idx + 1) % len(s.cfg.BulkLaneIDs)
		if !active[candidate] || s.bytes[candidate]+frameCost(frame) > s.cfg.MaxBulkFlightBytes {
			continue
		}
		id = candidate
		token = s.reserveLocked(candidate, frame)
		found = true
		break
	}
	s.mu.Unlock()
	if !found {
		return 0, ErrNoBulkCredit
	}
	if err := s.pool.SendOn(id, frame); err != nil {
		s.rollback(id, token)
		return id, err
	}
	s.commitSend(id, token)
	return id, nil
}

// SendControl uses the dedicated rescue carrier without charging DATA flight.
// Control frames are intentionally tiny; later M8/RBC work may add an explicit
// control-rate budget if benchmarks show it is necessary.
func (s *Scheduler) SendControl(frame any) error {
	if !activeSet(s.pool.ActiveLaneIDs())[s.cfg.RescueLaneID] {
		return lane.ErrLaneUnavailable
	}
	return s.pool.SendOn(s.cfg.RescueLaneID, frame)
}

// RescueSender returns a narrow view that satisfies recovery.LaneSender while
// exposing only the dedicated rescue lane. M6 therefore keeps its logical
// reinjection policy and cannot accidentally choose a bulk carrier here.
func (s *Scheduler) RescueSender() *RescueSender { return &RescueSender{s: s} }

type RescueSender struct{ s *Scheduler }

func (r *RescueSender) ActiveLaneIDs() []protocol.LaneID {
	if r == nil || r.s == nil {
		return nil
	}
	for _, id := range r.s.pool.ActiveLaneIDs() {
		if id == r.s.cfg.RescueLaneID {
			return []protocol.LaneID{id}
		}
	}
	return nil
}

func (r *RescueSender) SendOn(id protocol.LaneID, frame any) error {
	if r == nil || r.s == nil || id != r.s.cfg.RescueLaneID {
		return lane.ErrLaneUnavailable
	}
	data, ok := frame.(protocol.DataFrame)
	if !ok {
		return ErrUnsupportedData
	}
	if err := validateData(data); err != nil {
		return err
	}

	r.s.mu.Lock()
	if r.s.bytes[id]+frameCost(data) > r.s.cfg.MaxRescueFlightBytes {
		r.s.mu.Unlock()
		return ErrNoRescueCredit
	}
	token := r.s.reserveLocked(id, data)
	r.s.mu.Unlock()

	if err := r.s.pool.SendOn(id, data); err != nil {
		r.s.rollback(id, token)
		return err
	}
	r.s.commitSend(id, token)
	return nil
}

// ApplyACK releases acknowledged logical bytes from every copy in flight. A
// single logical ACK therefore frees both original bulk and rescue duplicate
// accounting. FIN acknowledgement removes zero-byte FIN state as well.
func (s *Scheduler) ApplyACK(ack protocol.AckFrame) error {
	if ack.Kind != protocol.AckStream {
		return ErrNotStreamACK
	}
	merged, err := mergeRanges(ack.Ranges)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, records := range s.flight {
		kept := records[:0]
		newBytes := 0
		for _, rec := range records {
			if rec.flow != ack.FlowID {
				kept = append(kept, rec)
				newBytes += rec.cost
				continue
			}
			rec.remaining = subtract(rec.remaining, merged)
			if ack.FIN {
				rec.fin = false
			}
			if rec.writePending {
				// A logically equivalent copy may be ACKed while this socket Write is
				// still blocked. Keep the original reservation until Write returns so
				// concurrent producers cannot over-admit the lane.
				kept = append(kept, rec)
				newBytes += rec.cost
				continue
			}
			if len(rec.remaining) == 0 && !rec.fin {
				continue
			}
			rec.cost = logicalCost(rec.remaining, rec.fin)
			kept = append(kept, rec)
			newBytes += rec.cost
		}
		if len(kept) == 0 {
			delete(s.flight, id)
			delete(s.bytes, id)
		} else {
			s.flight[id] = kept
			s.bytes[id] = newBytes
		}
	}
	return nil
}

func (s *Scheduler) FlightBytes(id protocol.LaneID) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bytes[id]
}

func (s *Scheduler) BulkLaneIDs() []protocol.LaneID {
	return append([]protocol.LaneID(nil), s.cfg.BulkLaneIDs...)
}

func (s *Scheduler) RescueLaneID() protocol.LaneID { return s.cfg.RescueLaneID }

// DropInactive releases accounting records for carriers that Pool has already
// declared inactive. Recovery source bytes remain owned by recovery.StreamSender,
// so dropping scheduler-only accounting cannot lose recoverability.
func (s *Scheduler) DropInactive() int {
	active := activeSet(s.pool.ActiveLaneIDs())
	s.mu.Lock()
	defer s.mu.Unlock()
	released := 0
	for id := range s.flight {
		if active[id] {
			continue
		}
		released += s.bytes[id]
		delete(s.flight, id)
		delete(s.bytes, id)
	}
	return released
}

func (s *Scheduler) reserveLocked(id protocol.LaneID, frame protocol.DataFrame) uint64 {
	token := s.nextToken
	s.nextToken++
	rec := flightRecord{token: token, flow: frame.FlowID, fin: frame.FIN, cost: frameCost(frame), writePending: true}
	if len(frame.Payload) > 0 {
		start := uint64(frame.Offset)
		rec.remaining = []protocol.Range{{Start: start, End: start + uint64(len(frame.Payload))}}
	}
	s.flight[id] = append(s.flight[id], rec)
	s.bytes[id] += rec.cost
	return token
}

func (s *Scheduler) commitSend(id protocol.LaneID, token uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := s.flight[id]
	kept := records[:0]
	bytes := 0
	for _, rec := range records {
		if rec.token == token {
			rec.writePending = false
			if len(rec.remaining) == 0 && !rec.fin {
				continue
			}
			rec.cost = logicalCost(rec.remaining, rec.fin)
		}
		kept = append(kept, rec)
		bytes += rec.cost
	}
	if len(kept) == 0 {
		delete(s.flight, id)
		delete(s.bytes, id)
	} else {
		s.flight[id] = kept
		s.bytes[id] = bytes
	}
}

func (s *Scheduler) rollback(id protocol.LaneID, token uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := s.flight[id]
	kept := records[:0]
	bytes := 0
	for _, rec := range records {
		if rec.token == token {
			continue
		}
		kept = append(kept, rec)
		bytes += rec.cost
	}
	if len(kept) == 0 {
		delete(s.flight, id)
		delete(s.bytes, id)
	} else {
		s.flight[id] = kept
		s.bytes[id] = bytes
	}
}

func validateData(frame protocol.DataFrame) error {
	start := uint64(frame.Offset)
	if uint64(frame.FlowID) > protocol.MaxValue || start > protocol.MaxValue || uint64(frame.TransmissionID) > protocol.MaxValue || len(frame.Payload) > protocol.MaxPayload || uint64(len(frame.Payload)) > protocol.MaxValue-start {
		return ErrUnsupportedData
	}
	return nil
}

func logicalCost(ranges []protocol.Range, fin bool) int {
	cost := rangesBytes(ranges)
	if cost == 0 && fin {
		return 1
	}
	return cost
}

func frameCost(frame protocol.DataFrame) int {
	if len(frame.Payload) > 0 {
		return len(frame.Payload)
	}
	if frame.FIN {
		return 1
	}
	return 0
}

func activeSet(ids []protocol.LaneID) map[protocol.LaneID]bool {
	out := make(map[protocol.LaneID]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func mergeRanges(in []protocol.Range) ([]protocol.Range, error) {
	if len(in) == 0 {
		return nil, nil
	}
	out := append([]protocol.Range(nil), in...)
	for _, r := range out {
		if r.End <= r.Start || r.End > protocol.MaxValue {
			return nil, protocol.ErrMalformed
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

func subtract(in []protocol.Range, acked []protocol.Range) []protocol.Range {
	var out []protocol.Range
	for _, src := range in {
		cursor := src.Start
		for _, a := range acked {
			if a.End <= cursor {
				continue
			}
			if a.Start >= src.End {
				break
			}
			if a.Start > cursor {
				end := a.Start
				if end > src.End {
					end = src.End
				}
				if end > cursor {
					out = append(out, protocol.Range{Start: cursor, End: end})
				}
			}
			if a.End > cursor {
				cursor = a.End
				if cursor >= src.End {
					break
				}
			}
		}
		if cursor < src.End {
			out = append(out, protocol.Range{Start: cursor, End: src.End})
		}
	}
	return out
}

func rangesBytes(ranges []protocol.Range) int {
	total := 0
	for _, r := range ranges {
		total += int(r.End - r.Start)
	}
	return total
}
