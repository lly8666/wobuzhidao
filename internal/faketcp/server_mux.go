package faketcp

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrMuxFull            = errors.New("faketcp: server association table full")
	ErrBadServerSYN       = errors.New("faketcp: invalid server-side SYN")
	ErrAssociationExists  = errors.New("faketcp: association already exists")
	ErrHandshakeState     = errors.New("faketcp: invalid association handshake state")
	ErrAssociationMissing = errors.New("faketcp: association not found")
)

type ServerFlow struct {
	ClientIP   [4]byte
	ClientPort uint16
	ServerIP   [4]byte
	ServerPort uint16
}

func ServerFlowFromSegment(seg Segment) ServerFlow {
	return ServerFlow{
		ClientIP: seg.SrcIP, ClientPort: seg.SrcPort,
		ServerIP: seg.DstIP, ServerPort: seg.DstPort,
	}
}

func (f ServerFlow) Matches(seg Segment) bool {
	return seg.SrcIP == f.ClientIP && seg.SrcPort == f.ClientPort &&
		seg.DstIP == f.ServerIP && seg.DstPort == f.ServerPort
}

type ServerAssociationState uint8

const (
	ServerAssociationAwaitACK ServerAssociationState = iota
	ServerAssociationEstablished
	ServerAssociationClosed
)

type ServerSegmentResult struct {
	FastRetransmit *Pending
	Deliver        []byte
	DeliverSeq     uint32
	AckNeeded      bool
	Ack            uint32
	SACK           [4]SACKBlock
	SACKN          int
}

type ServerAssociation struct {
	mu sync.Mutex

	flow      ServerFlow
	state     ServerAssociationState
	serverISN uint32
	peerNext  uint32
	sender    *Sender
	receiver  *Receiver
}

func NewServerAssociation(syn Segment, serverISN uint32, recovery RecoveryMode, initialRTO time.Duration) (*ServerAssociation, error) {
	if syn.Flags&FlagSYN == 0 || syn.Flags&FlagACK != 0 || len(syn.Payload) != 0 || syn.SrcPort == 0 || syn.DstPort == 0 {
		return nil, ErrBadServerSYN
	}
	peerNext := syn.Seq + 1
	return &ServerAssociation{
		flow:      ServerFlowFromSegment(syn),
		state:     ServerAssociationAwaitACK,
		serverISN: serverISN,
		peerNext:  peerNext,
		sender:    NewSenderWithRecovery(serverISN+1, initialRTO, recovery),
		receiver:  NewReceiver(peerNext),
	}, nil
}

func (a *ServerAssociation) Flow() ServerFlow {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.flow
}

func (a *ServerAssociation) State() ServerAssociationState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

func (a *ServerAssociation) SYNACK() (seq, ack uint32, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationAwaitACK {
		return 0, 0, ErrHandshakeState
	}
	return a.serverISN, a.peerNext, nil
}

// HandleHandshakeACK completes only this flow. Like TCP, the final ACK may
// carry the first application payload. Callers that need that payload must then
// process the same segment through HandleSegment after this method succeeds.
func (a *ServerAssociation) HandleHandshakeACK(seg Segment) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationAwaitACK || !a.flow.Matches(seg) || seg.Flags&FlagACK == 0 || seg.Ack != a.serverISN+1 {
		return ErrHandshakeState
	}
	a.state = ServerAssociationEstablished
	return nil
}

func (a *ServerAssociation) HandleSegment(seg Segment, now time.Time) (ServerSegmentResult, error) {
	var out ServerSegmentResult
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationEstablished || !a.flow.Matches(seg) {
		return out, ErrHandshakeState
	}
	if seg.Flags&FlagACK != 0 {
		out.FastRetransmit = a.sender.AckSelective(seg.Ack, seg.SACK[:seg.SACKN], now)
	}
	if len(seg.Payload) == 0 {
		return out, nil
	}
	deliver, sackNeeded := a.receiver.Accept(seg.Seq, len(seg.Payload))
	out.AckNeeded = true
	out.Ack = a.receiver.Next()
	if sackNeeded {
		out.SACKN = a.receiver.SACKBlocks(&out.SACK)
	}
	if deliver {
		out.Deliver = seg.Payload
		out.DeliverSeq = seg.Seq
	}
	return out, nil
}

func (a *ServerAssociation) Enqueue(payload []byte, now time.Time) (*Pending, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationEstablished {
		return nil, ErrHandshakeState
	}
	return a.sender.Enqueue(payload, now), nil
}

func (a *ServerAssociation) RetransmitDue(now time.Time) *Pending {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationEstablished {
		return nil
	}
	return a.sender.RetransmitDue(now)
}

func (a *ServerAssociation) SenderNext() uint32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sender.NextSeq()
}

func (a *ServerAssociation) SenderLastAck() uint32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sender.LastAck()
}

func (a *ServerAssociation) ReceiverNext() uint32 {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.receiver.Next()
}

func (a *ServerAssociation) SenderStats() SenderStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sender.Stats()
}

func (a *ServerAssociation) ReceiverStats() ReceiverStats {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.receiver.Stats()
}

func (a *ServerAssociation) Close() {
	a.mu.Lock()
	a.state = ServerAssociationClosed
	a.mu.Unlock()
}

type ServerAssociationTable struct {
	mu  sync.RWMutex
	max int
	m   map[ServerFlow]*ServerAssociation
}

func NewServerAssociationTable(max int) (*ServerAssociationTable, error) {
	if max <= 0 {
		return nil, ErrMuxFull
	}
	return &ServerAssociationTable{max: max, m: make(map[ServerFlow]*ServerAssociation)}, nil
}

func (t *ServerAssociationTable) AddSYN(syn Segment, serverISN uint32, recovery RecoveryMode, initialRTO time.Duration) (*ServerAssociation, error) {
	flow := ServerFlowFromSegment(syn)
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.m[flow]; ok {
		return nil, ErrAssociationExists
	}
	if len(t.m) >= t.max {
		return nil, ErrMuxFull
	}
	a, err := NewServerAssociation(syn, serverISN, recovery, initialRTO)
	if err != nil {
		return nil, err
	}
	t.m[flow] = a
	return a, nil
}

func (t *ServerAssociationTable) Get(flow ServerFlow) (*ServerAssociation, bool) {
	t.mu.RLock()
	a, ok := t.m[flow]
	t.mu.RUnlock()
	return a, ok
}

func (t *ServerAssociationTable) GetSegment(seg Segment) (*ServerAssociation, bool) {
	return t.Get(ServerFlowFromSegment(seg))
}

func (t *ServerAssociationTable) Remove(flow ServerFlow) bool {
	t.mu.Lock()
	a, ok := t.m[flow]
	if ok {
		delete(t.m, flow)
	}
	t.mu.Unlock()
	if ok {
		a.Close()
	}
	return ok
}

func (t *ServerAssociationTable) Len() int {
	t.mu.RLock()
	n := len(t.m)
	t.mu.RUnlock()
	return n
}
