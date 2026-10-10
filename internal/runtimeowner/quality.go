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
    pair qualityPairCandidate // authenticated peer TX window + control record PN
    sentSources [qualitySentHistoryCapacity]qualitySentSource
    returned qualityReturnedEcho
    received uint64
    rejected uint64
    sent uint64
    lastSentAt time.Time
    lastSentWindow QualityTransmitWindow
    lastEchoState datapath.QualityFeedbackState
    lastEchoSourcePN uint64
    lastEchoSourceTx uint64
    lastEchoMissing uint64
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

// tickQuality is selected by the one per-lane tickControl arbiter, never
// by a separate timer. It only snapshots O(256) windows at an eligible
// ~2s control decision, never once per packet/Tick.
func (t *laneTransport) tickQuality(now time.Time) (attempted bool,err error) {
 defer func() {
  if errors.Is(err,datapath.ErrLaneUnavailable)||
   errors.Is(err,logicaltunnel.ErrStaleLaneGeneration)||
   errors.Is(err,ErrRuntimeClosed)||
   errors.Is(err,ErrTransportWriteClosed)||
   errors.Is(err,ErrTransportPeerReset){err=nil}
 }()
 t.mu.Lock()
 if !t.quality.enabled||t.closed||t.localFINQueued||
  (t.health.waitForPeer&&t.stats.AuthenticatedRecords==0)||
  now.Before(t.quality.nextSend) {
  t.mu.Unlock();return false,nil
 }
 tx:=t.quality.tx.snapshot(now)
 pair:=t.quality.pair
 last:=t.quality
 srtt:=t.srtt
 idleSource:=t.health.idleSource
 t.mu.Unlock()
 var estimate QualityInboundEstimate
 if pair.accepted {
  rx,queryErr:=t.owner.QualityReceiveRange(t.ref,pair.report.WindowFirstPN,pair.report.WindowLastPN,now)
  if queryErr!=nil{return false,queryErr}
  estimate=evaluateQualityPair(pair,rx,now)
 }
 // A peer's growing *report sequence alone* is not new evidence. Compare
 // the true sender DATA window and our independently paired receiver state.
 // Stable values without new observations are not repeatedly stamped fresh.
 freshTX:=tx.HasAttempts&&(tx.LastPN!=last.lastSentWindow.LastPN||
  tx.Success!=last.lastSentWindow.Success||
  tx.LocalDrops!=last.lastSentWindow.LocalDrops||
  tx.CapacityLimited!=last.lastSentWindow.CapacityLimited)
 sourceChanged:=pair.accepted&&(pair.report.WindowLastPN!=last.lastEchoSourcePN||
  pair.report.TxSuccess!=last.lastEchoSourceTx)
 echoChanged:=pair.accepted&&(estimate.State!=last.lastEchoState||
  estimate.EstimatedMissing!=last.lastEchoMissing)
 // Stable busy-lane values may need a bounded freshness update, but a
 // genuinely idle source's stale PN window must NOT become a new sample.
 busy:=tx.Success>0&&tx.AgeMillis!=0xffff&&tx.AgeMillis<3000
 deadline:=busy&&!last.lastSentAt.IsZero()&&now.Sub(last.lastSentAt)>=3500*time.Millisecond
 bootstrap:=last.sent==0
 if !bootstrap&&!freshTX&&!sourceChanged&&!echoChanged&&!deadline {
  t.mu.Lock()
  t.control.SuppressedNoNewObservation++
  t.quality.nextSend=now.Add(500*time.Millisecond)
  t.mu.Unlock()
  return false,nil
 }
 // Source history/3s maturity and 10s expiry outrank optional cosmetic
 // delay: the next eligible tick gets the mature echo. No zero-wait reply.
 t.mu.Lock()
 if t.quality.reportSeq==^uint64(0) {t.mu.Unlock();return false,nil}
 t.quality.reportSeq++
 seq:=t.quality.reportSeq
 nonce:=t.quality.nonce
 t.quality.nextSend=now.Add(time.Second) // failure backoff; never hot-retry
 if bootstrap {t.control.QualityBootstrap++}
 if freshTX {t.control.QualityNewWindow++}
 if sourceChanged {t.control.QualityEchoChange++}
 if echoChanged {t.control.QualityStatusChange++}
 if deadline {t.control.QualityRefreshDeadline++}
 t.mu.Unlock()
 attempted=true
 idle:=time.Duration(0)
 if idleSource!=nil {idle=idleSource(now)}
 if idle<0{idle=0}
 if idle>datapath.MaxHealthIdle{idle=datapath.MaxHealthIdle}
 idle=idle.Truncate(time.Millisecond)
 report:=datapath.QualityHealthReport{
  Flags:datapath.QualityFlagInsufficient,IdleFor:idle,
  ReportSeq:seq,IncarnationNonce:nonce,Generation:t.ref.Generation,
  AgeMillis:0xffff,RTTMillis:0xffff,
 }
 if tx.Success>0 {
  report.Flags|=datapath.QualityFlagWindowValid
  report.WindowFirstPN=tx.FirstPN
  report.WindowLastPN=tx.LastPN
  report.TxSuccess=tx.Success
  report.AgeMillis=tx.AgeMillis
 }
 report.LocalDrops=tx.LocalDrops
 if tx.CapacityLimited{report.Flags|=datapath.QualityFlagCapacityLimited}
 if srtt>0&&srtt/time.Millisecond<0xffff{report.RTTMillis=uint16(srtt/time.Millisecond)}
 var plain []byte
 if pair.accepted {
  echo:=datapath.QualityEchoV4{
   Report:report,State:estimate.State,
   SourceReportSeq:pair.report.ReportSeq,
   SourceGeneration:pair.report.Generation,
   SourceLastPN:pair.report.WindowLastPN,
  }
  if estimate.State==datapath.QualityFeedbackEstimated {
   echo.ReceiverUnique=estimate.ReceiverUnique
   echo.ReceiverLate=estimate.ReceiverLate
   echo.ReceiverDuplicate=estimate.ReceiverDuplicate
   echo.EstimatedMissing=uint32(estimate.EstimatedMissing)
  }
  var bytes [datapath.QualityEchoV4Size]byte
  bytes,err=datapath.EncodeQualityEchoV4(echo)
  if err==nil {plain=bytes[:]}
 }else{
  var bytes [datapath.QualityHealthV3Size]byte
  bytes,err=datapath.EncodeQualityHealthV3(report)
  if err==nil {plain=bytes[:]}
 }
 if err!=nil {t.noteControlFailure(now,true);return attempted,err}
 rec,err:=t.sealControl(t.ref,plain)
 if err!=nil{t.noteControlFailure(now,true);return attempted,err}
 if err=t.owner.ValidateGeneration(t.ref);err!=nil{t.noteControlFailure(now,true);return attempted,err}
 if err=t.send([]datapath.WireRecord{rec},now);err!=nil{
  t.noteControlFailure(now,true);return attempted,err
 }
 delay:=qualityNextDelay()
 t.noteControlSuccess(now,rec,true)
 t.mu.Lock()
 t.quality.sent++
 t.quality.recordSentSource(report,now)
 t.quality.lastSentAt=now
 t.quality.lastSentWindow=tx
 t.quality.lastEchoState=estimate.State
 if pair.accepted {
  t.quality.lastEchoSourcePN=pair.report.WindowLastPN
  t.quality.lastEchoSourceTx=pair.report.TxSuccess
 }
 t.quality.lastEchoMissing=estimate.EstimatedMissing
 t.quality.nextSend=now.Add(delay)
 if estimate.Age>0{t.control.FeedbackAgeMillis=uint64(estimate.Age/time.Millisecond)}
 t.mu.Unlock()
 return attempted,nil
}

func (t *laneTransport) observeQualityLocked(result datapath.InboundResult, now time.Time) {
    mailbox:=t.quality.mailbox
    if !t.quality.enabled || mailbox==nil {return}
    for i,report:=range result.Quality {
        if err:=mailbox.AcceptDecoded(t.ref,report,now);err!=nil {
            t.quality.rejected++
        }else {
            t.quality.received++
            if i<len(result.QualityControlPN) {
                t.quality.pair.accept(report,result.QualityControlPN[i],now)
            }
            for _,candidate:=range result.QualityEcho {
                if candidate.Index==i {
                    if !t.quality.acceptReturnedEcho(candidate.Echo,now,t.ref.Generation) {
                        t.quality.rejected++
                    }
                    break
                }
            }
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
