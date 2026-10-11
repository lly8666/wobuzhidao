package runtimeowner

import (
    "errors"
    "time"

    "github.com/lly8666/wobuzhidao/internal/adaptivefec"
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
    pair qualityPairCandidate // most recent, not maturity timer
    pending [datapath.QualityTrackedReceiveCapacity]qualityPairCandidate
    lastMatureEchoSeq uint64
    sentSources [qualitySentHistoryCapacity]qualitySentSource
    returned qualityReturnedEcho
    // Only protected Normal auto/aggressive V3 admission installs this
    // strictly per-incarnation LOCAL TX controller. Fixed/off and Game nil.
    // The current Tick commits accepted proposals only to NEW FEC blocks.
    n2 *adaptivefec.Controller
    n2Last adaptivefec.Decision
    n2HasDecision bool
    n2DecisionAt time.Time
    n2DecisionVersion uint64 // incremented on every newly observed echo, including HOLD
    n2Evidence N2SourceEvidenceDiagnostic // most recently evaluated echo/source, scalar only
    n2Pending bool // one authenticated rise/lower proposal may apply only once
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
 maturePair,matureReady:=t.quality.maturePair(now,t.quality.lastMatureEchoSeq,false)
 echoPair:=pair
 if matureReady {echoPair=maturePair}
 // Copy only the scalar decision state, not the 16-report authenticated
 // source history or mailbox pointer on every eligible maintenance Tick.
 // No new business hot-path work, allocation, or lock is introduced.
 lastWindow:=t.quality.lastSentWindow
 lastSentAt:=t.quality.lastSentAt
 lastEchoState:=t.quality.lastEchoState
 lastEchoSourcePN:=t.quality.lastEchoSourcePN
 lastEchoSourceTx:=t.quality.lastEchoSourceTx
 lastEchoMissing:=t.quality.lastEchoMissing
 sentCount:=t.quality.sent
 srtt:=t.srtt
 idleSource:=t.health.idleSource
 t.mu.Unlock()
 var estimate QualityInboundEstimate
 if echoPair.accepted {
  var rx datapath.QualityReceiveRange
  var queryErr error
  if matureReady {rx,queryErr=t.owner.QualityTrackedReceiveRange(t.ref,echoPair.report.ReportSeq,now)
  }else{rx,queryErr=t.owner.QualityReceiveRange(t.ref,echoPair.report.WindowFirstPN,echoPair.report.WindowLastPN,now)}
  if queryErr!=nil{return false,queryErr}
  estimate=evaluateQualityPair(echoPair,rx,now)
 }
 // A peer's growing *report sequence alone* is not new evidence. Compare
 // the true sender DATA window and our independently paired receiver state.
 // Stable values without new observations are not repeatedly stamped fresh.
 freshTX:=tx.HasAttempts&&(tx.LastPN!=lastWindow.LastPN||
  tx.Success!=lastWindow.Success||
  tx.LocalDrops!=lastWindow.LocalDrops||
  tx.CapacityLimited!=lastWindow.CapacityLimited)
 sourceChanged:=pair.accepted&&(pair.report.WindowLastPN!=lastEchoSourcePN||
  pair.report.TxSuccess!=lastEchoSourceTx)
 echoChanged:=echoPair.accepted&&(estimate.State!=lastEchoState||
  estimate.EstimatedMissing!=lastEchoMissing)
 // Stable busy-lane values may need a bounded freshness update, but a
 // genuinely idle source's stale PN window must NOT become a new sample.
 busy:=tx.Success>0&&tx.AgeMillis!=0xffff&&tx.AgeMillis<3000
 deadline:=busy&&!lastSentAt.IsZero()&&now.Sub(lastSentAt)>=3500*time.Millisecond
 bootstrap:=sentCount==0
 if !bootstrap&&!freshTX&&!sourceChanged&&!echoChanged&&!matureReady&&!deadline {
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
 if echoPair.accepted {
  echo:=datapath.QualityEchoV4{
   Report:report,State:estimate.State,
   SourceReportSeq:echoPair.report.ReportSeq,
   SourceGeneration:echoPair.report.Generation,
   SourceLastPN:echoPair.report.WindowLastPN,
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
 if echoPair.accepted {
  t.quality.lastEchoSourcePN=echoPair.report.WindowLastPN
  t.quality.lastEchoSourceTx=echoPair.report.TxSuccess
 }
 if matureReady {
  t.quality.lastMatureEchoSeq=echoPair.report.ReportSeq
  // Only an actually emitted encrypted mature echo advances these counters.
  // Business SendNormal/KindLINK never examines or waits for them.
  switch estimate.State {
  case datapath.QualityFeedbackEstimated:
   t.control.MatureEchoEstimated++
   t.control.MatureEchoLastSourceSeq=echoPair.report.ReportSeq
   t.control.MatureEchoLastSenderSuccess=estimate.SenderSuccess
   t.control.MatureEchoLastReceiverUnique=estimate.ReceiverUnique
   t.control.MatureEchoLastDeficit=estimate.EstimatedMissing
   t.control.MatureEchoLastLate=uint64(estimate.ReceiverLate)
   t.control.MatureEchoLastDuplicate=uint64(estimate.ReceiverDuplicate)
   t.control.MatureEchoLastAgeMillis=uint64(estimate.Age/time.Millisecond)
  case datapath.QualityFeedbackCapacity:
   t.control.MatureEchoCapacity++
  case datapath.QualityFeedbackStale:
   t.control.MatureEchoStale++
  default:
   t.control.MatureEchoInsufficient++
  }
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
                marker:=result.QualityControlPN[i]
                t.quality.pair.accept(report,marker,now)
                if report.Flags&datapath.QualityFlagWindowValid!=0 {
                    var closed qualityPairCandidate
                    closed.accept(report,marker,now)
                    // Authenticated control association only, no LINK gate.
                    _=t.owner.QualityTrackReceiveRange(t.ref,report.ReportSeq,report.WindowFirstPN,report.WindowLastPN,now)
                    t.quality.pending[report.ReportSeq%datapath.QualityTrackedReceiveCapacity]=closed
                }
            }
            for _,candidate:=range result.QualityEcho {
                if candidate.Index==i {
                    if !t.quality.acceptReturnedEcho(candidate.Echo,now,t.ref.Generation) {
                        t.quality.rejected++
                    } else {
                        // Only the LOCAL sender's authenticated matured
                        // returned source enters its LOCAL TX controller.
                        // Incoming receiver quality cannot choose S2C/C2S
                        // parity. This does not send, change or wake anything.
                        t.quality.n2ObserveAuthenticatedReturn(t.ref.Generation,now)
                        t.n2PendingSignal.Store(t.quality.n2Pending)
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
