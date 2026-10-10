package runtimeowner

import (
    "errors"
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

const QualityReportInterval = 2 * time.Second

type qualityState struct {
    enabled bool
    nonce [16]byte
    nextSend time.Time
    reportSeq uint64
    mailbox *datapath.QualityFeedbackMailbox
    tx qualityTXWindow // guarded by existing laneTransport.mu
    received uint64
    rejected uint64
    sent uint64
}

// ConfigureQualityV3 is used only after authenticated V3 protected admission.
// It never enables quality on old V2 connections. The owner checks the lane's
// original handshake-incarnation nonce and current active generation before
// v2 health decoding or sending is allowed.
func (r *Runtime) ConfigureQualityV3(ref logicaltunnel.LaneRef, nonce [16]byte) error {
    if r == nil {return ErrRuntimeClosed}
    mailbox,err:=datapath.NewQualityFeedbackMailbox(ref,nonce)
    if err!=nil {return err}
    r.mu.Lock()
    lane:=r.lanes[ref]
    active:=!r.closed && r.active[ref.ID]==ref
    r.mu.Unlock()
    if lane==nil || !active {return ErrTransportMissing}
    // Make a new mailbox for every new incarnation, never copy quality
    // state from a retiring lane or preserve a stale estimate on rotation.
    lane.mu.Lock()
    if lane.closed {lane.mu.Unlock();return ErrRuntimeClosed}
    if lane.quality.enabled {lane.mu.Unlock();return ErrTransportConfig}
    lane.mu.Unlock()
    if err:=r.owner.EnableQualityHealthV3(ref,nonce);err!=nil{return err}
    lane.mu.Lock()
    lane.quality=qualityState{enabled:true,nonce:nonce,mailbox:mailbox}
    lane.mu.Unlock()
    return nil
}

// tickQuality is invoked only in the Runtime.Tick authoritative ACTIVE lane
// branch, not a timer created per lane or a data-path callback. A failed send
// consumes the bounded 2s slot, rather than retrying control in a hot loop.
func (t *laneTransport) tickQuality(now time.Time) (err error) {
    defer func() {
        if errors.Is(err,datapath.ErrLaneUnavailable) ||
            errors.Is(err,logicaltunnel.ErrStaleLaneGeneration) ||
            errors.Is(err,ErrRuntimeClosed) || errors.Is(err,ErrTransportWriteClosed) ||
            errors.Is(err,ErrTransportPeerReset) {err=nil}
    }()
    t.mu.Lock()
    if !t.quality.enabled || t.closed || t.localFINQueued ||
        (t.health.waitForPeer && t.stats.AuthenticatedRecords==0) ||
        (!t.quality.nextSend.IsZero() && now.Before(t.quality.nextSend)) {
        t.mu.Unlock();return nil
    }
    t.quality.nextSend=now.Add(QualityReportInterval)
    if t.quality.reportSeq==^uint64(0) {
        t.mu.Unlock()
        return nil // never wrap report sequence within incarnation
    }
    t.quality.reportSeq++
    seq:=t.quality.reportSeq
    nonce:=t.quality.nonce
    idleSource:=t.health.idleSource
    srtt:=t.srtt
    txWindow:=t.quality.tx.snapshot(now) // only once/2s, not hot packet path
    t.mu.Unlock()

    idle:=time.Duration(0)
    if idleSource!=nil {idle=idleSource(now)}
    if idle<0 {idle=0}
    if idle>datapath.MaxHealthIdle {idle=datapath.MaxHealthIdle}
    idle=idle.Truncate(time.Millisecond)
    report:=datapath.QualityHealthReport{
        Flags:datapath.QualityFlagInsufficient,
        IdleFor:idle,
        ReportSeq:seq,
        IncarnationNonce:nonce,
        Generation:t.ref.Generation,
        AgeMillis:0xffff, RTTMillis:0xffff,
    }
    // DATA-only PN window describes actual post-Emit successful fresh
    // records and local failures, not gaps between PNs (which may be control
    // or failed seals). The peer's matching receive window is not yet
    // aligned, so LossEstimated is NEVER set in this N1 increment.
    if txWindow.Success>0 {
        report.Flags|=datapath.QualityFlagWindowValid
        report.WindowFirstPN=txWindow.FirstPN
        report.WindowLastPN=txWindow.LastPN
        report.TxSuccess=txWindow.Success
        report.AgeMillis=txWindow.AgeMillis
    }
    report.LocalDrops=txWindow.LocalDrops
    if txWindow.CapacityLimited { report.Flags|=datapath.QualityFlagCapacityLimited }
    // SRTT is only a transport estimate, not application p99. Until the
    // matured sender/receiver DATA watermarks are aligned, *never* set the
    // window-valid/estimated-loss flags or emit an apparent zero WAN loss.
    if srtt>0 && srtt/time.Millisecond<0xffff {
        report.RTTMillis=uint16(srtt/time.Millisecond)
    }
    rec,err:=t.owner.QualityHealthRecord(t.ref,report)
    if err!=nil{return err}
    if err=t.owner.ValidateGeneration(t.ref);err!=nil{return err}
    if err=t.send([]datapath.WireRecord{rec},now);err!=nil{return err}
    t.mu.Lock()
    t.quality.sent++
    t.mu.Unlock()
    return nil
}

func (t *laneTransport) observeQualityLocked(result datapath.InboundResult, now time.Time) {
    mailbox:=t.quality.mailbox
    if !t.quality.enabled || mailbox==nil {return}
    for _,report:=range result.Quality {
        if err:=mailbox.AcceptDecoded(t.ref,report,now);err!=nil {
            t.quality.rejected++
        }else {
            t.quality.received++
        }
    }
}

// QualityFeedback is a constant-time snapshot of the most recent
// authenticated peer report on this exact generation, never a live query of
// packet buffers. The caller can extend stale threshold using 4*SRTT.
func (r *Runtime) QualityFeedback(ref logicaltunnel.LaneRef, now time.Time, staleAfter time.Duration) (datapath.QualityHealthReport,datapath.QualityFeedbackState,bool) {
    if r==nil {return datapath.QualityHealthReport{},datapath.QualityFeedbackUnknown,false}
    r.mu.Lock()
    lane:=r.lanes[ref]
    active:=!r.closed && r.active[ref.ID]==ref
    r.mu.Unlock()
    if lane==nil || !active {return datapath.QualityHealthReport{},datapath.QualityFeedbackUnknown,false}
    lane.mu.Lock()
    mailbox:=lane.quality.mailbox
    enabled:=lane.quality.enabled
    lane.mu.Unlock()
    if !enabled || mailbox==nil {return datapath.QualityHealthReport{},datapath.QualityFeedbackUnknown,false}
    snapshot,state:=mailbox.Snapshot(now,staleAfter)
    return snapshot,state,true
}
