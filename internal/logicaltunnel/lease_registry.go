package logicaltunnel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"math/big"
	"net/netip"
	"sync"
	"time"
)

const AutomaticLeaseTTL = 7 * 24 * time.Hour

// Device IDs distinguish installations sharing credentials. They are not secrets.
func DerivedTunnelID(account string, installation InstallationID) TunnelID {
	h := sha256.Sum256(append([]byte("wbd-installation-v1\x00"+account+"\x00"), installation[:]...))
	var id TunnelID
	copy(id[:], h[:16])
	return id
}

type timedLease struct {
	lease   Lease
	expires time.Time
}

// LeaseRegistry is memory-only. Restart deliberately loses address assignments.
// BeforeExpire must atomically detach an inactive runtime owner; a busy owner
// pins its address past the deadline. It runs only on authenticated admission.
type LeaseRegistry struct {
	mu           sync.Mutex
	pool         netip.Prefix
	max          int
	byID         map[TunnelID]timedLease
	used         map[netip.Addr]TunnelID
	BeforeExpire func(TunnelID) bool
	clock        func() time.Time
}

func NewLeaseRegistry(pool netip.Prefix, max int) (*LeaseRegistry, error) {
	if !pool.IsValid() || !pool.Addr().Is4() || pool.Bits() > 30 || max < 1 || max > 4096 {
		return nil, ErrInvalidPool
	}
	return &LeaseRegistry{pool: pool.Masked(), max: max, byID: map[TunnelID]timedLease{}, used: map[netip.Addr]TunnelID{}, clock: time.Now}, nil
}

func (s *LeaseRegistry) Acquire(account string, installation InstallationID, requested TunnelID) (Lease, error) {
	if _, err := identityKey(account, installation); err != nil || installation == (InstallationID{}) || requested != DerivedTunnelID(account, installation) {
		return Lease{}, ErrInvalidIdentity
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock()
	// Same installation renews its lease; its in-flight packets retain their address.
	if existing, ok := s.byID[requested]; ok {
		existing.expires = now.Add(AutomaticLeaseTTL)
		s.byID[requested] = existing
		return existing.lease.Clone(), nil
	}
	for id, existing := range s.byID {
		if now.Before(existing.expires) {
			continue
		}
		if s.BeforeExpire != nil && !s.BeforeExpire(id) {
			continue
		}
		addr, _ := existing.lease.Config.LeaseIPv4()
		delete(s.used, addr)
		delete(s.byID, id)
	}
	if len(s.byID) >= s.max {
		return Lease{}, ErrPoolExhausted
	}
	base := s.pool.Addr().As4()
	total := uint64(1) << uint(32-s.pool.Bits())
	offset, err := rand.Int(rand.Reader, new(big.Int).SetUint64(total-2))
	if err != nil {
		return Lease{}, err
	}
	n := offset.Uint64() + 1
	var address netip.Addr
	for attempts := 0; attempts <= len(s.used); attempts++ {
		var raw [4]byte
		binary.BigEndian.PutUint32(raw[:], binary.BigEndian.Uint32(base[:])+uint32(n))
		candidate := netip.AddrFrom4(raw)
		if _, used := s.used[candidate]; !used && !candidate.IsUnspecified() && !candidate.IsMulticast() {
			address = candidate
			break
		}
		n++
		if n >= total-1 {
			n = 1
		}
	}
	if !address.IsValid() {
		return Lease{}, ErrPoolExhausted
	}
	lease := Lease{Account: account, InstallationID: installation, Config: TunnelConfig{TunnelID: requested, Address4: netip.PrefixFrom(address, 32).String(), Routes4: []string{"0.0.0.0/0"}}}
	s.byID[requested] = timedLease{lease: lease, expires: now.Add(AutomaticLeaseTTL)}
	s.used[address] = requested
	return lease.Clone(), nil
}

func (s *LeaseRegistry) Lookup(id TunnelID) (Lease, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lease, ok := s.byID[id]
	if !ok {
		return Lease{}, ErrUnknownTunnel
	}
	return lease.lease.Clone(), nil
}
