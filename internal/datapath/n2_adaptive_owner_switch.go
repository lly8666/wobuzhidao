package datapath

import (
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// SwitchN2ActiveParityForTests is an opt-in N2 integration seam; production
// admission still cannot construct auto/aggressive lanes. Its Runtime caller
// MUST hold outboundMu.RLock for the entire operation, pinning promotion.
// Owner.mu is released before cold encoder preparation: fresh traffic never
// waits for the FEC change preparation under the tunnel-wide owner mutex.
// The final Lane commit is short; any outgoing record remains generation-
// fenced by the existing owner.FenceOutbound.
func (o *TunnelOwner) SwitchN2ActiveParityForTests(ref logicaltunnel.LaneRef,parity int) error {
	if o==nil {return ErrTunnelOwnerClosed}
	o.mu.Lock()
	if o.closed {o.mu.Unlock();return ErrTunnelOwnerClosed}
	if o.desired!=1 {o.mu.Unlock();return ErrNormalLaneMode}
	binding,ok:=o.active[ref.ID]
	if !ok {o.mu.Unlock();return ErrLaneUnavailable}
	if binding.ref!=ref {
		current:=binding.ref
		o.mu.Unlock()
		return staleGeneration(ref,current)
	}
	lane:=binding.lane
	o.mu.Unlock()
	return lane.SwitchN2ParityForTests(parity)
}
