package runtimeowner

import (
	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// ApplyN2ProposalForTests explicitly closes the test-only N2 observation
// loop. The caller is not a product policy or background timer; it can only
// act on an accepted RAISE/LOWER from THIS active TX incarnation. The outer
// outbound RLock is shared with fresh sending, but excludes promotion until
// the short BlockID-committing switch is complete. Encoder cold preparation
// itself occurs outside Lane.mu and TunnelOwner.mu. No ACK/repair wait.
func (r *Runtime) ApplyN2ProposalForTests(ref logicaltunnel.LaneRef) (bool,error) {
	if r==nil {return false,ErrRuntimeClosed}
	r.outboundMu.RLock()
	defer r.outboundMu.RUnlock()
	r.mu.Lock()
	t:=r.lanes[ref]
	if r.closed || r.active[ref.ID]!=ref || t==nil {
		r.mu.Unlock()
		return false,ErrTransportMissing
	}
	t.mu.Lock()
	valid:=!t.closed && t.quality.enabled && t.quality.n2!=nil &&
		t.quality.n2HasDecision &&
		(t.quality.n2Last.Reason==adaptivefec.Raise ||
		 t.quality.n2Last.Reason==adaptivefec.Lower)
	target:=t.quality.n2Last.Applied
	t.mu.Unlock()
	r.mu.Unlock()
	if !valid {return false,nil}
	stats,ok:=r.owner.LaneStats(ref)
	if !ok {return false,ErrTransportMissing}
	if stats.TxPath.ParityShards==target {return false,nil}
	if err:=r.owner.SwitchN2ActiveParityForTests(ref,target);err!=nil {
		// A bounded draining-encoder refusal is not a reason to queue,
		// retransmit, or delay fresh business; the test caller sees HOLD.
		return false,err
	}
	return true,nil
}
