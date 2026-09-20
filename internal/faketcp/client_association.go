package faketcp

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"
)

const MaxSYNRetries = 5

var (
	ErrBadClientFlow   = errors.New("faketcp: invalid client flow")
	ErrClientHandshake = errors.New("faketcp: client handshake failed")
	ErrClientDetached  = errors.New("faketcp: client association detached")
)

type ClientFlow struct {
	LocalIP   [4]byte
	PeerIP    [4]byte
	LocalPort uint16
	PeerPort  uint16
}

func (f ClientFlow) Validate() error {
	if f.LocalIP == ([4]byte{}) || f.PeerIP == ([4]byte{}) ||
		f.LocalPort == 0 || f.PeerPort == 0 {
		return ErrBadClientFlow
	}
	return nil
}

func (f ClientFlow) MatchesInbound(seg Segment) bool {
	return seg.SrcIP == f.PeerIP && seg.DstIP == f.LocalIP &&
		seg.SrcPort == f.PeerPort && seg.DstPort == f.LocalPort
}

type ClientAssociationState uint8

const (
	ClientAssociationInit ClientAssociationState = iota
	ClientAssociationSYNSent
	ClientAssociationEstablished
	ClientAssociationDetached
	ClientAssociationClosed
)

type ClientHandoff struct {
	SendNext    uint32
	ReceiveNext uint32
	Peer         PeerTCPProfile
}

type ClientAssociation struct {
	mu sync.Mutex

	flow      ClientFlow
	state     ClientAssociationState
	clientISN uint32
	serverISN uint32
	peer      PeerTCPProfile

	sender    *Sender
	bootstrap *BootstrapStream
	emit      SegmentEmitter

	handshakeRTO time.Duration
	synSent      time.Time
	synRetries   uint32

	notify chan struct{}
}

func NewClientAssociation(flow ClientFlow, clientISN uint32, initialRTO time.Duration, emit SegmentEmitter) (*ClientAssociation, error) {
	if err := flow.Validate(); err != nil || emit == nil {
		return nil, ErrBadClientFlow
	}
	return &ClientAssociation{
		flow:         flow,
		state:        ClientAssociationInit,
		clientISN:    clientISN,
		sender:       NewSender(clientISN+1, initialRTO),
		emit:         emit,
		handshakeRTO: clampSenderRTO(initialRTO),
		notify:       make(chan struct{}, 1),
	}, nil
}

func (a *ClientAssociation) Flow() ClientFlow {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.flow
}

func (a *ClientAssociation) State() ClientAssociationState {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.state
}

func (a *ClientAssociation) PeerTCPProfile() PeerTCPProfile {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.peer
}

func (a *ClientAssociation) BootstrapConn() net.Conn {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.state != ClientAssociationEstablished || a.bootstrap == nil {
		return nil
	}
	return a.bootstrap
}

func (a *ClientAssociation) Start(now time.Time) error {
	a.mu.Lock()
	if a.state != ClientAssociationInit {
		a.mu.Unlock()
		return ErrClientHandshake
	}
	a.state = ClientAssociationSYNSent
	a.synSent = now
	seg := a.synLocked()
	emit := a.emit
	a.mu.Unlock()
	a.signal()
	return emit(seg)
}

func (a *ClientAssociation) WaitEstablished(ctx context.Context) error {
	if ctx == nil {
		return ErrClientHandshake
	}
	for {
		a.mu.Lock()
		state := a.state
		a.mu.Unlock()
		switch state {
		case ClientAssociationEstablished:
			return nil
		case ClientAssociationClosed:
			return ErrClientHandshake
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.notify:
		}
	}
}

func (a *ClientAssociation) synLocked() Segment {
	return Segment{
		SrcIP: a.flow.LocalIP, DstIP: a.flow.PeerIP,
		SrcPort: a.flow.LocalPort, DstPort: a.flow.PeerPort,
		Seq: a.clientISN, Flags: FlagSYN, Window: 65535,
		MSS: DefaultMSS, MSSSet: true,
		SACKPermitted: true,
		WindowScale: DefaultWindowScale, WindowScaleSet: true,
	}
}

func (a *ClientAssociation) HandleSegment(seg Segment, now time.Time) error {
	if !a.flow.MatchesInbound(seg) {
		return ErrClientHandshake
	}

	a.mu.Lock()
	if a.state == ClientAssociationClosed {
		a.mu.Unlock()
		return ErrClientHandshake
	}
	if seg.Flags&FlagRST != 0 {
		a.closeLocked(ErrBootstrapClosed)
		a.mu.Unlock()
		a.signal()
		return ErrClientHandshake
	}

	if a.state == ClientAssociationSYNSent {
		if seg.Flags&(FlagSYN|FlagACK) != FlagSYN|FlagACK ||
			seg.Flags&(FlagFIN|FlagRST|FlagPSH) != 0 ||
			len(seg.Payload) != 0 || seg.Ack != a.clientISN+1 {
			a.mu.Unlock()
			return ErrClientHandshake
		}
		peerMSS := uint16(DefaultIPv4PeerMSS)
		if seg.MSSSet {
			peerMSS = seg.MSS
		}
		a.serverISN = seg.Seq
		a.peer = PeerTCPProfile{
			AdvertisedMSS: seg.MSSSet,
			MSS: peerMSS,
			SACKPermitted: seg.SACKPermitted,
			WindowScale: seg.WindowScale,
			WindowScaleSet: seg.WindowScaleSet,
			InitialWindow: seg.Window,
		}
		a.sender.UpdatePeerWindow(seg.Window, seg.WindowScale, seg.WindowScaleSet)
		local := &net.TCPAddr{
			IP: net.IPv4(a.flow.LocalIP[0], a.flow.LocalIP[1], a.flow.LocalIP[2], a.flow.LocalIP[3]),
			Port: int(a.flow.LocalPort),
		}
		remote := &net.TCPAddr{
			IP: net.IPv4(a.flow.PeerIP[0], a.flow.PeerIP[1], a.flow.PeerIP[2], a.flow.PeerIP[3]),
			Port: int(a.flow.PeerPort),
		}
		stream, err := NewBootstrapStream(seg.Seq+1, a.sendBootstrap, a.sender.WaitAck, local, remote)
		if err != nil {
			a.closeLocked(err)
			a.mu.Unlock()
			a.signal()
			return err
		}
		if peerMSS < DefaultBootstrapChunk {
			stream.chunk = int(peerMSS)
		}
		stream.ConfigureWriteWindow(a.sender.WaitWindow, MaxBootstrapFlightChunks)
		a.bootstrap = stream
		a.state = ClientAssociationEstablished
		ack := a.ackLocked()
		emit := a.emit
		a.mu.Unlock()
		a.signal()
		return emit(ack)
	}

	if a.state == ClientAssociationDetached {
		a.mu.Unlock()
		return ErrClientDetached
	}
	if a.state != ClientAssociationEstablished || a.bootstrap == nil {
		a.mu.Unlock()
		return ErrClientHandshake
	}

	if seg.Flags&FlagSYN != 0 {
		if seg.Flags&FlagACK != 0 && seg.Ack == a.clientISN+1 && seg.Seq == a.serverISN {
			ack := a.ackLocked()
			emit := a.emit
			a.mu.Unlock()
			return emit(ack)
		}
		a.mu.Unlock()
		return ErrClientHandshake
	}

	if seg.Flags&FlagACK != 0 {
		a.sender.UpdatePeerWindow(seg.Window, a.peer.WindowScale, a.peer.WindowScaleSet)
		if err := a.sender.AckChecked(seg.Ack, now); err != nil {
			a.mu.Unlock()
			return err
		}
	}

	needACK := false
	if len(seg.Payload) != 0 {
		a.bootstrap.Feed(seg.Seq, seg.Payload)
		needACK = true
	}
	if seg.Flags&FlagFIN != 0 {
		a.bootstrap.FeedFIN(seg.Seq + uint32(len(seg.Payload)))
		needACK = true
	}
	if !needACK {
		a.mu.Unlock()
		return nil
	}
	ack := a.ackLocked()
	emit := a.emit
	a.mu.Unlock()
	return emit(ack)
}

func (a *ClientAssociation) sendBootstrap(payload []byte) (uint32, error) {
	now := time.Now()
	p := a.sender.Enqueue(payload, now)

	a.mu.Lock()
	if a.state != ClientAssociationEstablished || a.bootstrap == nil {
		a.mu.Unlock()
		return 0, ErrClientHandshake
	}
	ack := a.bootstrap.NextSeq()
	window := a.advertisedWindowLocked()
	flow := a.flow
	emit := a.emit
	a.mu.Unlock()

	err := emit(Segment{
		SrcIP: flow.LocalIP, DstIP: flow.PeerIP,
		SrcPort: flow.LocalPort, DstPort: flow.PeerPort,
		Seq: p.Seq, Ack: ack, Flags: p.Flags, Window: window,
		Payload: p.Payload,
	})
	return p.End, err
}

func (a *ClientAssociation) ackLocked() Segment {
	return Segment{
		SrcIP: a.flow.LocalIP, DstIP: a.flow.PeerIP,
		SrcPort: a.flow.LocalPort, DstPort: a.flow.PeerPort,
		Seq: a.sender.NextSeq(), Ack: a.bootstrap.NextSeq(),
		Flags: FlagACK, Window: a.advertisedWindowLocked(),
	}
}

func (a *ClientAssociation) advertisedWindowLocked() uint16 {
	if a.bootstrap == nil {
		return 65535
	}
	n := a.bootstrap.AvailableReceiveWindow()
	if n <= 0 {
		return 0
	}
	if n > 65535 {
		return 65535
	}
	return uint16(n)
}

func (a *ClientAssociation) Detach() (ClientHandoff, error) {
	a.mu.Lock()
	if a.state != ClientAssociationEstablished || a.bootstrap == nil {
		a.mu.Unlock()
		return ClientHandoff{}, ErrClientHandshake
	}
	handoff := ClientHandoff{
		SendNext: a.sender.NextSeq(),
		ReceiveNext: a.bootstrap.NextSeq(),
		Peer: a.peer,
	}
	stream := a.bootstrap
	a.state = ClientAssociationDetached
	a.mu.Unlock()
	if err := stream.Detach(); err != nil {
		return ClientHandoff{}, err
	}
	a.signal()
	return handoff, nil
}

func (a *ClientAssociation) EmitRetransmitDue(now time.Time) (bool, error) {
	a.mu.Lock()
	switch a.state {
	case ClientAssociationSYNSent:
		if a.synSent.IsZero() || now.Sub(a.synSent) < a.handshakeRTO {
			a.mu.Unlock()
			return false, nil
		}
		if a.synRetries >= MaxSYNRetries {
			a.closeLocked(ErrBootstrapTimeout)
			a.mu.Unlock()
			a.signal()
			return false, ErrBootstrapTimeout
		}
		a.synRetries++
		a.synSent = now
		seg := a.synLocked()
		emit := a.emit
		a.mu.Unlock()
		return true, emit(seg)
	case ClientAssociationEstablished:
		flow := a.flow
		ack := a.bootstrap.NextSeq()
		window := a.advertisedWindowLocked()
		emit := a.emit
		a.mu.Unlock()
		pending := a.sender.RetransmitDue(now)
		if pending == nil {
			return false, nil
		}
		return true, emit(Segment{
			SrcIP: flow.LocalIP, DstIP: flow.PeerIP,
			SrcPort: flow.LocalPort, DstPort: flow.PeerPort,
			Seq: pending.Seq, Ack: ack,
			Flags: pending.Flags, Window: window,
			Payload: pending.Payload,
		})
	default:
		a.mu.Unlock()
		return false, nil
	}
}

func (a *ClientAssociation) closeLocked(err error) {
	if a.state == ClientAssociationClosed {
		return
	}
	a.state = ClientAssociationClosed
	if a.bootstrap != nil {
		a.bootstrap.Abort(err)
	}
	a.sender.Abort(err)
}

func (a *ClientAssociation) Close() {
	a.mu.Lock()
	a.closeLocked(ErrBootstrapClosed)
	a.mu.Unlock()
	a.signal()
}

func (a *ClientAssociation) signal() {
	select {
	case a.notify <- struct{}{}:
	default:
	}
}
