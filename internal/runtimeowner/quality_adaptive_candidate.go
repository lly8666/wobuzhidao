package runtimeowner

import (
	"time"

	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/datapath"
	"github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// QualityAdaptiveCandidate is READ ONLY. No production controller, timer,
// wake-up or mode admission calls it yet. The original bound is not exact PLR.
func (r *Runtime) QualityAdaptiveCandidate(ref logicaltunnel.LaneRef,now time.Time) (adaptivefec.Input,bool) {
	bound,ok:=r.QualityFirstAttemptBound(ref,now)
	if !ok {return adaptivefec.Input{},false}
	return qualityBoundToAdaptiveInput(bound,now),true
}

func qualityBoundToAdaptiveInput(b QualityFirstAttemptBound,now time.Time) adaptivefec.Input {
	s:=adaptivefec.Input{Evidence:adaptivefec.EvidenceUnknown}
	if b.State!=datapath.QualityFeedbackEstimated || !b.Complete ||
		b.Sent==0 || b.Sent>datapath.QualityPNWindowCapacity ||
		b.MaxFirstLost>b.Sent || b.MinFirstLost>b.MaxFirstLost ||
		b.Age<0 || now.IsZero() {
		return s
	}
	shape:=b.Shape
	s.Evidence=adaptivefec.EvidenceFirstAttemptBounded
	s.Generation=b.SourceGeneration
	s.Sequence=b.SourceReportSeq
	s.FirstPN=b.FirstPN
	s.LastPN=b.LastPN
	s.Sent=uint32(b.Sent)
	s.FirstAttemptLost=uint32(b.MinFirstLost)
	s.FirstAttemptLostMax=uint32(b.MaxFirstLost)
	s.At=now.Add(-b.Age)
	s.ClosedGroups=shape.ClosedBySizeK
	s.ClosedR=shape.ClosedByR
	s.ClosedBySizeKR=shape.ClosedBySizeKR
	s.ParityRecords=shape.Parity
	s.OffRecords=shape.Off
	s.CroppedRecords=shape.Cropped
	s.InvalidRecords=shape.Invalid
	s.ShapeVerified=shape.Sent==s.Sent && shape.Invalid==0
	return s
}
