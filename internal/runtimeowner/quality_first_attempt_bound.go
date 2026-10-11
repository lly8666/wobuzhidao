package runtimeowner

import (
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// We cannot tell the two transmissions apart on the receiver: exact original
// ciphertext/Seq is intentionally reused for every repair. However, a source
// that *was* received after a missing original must have been retried, while
// a source still missing after the receiver's mature watermark contributes one
// loss either way. Therefore:
//   mature_missing <= first_attempt_losses <= mature_missing + distinct_retried
// This is an upper BOUND, never an exact WAN first-attempt measurement.
// The stronger bound requires complete, source-PN-correlated repair tracking.
// No per-record wire fields or new quality control messages are introduced.

type QualityFirstAttemptBound struct {
    State datapath.QualityFeedbackState
    SourceReportSeq uint64
    SourceGeneration uint64
    FirstPN, LastPN uint64
    Sent, ReceiverUnique uint64
    MatureMissing uint64
    DistinctRepaired uint64
    MinFirstLost, MaxFirstLost uint64
    LowerFraction, UpperFraction float64
    Age time.Duration
    Complete bool
    // Metadata is independent of the WAN loss interval. Only actual
    // successful KindLINK first-emits for this SAME source report contribute.
    Shape QualitySourceShape
}

// Caller holds laneTransport.mu; called only for an actual selected repair,
// never for fresh business or on a timer. All attempts are counted BEFORE the
// socket Emit call, even if it later fails; that only widens the safe bound.
func (q *qualityState) noteRepairAttempt(pn uint64) {
    if !q.enabled { return }
    slot:=&q.tx.slots[pn%datapath.QualityPNWindowCapacity]
    if slot.used && slot.pn==pn && slot.success {
        slot.repaired=true
    }
    for i:=range q.sentSources {
        s:=&q.sentSources[i]
        if !s.repairCoverage || s.report.ReportSeq==0 ||
           pn<s.report.WindowFirstPN || pn>s.report.WindowLastPN { continue }
        s.markRepaired(pn)
    }
}

func (s *qualitySentSource) markRepaired(pn uint64) {
    if !s.repairCoverage || pn<s.report.WindowFirstPN || pn>s.report.WindowLastPN {return}
    idx:=pn-s.report.WindowFirstPN
    if idx>=datapath.QualityPNWindowCapacity {return}
    word:=idx/64
    mask:=uint64(1)<<(idx%64)
    if s.repairMask[word]&mask==0 {
        s.repairMask[word]|=mask
        s.repaired++
    }
}

// Called once after successful authenticated control Emit (not per DATA).
// A source window is qualified only when ALL successfully emitted DATA PNs
// in its report are still present in the bounded PN ring. If the traffic
// rotated the ring meanwhile, or local drops occurred, fail closed.
func (q *qualityState) captureSentSourceRepairCoverage(m datapath.QualityHealthReport,now time.Time) qualitySentSource {
    s:=qualitySentSource{report:m,sent:now}
    if m.Flags&datapath.QualityFlagWindowValid==0 ||
       m.Flags&datapath.QualityFlagCapacityLimited!=0 ||
       m.TxSuccess==0 || m.TxSuccess>datapath.QualityPNWindowCapacity ||
       m.LocalDrops!=0 || m.WindowLastPN<m.WindowFirstPN ||
       m.WindowLastPN-m.WindowFirstPN>=datapath.QualityPNWindowCapacity {
        return s
    }
    var successes uint64
    for i:=uint64(0);i<=m.WindowLastPN-m.WindowFirstPN;i++ {
        pn:=m.WindowFirstPN+i
        slot:=q.tx.slots[pn%datapath.QualityPNWindowCapacity]
        if !slot.used || !slot.success || slot.pn!=pn {continue}
        successes++
        if slot.repaired {
            idx:=i/64
            s.repairMask[idx]|=uint64(1)<<(i%64)
            s.repaired++
        }
    }
    s.repairCoverage=successes==m.TxSuccess
    if s.repairCoverage {
        s.shape=q.tx.sourceShape(m)
    } else {
        s.repaired=0
        s.repairMask=[4]uint64{}
    }
    return s
}

// Read-only snapshot derived from an authenticated return echo and an
// independently retained same-incarnation sender report. There is never an
// estimate for stale/insufficient/capacity/overwritten/untracked source data.
func (r *Runtime) QualityFirstAttemptBound(ref logicaltunnel.LaneRef, now time.Time) (QualityFirstAttemptBound,bool) {
    if r==nil {return QualityFirstAttemptBound{},false}
    r.mu.Lock()
    lane:=r.lanes[ref]
    active:=!r.closed && r.active[ref.ID]==ref
    r.mu.Unlock()
    if !active||lane==nil {return QualityFirstAttemptBound{},false}
    lane.mu.Lock()
    enabled:=lane.quality.enabled
    echo:=lane.quality.returned
    var src qualitySentSource
    if echo.valid {
        src=lane.quality.sentSources[echo.report.SourceReportSeq%qualitySentHistoryCapacity]
    }
    lane.mu.Unlock()
    if !enabled {return QualityFirstAttemptBound{},false}
    return qualityFirstAttemptBoundFromSnapshot(echo,src,ref.Generation,now),true
}

// Pure, caller-lock-owned evaluation of the same-source authenticated return.
// The protected RX receive callback can use this without reentering Runtime
// locks, while the public read-only snapshot keeps exactly the same gates.
func qualityFirstAttemptBoundFromSnapshot(echo qualityReturnedEcho,src qualitySentSource,generation uint64,now time.Time) QualityFirstAttemptBound {
    out:=QualityFirstAttemptBound{State:datapath.QualityFeedbackUnknown}
    if !echo.valid {return out}
    e:=echo.report
    out.SourceReportSeq=e.SourceReportSeq
    out.SourceGeneration=e.SourceGeneration
    if now.Before(echo.received)||!now.Before(echo.received.Add(qualityEchoStaleAfter)) ||
       now.Before(src.sent)||!now.Before(src.sent.Add(qualityEchoStaleAfter)) {
        out.State=datapath.QualityFeedbackStale
        return out
    }
    out.Age=now.Sub(echo.received)
    if e.State!=datapath.QualityFeedbackEstimated {
        out.State=e.State
        return out
    }
    m:=src.report
    if m.ReportSeq!=e.SourceReportSeq || m.Generation!=generation ||
       m.IncarnationNonce!=e.Report.IncarnationNonce ||
       m.WindowLastPN!=e.SourceLastPN ||
       !src.repairCoverage || m.TxSuccess<qualityPairMinTransmitted ||
       e.ReceiverUnique>m.TxSuccess ||
       uint64(e.EstimatedMissing)!=m.TxSuccess-e.ReceiverUnique {
        out.State=datapath.QualityFeedbackInsufficient
        return out
    }
    out.State=datapath.QualityFeedbackEstimated
    out.FirstPN,out.LastPN=m.WindowFirstPN,m.WindowLastPN
    out.Sent,out.ReceiverUnique=m.TxSuccess,e.ReceiverUnique
    out.MatureMissing=uint64(e.EstimatedMissing)
    out.DistinctRepaired=uint64(src.repaired)
    out.MinFirstLost=out.MatureMissing
    out.MaxFirstLost=out.MatureMissing+out.DistinctRepaired
    if out.MaxFirstLost>out.Sent {out.MaxFirstLost=out.Sent}
    out.LowerFraction=float64(out.MinFirstLost)/float64(out.Sent)
    out.UpperFraction=float64(out.MaxFirstLost)/float64(out.Sent)
    out.Complete=true
    out.Shape=src.shape
    return out
}
