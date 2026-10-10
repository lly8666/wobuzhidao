package runtimeowner

import (
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

const qualitySentHistoryCapacity=16
const qualityEchoStaleAfter=10*time.Second

type qualitySentSource struct {
    report datapath.QualityHealthReport
    sent time.Time
}
type qualityReturnedEcho struct {
    report datapath.QualityEchoV4
    received time.Time
    valid bool
}

// QualityEchoFeedback is a read-only, bounded, authenticated RETURN path.
// It never touches an automatic FEC decision (N2 remains disabled).
type QualityEchoFeedback struct {
    State datapath.QualityFeedbackState
    SourceReportSeq uint64
    SourceGeneration uint64
    SourceLastPN uint64
    ReceiverUnique uint64
    EstimatedMissing uint32
    Fraction float64
    Age time.Duration
    Valid bool
}

func (q *qualityState) recordSentSource(m datapath.QualityHealthReport,now time.Time) {
    q.sentSources[m.ReportSeq%qualitySentHistoryCapacity]=qualitySentSource{report:m,sent:now}
}

// Called with laneTransport.mu held AFTER the authenticated, strictly
// monotonic peer control report passed the existing mailbox gates.
func (q *qualityState) acceptReturnedEcho(e datapath.QualityEchoV4, now time.Time, localGeneration uint64) bool {
    if e.SourceGeneration!=localGeneration || e.Report.IncarnationNonce!=q.nonce {
        return false
    }
    source:=q.sentSources[e.SourceReportSeq%qualitySentHistoryCapacity]
    original:=source.report
    if original.ReportSeq!=e.SourceReportSeq || original.Generation!=localGeneration ||
        original.IncarnationNonce!=q.nonce ||
        original.WindowLastPN!=e.SourceLastPN ||
        source.sent.IsZero() || now.Before(source.sent) ||
        !now.Before(source.sent.Add(qualityEchoStaleAfter)) {
        return false
    }
    if e.State==datapath.QualityFeedbackEstimated {
        if original.Flags&datapath.QualityFlagWindowValid==0 ||
            original.Flags&datapath.QualityFlagCapacityLimited!=0 ||
            original.TxSuccess<qualityPairMinTransmitted ||
            e.ReceiverUnique<qualityPairMinReceived ||
            e.ReceiverUnique>original.TxSuccess ||
            uint64(e.EstimatedMissing)!=original.TxSuccess-e.ReceiverUnique ||
            e.ReceiverLate>uint32(e.ReceiverUnique) {
            return false
        }
    }
    q.returned=qualityReturnedEcho{report:e,received:now,valid:true}
    return true
}

// A stale echo is surfaced as STALE and is not transformed into a reported
// zero deficit or reclassified as a new estimate.
func (r *Runtime) QualityEchoFeedback(ref logicaltunnel.LaneRef,now time.Time) (QualityEchoFeedback,bool) {
    if r==nil{return QualityEchoFeedback{},false}
    r.mu.Lock()
    lane:=r.lanes[ref]
    active:=!r.closed && r.active[ref.ID]==ref
    r.mu.Unlock()
    if !active||lane==nil{return QualityEchoFeedback{},false}
    lane.mu.Lock()
    enabled:=lane.quality.enabled
    snapshot:=lane.quality.returned
    lane.mu.Unlock()
    if !enabled{return QualityEchoFeedback{},false}
    if !snapshot.valid{return QualityEchoFeedback{State:datapath.QualityFeedbackUnknown},true}
    e:=snapshot.report
    result:=QualityEchoFeedback{
        State:e.State,
        SourceReportSeq:e.SourceReportSeq,
        SourceGeneration:e.SourceGeneration,
        SourceLastPN:e.SourceLastPN,
        ReceiverUnique:e.ReceiverUnique,
        EstimatedMissing:e.EstimatedMissing,
        Valid:true,
    }
    if now.Before(snapshot.received)||!now.Before(snapshot.received.Add(qualityEchoStaleAfter)) {
        result.State=datapath.QualityFeedbackStale
        return result,true
    }
    result.Age=now.Sub(snapshot.received)
    if result.State==datapath.QualityFeedbackEstimated {
        // Original sender history was validated before publication.
        // Avoid dividing by an unrelated current outbound PN range.
        m:=laneQualitySourceForFeedback(r,ref,e.SourceReportSeq)
        if m.TxSuccess==0 || uint64(e.EstimatedMissing)>m.TxSuccess {
            result.State=datapath.QualityFeedbackUnknown
            return result,true
        }
        result.Fraction=float64(e.EstimatedMissing)/float64(m.TxSuccess)
    }
    return result,true
}

func laneQualitySourceForFeedback(r *Runtime,ref logicaltunnel.LaneRef,seq uint64) datapath.QualityHealthReport {
    r.mu.Lock()
    lane:=r.lanes[ref]
    r.mu.Unlock()
    if lane==nil{return datapath.QualityHealthReport{}}
    lane.mu.Lock()
    defer lane.mu.Unlock()
    source:=lane.quality.sentSources[seq%qualitySentHistoryCapacity].report
    if source.ReportSeq!=seq{return datapath.QualityHealthReport{}}
    return source
}
