package runtimeowner

import (
    "net/netip"
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestN1QualityV3ActualEchoReturnedSenderCorrelatedAndN2Hold(t *testing.T) {
    lease:=runtimeLease(t)
    leaseAddr,_:=lease.Config.LeaseIPv4()
    cOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
    if err!=nil{t.Fatal(err)}
    sOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
    if err!=nil{t.Fatal(err)}
    client,err:=New(cOwner,nil);if err!=nil{t.Fatal(err)}
    defer client.Close()
    server,err:=New(sOwner,nil);if err!=nil{t.Fatal(err)}
    defer server.Close()
    var c2s,s2c []faketcp.Segment
    cc,sc:=transportPair(
        func(seg faketcp.Segment)error{c2s=append(c2s,seg);return nil},
        func(seg faketcp.Segment)error{s2c=append(s2c,seg);return nil},
        1,52001)
    cl:=runtimeLane(t,datapath.RoleClient,lease,0,119)
    sl:=runtimeLane(t,datapath.RoleServer,lease,0,119)
    nonce:=cl.Config().IncarnationNonce
    cr,err:=client.AttachInitial(1,cl,cc);if err!=nil{t.Fatal(err)}
    sr,err:=server.AttachInitial(1,sl,sc);if err!=nil{t.Fatal(err)}
    if err:=client.ConfigureHealth(cr.Ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
    if err:=client.ConfigureQualityV3(cr.Ref,nonce);err!=nil{t.Fatal(err)}
    if err:=server.ConfigureHealth(sr.Ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
    if err:=server.ConfigureQualityV3(sr.Ref,nonce);err!=nil{t.Fatal(err)}
    start:=time.Unix(900,0)
    if err:=client.Tick(start);err!=nil{t.Fatal(err)}
    if err:=server.Tick(start);err!=nil{t.Fatal(err)}
    // The original v2 sender report goes in each independent direction.
    for _,seg:=range c2s {
        if len(seg.Payload)!=0 {
            if err:=server.HandleSegment(sr.Ref,seg,start);err!=nil{t.Fatal(err)}
        }
    }
    for _,seg:=range s2c {
        if len(seg.Payload)!=0 {
            if err:=client.HandleSegment(cr.Ref,seg,start);err!=nil{t.Fatal(err)}
        }
    }
    initialSent:=len(c2s)
    var omittedSeq uint32
    for i:=0;i<20;i++ {
        pkt:=runtimeIPv4(leaseAddr,netip.MustParseAddr("1.1.1.1"),[]byte{byte(i),4,5,6})
        records,err:=cOwner.NormalOutbound(pkt,start.Add(time.Millisecond))
        if err!=nil||len(records)!=1{t.Fatalf("data %d records=%d err=%v",i,len(records),err)}
        if err:=client.SendNormal(records,start.Add(time.Millisecond));err!=nil{t.Fatal(err)}
    }
    if len(c2s)!=initialSent+20 {t.Fatalf("DATA emission count=%d",len(c2s)-initialSent)}
    omittedSeq=c2s[len(c2s)-1].Seq
    sendToServer:=func(from int,now time.Time){
        t.Helper()
        for _,seg:=range c2s[from:] {
            if len(seg.Payload)==0||seg.Seq==omittedSeq {continue}
            if err:=server.HandleSegment(sr.Ref,seg,now);err!=nil{t.Fatal(err)}
        }
    }
    sendToClient:=func(from int,now time.Time){
        t.Helper()
        for _,seg:=range s2c[from:] {
            if len(seg.Payload)==0{continue}
            if err:=client.HandleSegment(cr.Ref,seg,now);err!=nil{t.Fatal(err)}
        }
    }
    sendToServer(initialSent,start.Add(100*time.Millisecond))
    beforeC:=len(c2s)
    if err:=client.Tick(start.Add(2*time.Second));err!=nil{t.Fatal(err)}
    sendToServer(beforeC,start.Add(2*time.Second))
    beforeS:=len(s2c)
    if err:=server.Tick(start.Add(2*time.Second));err!=nil{t.Fatal(err)}
    sendToClient(beforeS,start.Add(2*time.Second))
    pre,ok:=client.QualityEchoFeedback(cr.Ref,start.Add(2*time.Second))
    if !ok||pre.State==datapath.QualityFeedbackEstimated{
        t.Fatalf("early echo cannot advertise a mature loss estimate: %+v enabled=%v",pre,ok)
    }
    // Same sender 20-record watermark repeated; server may include an
    // insufficient echo in the same 2s slot instead of 104B health.
    beforeC=len(c2s)
    if err:=client.Tick(start.Add(4*time.Second));err!=nil{t.Fatal(err)}
    sendToServer(beforeC,start.Add(4*time.Second))
    beforeS=len(s2c)
    if err:=server.Tick(start.Add(4*time.Second));err!=nil{t.Fatal(err)}
    sendToClient(beforeS,start.Add(4*time.Second))
    if status,_:=client.QualityEchoFeedback(cr.Ref,start.Add(4*time.Second));status.State==datapath.QualityFeedbackEstimated{
        t.Fatalf("2s stable is not mature: %+v",status)
    }
    beforeS=len(s2c)
    if err:=server.Tick(start.Add(6*time.Second));err!=nil{t.Fatal(err)}
    sendToClient(beforeS,start.Add(6*time.Second))
    result,enabled:=client.QualityEchoFeedback(cr.Ref,start.Add(6*time.Second))
    if !enabled || !result.Valid || result.State!=datapath.QualityFeedbackEstimated ||
        result.EstimatedMissing!=1 || result.ReceiverUnique!=19 ||
        result.Fraction!=0.05 || result.SourceGeneration!=cr.Ref.Generation {
        t.Fatalf("auth correlated remote echo not received: %+v enabled=%v",result,enabled)
    }
    // The original (unextended) v2 quality summary still describes only
    // the server's OWN outbound TX PN window; no auto FEC may read it as a
    // same-direction physical loss measurement.
    own,state,ok:=client.QualityFeedback(cr.Ref,start.Add(6*time.Second),0)
    if !ok||state!=datapath.QualityFeedbackInsufficient ||
        own.Flags&datapath.QualityFlagLossEstimated!=0 || own.EstimatedMissing!=0 {
        t.Fatalf("N2 hold / own outbound field pollution: %+v state=%v",own,state)
    }
    old,_:=client.QualityEchoFeedback(cr.Ref,start.Add(17*time.Second))
    if old.State!=datapath.QualityFeedbackStale {
        t.Fatalf("echo without fresh source must go STALE, not 0%% loss: %+v",old)
    }
    if _,err:=client.Dormant();err!=nil{t.Fatal(err)}
    if _,enabled:=client.QualityEchoFeedback(cr.Ref,start.Add(18*time.Second));enabled {
        t.Fatal("dormant lane disclosed old feedback as current")
    }
}

func TestN1QualityV3ReturnedEchoIdentityAndHistoricalSourceFences(t *testing.T){
    now:=time.Unix(1234,0)
    var q qualityState
    q.nonce[0]=8
    m:=datapath.QualityHealthReport{
        Flags:datapath.QualityFlagWindowValid|datapath.QualityFlagInsufficient,
        ReportSeq:7,Generation:5,
        WindowFirstPN:1,WindowLastPN:30,TxSuccess:20,
        AgeMillis:5,RTTMillis:0xffff,
    }
    m.IncarnationNonce=q.nonce
    q.recordSentSource(m,now)
    e:=datapath.QualityEchoV4{
        Report:m,State:datapath.QualityFeedbackEstimated,
        SourceReportSeq:7,SourceGeneration:5,SourceLastPN:30,
        ReceiverUnique:19,EstimatedMissing:1,
    }
    if !q.acceptReturnedEcho(e,now.Add(time.Second),5) {
        t.Fatal("valid recent current-source echo rejected")
    }
    for _,tc:=range []struct{name string;change func(*datapath.QualityEchoV4);at time.Time;gen uint64}{
        {"remote-local-generation-mismatch",func(e *datapath.QualityEchoV4){e.SourceGeneration=6},now.Add(time.Second),5},
        {"not-in-history",func(e *datapath.QualityEchoV4){e.SourceReportSeq=8},now.Add(time.Second),5},
        {"same-seq-wrong-PN",func(e *datapath.QualityEchoV4){e.SourceLastPN=29},now.Add(time.Second),5},
        {"wrong-admission-nonce",func(e *datapath.QualityEchoV4){e.Report.IncarnationNonce[0]++},now.Add(time.Second),5},
        {"invented-missing",func(e *datapath.QualityEchoV4){e.EstimatedMissing=2},now.Add(time.Second),5},
        {"receive-exceeds-sent",func(e *datapath.QualityEchoV4){e.ReceiverUnique=21},now.Add(time.Second),5},
        {"report-too-old",func(e *datapath.QualityEchoV4){},now.Add(11*time.Second),5},
        {"retired-generation",func(e *datapath.QualityEchoV4){},now.Add(time.Second),6},
    } {
        t.Run(tc.name,func(t *testing.T) {
            other:=e
            tc.change(&other)
            if q.acceptReturnedEcho(other,tc.at,tc.gen){
                t.Fatalf("unmatched source report accepted: %+v",other)
            }
        })
    }
}
