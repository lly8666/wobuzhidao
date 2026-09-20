package datapath

import "github.com/lly8666/wobuzhidao/internal/logicaltunnel"

// NewLeasedTunnelOwner binds transport membership to one stable Logical Tunnel
// lease before any Lane incarnation is admitted. Lane replacement, DORMANT and
// wake may change transport generations but cannot change this lease identity.
func NewLeasedTunnelOwner(lease logicaltunnel.Lease, desiredLanes, maxFlows int) (*TunnelOwner, error) {
	if err := lease.Validate(); err != nil {
		return nil, err
	}
	owner, err := NewTunnelOwner(desiredLanes, maxFlows)
	if err != nil {
		return nil, err
	}
	owner.mu.Lock()
	owner.lease = lease.Clone()
	owner.hasLease = true
	owner.mu.Unlock()
	return owner, nil
}

// Lease returns an owned copy of the stable Logical Tunnel lease. An unleased
// test/adapter owner reports ok=false.
func (o *TunnelOwner) Lease() (logicaltunnel.Lease, bool) {
	if o == nil {
		return logicaltunnel.Lease{}, false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.hasLease {
		return logicaltunnel.Lease{}, false
	}
	return o.lease.Clone(), true
}
