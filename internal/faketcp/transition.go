package faketcp

import (
	"bytes"
	"errors"
	"sync"
)

const MaxTransitionRecords = 64

var (
	ErrTransitionState       = errors.New("faketcp: invalid transition state")
	ErrTransitionOverflow    = errors.New("faketcp: transition queue limit exceeded")
	ErrTransitionRecordSize  = errors.New("faketcp: transition record exceeds negotiated wire limit")
	ErrTransitionOverlap     = errors.New("faketcp: payload crosses bootstrap transition boundary")
	ErrTransitionConflict    = errors.New("faketcp: same-sequence transition retransmit changed payload")
	ErrTransitionAborted     = errors.New("faketcp: transition aborted")
)

type TransitionState uint8

const (
	TransitionBootstrap TransitionState = iota
	TransitionPrepared
	TransitionDetached
	TransitionAborted
)

type TransitionDisposition uint8

const (
	RouteBootstrap TransitionDisposition = iota
	RouteQueuedRecord
	RouteQueuedDuplicate
	RouteRecord
	RouteLateBootstrap
	RouteAckOnly
)

type TransitionPacket struct {
	Seq     uint32
	Payload []byte
}

// StageTransition owns the receive-side mode boundary between the temporary
// ordered TLS bootstrap stream and TLS-like record datagrams.
//
// Prepare records the exact bootstrap end sequence before the final TLS reply is
// complete. Payload at or beyond that boundary is then copied into a bounded
// queue instead of being fed into TLS. Detach transfers that queue to the record
// receiver. Pure ACKs never enter either payload path.
type StageTransition struct {
	mu sync.Mutex

	state      TransitionState
	boundary   uint32
	maxWire    int
	maxBytes   int
	queuedBytes int
	queue      []TransitionPacket
	bySeq      map[uint32]int
}

func NewStageTransition(maxRecordWire int) (*StageTransition, error) {
	maxInt := int(^uint(0) >> 1)
	if maxRecordWire <= 0 || maxRecordWire > maxInt/MaxTransitionRecords {
		return nil, ErrTransitionRecordSize
	}
	return &StageTransition{
		state:    TransitionBootstrap,
		maxWire:  maxRecordWire,
		maxBytes: maxRecordWire * MaxTransitionRecords,
	}, nil
}

func (t *StageTransition) State() TransitionState {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state
}

func (t *StageTransition) Prepare(bootstrapEnd uint32) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != TransitionBootstrap {
		return ErrTransitionState
	}
	t.boundary = bootstrapEnd
	t.state = TransitionPrepared
	t.bySeq = make(map[uint32]int)
	return nil
}

// Route classifies one FakeTCP payload using the explicit sequence boundary.
// The caller must still send/consume ordinary TCP ACK information separately.
func (t *StageTransition) Route(seq uint32, payload []byte) (TransitionDisposition, error) {
	if len(payload) == 0 {
		return RouteAckOnly, nil
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	switch t.state {
	case TransitionAborted:
		return 0, ErrTransitionAborted
	case TransitionBootstrap:
		return RouteBootstrap, nil
	}

	end := seq + uint32(len(payload))
	if seqLT(seq, t.boundary) {
		if seqLT(t.boundary, end) {
			t.abortLocked()
			return 0, ErrTransitionOverlap
		}
		if t.state == TransitionDetached {
			return RouteLateBootstrap, nil
		}
		return RouteBootstrap, nil
	}

	if len(payload) > t.maxWire {
		t.abortLocked()
		return 0, ErrTransitionRecordSize
	}
	if t.state == TransitionDetached {
		return RouteRecord, nil
	}

	if idx, ok := t.bySeq[seq]; ok {
		if bytes.Equal(t.queue[idx].Payload, payload) {
			return RouteQueuedDuplicate, nil
		}
		t.abortLocked()
		return 0, ErrTransitionConflict
	}
	if len(t.queue) >= MaxTransitionRecords || t.queuedBytes+len(payload) > t.maxBytes {
		t.abortLocked()
		return 0, ErrTransitionOverflow
	}

	cp := append([]byte(nil), payload...)
	t.bySeq[seq] = len(t.queue)
	t.queue = append(t.queue, TransitionPacket{Seq: seq, Payload: cp})
	t.queuedBytes += len(cp)
	return RouteQueuedRecord, nil
}

// Detach permanently stops TLS payload ownership and transfers queued early
// record payloads to the new receiver. Returned payloads are owned by caller.
func (t *StageTransition) Detach() ([]TransitionPacket, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != TransitionPrepared {
		return nil, ErrTransitionState
	}
	out := t.queue
	t.queue = nil
	t.bySeq = nil
	t.queuedBytes = 0
	t.state = TransitionDetached
	return out, nil
}

// Abort clears all candidate-owned payload. FakeTCP ACK processing may continue
// outside this object so a failed candidate cannot poison an existing lane.
func (t *StageTransition) Abort() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.abortLocked()
}

func (t *StageTransition) QueueUsage() (records, bytes int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.queue), t.queuedBytes
}

func (t *StageTransition) Boundary() (uint32, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == TransitionBootstrap {
		return 0, false
	}
	return t.boundary, true
}

func (t *StageTransition) abortLocked() {
	t.queue = nil
	t.bySeq = nil
	t.queuedBytes = 0
	t.state = TransitionAborted
}
