package runtimeowner

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// ApplyN2ProposalForTests explicitly closes the test-only N2 observation
// loop. The caller is not a product policy or background timer; it can only
// act on an accepted RAISE/LOWER from THIS active TX incarnation. The outer
// outbound RLock is shared with fresh sending, but excludes promotion until
// the short BlockID-committing switch is complete. Encoder cold preparation
// itself occurs outside Lane.mu and TunnelOwner.mu. No ACK/repair wait.
func (r *Runtime) ApplyN2ProposalForTests(ref logicaltunnel.LaneRef,now time.Time) (bool,error) {
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
	// A returned echo can vanish for many seconds without a fresh one.
	// Never apply an indefinitely cached Raise/Lower after feedback loss,
	// or spend one accepted source report to change profiles repeatedly.
	valid:=!t.closed && t.quality.enabled && t.quality.n2!=nil &&
		t.quality.n2HasDecision && t.quality.n2Pending &&
		!now.Before(t.quality.n2DecisionAt) &&
		now.Sub(t.quality.n2DecisionAt)<qualityEchoStaleAfter &&
		(t.quality.n2Last.Reason==adaptivefec.Raise ||
		 t.quality.n2Last.Reason==adaptivefec.Lower)
	target:=t.quality.n2Last.Applied
	if t.quality.n2Pending {
		t.quality.n2Pending=false // a single attempt, even when budget refuses
	}
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
