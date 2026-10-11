package runtimeowner

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// n2ObserveAuthenticatedReturn runs ONLY after the regular V3 mailbox and
// same-incarnation source echo gates. No N2 decision is made from this lane's
// RX-side QualityInboundEstimate: this is the LOCAL outbound TX direction.
// Caller holds laneTransport.mu. It stores a bounded proposal, but never
// changes an encoder, sends traffic, or starts a timer.
func (q *qualityState) n2ObserveAuthenticatedReturn(generation uint64,now time.Time) {
	if q==nil || q.n2==nil || !q.enabled || !q.returned.valid {
		return
	}
	e:=q.returned.report
	src:=q.sentSources[e.SourceReportSeq%qualitySentHistoryCapacity]
	bound:=qualityFirstAttemptBoundFromSnapshot(q.returned,src,generation,now)
	input:=qualityBoundToAdaptiveInput(bound,now)
	if q.n2DecisionVersion==^uint64(0) {
		q.n2Pending=false // exhaustion: never reuse an old token
		return
	}
	q.n2DecisionVersion++
	q.n2Last=q.n2.Step(now,input)
	q.n2HasDecision=true
	q.n2DecisionAt=now
	q.n2Pending=q.n2Last.Reason==adaptivefec.Raise ||
		q.n2Last.Reason==adaptivefec.Lower
}

// ConfigureN2ControllerForTests enables ONLY guarded proposal observation
// on one already ACTIVE, quality-enabled, single-Normal-lane incarnation.
// Protected V3 admission still rejects FECAuto/FECAggressive via SupportedNow.
// No product/CLI/GUI entrypoint calls this test seam or switches any FEC mode.
func (r *Runtime) ConfigureN2ControllerForTests(ref logicaltunnel.LaneRef, cfg adaptivefec.Config) error {
	if r==nil {return ErrRuntimeClosed}
	if r.owner.Stats().DesiredLanes!=1 {return ErrTransportConfig}
	c,err:=adaptivefec.New(cfg,ref.Generation)
	if err!=nil {return err}
	r.mu.Lock()
	transport:=r.lanes[ref]
	if r.closed || r.active[ref.ID]!=ref || transport==nil {
		r.mu.Unlock()
		return ErrTransportMissing
	}
	// Lock order remains Runtime.mu -> transport.mu, preventing concurrent
	// promotion from configuring the already retired generation.
	transport.mu.Lock()
	r.mu.Unlock()
	defer transport.mu.Unlock()
	if transport.closed || !transport.quality.enabled || transport.quality.n2!=nil {
		return ErrTransportConfig
	}
	transport.quality.n2=c
	return nil
}

// N2ProposalForTests exposes the latest per-incarnation TX recommendation.
// Nil/unconfigured, sleeping or retired incarnations never leak old advice.
func (r *Runtime) N2ProposalForTests(ref logicaltunnel.LaneRef) (adaptivefec.Decision,bool) {
	if r==nil {return adaptivefec.Decision{},false}
	r.mu.Lock()
	transport:=r.lanes[ref]
	if r.closed || r.active[ref.ID]!=ref || transport==nil {
		r.mu.Unlock()
		return adaptivefec.Decision{},false
	}
	transport.mu.Lock()
	r.mu.Unlock()
	defer transport.mu.Unlock()
	if transport.closed || transport.quality.n2==nil || !transport.quality.n2HasDecision {
		return adaptivefec.Decision{},false
	}
	return transport.quality.n2Last,true
}
