package faketcp

import (
	"errors"
	"net"
	"sync"
	"time"
)

const (
	MaxSYNACKRetries    = 5
	MaxHalfOpenLifetime = 30 * time.Second
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

type SegmentEmitter func(Segment) error

type ServerSegmentResult struct {
	Disposition TransitionDisposition
	AckNeeded   bool
	Ack         uint32
	Record      *TransitionPacket
}

type PeerTCPProfile struct {
	AdvertisedMSS  bool
	MSS            uint16
	SACKPermitted  bool
	WindowScale    uint8
	WindowScaleSet bool
	InitialWindow  uint16
}

// ServerAssociation owns the short TCP-compatible establishment/fallback
// lifecycle. Steady-state finite repair remains outside this type.
type ServerAssociation struct {
	mu sync.Mutex

	flow      ServerFlow
	state     ServerAssociationState
	serverISN uint32
	clientISN uint32
	peerStart uint32
	peer      PeerTCPProfile

	sender     *Sender
	bootstrap  *BootstrapStream
	transition *StageTransition
	emit       SegmentEmitter

	handshakeRTO     time.Duration
	synACKSent       time.Time
	synACKRetries    uint32
	halfOpenDeadline time.Time
	peerFIN          bool
	localFINQueued   bool
}

func NewServerAssociation(syn Segment, serverISN uint32, initialRTO time.Duration, emit SegmentEmitter) (*ServerAssociation, error) {
	if !IsInitialSYN(syn) {
		return nil, ErrBadServerSYN
	}

	peerMSS := uint16(DefaultIPv4PeerMSS)
	if syn.MSSSet {
		peerMSS = syn.MSS
	}
	now := time.Now()
	a := &ServerAssociation{
		flow:      ServerFlowFromSegment(syn),
		state:     ServerAssociationAwaitACK,
		serverISN: serverISN,
		clientISN: syn.Seq,
		peerStart: syn.Seq + 1,
		peer: PeerTCPProfile{
			AdvertisedMSS:  syn.MSSSet,
			MSS:            peerMSS,
			SACKPermitted:  syn.SACKPermitted,
			WindowScale:    syn.WindowScale,
			WindowScaleSet: syn.WindowScaleSet,
			InitialWindow:  syn.Window,
		},
		sender:           NewSender(serverISN+1, initialRTO),
		emit:             emit,
		handshakeRTO:     clampSenderRTO(initialRTO),
		halfOpenDeadline: now.Add(MaxHalfOpenLifetime),
	}
	a.sender.UpdatePeerWindow(syn.Window, 0, false)

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
	if peerMSS < DefaultBootstrapChunk {
		stream.chunk = int(peerMSS)
	}
	stream.ConfigureWriteWindow(a.sender.WaitWindow, MaxBootstrapFlightChunks)
	stream.ConfigureCloseWrite(a.closeWrite)
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

func (a *ServerAssociation) PeerTCPProfile() PeerTCPProfile {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.peer
}

func (a *ServerAssociation) BootstrapConn() net.Conn { return a.bootstrap }
func (a *ServerAssociation) BootstrapNext() uint32   { return a.bootstrap.NextSeq() }
func (a *ServerAssociation) SenderNext() uint32      { return a.sender.NextSeq() }
func (a *ServerAssociation) SenderLastAck() uint32   { return a.sender.LastAck() }
func (a *ServerAssociation) SenderStats() SenderStats { return a.sender.Stats() }
func (a *ServerAssociation) SenderPending() int       { return a.sender.Pending() }

func (a *ServerAssociation) SYNACKSegment() (Segment, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ServerAssociationAwaitACK {
		return Segment{}, ErrHandshakeState
	}
	if a.synACKSent.IsZero() {
		a.synACKSent = time.Now()
	}
	return a.synackLocked(), nil
}

func (a *ServerAssociation) synackLocked() Segment {
	return Segment{
		SrcIP: a.flow.ServerIP, DstIP: a.flow.ClientIP,
		SrcPort: a.flow.ServerPort, DstPort: a.flow.ClientPort,
		Seq: a.serverISN, Ack: a.peerStart,
		Flags: FlagSYN | FlagACK, Window: a.advertisedWindowLocked(false),
		MSS: DefaultMSS, MSSSet: true,
		SACKPermitted: a.peer.SACKPermitted,
		WindowScale: DefaultWindowScale, WindowScaleSet: a.peer.WindowScaleSet,
	}
}

func (a *ServerAssociation) HandleHandshakeACK(seg Segment) error {
	_, err := a.HandleSegment(seg, time.Now())
	return err
}

// HandleSegment keeps ACK/control processing in FakeTCP while routing payload by
// the explicit bootstrap/record boundary. FIN is sequenced after tail payload.
func (a *ServerAssociation) HandleSegment(seg Segment, now time.Time) (ServerSegmentResult, error) {
	var out ServerSegmentResult

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.state == ServerAssociationClosed || !a.flow.Matches(seg) {
		return out, ErrHandshakeState
	}

	if seg.Flags&FlagRST != 0 {
		if a.rstAcceptableLocked(seg) {
			a.closeLocked(ErrBootstrapClosed)
			out.Disposition = RouteAckOnly
			return out, nil
		}
		out.Disposition = RouteAckOnly
		if a.state == ServerAssociationEstablished {
			out.AckNeeded = true
			out.Ack = a.bootstrap.NextSeq()
		}
		return out, nil
	}

	if a.state == ServerAssociationAwaitACK {
		if seg.Flags&FlagACK == 0 || seg.Ack != a.serverISN+1 || seg.Seq != a.peerStart {
			return out, ErrHandshakeState
		}
		a.state = ServerAssociationEstablished
	}

	if seg.Flags&FlagACK != 0 {
		a.sender.UpdatePeerWindow(seg.Window, a.peer.WindowScale, a.peer.WindowScaleSet)
		detached := a.transition != nil && a.transition.State() == TransitionDetached
		// Bootstrap sender ownership ends at detach. A steady-state ACK may
		// legitimately advance beyond the final TLS/admission byte; the runtime
		// owner consumes that sequence space instead of treating it as a bad
		// bootstrap ACK. ACKs inside the bootstrap range remain validated here.
		if !detached || !seqLT(a.sender.NextSeq(), seg.Ack) {
			if err := a.sender.AckChecked(seg.Ack, now); err != nil {
				return out, err
			}
		}
	}

	hasPayload := len(seg.Payload) != 0
	hasFIN := seg.Flags&FlagFIN != 0
	if !hasPayload && !hasFIN {
		out.Disposition = RouteAckOnly
		a.maybeCloseAfterFINLocked()
		return out, nil
	}

	if hasPayload {
		out.AckNeeded = true
		if a.transition == nil {
			a.bootstrap.Feed(seg.Seq, seg.Payload)
			out.Disposition = RouteBootstrap
			out.Ack = a.bootstrap.NextSeq()
		} else {
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
		}
	}

	if hasFIN {
		out.AckNeeded = true
		finSeq := seg.Seq + uint32(len(seg.Payload))
		// Ordinary fallback and any late bootstrap FIN use stream ordering.
		// A FIN beyond a detached record boundary is not consumed by TLS.
		if a.transition == nil || out.Disposition == RouteBootstrap || out.Disposition == RouteLateBootstrap || !hasPayload {
			a.bootstrap.FeedFIN(finSeq)
			if a.bootstrap.ReadEOF() {
				a.peerFIN = true
			}
		}
		out.Ack = a.bootstrap.NextSeq()
		if !hasPayload {
			out.Disposition = RouteAckOnly
		}
	}
	a.maybeCloseAfterFINLocked()
	return out, nil
}

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

// DetachTransition ends TLS adapter ownership only. It does not send FIN or
// close the outer FakeTCP association.
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
	_ = a.bootstrap.Detach()
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
	a.bootstrap.Abort(ErrBootstrapClosed)
	a.mu.Unlock()
}

func (a *ServerAssociation) ACKSegment(ack uint32) Segment {
	a.mu.Lock()
	defer a.mu.Unlock()
	return Segment{
		SrcIP: a.flow.ServerIP, DstIP: a.flow.ClientIP,
		SrcPort: a.flow.ServerPort, DstPort: a.flow.ClientPort,
		Seq: a.sender.NextSeq(), Ack: ack,
		Flags: FlagACK, Window: a.advertisedWindowLocked(true),
	}
}

func (a *ServerAssociation) advertisedWindowLocked(postHandshake bool) uint16 {
	available := a.bootstrap.AvailableReceiveWindow()
	if available <= 0 {
		return 0
	}
	if postHandshake && a.peer.WindowScaleSet {
		available >>= DefaultWindowScale
		if available == 0 {
			available = 1
		}
	}
	if available > 65535 {
		available = 65535
	}
	return uint16(available)
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
	window := a.advertisedWindowLocked(true)
	a.mu.Unlock()

	pending := a.sender.Enqueue(payload, time.Now())
	if emit == nil {
		return pending.End, ErrAssociationNoEmitter
	}
	seg := Segment{
		SrcIP: flow.ServerIP, DstIP: flow.ClientIP,
		SrcPort: flow.ServerPort, DstPort: flow.ClientPort,
		Seq: pending.Seq, Ack: ack,
		Flags: pending.Flags, Window: window,
		Payload: pending.Payload,
	}
	return pending.End, emit(seg)
}

func (a *ServerAssociation) closeWrite() error {
	a.mu.Lock()
	if a.state != ServerAssociationEstablished {
		a.mu.Unlock()
		return ErrHandshakeState
	}
	if a.localFINQueued {
		a.mu.Unlock()
		return nil
	}
	a.localFINQueued = true
	flow := a.flow
	emit := a.emit
	ack := a.bootstrap.NextSeq()
	window := a.advertisedWindowLocked(true)
	a.mu.Unlock()

	pending := a.sender.EnqueueFIN(time.Now())
	if emit == nil {
		return ErrAssociationNoEmitter
	}
	return emit(Segment{
		SrcIP: flow.ServerIP, DstIP: flow.ClientIP,
		SrcPort: flow.ServerPort, DstPort: flow.ClientPort,
		Seq: pending.Seq, Ack: ack,
		Flags: pending.Flags, Window: window,
	})
}

// EmitRetransmitDue covers both half-open SYN-ACK and bootstrap data/FIN.
// SYN-ACK retries are capped and retain the original server ISN/options.
func (a *ServerAssociation) EmitRetransmitDue(now time.Time) (bool, error) {
	a.mu.Lock()
	if a.state == ServerAssociationAwaitACK {
		if !now.Before(a.halfOpenDeadline) || a.synACKRetries >= MaxSYNACKRetries {
			a.closeLocked(ErrBootstrapTimeout)
			a.mu.Unlock()
			return false, nil
		}
		if a.synACKSent.IsZero() || now.Sub(a.synACKSent) < a.handshakeRTO {
			a.mu.Unlock()
			return false, nil
		}
		a.synACKSent = now
		a.synACKRetries++
		seg := a.synackLocked()
		emit := a.emit
		a.mu.Unlock()
		if emit == nil {
			return true, ErrAssociationNoEmitter
		}
		return true, emit(seg)
	}
	if a.state != ServerAssociationEstablished {
		a.mu.Unlock()
		return false, nil
	}
	flow := a.flow
	emit := a.emit
	ack := a.bootstrap.NextSeq()
	window := a.advertisedWindowLocked(true)
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
		Flags: pending.Flags, Window: window,
		Payload: pending.Payload,
	})
}

func (a *ServerAssociation) rstAcceptableLocked(seg Segment) bool {
	switch a.state {
	case ServerAssociationAwaitACK:
		return seg.Flags&FlagACK != 0 && seg.Ack == a.serverISN+1 && seg.Seq == a.peerStart
	case ServerAssociationEstablished:
		return seg.Seq == a.bootstrap.NextSeq()
	default:
		return false
	}
}

func (a *ServerAssociation) maybeCloseAfterFINLocked() {
	if a.peerFIN && a.localFINQueued && a.sender.Pending() == 0 {
		a.closeLocked(ErrBootstrapClosed)
	}
}

func (a *ServerAssociation) closeLocked(err error) {
	if a.state == ServerAssociationClosed {
		return
	}
	a.state = ServerAssociationClosed
	if a.transition != nil {
		a.transition.Abort()
	}
	a.bootstrap.Abort(err)
	a.sender.Abort(err)
}

func (a *ServerAssociation) Close() {
	a.mu.Lock()
	a.closeLocked(ErrBootstrapClosed)
	a.mu.Unlock()
}

func (a *ServerAssociation) IsDuplicateSYN(syn Segment) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state == ServerAssociationAwaitACK &&
		a.flow == ServerFlowFromSegment(syn) &&
		syn.Seq == a.clientISN &&
		syn.Window == a.peer.InitialWindow &&
		syn.MSSSet == a.peer.AdvertisedMSS &&
		(!syn.MSSSet || syn.MSS == a.peer.MSS) &&
		syn.SACKPermitted == a.peer.SACKPermitted &&
		syn.WindowScaleSet == a.peer.WindowScaleSet &&
		(!syn.WindowScaleSet || syn.WindowScale == a.peer.WindowScale)
}

func (a *ServerAssociation) ShouldRetire(now time.Time) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state == ServerAssociationClosed {
		return true
	}
	return a.state == ServerAssociationAwaitACK && !now.Before(a.halfOpenDeadline)
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

// AddSYN reuses an exact duplicate half-open SYN so retransmission cannot create
// a second TLS session or a new server ISN. A genuinely reused four-tuple is
// accepted only after the previous association has retired.
func (t *ServerAssociationTable) AddSYN(syn Segment, serverISN uint32, initialRTO time.Duration) (*ServerAssociation, error) {
	flow := ServerFlowFromSegment(syn)
	t.mu.Lock()
	defer t.mu.Unlock()
	if old, ok := t.m[flow]; ok {
		if old.IsDuplicateSYN(syn) {
			return old, nil
		}
		if old.State() == ServerAssociationClosed {
			delete(t.m, flow)
		} else {
			return nil, ErrAssociationExists
		}
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

// Sweep releases closed/expired half-open associations without touching other
// flows. It is safe for a periodic owner timer to call with no network traffic.
func (t *ServerAssociationTable) Sweep(now time.Time) int {
	t.mu.Lock()
	var retired []*ServerAssociation
	for flow, a := range t.m {
		if a.ShouldRetire(now) {
			delete(t.m, flow)
			retired = append(retired, a)
		}
	}
	t.mu.Unlock()
	for _, a := range retired {
		a.Close()
	}
	return len(retired)
}

func (t *ServerAssociationTable) Len() int {
	t.mu.RLock()
	n := len(t.m)
	t.mu.RUnlock()
	return n
}
