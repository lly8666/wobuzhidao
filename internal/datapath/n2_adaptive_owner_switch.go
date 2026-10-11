package datapath

import (
	"github.com/lly8666/wobuzhidao/internal/linkdata"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// N2PreparedActiveSwitch is a single-use, lane-bound cold allocation. It is
// safe to prepare while promotion/normal outbound happens concurrently.
// ONLY a generation-pinning Runtime may call CommitN2ActiveParityForTests.
type N2PreparedActiveSwitch struct {
	ref logicaltunnel.LaneRef
	lane *Lane
	prepared *linkdata.PreparedParitySwitch
}

func (o *TunnelOwner) n2ActiveLane(ref logicaltunnel.LaneRef) (*Lane,error) {
	if o==nil{return nil,ErrTunnelOwnerClosed}
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closed{return nil,ErrTunnelOwnerClosed}
	if o.desired!=1{return nil,ErrNormalLaneMode}
	b,ok:=o.active[ref.ID]
	if !ok{return nil,ErrLaneUnavailable}
	if b.ref!=ref{return nil,staleGeneration(ref,b.ref)}
	return b.lane,nil
}

// PrepareN2ActiveParityForTests never holds generation, owner or TX locks
// across cold codec allocation. It cannot block fresh packets or prolong a
// pending promotion's writer lock. A stale preparation may be discarded.
func (o *TunnelOwner) PrepareN2ActiveParityForTests(
	ref logicaltunnel.LaneRef, parity int,
) (*N2PreparedActiveSwitch,error) {
	lane,err:=o.n2ActiveLane(ref)
	if err!=nil{return nil,err}
	cold,err:=lane.PrepareN2ParityForTests(parity)
	if err!=nil{return nil,err}
	return &N2PreparedActiveSwitch{ref:ref,lane:lane,prepared:cold},nil
}

// CommitN2ActiveParityForTests must be called with the owning Runtime's
// outboundMu.RLock held. Its ref and actual Lane pointer must both remain
// authoritative. The prepared handle is internally one-use; rejected old
// generation blocks never mutate a new encoder or reassign BlockIDs.
func (o *TunnelOwner) CommitN2ActiveParityForTests(ref logicaltunnel.LaneRef,
	pending *N2PreparedActiveSwitch) error {
	if pending==nil || pending.ref!=ref {return ErrLaneUnavailable}
	lane,err:=o.n2ActiveLane(ref)
	if err!=nil{return err}
	if lane!=pending.lane {return ErrLaneUnavailable}
	return lane.CommitN2ParityForTests(pending.prepared)
}

// Old explicit single-operation seam retains compatibility with existing
// tests. The Runtime controller uses the split prepare / pinned commit API.
func (o *TunnelOwner) SwitchN2ActiveParityForTests(ref logicaltunnel.LaneRef,parity int) error {
	p,err:=o.PrepareN2ActiveParityForTests(ref,parity)
	if err!=nil{return err}
	return o.CommitN2ActiveParityForTests(ref,p)
}
