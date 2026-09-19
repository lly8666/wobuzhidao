package faketcp

import (
	"errors"
	"net"
	"sync"
	"time"
)

var (
	ErrMuxFull              = errors.New("faketcp: server association table full")
	ErrBadServerSYN         = errors.New("faketcp: invalid server-side SYN")
	ErrAssociationExists    = errors.New("faketcp: association already exists")
	ErrHandshakeState       = errors.New("faketcp: invalid association handshake state")
	ErrAssociationMissing   = errors.New("faketcp: association not found")
	ErrAssociationNoEmitter = errors.New("faketcp: association has no packet emitter")
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

// SegmentEmitter is the association's raw-I/O handoff. The P2 association owns
// sequence construction; platform raw sockets are intentionally a later layer.
type SegmentEmitter func(Segment) error

type ServerSegmentResult struct {
	Disposition TransitionDisposition
	AckNeeded   bool
	Ack         uint32
	Record      *TransitionPacket
}

// ServerAssociation is the P2 handshake/bootstrap closure. It keeps one
// four-tuple and one pair of TCP sequence spaces from SYN through TLS bootstrap
// and the prepare/detach barrier. It intentionally contains no steady-state
// Receiver, SACK/RACK, FEC, or partial-reliability policy.
type ServerAssociation struct {
	mu sync.Mutex

	flow      ServerFlow
	state     ServerAssociationState
	serverISN uint32
	peerStart uint32

	sender     *Sender
	bootstrap  *BootstrapStream
	transition *StageTransition
	emit       SegmentEmitter
}

func NewServerAssociation(syn Segment, serverISN uint32, initialRTO time.Duration, emit SegmentEmitter) (*ServerAssociation, error) {
	if syn.Flags&FlagSYN == 0 || syn.Flags&FlagACK != 0 || !IsWBDHandshakeSegment(syn) ||
		syn.SrcPort == 0 || syn.DstPort == 0 {
		return nil, ErrBadServerSYN
	}

	a := &ServerAssociation{
		flow:      ServerFlowFromSegment(syn),
		state:     ServerAssociationAwaitACK,
		serverISN: serverISN,
		peerStart: syn.Seq + 1,
		sender:    NewSender(serverISN+1, initialRTO),
		emit:      emit,
	}
	local := &net.TCPAddr{
		IP:   net.IPv4(a.flow.ServerIP[0], a.flow.ServerIP[1], a.flow.ServerIP[2], a.flow.ServerIP[3]),
		Port: int(a.flow.ServerPort),
	}
	remote := &net.TCPAddr{
		IP:   net.IPv4(a.flow.ClientIP[0], a.flow.ClientIP[1], a.flow.ClientIP[2], a.flow.ClientIP[3]),
		Port: int(a.flow.ClientPort),
	}
	stream, err := NewBootstrapStream(a.peerStart, a.sendBootstrap, a.sender.WaitAck, local, remote)
	if err != nil {
		return nil, err
	}
	a.bootstrap = stream
	return a, nil
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

func (a *ServerAssociation) BootstrapConn() net.Conn {
	return a.bootstrap
}

func (a *ServerAssociation) BootstrapNext() uint32 {
	return a.bootstrap.NextSeq()
}

func (a *ServerAssociation) SenderNext() uint32 {
	return a.sender.NextSeq()
}

func (a *ServerAssociation) SenderLastAck() uint32 {
	return a.sender.LastAck()
}

func (a *ServerAssociation) SenderStats() SenderStats {
	return a.sender.Stats()
}

func (a *ServerAssociation) SYNACKSegment() (Segment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationAwaitACK {
		return Segment{}, ErrHandshakeState
	}
	return Segment{
		SrcIP: a.flow.ServerIP, DstIP: a.flow.ClientIP,
		SrcPort: a.flow.ServerPort, DstPort: a.flow.ClientPort,
		Seq: a.serverISN, Ack: a.peerStart,
		Flags: FlagSYN | FlagACK, Window: 65535,
		MSS: DefaultMSS, MSSSet: true,
		SACKPermitted: true,
		WindowScale: DefaultWindowScale, WindowScaleSet: true,
	}, nil
}

// HandleHandshakeACK is retained as a narrow handshake entry point, but it uses
// the same payload path as HandleSegment so a data-bearing final ACK cannot lose
// the first TLS bytes.
func (a *ServerAssociation) HandleHandshakeACK(seg Segment) error {
	_, err := a.HandleSegment(seg, time.Now())
	return err
}

// HandleSegment serializes receive ownership changes with PrepareTransition and
// DetachTransition. ACK processing always remains in FakeTCP. Payload routing is
// based only on the explicit sequence boundary, never on TLS-looking bytes.
func (a *ServerAssociation) HandleSegment(seg Segment, now time.Time) (ServerSegmentResult, error) {
	var out ServerSegmentResult

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.state == ServerAssociationClosed || !a.flow.Matches(seg) {
		return out, ErrHandshakeState
	}

	if a.state == ServerAssociationAwaitACK {
		if seg.Flags&FlagACK == 0 || seg.Ack != a.serverISN+1 {
			return out, ErrHandshakeState
		}
		a.state = ServerAssociationEstablished
	}

	if seg.Flags&FlagACK != 0 {
		a.sender.Ack(seg.Ack, now)
	}
	if len(seg.Payload) == 0 {
		out.Disposition = RouteAckOnly
		return out, nil
	}

	out.AckNeeded = true
	if a.transition == nil {
		a.bootstrap.Feed(seg.Seq, seg.Payload)
		out.Disposition = RouteBootstrap
		out.Ack = a.bootstrap.NextSeq()
		return out, nil
	}

	disposition, err := a.transition.Route(seg.Seq, seg.Payload)
	if err != nil {
		return out, err
	}
	out.Disposition = disposition

	switch disposition {
	case RouteBootstrap:
		a.bootstrap.Feed(seg.Seq, seg.Payload)
		out.Ack = a.bootstrap.NextSeq()
	case RouteQueuedRecord, RouteQueuedDuplicate, RouteLateBootstrap:
		// Until the steady-state receiver is installed, cumulative ACK ownership
		// stays at the exact bootstrap boundary. Early records can retransmit and
		// are deduplicated by StageTransition without being exposed to TLS.
		out.Ack = a.bootstrap.NextSeq()
	case RouteRecord:
		out.Ack = a.bootstrap.NextSeq()
		out.Record = &TransitionPacket{
			Seq:     seg.Seq,
			Payload: append([]byte(nil), seg.Payload...),
		}
	default:
		out.Ack = a.bootstrap.NextSeq()
	}
	return out, nil
}

// PrepareTransition freezes the exact next bootstrap sequence as the ownership
// barrier. The negotiated record wire maximum defines the temporary early-record
// queue's byte bound.
func (a *ServerAssociation) PrepareTransition(maxRecordWire int) (uint32, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationEstablished || a.transition != nil {
		return 0, ErrTransitionState
	}
	boundary := a.bootstrap.NextSeq()
	tr, err := NewStageTransition(maxRecordWire)
	if err != nil {
		return 0, err
	}
	if err := tr.Prepare(boundary); err != nil {
		return 0, err
	}
	a.transition = tr
	return boundary, nil
}

// DetachTransition ends TLS payload ownership and returns any copied early
// records. FakeTCP ACK handling remains active on this association.
func (a *ServerAssociation) DetachTransition() ([]TransitionPacket, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationEstablished || a.transition == nil {
		return nil, ErrTransitionState
	}
	queued, err := a.transition.Detach()
	if err != nil {
		return nil, err
	}
	_ = a.bootstrap.Close()
	return queued, nil
}

func (a *ServerAssociation) TransitionState() (TransitionState, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.transition == nil {
		return 0, false
	}
	return a.transition.State(), true
}

func (a *ServerAssociation) AbortTransition() {
	a.mu.Lock()
	if a.transition != nil {
		a.transition.Abort()
	}
	_ = a.bootstrap.Close()
	a.mu.Unlock()
}

func (a *ServerAssociation) ACKSegment(ack uint32) Segment {
	a.mu.Lock()
	flow := a.flow
	a.mu.Unlock()
	return Segment{
		SrcIP: flow.ServerIP, DstIP: flow.ClientIP,
		SrcPort: flow.ServerPort, DstPort: flow.ClientPort,
		Seq: a.sender.NextSeq(), Ack: ack,
		Flags: FlagACK, Window: 65535,
	}
}

func (a *ServerAssociation) sendBootstrap(payload []byte) (uint32, error) {
	a.mu.Lock()
	if a.state != ServerAssociationEstablished {
		a.mu.Unlock()
		return 0, ErrHandshakeState
	}
	flow := a.flow
	emit := a.emit
	ack := a.bootstrap.NextSeq()
	a.mu.Unlock()

	pending := a.sender.Enqueue(payload, time.Now())
	if emit == nil {
		return pending.End, ErrAssociationNoEmitter
	}
	seg := Segment{
		SrcIP: flow.ServerIP, DstIP: flow.ClientIP,
		SrcPort: flow.ServerPort, DstPort: flow.ClientPort,
		Seq: pending.Seq, Ack: ack,
		Flags: FlagACK | FlagPSH, Window: 65535,
		Payload: pending.Payload,
	}
	return pending.End, emit(seg)
}

// EmitRetransmitDue emits the oldest due retransmission with its original
// sequence and copied payload. Bootstrap retries therefore stay on the exact
// same association and sequence space.
func (a *ServerAssociation) EmitRetransmitDue(now time.Time) (bool, error) {
	a.mu.Lock()
	if a.state != ServerAssociationEstablished {
		a.mu.Unlock()
		return false, nil
	}
	flow := a.flow
	emit := a.emit
	ack := a.bootstrap.NextSeq()
	a.mu.Unlock()

	pending := a.sender.RetransmitDue(now)
	if pending == nil {
		return false, nil
	}
	if emit == nil {
		return true, ErrAssociationNoEmitter
	}
	return true, emit(Segment{
		SrcIP: flow.ServerIP, DstIP: flow.ClientIP,
		SrcPort: flow.ServerPort, DstPort: flow.ClientPort,
		Seq: pending.Seq, Ack: ack,
		Flags: FlagACK | FlagPSH, Window: 65535,
		Payload: pending.Payload,
	})
}

func (a *ServerAssociation) Close() {
	a.mu.Lock()
	if a.state != ServerAssociationClosed {
		a.state = ServerAssociationClosed
		if a.transition != nil {
			a.transition.Abort()
		}
		_ = a.bootstrap.Close()
	}
	a.mu.Unlock()
}

type ServerAssociationTable struct {
	mu   sync.RWMutex
	max  int
	emit SegmentEmitter
	m    map[ServerFlow]*ServerAssociation
}

func NewServerAssociationTable(max int, emit SegmentEmitter) (*ServerAssociationTable, error) {
	if max <= 0 {
		return nil, ErrMuxFull
	}
	return &ServerAssociationTable{
		max: max, emit: emit, m: make(map[ServerFlow]*ServerAssociation),
	}, nil
}

func (t *ServerAssociationTable) AddSYN(syn Segment, serverISN uint32, initialRTO time.Duration) (*ServerAssociation, error) {
	flow := ServerFlowFromSegment(syn)
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := t.m[flow]; ok {
		return nil, ErrAssociationExists
	}
	if len(t.m) >= t.max {
		return nil, ErrMuxFull
	}
	a, err := NewServerAssociation(syn, serverISN, initialRTO, t.emit)
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
