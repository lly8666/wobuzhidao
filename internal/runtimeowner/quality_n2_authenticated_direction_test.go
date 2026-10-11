package runtimeowner

import (
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/datapath"
)

func TestN2ControllerConsumesOnlyAuthenticatedLocalTXEchoOnce(t *testing.T) {
	at:=time.Unix(160000,0)
	var q qualityState
	q.enabled=true
	q.nonce[0]=77
	c,err:=adaptivefec.New(adaptivefec.Config{
		Mode:adaptivefec.Auto,InitialParity:20,MinParity:4,MaxParity:20},5)
	if err!=nil {t.Fatal(err)}
	q.n2=c
	// A natural 256-PN geometry: sixteen cropped old parity followed by
	// six complete 20:20 groups. These are sender-local outgoing PNs,
	// NEVER the local receiver's independent PN range.
	var pn uint64
	for i:=4;i<20;i++ {
		pn++
		q.tx.noteWithMeta(pn,true,at,n2Meta(1,20,uint8(20+i),20,0))
	}
	for block:=uint32(2);block<=7;block++ {
		for i:=0;i<40;i++ {
			pn++
			k:=uint8(0)
			if i>=20{k=20}
			q.tx.noteWithMeta(pn,true,at,n2Meta(block,20,uint8(i),k,0))
		}
	}
	if pn!=256 {t.Fatal(pn)}
	m:=datapath.QualityHealthReport{
		Flags:datapath.QualityFlagWindowValid,
		ReportSeq:7,Generation:5,IncarnationNonce:q.nonce,
		WindowFirstPN:1,WindowLastPN:256,TxSuccess:256,
	}
	q.recordSentSource(m,at)
	e:=datapath.QualityEchoV4{
		Report:m,State:datapath.QualityFeedbackEstimated,
		SourceReportSeq:7,SourceGeneration:5,SourceLastPN:256,
		ReceiverUnique:256,
	}
	if !q.acceptReturnedEcho(e,at.Add(4*time.Second),5) {
		t.Fatal("source-correlated local outbound echo unexpectedly rejected")
	}
	q.n2ObserveAuthenticatedReturn(5,at.Add(4*time.Second))
	// The controller must be able to explain a 256-PN sample containing
	// exactly one cropped source edge, without declaring invalid metadata
	// or lying that the crop contributed a known full k20 group.
	evidence:=q.n2Evidence
	if !evidence.SourceMatched || !evidence.RepairCoverage ||
		!evidence.BoundComplete || evidence.SourceWindowSuccess!=256 ||
		evidence.ShapeSent!=256 || evidence.ShapeSystematic!=120 ||
		evidence.ShapeParity!=120 || evidence.ShapeCropped!=16 ||
		evidence.ShapeUnknown!=16 || evidence.ShapeInvalid!=0 ||
		evidence.ShapeFullBlocks!=6 || evidence.LowerFirstLost!=0 ||
		evidence.UpperFirstLost!=0 {
		t.Fatalf("genuine raw TX source identity/geometry not explained %+v",evidence)
	}
	if !q.n2HasDecision || q.n2Last.Reason!=adaptivefec.HoldSampleCount ||
		q.n2Last.AcceptedRecords!=256 || q.n2Last.ShapeRecords!=120 ||
		q.n2Last.CroppedRecords!=16 {
		t.Fatalf("valid local TX result did not qualify %+v",q.n2Last)
	}
	// Reobserving an identical returned echo cannot spend source PN counts
	// again, even when the caller's observation time advances.
	q.n2ObserveAuthenticatedReturn(5,at.Add(4100*time.Millisecond))
	if q.n2Last.Reason!=adaptivefec.HoldReplay ||
		q.n2Last.AcceptedRecords!=256 {t.Fatalf("double consumption %+v",q.n2Last)}
	// An old echo after the 10s source/return expiry becomes UNKNOWN/HOLD,
	// never zero loss or a fresh report.
	q.n2ObserveAuthenticatedReturn(5,at.Add(15*time.Second))
	if q.n2Last.Reason!=adaptivefec.HoldUnqualified ||
		q.n2Last.AcceptedRecords!=256 {t.Fatalf("stale echo reused %+v",q.n2Last)}
}

func TestN2InboundPeerSourceCannotDriveLocalTXController(t *testing.T) {
	now:=time.Unix(170000,0)
	q:=qualityState{enabled:true}
	q.nonce[0]=11
	c,err:=adaptivefec.New(adaptivefec.Config{
		Mode:adaptivefec.Aggressive,InitialParity:20,MinParity:4,MaxParity:20},9)
	if err!=nil {t.Fatal(err)}
	q.n2=c
	// A peer's incoming quality report, unlike a returned echo referencing
	// our own sender PN source history, carries no local TX damage witness.
	foreign:=datapath.QualityEchoV4{
		Report:datapath.QualityHealthReport{IncarnationNonce:q.nonce},
		SourceReportSeq:4,SourceGeneration:9,SourceLastPN:500,
		State:datapath.QualityFeedbackEstimated,ReceiverUnique:100,
	}
	if q.acceptReturnedEcho(foreign,now,9) {
		t.Fatal("foreign receive-direction quality reused as local send evidence")
	}
	q.n2ObserveAuthenticatedReturn(9,now)
	if q.n2HasDecision {t.Fatal("unmatched peer report changed TX advice")}
	// Generation mismatch must fail even if the source table is otherwise
	// filled with plausible counters. Controller lives only for one lane ref.
	q.returned=qualityReturnedEcho{report:foreign,received:now,valid:true}
	q.n2ObserveAuthenticatedReturn(9,now)
	if !q.n2HasDecision || q.n2Last.Reason!=adaptivefec.HoldUnqualified {
		t.Fatalf("unmatched return treated as zero first loss %+v",q.n2Last)
	}
}
