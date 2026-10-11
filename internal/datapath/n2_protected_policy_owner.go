package datapath

import (
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
	"github.com/lly8666/wobuzhidao/internal/realityfront"
)

// N2VerifyActiveProtectedPolicyForTests proves the exact per-client V3
// policy was immutably bound to this ACTIVE Normal Lane before its local
// quality controller is installed. It never grants actual SupportedNow
// admission, and never confuses two customers/rotations with equal lane ID.
func (o *TunnelOwner) N2VerifyActiveProtectedPolicyForTests(ref logicaltunnel.LaneRef,p realityfront.AdmissionPolicy) error {
	if o==nil{return ErrTunnelOwnerClosed}
	o.mu.Lock()
	if o.closed {o.mu.Unlock();return ErrTunnelOwnerClosed}
	if o.desired!=1 {o.mu.Unlock();return ErrNormalLaneMode}
	b,ok:=o.active[ref.ID]
	if !ok {o.mu.Unlock();return ErrLaneUnavailable}
	if b.ref!=ref {current:=b.ref;o.mu.Unlock();return staleGeneration(ref,current)}
	lane:=b.lane
	o.mu.Unlock()
	actual,valid:=lane.N2ProtectedPolicyForTests()
	if !valid || actual!=p {return ErrTunnelMismatch}
	return o.ValidateGeneration(ref)
}
