package session

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrRegistryFull     = errors.New("session: registry full")
	ErrDuplicateSession = errors.New("session: duplicate session id")
	ErrInvalidSession   = errors.New("session: invalid account or session id")
	ErrInvalidPeer      = errors.New("session: invalid data peer")
	ErrPeerInUse        = errors.New("session: data peer already belongs to another session")
	ErrSessionNotFound  = errors.New("session: session not found")
)

// LiveID is the server-side identity of one admitted transport session. The
// Reality-like front supplies a random 32-byte one-time ticket; the live table
// deliberately never keys state by username.
type LiveID [32]byte

type LiveEntry struct {
	ID        LiveID
	Account   string
	Peer      string
	CreatedAt time.Time
}

// AccountRegistry is the deliberately small personal-server live-session
// table. One shared account may have many simultaneous session IDs. Session ID
// is the identity key; Peer is only a fast data-plane demux index learned after
// the authenticated DTLS/ticket association is established.
type AccountRegistry struct {
	mu      sync.RWMutex
	maxLive int
	byID    map[LiveID]LiveEntry
	byPeer  map[string]LiveID
}

func NewAccountRegistry(maxLive int) (*AccountRegistry, error) {
	if maxLive <= 0 {
		return nil, ErrRegistryFull
	}
	return &AccountRegistry{
		maxLive: maxLive,
		byID:    make(map[LiveID]LiveEntry),
		byPeer:  make(map[string]LiveID),
	}, nil
}

func (r *AccountRegistry) Add(account string, id LiveID, now time.Time) error {
	if account == "" || id == (LiveID{}) {
		return ErrInvalidSession
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; ok {
		return ErrDuplicateSession
	}
	if len(r.byID) >= r.maxLive {
		return ErrRegistryFull
	}
	r.byID[id] = LiveEntry{ID: id, Account: account, CreatedAt: now}
	return nil
}

// BindPeer attaches the learned DTLS plaintext peer to an already admitted
// session. It never changes account identity. A peer can belong to only one
// live session at a time so one datagram cannot be routed to two devices.
func (r *AccountRegistry) BindPeer(id LiveID, peer string) error {
	if id == (LiveID{}) || peer == "" {
		return ErrInvalidPeer
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok {
		return ErrSessionNotFound
	}
	if other, ok := r.byPeer[peer]; ok && other != id {
		return ErrPeerInUse
	}
	if e.Peer != "" && e.Peer != peer {
		delete(r.byPeer, e.Peer)
	}
	e.Peer = peer
	r.byID[id] = e
	r.byPeer[peer] = id
	return nil
}

func (r *AccountRegistry) Get(id LiveID) (LiveEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byID[id]
	return e, ok
}

// GetByPeer is the hot data-plane lookup. The returned entry is still keyed
// and owned by its LiveID; the peer string is only a routing index.
func (r *AccountRegistry) GetByPeer(peer string) (LiveEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byPeer[peer]
	if !ok {
		return LiveEntry{}, false
	}
	e, ok := r.byID[id]
	return e, ok
}

func (r *AccountRegistry) Remove(id LiveID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.byID[id]
	if !ok {
		return false
	}
	if e.Peer != "" {
		delete(r.byPeer, e.Peer)
	}
	delete(r.byID, id)
	return true
}

func (r *AccountRegistry) Len() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.byID)
}

func (r *AccountRegistry) CountAccount(account string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	n := 0
	for _, e := range r.byID {
		if e.Account == account {
			n++
		}
	}
	return n
}
