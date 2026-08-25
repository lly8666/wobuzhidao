package session

import (
	"errors"
	"sync"
	"time"

	"github.com/lly8666/wobuzhidao/internal/control"
	"github.com/lly8666/wobuzhidao/internal/linkdata"
)

var (
	ErrSessionInactive      = errors.New("session: data session is not active")
	ErrSessionAlreadyActive = errors.New("session: data session is already active")
)

type dataSession struct {
	mu   sync.Mutex
	path *linkdata.Path
}

// DataPlane is the smallest server-side multi-session routing boundary. Account
// names are admission metadata only. Sustained packets are routed by learned
// DTLS plaintext peer on ingress and immutable LiveID on egress. Each LiveID
// owns an independent LinkConfig/FEC path so two devices sharing one username
// never share block IDs, decoder windows, repair timers or routing state.
type DataPlane struct {
	registry  *AccountRegistry
	maxBlocks int

	mu       sync.RWMutex
	sessions map[LiveID]*dataSession
}

func NewDataPlane(maxLive, maxBlocks int) (*DataPlane, error) {
	if maxBlocks <= 0 {
		return nil, ErrSessionInactive
	}
	r, err := NewAccountRegistry(maxLive)
	if err != nil {
		return nil, err
	}
	return &DataPlane{
		registry:  r,
		maxBlocks: maxBlocks,
		sessions:  make(map[LiveID]*dataSession),
	}, nil
}

// Reserve admits one authenticated ticket/session and binds its currently
// learned DTLS plaintext peer. It deliberately does not activate data until
// immutable LINK_INIT/LINK_ACCEPT has selected the per-association LinkConfig.
func (d *DataPlane) Reserve(account string, id LiveID, peer string, now time.Time) error {
	if err := d.registry.Add(account, id, now); err != nil {
		return err
	}
	if err := d.registry.BindPeer(id, peer); err != nil {
		d.registry.Remove(id)
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.sessions[id] = &dataSession{}
	return nil
}

// Activate installs exactly one immutable data path for the reserved session.
// A second activation is rejected; changing FEC/MTU/lane parameters requires a
// fresh association and therefore a fresh LiveID reservation.
func (d *DataPlane) Activate(id LiveID, cfg control.LinkConfig) error {
	p, err := linkdata.New(cfg, d.maxBlocks)
	if err != nil {
		return err
	}
	d.mu.RLock()
	s, ok := d.sessions[id]
	d.mu.RUnlock()
	if !ok {
		return ErrSessionNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path != nil {
		return ErrSessionAlreadyActive
	}
	s.path = p
	return nil
}

func (d *DataPlane) Entry(id LiveID) (LiveEntry, bool) {
	return d.registry.Get(id)
}

func (d *DataPlane) EntryByPeer(peer string) (LiveEntry, bool) {
	return d.registry.GetByPeer(peer)
}

func (d *DataPlane) Len() int { return d.registry.Len() }

// Inbound routes one already-authenticated DTLS plaintext datagram by peer and
// decodes it only through that session's immutable path.
func (d *DataPlane) Inbound(peer string, wire []byte) (LiveID, [][]byte, error) {
	e, ok := d.registry.GetByPeer(peer)
	if !ok {
		return LiveID{}, nil, ErrSessionNotFound
	}
	s, err := d.session(e.ID)
	if err != nil {
		return LiveID{}, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == nil {
		return LiveID{}, nil, ErrSessionInactive
	}
	packets, err := s.path.Decode(wire)
	return e.ID, packets, err
}

// Outbound encodes one service/VPN packet for exactly one LiveID and returns
// the currently bound DTLS plaintext peer. Username never participates in this
// lookup, so one shared account can have many simultaneous independent flows.
func (d *DataPlane) Outbound(id LiveID, packet []byte, now time.Time) (string, [][]byte, error) {
	e, ok := d.registry.Get(id)
	if !ok || e.Peer == "" {
		return "", nil, ErrSessionNotFound
	}
	s, err := d.session(id)
	if err != nil {
		return "", nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == nil {
		return "", nil, ErrSessionInactive
	}
	wire, err := s.path.Encode(packet, now)
	return e.Peer, wire, err
}

func (d *DataPlane) FlushDue(id LiveID, now time.Time) (string, [][]byte, error) {
	e, ok := d.registry.Get(id)
	if !ok || e.Peer == "" {
		return "", nil, ErrSessionNotFound
	}
	s, err := d.session(id)
	if err != nil {
		return "", nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == nil {
		return "", nil, ErrSessionInactive
	}
	wire, err := s.path.FlushDue(now)
	return e.Peer, wire, err
}

func (d *DataPlane) Flush(id LiveID) (string, [][]byte, error) {
	e, ok := d.registry.Get(id)
	if !ok || e.Peer == "" {
		return "", nil, ErrSessionNotFound
	}
	s, err := d.session(id)
	if err != nil {
		return "", nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.path == nil {
		return "", nil, ErrSessionInactive
	}
	wire, err := s.path.Flush()
	return e.Peer, wire, err
}

// Remove drops both identity and peer-route state. The caller owns socket/path
// shutdown ordering; no other shared-account session is touched.
func (d *DataPlane) Remove(id LiveID) bool {
	d.mu.Lock()
	delete(d.sessions, id)
	d.mu.Unlock()
	return d.registry.Remove(id)
}

func (d *DataPlane) session(id LiveID) (*dataSession, error) {
	d.mu.RLock()
	s, ok := d.sessions[id]
	d.mu.RUnlock()
	if !ok {
		return nil, ErrSessionNotFound
	}
	return s, nil
}
