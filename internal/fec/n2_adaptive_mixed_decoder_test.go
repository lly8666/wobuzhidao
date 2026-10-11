package fec

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

type n2WireBlock struct {
	data [][]byte
	sources [][]byte
	parities [][]byte
}

func n2MakeBlock(t *testing.T,parity,k int,id uint32,tag byte) n2WireBlock {
	t.Helper()
	e,err:=NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(),512,32*time.Millisecond,id,parity)
	if err!=nil {t.Fatal(err)}
	out:=n2WireBlock{}
	for i:=0;i<k;i++ {
		p:=bytes.Repeat([]byte{tag+byte(i)},80+i)
		out.data=append(out.data,p)
		wire,err:=e.Add(p,time.Unix(100,0))
		if err!=nil {t.Fatal(err)}
		for _,pkt:=range wire {
			h,err:=ParseBlockHeader(pkt)
			if err!=nil {t.Fatal(err)}
			own:=append([]byte(nil),pkt...)
			if h.ShardIndex<DataShards {out.sources=append(out.sources,own)}else{out.parities=append(out.parities,own)}
		}
	}
	flush,err:=e.Flush()
	if err!=nil {t.Fatal(err)}
	for _,pkt:=range flush {
		h,err:=ParseBlockHeader(pkt)
		if err!=nil||h.ShardIndex<DataShards {t.Fatalf("bad parity header %v %+v",err,h)}
		out.parities=append(out.parities,append([]byte(nil),pkt...))
	}
	want:=k
	if want>parity {want=parity}
	if len(out.sources)!=k||len(out.parities)!=want {
		t.Fatalf("20:%d k=%d sources=%d parity=%d",parity,k,len(out.sources),len(out.parities))
	}
	return out
}

// One instance decodes the entire family; late old-profile blocks are not
// reinterpreted under the currently selected NEW profile.
func TestN2OneDecoderAllParitiesLateOldAndImmediateNew(t *testing.T) {
	d,err:=NewAdaptiveBlockDecoder(NewFastReedSolomon20x20(),512,8)
	if err!=nil {t.Fatal(err)}
	family:=[]int{4,8,10,12,16,20}
	blocks:=make([]n2WireBlock,0,len(family))
	for i,p:=range family {
		b:=n2MakeBlock(t,p,i%6+2,uint32(100+i),byte(i*20+1))
		blocks=append(blocks,b)
	}
	got:=map[byte]bool{}
	push:=func(wire []byte){
		t.Helper()
		packets,_,err:=d.AddLive(wire)
		if err!=nil {t.Fatal(err)}
		for _,pkt:=range packets {
			if got[pkt[0]] {t.Fatalf("duplicate first delivery %d",pkt[0])}
			got[pkt[0]]=true
		}
	}
	// Leave one original missing from EVERY profile; new systematic first
	// arrivals still exit immediately even though older blocks are pending.
	for _,b:=range blocks {
		for i:=1;i<len(b.sources);i++ {push(b.sources[i])}
	}
	wantInitial:=0
	for _,b:=range blocks {wantInitial+=len(b.sources)-1}
	if len(got)!=wantInitial {t.Fatalf("pending old blocked fresh: got=%d want=%d",len(got),wantInitial)}
	for i:=len(blocks)-1;i>=0;i-- {
		for _,p:=range blocks[i].parities {push(p)}
	}
	wantFinal:=0
	for _,b:=range blocks {wantFinal+=len(b.data)}
	if len(got)!=wantFinal||d.InFlight()>8 {
		t.Fatalf("shared decoder recovered=%d/%d in-flight=%d",len(got),wantFinal,d.InFlight())
	}
	for _,b:=range blocks {
		for _,pkt:=range b.sources {push(pkt)} // late old systematic must not redeliver
	}
	if len(got)!=wantFinal {t.Fatal("late source first-delivered twice",len(got))}
}

func TestN2MixedParityCollisionRejectsActiveAndCompactRetired(t *testing.T) {
	old:=n2MakeBlock(t,4,3,77,1)
	wrong:=n2MakeBlock(t,20,3,77,21)
	newer:=n2MakeBlock(t,20,3,78,41)
	d,err:=NewAdaptiveBlockDecoder(NewFastReedSolomon20x20(),512,1)
	if err!=nil {t.Fatal(err)}
	if out,_,err:=d.AddLive(old.sources[0]);err!=nil||len(out)!=1 {t.Fatalf("old systematic err=%v out=%d",err,len(out))}
	if out,_,err:=d.AddLive(wrong.sources[1]);!errors.Is(err,ErrHeaderMismatch)||len(out)!=0 {
		t.Fatalf("same block parity change mutated active decoder err=%v out=%d",err,len(out))
	}
	if out,_,err:=d.AddLive(newer.sources[0]);err!=nil||len(out)!=1||d.InFlight()>1 {
		t.Fatalf("new profile must not HOL under capacity err=%v out=%d in-flight=%d",err,len(out),d.InFlight())
	}
	if out,_,err:=d.AddLive(wrong.parities[0]);!errors.Is(err,ErrHeaderMismatch)||len(out)!=0 {
		t.Fatalf("retired old BlockID accepted new profile err=%v out=%d",err,len(out))
	}
	// Same profile's late systematic is first-delivered once, even after its
	// heavy reconstruction state was shed in favor of newer fresh traffic.
	if out,_,err:=d.AddLive(old.sources[1]);err!=nil||len(out)!=1 {
		t.Fatalf("compact old first arrival lost err=%v out=%d",err,len(out))
	}
	if out,_,err:=d.AddLive(old.sources[1]);err!=nil||len(out)!=0 {
		t.Fatalf("compact duplicate redelivered err=%v out=%d",err,len(out))
	}
}

func TestN2AdaptiveDecoderSharedHeavyBudgetUnderMixedProfileChurn(t *testing.T) {
	d,err:=NewAdaptiveBlockDecoder(NewFastReedSolomon20x20(),256,2)
	if err!=nil {t.Fatal(err)}
	for i:=0;i<100;i++ {
		parity:=SupportedParityShards()[i%len(SupportedParityShards())]
		e,err:=NewFastBlockEncoderWithParity(NewFastReedSolomon20x20(),256,time.Second,uint32(i+1),parity)
		if err!=nil {t.Fatal(err)}
		wire,err:=e.Add([]byte{byte(i)},time.Unix(200,0))
		if err!=nil {t.Fatal(err)}
		packets,_,err:=d.AddLive(wire[0])
		if err!=nil||len(packets)!=1||d.InFlight()>2 {
			t.Fatalf("i=%d parity=%d immediate first=%d inFlight=%d err=%v",i,parity,len(packets),d.InFlight(),err)
		}
	}
	// Old fixed decoder STILL rejects a different per-incarnation profile.
	fixed,_:=NewBlockDecoderWithParity(NewFastReedSolomon20x20(),256,2,4)
	b:=n2MakeBlock(t,20,1,999,2)
	if _,_,err:=fixed.AddLive(b.sources[0]);!errors.Is(err,ErrHeaderMismatch) {t.Fatal("fixed parser accepted mixed profile",err)}
}
