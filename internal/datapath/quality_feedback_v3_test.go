package datapath

import (
    "errors"
    "sync"
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

func TestN1QualityFeedbackMailboxGenerationNonceAndReplay(t *testing.T) {
    msg := qualityHealthVector()
    ref := logicaltunnel.LaneRef{ID:1, Generation:msg.Generation}
    mailbox,err := NewQualityFeedbackMailbox(ref,msg.IncarnationNonce)
    if err!=nil {t.Fatal(err)}
    t0:=time.Unix(100,0)
    if _,status:=mailbox.Snapshot(t0,0);status!=QualityFeedbackUnknown {
        t.Fatalf("uninitialized quality should be UNKNOWN: %v",status)
    }
    wire,err:=EncodeQualityHealthV3(msg);if err!=nil{t.Fatal(err)}
    if err:=mailbox.Accept(ref,wire[:],t0);err!=nil {t.Fatal(err)}
    observed,status:=mailbox.Snapshot(t0.Add(time.Second),0)
    if status!=QualityFeedbackEstimated || observed.ReportSeq!=msg.ReportSeq {
        t.Fatalf("first quality=%+v state=%d",observed,status)
    }
    if err:=mailbox.Accept(ref,wire[:],t0.Add(2*time.Second));!errors.Is(err,ErrQualityFeedbackReplay) {
        t.Fatalf("duplicate report was not rejected: %v",err)
    }
    older:=msg;older.ReportSeq--
    oldWire,_:=EncodeQualityHealthV3(older)
    if err:=mailbox.Accept(ref,oldWire[:],t0.Add(3*time.Second));!errors.Is(err,ErrQualityFeedbackReplay) {
        t.Fatalf("older sequence accepted: %v",err)
    }
    differentRef:=logicaltunnel.LaneRef{ID:2,Generation:ref.Generation}
    if err:=mailbox.Accept(differentRef,wire[:],t0.Add(3*time.Second));!errors.Is(err,ErrQualityFeedbackIdentity) {
        t.Fatalf("cross-lane replay accepted: %v",err)
    }
    rotated:=msg;rotated.ReportSeq++;rotated.Generation++;rotated.IncarnationNonce[0]^=0xff
    rotatedWire,_:=EncodeQualityHealthV3(rotated)
    if err:=mailbox.Accept(ref,rotatedWire[:],t0.Add(3*time.Second));!errors.Is(err,ErrQualityFeedbackIdentity) {
        t.Fatalf("old lane accepted new-incarnation report: %v",err)
    }
    another:=msg;another.ReportSeq++;another.IncarnationNonce[1]^=1
    anotherWire,_:=EncodeQualityHealthV3(another)
    if err:=mailbox.Accept(ref,anotherWire[:],t0.Add(3*time.Second));!errors.Is(err,ErrQualityFeedbackIdentity) {
        t.Fatalf("same generation but different incarnation accepted: %v",err)
    }
    if got,state:=mailbox.Snapshot(t0.Add(4*time.Second),0);state!=QualityFeedbackEstimated || got.ReportSeq!=msg.ReportSeq {
        t.Fatalf("rejected inputs mutated accepted snapshot: %+v %d",got,state)
    }
    // A new gate per rotation must never inherit the previous report sequence.
    nextRef:=logicaltunnel.LaneRef{ID:1,Generation:rotated.Generation}
    next,err:=NewQualityFeedbackMailbox(nextRef,rotated.IncarnationNonce)
    if err!=nil {t.Fatal(err)}
    if err:=next.Accept(nextRef,rotatedWire[:],t0.Add(time.Second));err!=nil {t.Fatal(err)}
    if err:=next.Accept(ref,wire[:],t0.Add(3*time.Second));!errors.Is(err,ErrQualityFeedbackIdentity) {
        t.Fatalf("old generation updated new mailbox: %v",err)
    }
}

func TestN1QualityFeedbackMailboxStaleInsufficientCapacityUnknown(t *testing.T) {
    base:=qualityHealthVector()
    ref:=logicaltunnel.LaneRef{ID:1,Generation:base.Generation}
    cases:=[]struct{
        name string
        flags byte
        age uint16
        state QualityFeedbackState
    }{
        {"estimated",base.Flags,100,QualityFeedbackEstimated},
        {"missing-watermark",QualityFlagWindowValid,100,QualityFeedbackUnknown},
        {"unknown-age",base.Flags,0xffff,QualityFeedbackUnknown},
        {"insufficient",base.Flags|QualityFlagInsufficient,100,QualityFeedbackInsufficient},
        {"local-capacity",base.Flags|QualityFlagCapacityLimited,100,QualityFeedbackCapacity},
        {"explicit-peer-stale",base.Flags|QualityFlagStale,100,QualityFeedbackStale},
    }
    t0:=time.Unix(200,0)
    for _,tc:=range cases {
        t.Run(tc.name,func(t *testing.T){
            mailbox,err:=NewQualityFeedbackMailbox(ref,base.IncarnationNonce)
            if err!=nil{t.Fatal(err)}
            data:=base;data.Flags=tc.flags;data.AgeMillis=tc.age
            if tc.flags & QualityFlagLossEstimated == 0 {
                data.EstimatedMissing=0;data.PeakMissingBurst=0
            }
            wire,err:=EncodeQualityHealthV3(data);if err!=nil{t.Fatal(err)}
            if err:=mailbox.Accept(ref,wire[:],t0);err!=nil{t.Fatal(err)}
            if _,state:=mailbox.Snapshot(t0.Add(9*time.Second),time.Second);state!=tc.state {
                t.Fatalf("state=%d want=%d",state,tc.state)
            }
            if _,state:=mailbox.Snapshot(t0.Add(10*time.Second),time.Second);state!=QualityFeedbackStale {
                t.Fatalf("stale threshold failed: %d",state)
            }
            if _,state:=mailbox.Snapshot(t0.Add(15*time.Second),20*time.Second);state!=tc.state {
                t.Fatalf("caller extended >=10s freshness invalid: state=%d want=%d",state,tc.state)
            }
        })
    }
}

func TestN1QualityFeedbackMailboxConcurrentLatestBounded(t *testing.T) {
    base:=qualityHealthVector()
    ref:=logicaltunnel.LaneRef{ID:1,Generation:base.Generation}
    m,err:=NewQualityFeedbackMailbox(ref,base.IncarnationNonce)
    if err!=nil{t.Fatal(err)}
    t0:=time.Unix(300,0)
    const reports=128
    var wg sync.WaitGroup
    for i:=1;i<=reports;i++ {
        i:=i
        wg.Add(1)
        go func(){
            defer wg.Done()
            report:=base
            report.ReportSeq=uint64(i)
            wire,e:=EncodeQualityHealthV3(report)
            if e!=nil {t.Error(e);return}
            // Reordering is legal: older reports may lose the publication
            // race, but they cannot replace the latest accepted sequence.
            e=m.Accept(ref,wire[:],t0.Add(time.Duration(i)*time.Millisecond))
            if e!=nil && !errors.Is(e,ErrQualityFeedbackReplay) {
                t.Error(e)
            }
        }()
    }
    wg.Wait()
    got,state:=m.Snapshot(t0.Add(2*time.Second),0)
    if state!=QualityFeedbackEstimated || got.ReportSeq!=reports {
        t.Fatalf("latest retained seq=%d state=%d; want %d",got.ReportSeq,state,reports)
    }
    if err:=m.Accept(ref,[]byte{1,2,3},t0.Add(3*time.Second));!errors.Is(err,ErrQualityHealthV3) {
        t.Fatalf("malformed control accepted: %v",err)
    }
    if got,state:=m.Snapshot(t0.Add(4*time.Second),0);state!=QualityFeedbackEstimated||got.ReportSeq!=reports {
        t.Fatalf("malformed report polluted latest: %+v state=%d",got,state)
    }
}

func TestN1QualityFeedbackMailboxPeerGenerationMayDifferFromLocal(t *testing.T) {
    m:=qualityHealthVector()
    local:=logicaltunnel.LaneRef{ID:2,Generation:99}
    gate,err:=NewQualityFeedbackMailbox(local,m.IncarnationNonce)
    if err!=nil {t.Fatal(err)}
    t0:=time.Unix(1000,0)
    first,err:=EncodeQualityHealthV3(m)
    if err!=nil {t.Fatal(err)}
    if err:=gate.Accept(local,first[:],t0);err!=nil{
        t.Fatalf("independent peer generation must be accepted: %v",err)
    }
    changed:=m;changed.ReportSeq++;changed.Generation++
    wrong,err:=EncodeQualityHealthV3(changed)
    if err!=nil {t.Fatal(err)}
    if err:=gate.Accept(local,wrong[:],t0.Add(time.Second));!errors.Is(err,ErrQualityFeedbackIdentity){
        t.Fatalf("same nonce changed peer generation accepted: %v",err)
    }
    next:=m;next.ReportSeq++
    ok,err:=EncodeQualityHealthV3(next)
    if err!=nil {t.Fatal(err)}
    if err:=gate.Accept(local,ok[:],t0.Add(2*time.Second));err!=nil {
        t.Fatalf("peer generation pinned but valid new seq rejected: %v",err)
    }
}
