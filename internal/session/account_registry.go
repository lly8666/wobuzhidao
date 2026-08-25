package session

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrRegistryFull      = errors.New("session: registry full")
	ErrDuplicateSession  = errors.New("session: duplicate session id")
	ErrInvalidSession    = errors.New("session: invalid account or session id")
)

// LiveID is the server-side identity of one admitted transport session. The
// Reality-like front currently supplies a random 32-byte one-time ticket; the
// session registry deliberately does not key state by username.
type LiveID [32]byte

type LiveEntry struct {
	ID        LiveID
	Account   string
	CreatedAt time.Time
}

// AccountRegistry is intentionally small: one shared account may have many
// simultaneous session IDs. There is no per-device credential/revocation
// database in the personal product path.
type AccountRegistry struct {
	mu      sync.RWMutex
	maxLive int
	byID    map[LiveID]LiveEntry
}

func NewAccountRegistry(maxLive int) (*AccountRegistry, error) {
	if maxLive <= 0 {
		return nil, ErrRegistryFull
	}
	return &AccountRegistry{maxLive: maxLive, byID: make(map[LiveID]LiveEntry)}, nil
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

func (r *AccountRegistry) Get(id LiveID) (LiveEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	e, ok := r.byID[id]
	return e, ok
}

func (r *AccountRegistry) Remove(id LiveID) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byID[id]; !ok {
		return false
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
