package runtimeowner

import (
 "time"
 "github.com/lly8666/wobuzhidao/internal/datapath"
 "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

const QualityPairReorderHorizon = 3*time.Second
const qualityPairStaleAfter = 10*time.Second
const qualityPairMinTransmitted = 16
const qualityPairMinReceived = 8

type qualityPairCandidate struct {
 report datapath.QualityHealthReport
 firstSeen time.Time
 lastSeen time.Time
 controlPN uint64
 accepted bool
}

func (c *qualityPairCandidate) accept(m datapath.QualityHealthReport, marker uint64, now time.Time) {
 stable:=c.accepted &&
  c.report.IncarnationNonce==m.IncarnationNonce &&
  c.report.Generation==m.Generation &&
  c.report.WindowFirstPN==m.WindowFirstPN &&
  c.report.WindowLastPN==m.WindowLastPN &&
  c.report.TxSuccess==m.TxSuccess &&
  c.report.LocalDrops==m.LocalDrops &&
  c.report.Flags&(datapath.QualityFlagWindowValid|datapath.QualityFlagCapacityLimited)==
   m.Flags&(datapath.QualityFlagWindowValid|datapath.QualityFlagCapacityLimited)
 if !stable {c.firstSeen=now}
 c.report=m
 c.lastSeen=now
 c.controlPN=marker
 c.accepted=true
}

// QualityInboundEstimate covers authenticated peer TX records as observed
// in this endpoint's RX direction. It is an estimated first-arrival deficit,
// not a raw physical-loss measurement or an enabled N2 controller signal.
type QualityInboundEstimate struct {
 State datapath.QualityFeedbackState
 SourceReportSeq uint64
 SenderFirstPN, SenderLastPN, ControlPN uint64
 SenderSuccess, ReceiverUnique uint64
 ReceiverLate, ReceiverDuplicate uint32
 EstimatedMissing uint64
 Fraction float64
 Age time.Duration
}

func evaluateQualityPair(c qualityPairCandidate, rx datapath.QualityReceiveRange, now time.Time) QualityInboundEstimate {
 out:=QualityInboundEstimate{State:datapath.QualityFeedbackUnknown}
 if !c.accepted {return out}
 m:=c.report
 out.SourceReportSeq=m.ReportSeq
 out.SenderFirstPN=m.WindowFirstPN
 out.SenderLastPN=m.WindowLastPN
 out.ControlPN=c.controlPN
 out.SenderSuccess=m.TxSuccess
 out.ReceiverUnique=rx.Unique
 out.ReceiverLate=rx.Late
 out.ReceiverDuplicate=rx.Duplicate
 if now.Before(c.lastSeen)||now.Before(c.firstSeen)||!now.Before(c.lastSeen.Add(qualityPairStaleAfter)){
  out.State=datapath.QualityFeedbackStale
  return out
 }
 out.Age=now.Sub(c.firstSeen)
 if m.Flags&datapath.QualityFlagCapacityLimited!=0||rx.CapacityLimited{
  out.State=datapath.QualityFeedbackCapacity
  return out
 }
 out.State=datapath.QualityFeedbackInsufficient
 if m.Flags&datapath.QualityFlagWindowValid==0 ||
  m.Flags&datapath.QualityFlagInsufficient==0 ||
  m.Flags&datapath.QualityFlagLossEstimated!=0 ||
  m.AgeMillis==0xffff || m.AgeMillis>10000 ||
  m.WindowFirstPN>m.WindowLastPN ||
  m.WindowLastPN-m.WindowFirstPN>=datapath.QualityPNWindowCapacity ||
  c.controlPN<=m.WindowLastPN ||
  m.TxSuccess<qualityPairMinTransmitted ||
  m.TxSuccess>datapath.QualityPNWindowCapacity ||
  !rx.RangeRetained || !rx.HaveAnchor ||
  rx.Unique<qualityPairMinReceived ||
  rx.Unique>m.TxSuccess ||
  out.Age<QualityPairReorderHorizon {
  return out
 }
 out.EstimatedMissing=m.TxSuccess-rx.Unique
 out.Fraction=float64(out.EstimatedMissing)/float64(m.TxSuccess)
 out.State=datapath.QualityFeedbackEstimated
 return out
}

func (r *Runtime) QualityInboundEstimate(ref logicaltunnel.LaneRef, now time.Time) (QualityInboundEstimate,bool) {
 if r==nil {return QualityInboundEstimate{},false}
 r.mu.Lock()
 lane:=r.lanes[ref]
 active:=!r.closed && r.active[ref.ID]==ref
 r.mu.Unlock()
 if !active||lane==nil {return QualityInboundEstimate{},false}
 lane.mu.Lock()
 enabled:=lane.quality.enabled
 c:=lane.quality.pair
 lane.mu.Unlock()
 if !enabled {return QualityInboundEstimate{},false}
 if !c.accepted {return QualityInboundEstimate{State:datapath.QualityFeedbackUnknown},true}
 rx,err:=r.owner.QualityReceiveRange(ref,c.report.WindowFirstPN,c.report.WindowLastPN,now)
 if err!=nil {return QualityInboundEstimate{},false}
 return evaluateQualityPair(c,rx,now),true
}
