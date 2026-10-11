package runtimeowner

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// ApplyN2ProposalForTests consumes one fresh authenticated local-TX proposal.
// Cold encoder preparation has NO Runtime outbound generation read lock and
// NO Lane/owner locks. Hence a simultaneous promotion writer never turns
// cold allocation into a fresh-packet HOL barrier. The bounded short commit
// alone is generation-pinned; any new quality HOLD/damage update supersedes
// the proposal before commit. No timer/ACK/repair dependence.
func (r *Runtime) ApplyN2ProposalForTests(ref logicaltunnel.LaneRef,now time.Time) (bool,error) {
	if r==nil {return false,ErrRuntimeClosed}

	r.mu.Lock()
	t:=r.lanes[ref]
	if r.closed || r.active[ref.ID]!=ref || t==nil {
		r.mu.Unlock()
		return false,ErrTransportMissing
	}
	t.mu.Lock()
	valid:=!t.closed && t.quality.enabled && t.quality.n2!=nil &&
		t.quality.n2HasDecision && t.quality.n2Pending &&
		!now.Before(t.quality.n2DecisionAt) &&
		now.Sub(t.quality.n2DecisionAt)<qualityEchoStaleAfter &&
		(t.quality.n2Last.Reason==adaptivefec.Raise ||
		 t.quality.n2Last.Reason==adaptivefec.Lower)
	target:=t.quality.n2Last.Applied
	version:=t.quality.n2DecisionVersion
	if t.quality.n2Pending {
		t.quality.n2Pending=false // exactly one attempt, even on busy/noop
		t.n2PendingSignal.Store(false)
	}
	t.mu.Unlock()
	r.mu.Unlock()
	if !valid {return false,nil}
	if stats,ok:=r.owner.LaneStats(ref);!ok {return false,ErrTransportMissing
	}else if stats.TxPath.ParityShards==target {return false,nil}

	// This is the potentially expensive memory allocation/geometry setup.
	// It is deliberately OUTSIDE outboundMu.RLock. A retired lane's cold
	// object cannot commit to a replacement and will simply be discarded.
	prepared,err:=r.owner.PrepareN2ActiveParityForTests(ref,target)
	if err!=nil{return false,err}
	r.outboundMu.RLock()
	r.mu.Lock()
	if r.closed || r.active[ref.ID]!=ref || r.lanes[ref]!=t {
		r.mu.Unlock()
		r.outboundMu.RUnlock()
		return false,ErrTransportMissing
	}
	t.mu.Lock()
	r.mu.Unlock()
	// An authenticated newer HOLD or new loss interval beats an old ready
	// encoder, even if it arrived while cold preparation was in progress.
	if t.closed || !t.quality.enabled || t.quality.n2==nil ||
		t.quality.n2DecisionVersion!=version {
		t.mu.Unlock()
		r.outboundMu.RUnlock()
		return false,nil
	}
	err=r.owner.CommitN2ActiveParityForTests(ref,prepared)
	t.mu.Unlock()
	r.outboundMu.RUnlock()
	if err!=nil{return false,err}
	return true,nil
}
