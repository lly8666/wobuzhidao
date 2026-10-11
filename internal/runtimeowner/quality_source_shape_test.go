package runtimeowner

import (
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/datapath"
)

func n2Meta(block uint32, parity, idx, k, bin uint8) datapath.FECRecordMeta {
	return datapath.FECRecordMeta{
		Known:true,Enabled:true,BlockID:block,ConfiguredParity:parity,
		ShardIndex:idx,ClosedK:k,ShardLengthBin:bin,
	}
}

func TestN2SourceWindowShapeCountsActualCompleteBlocksAndOff(t *testing.T) {
	q:=qualityState{enabled:true}
	at:=time.Unix(1700,0)
	pn:=uint64(0)
	add:=func(meta datapath.FECRecordMeta) {
		pn++
		q.tx.noteWithMeta(pn,true,at,meta)
	}
	// First group 3 sources + actual 3 parity (20:20).
	for i:=0;i<3;i++ {add(n2Meta(7,20,uint8(i),0,0))}
	for i:=0;i<3;i++ {add(n2Meta(7,20,uint8(20+i),3,0))}
	// Second group 4 sources + 4 parity in >512 length bin.
	for i:=0;i<4;i++ {add(n2Meta(8,20,uint8(i),0,0))}
	for i:=0;i<4;i++ {add(n2Meta(8,20,uint8(20+i),4,2))}
	add(datapath.FECRecordMeta{Known:true}) // bare LINK off coexists on wire
	m:=datapath.QualityHealthReport{
		Flags:datapath.QualityFlagWindowValid,ReportSeq:3,Generation:5,
		WindowFirstPN:1,WindowLastPN:pn,TxSuccess:pn,
	}
	s:=q.captureSentSourceRepairCoverage(m,at)
	got:=s.shape
	if !s.repairCoverage||!got.Complete||got.Sent!=15||got.Systematic!=7||
		got.Parity!=7||got.Off!=1||got.Unknown!=0||got.ClosedByR[3]!=1||
		got.ClosedByR[4]!=1||got.ClosedBySizeK[0][3]!=1||
		got.ClosedBySizeK[2][4]!=1||
		got.ClosedBySizeKR[0][3][3]!=1||
		got.ClosedBySizeKR[2][4][4]!=1||got.PartialBlocks!=2 {
		t.Fatalf("source proof %+v repairCoverage=%v",got,s.repairCoverage)
	}
	// Same first-send PN duplicate does not create another transmitted
	// packet, block group or confidence sample.
	q.tx.noteWithMeta(1,true,at,n2Meta(99,20,0,0,0))
	if next:=q.tx.sourceShape(m);next!=got {t.Fatalf("duplicate changed witness %+v vs %+v",next,got)}
}

func TestN2SourceShapeFailsClosedOnPartialGroupCutAndUnknownMeta(t *testing.T) {
	at:=time.Unix(1800,0)
	var q qualityState
	for pn:=uint64(1);pn<=4;pn++ {
		meta:=n2Meta(4,4,uint8(pn-1),0,0)
		if pn==4 {meta=n2Meta(4,4,20,4,0)}
		q.tx.noteWithMeta(pn,true,at,meta)
	}
	m:=datapath.QualityHealthReport{
		Flags:datapath.QualityFlagWindowValid,ReportSeq:4,Generation:5,
		WindowFirstPN:1,WindowLastPN:4,TxSuccess:4,
	}
	src:=q.captureSentSourceRepairCoverage(m,at)
	if !src.repairCoverage||src.shape.Complete||src.shape.Unknown!=4||
		src.shape.ClosedByR[4]!=0 ||
		src.shape.CroppedSource!=3 || src.shape.CroppedParity!=1 {
		t.Fatalf("cropped k4/r4 claimed exact: %+v",src.shape)
	}
	q.tx.noteWithMeta(5,true,at,datapath.FECRecordMeta{}) // UNKNOWN != off
	m.WindowLastPN=5;m.TxSuccess=5
	src=q.captureSentSourceRepairCoverage(m,at)
	if src.shape.Off!=0||src.shape.Complete||src.shape.Unknown!=5 {
		t.Fatalf("unknown metadata treated as off %+v",src.shape)
	}
	m.LocalDrops=1
	src=q.captureSentSourceRepairCoverage(m,at)
	if src.repairCoverage||src.shape.Complete {
		t.Fatalf("local drops qualified %+v",src)
	}
}

func TestN2SourceShapeWindowCannotUseOverwrittenRecords(t *testing.T) {
	at:=time.Unix(1900,0)
	var q qualityState
	for pn:=uint64(1);pn<=300;pn++ {
		q.tx.noteWithMeta(pn,true,at,datapath.FECRecordMeta{Known:true})
	}
	m:=datapath.QualityHealthReport{
		Flags:datapath.QualityFlagWindowValid,ReportSeq:5,Generation:5,
		WindowFirstPN:1,WindowLastPN:20,TxSuccess:20,
	}
	src:=q.captureSentSourceRepairCoverage(m,at)
	if src.repairCoverage||src.shape.Complete {t.Fatalf("overwritten source qualified %+v",src)}
}
