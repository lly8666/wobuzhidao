package runtimeowner

import (
	"bytes"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/adaptivefec"
	"github.com/lly8666/wobuzhidao/internal/datapath"
)

// This test uses the real SizeClass/FastBlock encoder and protected Lane
// sealer. It observes the resulting first-sealed PN metadata at the same
// post-Emit quality-ring seam. Return-echo deficits remain test-supplied:
// this is NOT a WAN impairment or production controller qualification.
func n2NaturalSender(t *testing.T) *datapath.Lane {
	t.Helper()
	cfg:=datapath.LaneConfig{
		Role:datapath.RoleClient, ClientRecordLimit:1300,
		ServerRecordLimit:1250,Keys:runtimeKeys(),ParityShards:20,
		TunnelID:[]byte("0123456789abcdef"),
		FlushAfter:32*time.Millisecond,MaxBlocks:8,
	}
	cfg.IncarnationNonce[0]=9
	cfg.TxMTU=runtimeMTU(cfg.ServerRecordLimit,20)
	cfg.RxMTU=runtimeMTU(cfg.ClientRecordLimit,20)
	l,err:=datapath.NewN2AdaptiveLaneForTests(cfg)
	if err!=nil {t.Fatal(err)}
	return l
}

func n2NoteActualRecords(t *testing.T,q *qualityState,w []datapath.WireRecord,at time.Time) uint64 {
	t.Helper()
	var last uint64
	for _,record:=range w {
		if !record.FEC.Known || len(record.Wire)==0 {
			t.Fatalf("no sealed first-emission provenance %+v",record.FEC)
		}
		q.tx.noteWithMeta(record.PN,true,at,record.FEC)
		last=record.PN
	}
	return last
}

func TestN2NaturalProtectedEncoderCohortsDoNotPermanentlyHold(t *testing.T) {
	l:=n2NaturalSender(t)
	defer l.Close()
	start:=time.Unix(123000,0)
	var q qualityState
	c,err:=adaptivefec.New(adaptivefec.Config{
		Mode:adaptivefec.Auto,InitialParity:20,MinParity:4,MaxParity:20},5)
	if err!=nil {t.Fatal(err)}
	var lastPN uint64
	lowers:=0
	for period:=1;period<=45;period++ {
		at:=start.Add(time.Duration(period)*2*time.Second)
		var sent uint64
		// 140 systematic records, seven naturally FULL 20:20 groups.
		// This is 280 unique sealed record PNs. Sampling the last 256
		// begins inside the oldest parity group; exactly 16 are cropped.
		for i:=0;i<140;i++ {
			wire,err:=l.Outbound(bytes.Repeat([]byte{byte((period+i)%253+1)},96),
				at.Add(time.Duration(i)*time.Millisecond))
			if err!=nil {t.Fatalf("period=%d i=%d: %v",period,i,err)}
			if len(wire)==0 {t.Fatalf("original waited period=%d i=%d",period,i)}
			for _,record:=range wire {
				q.tx.noteWithMeta(record.PN,true,at,record.FEC)
				sent++
				lastPN=record.PN
			}
		}
		// Record PN starts at ZERO (not one). An unbroken batch of 280
		// records ends at 279 on the first cohort; 2nd ends at 559.
		if sent!=280 || lastPN!=uint64(period)*280-1 {
			t.Fatalf("unexpected natural closure period=%d count=%d pn=%d",period,sent,lastPN)
		}
		m:=datapath.QualityHealthReport{
			Flags:datapath.QualityFlagWindowValid,
			ReportSeq:uint64(period),Generation:5,
			WindowFirstPN:lastPN-255,WindowLastPN:lastPN,TxSuccess:256,
		}
		source:=q.captureSentSourceRepairCoverage(m,at.Add(time.Second))
		s:=source.shape
		if !source.repairCoverage || s.Invalid!=0 || s.Cropped!=16 ||
			s.Systematic!=120 || s.Parity!=120 || s.Off!=0 ||
			s.ClosedBySizeK[0][20]!=6 || s.ClosedByR[20]!=6 {
			t.Fatalf("natural source period=%d shape=%+v covered=%v",period,s,source.repairCoverage)
		}
		b:=QualityFirstAttemptBound{
			State:datapath.QualityFeedbackEstimated,Complete:true,
			SourceReportSeq:m.ReportSeq,SourceGeneration:5,
			FirstPN:m.WindowFirstPN,LastPN:m.WindowLastPN,
			Sent:256,ReceiverUnique:256,
			MinFirstLost:0,MaxFirstLost:0,Age:time.Second,Shape:s,
		}
		in:=qualityBoundToAdaptiveInput(b,at.Add(time.Second))
		d:=c.Step(at.Add(time.Second),in)
		if d.Reason==adaptivefec.HoldShape||d.Reason==adaptivefec.HoldStale||
			d.Reason==adaptivefec.HoldUnqualified||d.Reason==adaptivefec.HoldReplay {
			t.Fatalf("normal real k/r sample stuck period=%d reason=%s",period,d.Reason)
		}
		if d.Reason==adaptivefec.Lower {lowers++}
		if dup:=c.Step(at.Add(time.Second),in);dup.Reason!=adaptivefec.HoldReplay {
			t.Fatalf("period=%d replay counted again: %+v",period,dup)
		}
	}
	if lowers!=5 || c.Step(start.Add(93*time.Second),
		adaptivefec.Input{}).Ordinary!=4 {
		t.Fatalf("real encoder natural source cannot walk profiles: lowers=%d",lowers)
	}
}

func TestN2NaturalPartialKRAndOffSourceMetadataNoImplicitFull20(t *testing.T) {
	l:=n2NaturalSender(t)
	defer l.Close()
	now:=time.Unix(124000,0)
	if err:=l.SwitchN2ParityForTests(4);err!=nil {t.Fatal(err)}
	var q qualityState
	var lastPN uint64
	for i:=0;i<7;i++ {
		wire,err:=l.Outbound([]byte{byte(i+1),42},now)
		if err!=nil||len(wire)!=1 {t.Fatalf("partial systematic=%d err=%v",len(wire),err)}
		lastPN=n2NoteActualRecords(t,&q,wire,now)
	}
	if early,err:=l.FlushDue(now.Add(31*time.Millisecond));err!=nil||len(early)!=0 {
		t.Fatalf("partial emitted before original deadline %d %v",len(early),err)
	}
	oldParity,err:=l.FlushDue(now.Add(32*time.Millisecond))
	if err!=nil||len(oldParity)!=4 {t.Fatalf("real min(k,R) 7:4 expected 4 got=%d %v",len(oldParity),err)}
	lastPN=n2NoteActualRecords(t,&q,oldParity,now)
	if err:=l.SwitchN2ParityForTests(0);err!=nil {t.Fatal(err)}
	for i:=0;i<9;i++ {
		wire,err:=l.Outbound([]byte{byte(80+i)},now.Add(33*time.Millisecond))
		if err!=nil||len(wire)!=1||wire[0].FEC.Enabled {t.Fatalf("new off blocked %d %v",len(wire),err)}
		lastPN=n2NoteActualRecords(t,&q,wire,now)
	}
	m:=datapath.QualityHealthReport{
		Flags:datapath.QualityFlagWindowValid,
		ReportSeq:1,Generation:5,WindowFirstPN:0,WindowLastPN:lastPN,
		TxSuccess:lastPN+1,
	}
	shape:=q.captureSentSourceRepairCoverage(m,now.Add(time.Second)).shape
	if !shape.Complete||shape.Invalid!=0||shape.Cropped!=0 ||
		shape.Systematic!=7||shape.Parity!=4||shape.Off!=9||
		shape.ClosedBySizeK[0][7]!=1||shape.ClosedByR[4]!=1 {
		t.Fatalf("true 7:4 and bare LINK wrongly collapsed to 20:20: %+v",shape)
	}
	b:=QualityFirstAttemptBound{
		State:datapath.QualityFeedbackEstimated,Complete:true,
		SourceReportSeq:1,SourceGeneration:5,FirstPN:0,LastPN:lastPN,
		Sent:lastPN+1,ReceiverUnique:lastPN+1,Age:time.Second,Shape:shape,
	}
	in:=qualityBoundToAdaptiveInput(b,now.Add(time.Second))
	c,err:=adaptivefec.New(adaptivefec.Config{
		Mode:adaptivefec.Aggressive,InitialParity:20,MinParity:0,MaxParity:20},5)
	if err!=nil {t.Fatal(err)}
	if d:=c.Step(now.Add(time.Second),in);d.Reason!=adaptivefec.HoldSampleCount ||
		d.ShapeRecords!=7||d.OffRecords!=9||d.ShapeBlocks!=1 {
		t.Fatalf("real k=7 r=4 off=9 not qualified: %+v",d)
	}
}
