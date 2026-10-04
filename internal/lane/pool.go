package lane

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/lly8666/wobuzhidao/internal/protocol"
)

var (
	ErrPoolClosed      = errors.New("WBD lane pool closed")
	ErrDuplicateLaneID = errors.New("duplicate WBD lane id")
	ErrLaneUnavailable = errors.New("WBD lane unavailable")
	ErrNoActiveLanes   = errors.New("WBD lane pool has no active lanes")
)

type laneSlot struct {
	conn   *TCP
	active bool
}

// Event is one received frame or one terminal lane error. LaneID is local
// carrier context and is intentionally not encoded into DATA/DATAGRAM bodies.
type Event struct {
	LaneID protocol.LaneID
	Frame  any
	Err    error
}

// Pool joins independent real TCP carriers into one local fan-in/fan-out.
// M4 intentionally provides only deterministic lane selection; it has no
// logical ACK recovery, reinjection, FEC or adaptive policy.
type Pool struct {
	mu        sync.Mutex
	lanes     map[protocol.LaneID]*laneSlot
	order     []protocol.LaneID
	next      int
	events    chan Event
	closed    chan struct{}
	closeOnce sync.Once
	wg        sync.WaitGroup
}

func NewPool(eventBuffer int) *Pool {
	if eventBuffer <= 0 {
		eventBuffer = 64
	}
	return &Pool{
		lanes:  make(map[protocol.LaneID]*laneSlot),
		events: make(chan Event, eventBuffer),
		closed: make(chan struct{}),
	}
}

func (p *Pool) Add(id protocol.LaneID, conn *TCP) error {
	if conn == nil {
		return fmt.Errorf("%w: nil lane %d", ErrLaneUnavailable, id)
	}
	p.mu.Lock()
	select {
	case <-p.closed:
		p.mu.Unlock()
		return ErrPoolClosed
	default:
	}
	if _, exists := p.lanes[id]; exists {
		p.mu.Unlock()
		return fmt.Errorf("%w: %d", ErrDuplicateLaneID, id)
	}
	p.lanes[id] = &laneSlot{conn: conn, active: true}
	p.order = append(p.order, id)
	sort.Slice(p.order, func(i, j int) bool { return p.order[i] < p.order[j] })
	if p.next >= len(p.order) {
		p.next = 0
	}
	p.wg.Add(1)
	p.mu.Unlock()

	go p.receiveLoop(id, conn)
	return nil
}

func (p *Pool) SendOn(id protocol.LaneID, frame any) error {
	p.mu.Lock()
	slot := p.lanes[id]
	if slot == nil || !slot.active {
		p.mu.Unlock()
		return fmt.Errorf("%w: %d", ErrLaneUnavailable, id)
	}
	conn := slot.conn
	p.mu.Unlock()
	if err := conn.Send(frame); err != nil {
		p.markDead(id)
		return err
	}
	return nil
}

// SendNext selects the next active lane in ascending LaneID round-robin order.
func (p *Pool) SendNext(frame any) (protocol.LaneID, error) {
	p.mu.Lock()
	select {
	case <-p.closed:
		p.mu.Unlock()
		return 0, ErrPoolClosed
	default:
	}
	if len(p.order) == 0 {
		p.mu.Unlock()
		return 0, ErrNoActiveLanes
	}
	var id protocol.LaneID
	var conn *TCP
	found := false
	for tried := 0; tried < len(p.order); tried++ {
		idx := p.next % len(p.order)
		candidate := p.order[idx]
		p.next = (idx + 1) % len(p.order)
		slot := p.lanes[candidate]
		if slot != nil && slot.active {
			id, conn, found = candidate, slot.conn, true
			break
		}
	}
	p.mu.Unlock()
	if !found {
		return 0, ErrNoActiveLanes
	}
	if err := conn.Send(frame); err != nil {
		p.markDead(id)
		return id, err
	}
	return id, nil
}

func (p *Pool) Events() <-chan Event { return p.events }

func (p *Pool) ActiveLaneIDs() []protocol.LaneID {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]protocol.LaneID, 0, len(p.order))
	for _, id := range p.order {
		if slot := p.lanes[id]; slot != nil && slot.active {
			out = append(out, id)
		}
	}
	return out
}

func (p *Pool) Close() error {
	var first error
	p.closeOnce.Do(func() {
		close(p.closed)
		p.mu.Lock()
		for _, slot := range p.lanes {
			slot.active = false
			if err := slot.conn.Close(); err != nil && first == nil {
				first = err
			}
		}
		p.mu.Unlock()
		p.wg.Wait()
		close(p.events)
	})
	return first
}

func (p *Pool) receiveLoop(id protocol.LaneID, conn *TCP) {
	defer p.wg.Done()
	for {
		frame, err := conn.Receive()
		if err != nil {
			p.markDead(id)
			p.emit(Event{LaneID: id, Err: err})
			return
		}
		if !p.emit(Event{LaneID: id, Frame: frame}) {
			return
		}
	}
}

func (p *Pool) markDead(id protocol.LaneID) {
	p.mu.Lock()
	if slot := p.lanes[id]; slot != nil {
		slot.active = false
	}
	p.mu.Unlock()
}

func (p *Pool) emit(ev Event) bool {
	// Prefer shutdown over publishing a late terminal event. The second select
	// is still required because Close may race after the first check.
	select {
	case <-p.closed:
		return false
	default:
	}
	select {
	case <-p.closed:
		return false
	case p.events <- ev:
		return true
	}
}
