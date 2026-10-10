package runtimeowner

import (
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/faketcp"
)

// These tests use the production independent-record sealer, the FakeTCP
// transport and the actual runtime health/tick paths, not a synthetic
// quality codec-only callback. No Linux netns, physical WAN loss or GUI.
func TestN1QualityV3ActualProtectedControlBothDirectionsAndTwoSecondGate(t *testing.T) {
    lease:=runtimeLease(t)
    clientOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil {t.Fatal(err)}
    serverOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil {t.Fatal(err)}
    client,err:=New(clientOwner,nil);if err!=nil {t.Fatal(err)}
    defer client.Close()
    server,err:=New(serverOwner,nil);if err!=nil {t.Fatal(err)}
    defer server.Close()
    var c2s,s2c []faketcp.Segment
    clientCfg,serverCfg:=transportPair(
        func(seg faketcp.Segment) error {c2s=append(c2s,seg);return nil},
        func(seg faketcp.Segment) error {s2c=append(s2c,seg);return nil},1,13579)
    const seed=76
    clientLane:=runtimeLane(t,datapath.RoleClient,lease,4,seed)
    serverLane:=runtimeLane(t,datapath.RoleServer,lease,4,seed)
    nonce:=clientLane.Config().IncarnationNonce
    if nonce==([16]byte{}) || nonce!=serverLane.Config().IncarnationNonce {t.Fatal("not a shared authenticated test-incarnation nonce")}
    cr,err:=client.AttachInitial(1,clientLane,clientCfg);if err!=nil {t.Fatal(err)}
    sr,err:=server.AttachInitial(1,serverLane,serverCfg);if err!=nil {t.Fatal(err)}
    for _,side:=range []struct{r *Runtime;ref uint8;generation uint64}{{client,cr.Ref.ID,cr.Ref.Generation},{server,sr.Ref.ID,sr.Ref.Generation}}{
        realRef:=cr.Ref
        if side.r==server {realRef=sr.Ref}
        if err:=side.r.ConfigureHealth(realRef,15*time.Second,func(time.Time)time.Duration{return 0});err!=nil{t.Fatal(err)}
        if err:=side.r.ConfigureQualityV3(realRef,nonce);err!=nil{t.Fatal(err)}
    }
    t0:=time.Unix(500,0)
    if err:=client.Tick(t0);err!=nil{t.Fatal(err)}
    if err:=server.Tick(t0);err!=nil{t.Fatal(err)}
    // Deliver the entire issued protected FakeTCP record path both ways.
    // Control ACKs (if any) are not business or quality reports.
    for _,seg:=range c2s {
        if len(seg.Payload)>0 {
            if err:=server.HandleSegment(sr.Ref,seg,t0);err!=nil{t.Fatal(err)}
        }
    }
    for _,seg:=range s2c {
        if len(seg.Payload)>0 {
            if err:=client.HandleSegment(cr.Ref,seg,t0);err!=nil{t.Fatal(err)}
        }
    }
    for _,side:=range []struct{
        name string
        r *Runtime
        ref uint8
        generation uint64
    }{{"client",client,cr.Ref.ID,cr.Ref.Generation},{"server",server,sr.Ref.ID,sr.Ref.Generation}} {
        actual:=cr.Ref
        if side.r==server {actual=sr.Ref}
        got,state,enabled:=side.r.QualityFeedback(actual,t0.Add(time.Second),0)
        if !enabled || state!=datapath.QualityFeedbackInsufficient ||
            got.ReportSeq!=1 || got.Flags!=datapath.QualityFlagInsufficient ||
            got.AgeMillis!=0xffff || got.IncarnationNonce!=nonce {
            t.Fatalf("%s protected quality feedback=%+v status=%d enabled=%v",side.name,got,state,enabled)
        }
    }
    initial:=countN1PayloadSegments(c2s)
    if initial!=2 {t.Fatalf("initial c2s should emit one legacy health plus one quality control; got=%d",initial)}
    if err:=client.Tick(t0.Add(time.Second));err!=nil{t.Fatal(err)}
    if countN1PayloadSegments(c2s)!=initial {t.Fatal("quality control emitted before the 2s bound")}
    if err:=client.Tick(t0.Add(2*time.Second));err!=nil{t.Fatal(err)}
    if countN1PayloadSegments(c2s)!=initial+1 {t.Fatal("one 2s quality report not emitted")}
    newest:=c2s[len(c2s)-1]
    if err:=server.HandleSegment(sr.Ref,newest,t0.Add(2*time.Second));err!=nil{t.Fatal(err)}
    report,state,enabled:=server.QualityFeedback(sr.Ref,t0.Add(3*time.Second),0)
    if !enabled || state!=datapath.QualityFeedbackInsufficient || report.ReportSeq!=2 {
        t.Fatalf("bounded latest after second report=%+v state=%d enabled=%v",report,state,enabled)
    }
    if _,state,_:=server.QualityFeedback(sr.Ref,t0.Add(13*time.Second),0);state!=datapath.QualityFeedbackStale {
        t.Fatalf("quality incorrectly fresh after max(10s,4*SRTT); state=%d",state)
    }
    if _,_,enabled:=server.QualityFeedback(cr.Ref,t0,0);enabled && cr.Ref!=sr.Ref {
        t.Fatal("foreign lane ref leaked quality")
    }
    // After retiring/dormancy no periodic quality control may revive an owner.
    if _,err:=client.Dormant();err!=nil {t.Fatal(err)}
    before:=countN1PayloadSegments(c2s)
    if err:=client.Tick(t0.Add(30*time.Second));err!=nil{t.Fatal(err)}
    if after:=countN1PayloadSegments(c2s);after!=before {
        t.Fatalf("dormant client sent quality control: %d -> %d",before,after)
    }
}

func countN1PayloadSegments(in []faketcp.Segment) int {
    n:=0
    for _,seg:=range in {if len(seg.Payload)>0 {n++}}
    return n
}

func TestN1QualityV3DisabledLaneRetainsLegacyStrictHealth(t *testing.T) {
    lease:=runtimeLease(t)
    receiverOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,4);if err!=nil {t.Fatal(err)}
    senderOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,4);if err!=nil {t.Fatal(err)}
    recv,err:=New(receiverOwner,nil);if err!=nil{t.Fatal(err)}
    defer recv.Close()
    send,err:=New(senderOwner,nil);if err!=nil{t.Fatal(err)}
    defer send.Close()
    var wire []faketcp.Segment
    cCfg,sCfg:=transportPair(func(seg faketcp.Segment)error{wire=append(wire,seg);return nil},
        func(faketcp.Segment)error{return nil},1,14700)
    source:=runtimeLane(t,datapath.RoleClient,lease,0,44)
    target:=runtimeLane(t,datapath.RoleServer,lease,0,44)
    c,err:=send.AttachInitial(1,source,cCfg);if err!=nil{t.Fatal(err)}
    s,err:=recv.AttachInitial(1,target,sCfg);if err!=nil{t.Fatal(err)}
    if err:=send.ConfigureHealth(c.Ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
    if err:=send.ConfigureQualityV3(c.Ref,source.Config().IncarnationNonce);err!=nil{t.Fatal(err)}
    // target remains old-V2 equivalent: no EnableQualityHealthV3.
    if err:=send.Tick(time.Unix(800,0));err!=nil{t.Fatal(err)}
    for _,seg:=range wire {
        if len(seg.Payload)>0 {
            if err:=recv.HandleSegment(s.Ref,seg,time.Unix(800,0));err!=nil{t.Fatal(err)}
        }
    }
    _,_,enabled:=recv.QualityFeedback(s.Ref,time.Unix(801,0),0)
    if enabled {t.Fatal("unqualified V2-like receiver silently accepted V3 quality")}
    if _,err:=receiverOwner.QualityHealthRecord(s.Ref,datapath.QualityHealthReport{});err==nil {
        t.Fatal("unqualified lane was allowed to seal V3 quality")
    }
}
