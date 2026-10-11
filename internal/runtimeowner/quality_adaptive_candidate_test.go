package runtimeowner

import (
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/datapath"
)

func TestN2BoundedSourceCandidateUsesParityPNAndCroppedEdge(t *testing.T) {
	at:=time.Unix(13000,0)
	var q qualityState
	var pn uint64
	// Eight 20:20 groups require 320 KindLINK PNs. The last 256 contain
	// six complete 40-PN blocks plus 16 shards of a cropped older group.
	for block:=uint32(1);block<=8;block++ {
		for i:=0;i<40;i++ {
			pn++
			meta:=n2Meta(block,20,uint8(i),0,0)
			if i>=20 {meta=n2Meta(block,20,uint8(i),20,2)}
			q.tx.noteWithMeta(pn,true,at,meta)
		}
	}
	m:=datapath.QualityHealthReport{
		Flags:datapath.QualityFlagWindowValid,ReportSeq:30,Generation:5,
		WindowFirstPN:pn-255,WindowLastPN:pn,TxSuccess:256,
	}
	got:=q.captureSentSourceRepairCoverage(m,at)
	if !got.repairCoverage||got.shape.Complete||got.shape.Invalid!=0||
		got.shape.Cropped!=16||got.shape.Systematic!=120||
		got.shape.Parity!=120||got.shape.ClosedBySizeK[2][20]!=6||
		got.shape.ClosedByR[20]!=6||got.shape.Unknown!=16 {
		t.Fatalf("physical source cohort wrong %+v repair=%v",got.shape,got.repairCoverage)
	}
	b:=QualityFirstAttemptBound{State:datapath.QualityFeedbackEstimated,
		Complete:true,SourceReportSeq:30,SourceGeneration:5,
		FirstPN:m.WindowFirstPN,LastPN:m.WindowLastPN,Sent:256,
		MinFirstLost:0,MaxFirstLost:1,Age:time.Second,Shape:got.shape,
	}
	in:=qualityBoundToAdaptiveInput(b,at.Add(time.Second))
	if in.Evidence!=adaptivefec.EvidenceFirstAttemptBounded||
		!in.ShapeVerified||in.CroppedRecords!=16||in.ParityRecords!=120||
		in.ClosedGroups[2][20]!=6||in.ClosedR[20]!=6||
		in.ClosedBySizeKR[2][20][20]!=6 {
		t.Fatalf("source candidate wrong %+v",in)
	}
	c,err:=adaptivefec.New(adaptivefec.Config{
		Mode:adaptivefec.Auto,InitialParity:20,MinParity:4,MaxParity:20},5)
	if err!=nil {t.Fatal(err)}
	if dec:=c.Step(at.Add(time.Second),in);dec.Reason!=adaptivefec.HoldSampleCount||
		dec.ShapeRecords!=120||dec.CroppedRecords!=16 {
		t.Fatalf("credible partial source held forever %+v",dec)
	}
	if dec:=c.Step(at.Add(time.Second),in);dec.Reason!=adaptivefec.HoldReplay {
		t.Fatalf("duplicate counted twice %+v",dec)
	}
}

func TestN2SourceAdapterNeverPromotesUnavailableToClean(t *testing.T) {
	now:=time.Unix(14000,0)
	for _,b:=range []QualityFirstAttemptBound{
		{State:datapath.QualityFeedbackUnknown},
		{State:datapath.QualityFeedbackStale,Complete:true,Sent:256},
		{State:datapath.QualityFeedbackCapacity,Complete:false,Sent:256},
		{State:datapath.QualityFeedbackEstimated,Complete:false,Sent:256},
		{State:datapath.QualityFeedbackEstimated,Complete:true,Sent:257},
	} {
		if in:=qualityBoundToAdaptiveInput(b,now);in.Evidence!=adaptivefec.EvidenceUnknown {
			t.Fatalf("unqualified bound promoted %+v",in)
		}
	}
}
