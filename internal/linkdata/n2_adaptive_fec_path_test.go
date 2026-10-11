package linkdata

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/lly8666/wobuzhidao/internal/fec"
)

func n2AdaptivePath(t *testing.T, start int) *FECPath {
	t.Helper()
	p,err:=NewAdaptiveFECPath(FECPathConfig{SourceMTU:1200,
		ParityShards:start,FlushAfter:32*time.Millisecond,MaxBlocks:8})
	if err!=nil {t.Fatal(err)}
	return p
}

func n2Decode(t *testing.T,p *FECPath,wires [][]byte,now time.Time) [][]byte {
	t.Helper()
	var out [][]byte
	for _,w:=range wires {
		delivered,err:=p.Decode(w,now)
		if err!=nil {t.Fatalf("authenticated LINK dispatch err=%v payload_len=%d",err,len(w))}
		out=append(out,delivered...)
	}
	return out
}

func TestN2AdaptiveOffOnOldPartialAndFreshNoHOL(t *testing.T) {
	tx,rx:=n2AdaptivePath(t,4),n2AdaptivePath(t,0)
	base:=time.Unix(20000,0)
	old,err:=tx.Encode([]byte("old-partial-4"),base)
	if err!=nil||len(old)!=1 {t.Fatalf("old sources %d err=%v",len(old),err)}
	if err:=tx.SwitchParity(0);err!=nil {t.Fatal(err)}
	if tx.NextFlushDeadline()!=base.Add(32*time.Millisecond) {
		t.Fatalf("old partial deadline moved %v",tx.NextFlushDeadline())
	}
	// The entire bare-payload first delivery is immediate, even though an old
	// 20:4 group remains open and may have lost all original systematic wire.
	fresh,err:=tx.Encode([]byte("fresh-off"),base.Add(time.Millisecond))
	if err!=nil||len(fresh)!=1 {t.Fatalf("off blocked %d err=%v",len(fresh),err)}
	if got:=n2Decode(t,rx,fresh,base.Add(time.Millisecond));len(got)!=1||!bytes.Equal(got[0],[]byte("fresh-off")) {
		t.Fatalf("fresh HOL or corruption %+v",got)
	}
	if err:=tx.SwitchParity(20);err!=nil {t.Fatal(err)}
	newer,err:=tx.Encode([]byte("new-on-20"),base.Add(2*time.Millisecond))
	if err!=nil||len(newer)!=1 {t.Fatalf("new profile fresh blocked %d err=%v",len(newer),err)}
	if got:=n2Decode(t,rx,newer,base.Add(2*time.Millisecond));len(got)!=1||string(got[0])!="new-on-20" {
		t.Fatalf("new systematic unavailable %+v",got)
	}
	if out,err:=tx.FlushDue(base.Add(31*time.Millisecond));err!=nil||len(out)!=0 {
		t.Fatalf("premature switch flush %d err=%v",len(out),err)
	}
	// Old parity follows its original timestamp and arrives AFTER new mode.
	parity,err:=tx.FlushDue(base.Add(32*time.Millisecond))
	if err!=nil||len(parity)!=1 {t.Fatalf("old parity missing %d err=%v",len(parity),err)}
	h,err:=fec.ParseBlockHeader(parity[0])
	if err!=nil||h.EffectiveParityCount()!=4||h.DataCount!=1 {
		t.Fatalf("old parity reprofiled %+v err=%v",h,err)
	}
	got:=n2Decode(t,rx,parity,base.Add(32*time.Millisecond))
	if len(got)!=1||string(got[0])!="old-partial-4" {t.Fatalf("old first delivery missing %+v",got)}
	if got:=n2Decode(t,rx,old,base.Add(100*time.Millisecond));len(got)!=0 {
		t.Fatalf("late old systematic duplicated %+v",got)
	}
	current:=tx.State()
	if current.DrainingEncoders!=0||current.Encoder.ClosedByK[1]!=1||
		current.Encoder.ClosedByR[1]!=1||current.Decoder.InFlight!=0 {
		t.Fatalf("old closed while new active stats=%+v",current)
	}
}

func TestN2AdaptiveAllFixedParitiesAndOffMultipleSwitches(t *testing.T) {
	tx,rx:=n2AdaptivePath(t,0),n2AdaptivePath(t,0)
	now:=time.Unix(21000,0)
	seq:=[]int{0,4,8,10,12,16,20,0,20,4,0}
	var oldWire [][]byte
	for i,p:=range seq {
		if err:=tx.SwitchParity(p);err!=nil {t.Fatalf("switch %d to %d: %v",i,p,err)}
		payload:=[]byte{byte('a'+i),byte('Z')}
		out,err:=tx.Encode(payload,now)
		if err!=nil||len(out)!=1 {t.Fatalf("new immediate source p=%d out=%d err=%v",p,len(out),err)}
		if p==0 {
			got:=n2Decode(t,rx,out,now)
			if len(got)!=1||!bytes.Equal(got[0],payload) {t.Fatalf("off first delivery p=%d got=%q",p,got)}
		} else {
			// Delay first systematic until after parity; it must still reach
			// app exactly once even though peer sees future parity profiles.
			oldWire=append(oldWire,append([]byte(nil),out[0]...))
		}
		parity,err:=tx.FlushDue(now.Add(32*time.Millisecond))
		if err!=nil {t.Fatal(err)}
		if p!=0 {
			got:=n2Decode(t,rx,parity,now.Add(32*time.Millisecond))
			if len(got)!=1||!bytes.Equal(got[0],payload) {
				t.Fatalf("lost all initial source p=%d parity=%d recovered=%q",p,len(parity),got)
			}
		} else if len(parity)!=0 {t.Fatal("off emitted parity")}
		now=now.Add(40*time.Millisecond)
	}
	for _,w:=range oldWire {
		if out:=n2Decode(t,rx,[][]byte{w},now);len(out)!=0 {
			t.Fatalf("old systematic double first delivery %+v",out)
		}
	}
	if state:=rx.State();!state.Adaptive||state.Decoder.MaxBlocks!=8||
		state.Decoder.InFlight>8||tx.FECWireLimit()!=fec.HeaderSize+1200 {
		t.Fatalf("dynamic shared budgets/MTU not stable state=%+v mtu=%d",state,tx.FECWireLimit())
	}
}

func TestN2AdaptiveBoundedRapidSwitchesPreserveFreshAndBlockIDs(t *testing.T) {
	tx:=n2AdaptivePath(t,20)
	base:=time.Unix(22000,0)
	seq:=[]int{20,4,8,10,12,16,20}
	seen:=map[uint32]bool{}
	for i,p:=range seq {
		if i>0 {
			if err:=tx.SwitchParity(p);err!=nil {t.Fatalf("early switch i=%d err=%v",i,err)}
		}
		out,err:=tx.Encode([]byte{byte('A'+i)},base)
		if err!=nil||len(out)!=1 {t.Fatalf("fresh blocked i=%d err=%v",i,err)}
		h,err:=fec.ParseBlockHeader(out[0])
		if err!=nil||seen[h.BlockID] {t.Fatalf("BlockID reused i=%d %+v err=%v",i,h,err)}
		seen[h.BlockID]=true
	}
	if s:=tx.State();s.DrainingEncoders!=maxAdaptiveDrainingEncoders {
		t.Fatalf("unbounded/early flush drains=%d",s.DrainingEncoders)
	}
	if err:=tx.SwitchParity(4);!errors.Is(err,ErrAdaptiveSwitchBusy) {
		t.Fatalf("seventh draining encoder accepted %v",err)
	}
	out,err:=tx.Encode([]byte("still-fresh"),base.Add(time.Millisecond))
	if err!=nil||len(out)!=1 {t.Fatalf("budget error stalled fresh send %d %v",len(out),err)}
	if h,_:=fec.ParseBlockHeader(out[0]);h.EffectiveParityCount()!=20 {
		t.Fatalf("rejected switch changed current geometry %+v",h)
	}
	if par,err:=tx.FlushDue(base.Add(31*time.Millisecond));err!=nil||len(par)!=0 {
		t.Fatalf("draining early flush %d %v",len(par),err)
	}
	if par,err:=tx.FlushDue(base.Add(32*time.Millisecond));err!=nil||len(par)<7 {
		t.Fatalf("drained prematurely/lost old sources %d %v",len(par),err)
	}
	if s:=tx.State();s.DrainingEncoders!=0||s.Encoder.PendingSources!=0 {
		t.Fatalf("after deadline drain not released %+v",s)
	}
	if err:=tx.SwitchParity(4);err!=nil {t.Fatalf("switch still busy after deadline %v",err)}
}

func TestN2AdaptiveOffWFEscapeAndStrictAuthenticatedDispatch(t *testing.T) {
	tx,rx:=n2AdaptivePath(t,0),n2AdaptivePath(t,20)
	now:=time.Unix(23000,0)
	for _,packet:=range [][]byte{[]byte("WF-a-business-prefix"),[]byte("WBDLFRG1-a-business-prefix")} {
		wire,err:=tx.Encode(packet,now)
		if err!=nil {t.Fatal(err)}
		if len(wire)==0||bytes.HasPrefix(wire[0],[]byte("WF")) {
			t.Fatalf("ambiguous raw off payload passed WF prefix len=%d",len(wire))
		}
		got:=n2Decode(t,rx,wire,now)
		if len(got)!=1||!bytes.Equal(got[0],packet) {
			t.Fatalf("escaped off datagram changed got=%q want=%q",got,packet)
		}
	}
	bad:=append([]byte("WF"),bytes.Repeat([]byte{0},fec.HeaderSize+20)...)
	if got,err:=rx.Decode(bad,now);err==nil||len(got)!=0 {
		t.Fatalf("malformed WF fell back to unprotected bare LINK: err=%v got=%q",err,got)
	}
	fixed,err:=NewFECPath(FECPathConfig{SourceMTU:1200,ParityShards:4,
		FlushAfter:32*time.Millisecond,MaxBlocks:8})
	if err!=nil {t.Fatal(err)}
	if err:=fixed.SwitchParity(20);!errors.Is(err,ErrAdaptiveSwitchUnavailable) {
		t.Fatalf("legacy fixed profile silently migrated: %v",err)
	}
}
