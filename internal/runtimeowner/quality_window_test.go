package runtimeowner

import (
    "errors"
    "net/netip"
    "testing"
    "time"

    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestN1QualityV3TransmitWindowOnlyPostEmitDataAndBoundedNoPNGapLoss(t *testing.T) {
    var w qualityTXWindow
    now:=time.Unix(300,0)
    if got:=w.snapshot(now);got.HasAttempts || got.Success!=0 || got.AgeMillis!=0xffff {
        t.Fatalf("no outbound attempted DATA must remain UNKNOWN: %+v",got)
    }
    w.note(0,true,now)
    w.note(1,false,now)
    // PN 2 skipped (could be KindHealth, failed sealing or other control).
    w.note(3,true,now.Add(time.Millisecond))
    w.note(3,true,now.Add(time.Millisecond)) // idempotent repeat
    got:=w.snapshot(now.Add(2*time.Millisecond))
    if !got.HasAttempts || got.FirstPN!=0 || got.LastPN!=3 ||
        got.Success!=2 || got.LocalDrops!=1 || got.AgeMillis!=1 {
        t.Fatalf("gaps/retries must not be called WAN loss: %+v",got)
    }
    w.note(300,false,now.Add(3*time.Millisecond))
    got=w.snapshot(now.Add(4*time.Millisecond))
    if got.Success!=0 || got.LocalDrops!=1 || got.AgeMillis!=0xffff ||
        got.FirstPN!=45 || got.LastPN!=300 {
        t.Fatalf("old success leaked into recent range: %+v",got)
    }
    w.note(2,true,now.Add(4*time.Millisecond))
    if got=w.snapshot(now.Add(5*time.Millisecond));!got.CapacityLimited ||
        got.Success!=0 || got.LocalDrops!=1 {
        t.Fatalf("out-of-capacity completion must not corrupt current PN span: %+v",got)
    }
}

func TestN1QualityV3TransmitPNNearMaxAndFeedbackAgeSaturates(t *testing.T) {
    var w qualityTXWindow
    now:=time.Unix(400,0)
    hi:=^uint64(0)
    w.note(hi-2,true,now)
    w.note(hi-1,false,now)
    w.note(hi,true,now.Add(time.Millisecond))
    got:=w.snapshot(now.Add(90*time.Second))
    if got.LastPN!=hi || got.FirstPN!=hi-(datapath.QualityPNWindowCapacity-1) ||
        got.Success!=2 || got.LocalDrops!=1 || got.AgeMillis!=0xfffe {
        t.Fatalf("no PN wrap/zero-age overflow: %+v",got)
    }
}

func TestN1QualityV3RealEmitOutcomeAndAuthenticatedInboundWindow(t *testing.T) {
    lease:=runtimeLease(t)
    addr,_:=lease.Config.LeaseIPv4()
    cOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil {t.Fatal(err)}
    sOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil {t.Fatal(err)}
    client,err:=New(cOwner,nil);if err!=nil {t.Fatal(err)}
    defer client.Close()
    server,err:=New(sOwner,nil);if err!=nil {t.Fatal(err)}
    defer server.Close()
    var c2s []faketcp.Segment
    failNext:=false
    failed:=errors.New("injected local Emit DATA failure, never WAN loss")
    cc,sc:=transportPair(func(seg faketcp.Segment)error{
        if failNext && len(seg.Payload)>0 {failNext=false;return failed}
        c2s=append(c2s,seg);return nil
    },func(faketcp.Segment)error{return nil},1,75100)
    cl:=runtimeLane(t,datapath.RoleClient,lease,0,94)
    sl:=runtimeLane(t,datapath.RoleServer,lease,0,94)
    nonce:=cl.Config().IncarnationNonce
    c,err:=client.AttachInitial(1,cl,cc);if err!=nil {t.Fatal(err)}
    d,err:=server.AttachInitial(1,sl,sc);if err!=nil {t.Fatal(err)}
    for _,endpoint:=range []struct{r *Runtime;ref uint8}{ {client,c.Ref.ID},{server,d.Ref.ID} }{
        ref:=c.Ref
        if endpoint.r==server {ref=d.Ref}
        if err:=endpoint.r.ConfigureHealth(ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
        if err:=endpoint.r.ConfigureQualityV3(ref,nonce);err!=nil{t.Fatal(err)}
    }
    t0:=time.Unix(800,0)
    if err:=client.Tick(t0);err!=nil{t.Fatal(err)} // emits health v1 and v2 controls
    if got,ok:=client.QualityTransmitWindow(c.Ref,t0);!ok || got.HasAttempts {
        t.Fatalf("health-only PNs must not count DATA: %+v enabled=%v",got,ok)
    }
    packet:=runtimeIPv4(addr,netip.MustParseAddr("1.1.1.1"),[]byte("N1-first-business"))
    records,err:=cOwner.NormalOutbound(packet,t0.Add(time.Millisecond))
    if err!=nil || len(records)!=1 {t.Fatalf("client normal outbound=%d err=%v",len(records),err)}
    successPN:=records[0].PN
    if err:=client.SendNormal(records,t0.Add(time.Millisecond));err!=nil {t.Fatal(err)}
    failedRecords,err:=cOwner.NormalOutbound(packet,t0.Add(2*time.Millisecond))
    if err!=nil || len(failedRecords)!=1 {t.Fatalf("second normal outbound=%d err=%v",len(failedRecords),err)}
    failNext=true
    if err:=client.SendNormal(failedRecords,t0.Add(2*time.Millisecond));!errors.Is(err,failed){
        t.Fatalf("failed local Emit not surfaced: %v",err)
    }
    got,ok:=client.QualityTransmitWindow(c.Ref,t0.Add(3*time.Millisecond))
    if !ok || got.Success!=1 || got.LocalDrops!=1 || got.LastPN!=failedRecords[0].PN ||
        got.FirstPN!=0 || got.AgeMillis!=2 {
        t.Fatalf("post-Emit DATA ledger should exclude health and local failure from sent: %+v enabled=%v",got,ok)
    }
    for _,seg:=range c2s {
        if len(seg.Payload)>0 {
            if err:=server.HandleSegment(d.Ref,seg,t0.Add(4*time.Millisecond));err!=nil{t.Fatal(err)}
        }
    }
    inbound,ok:=server.QualityReceiveWindow(d.Ref,t0.Add(5*time.Millisecond))
    if !ok || inbound.Unique!=1 || inbound.LastPN!=successPN || inbound.Duplicate!=0 {
        t.Fatalf("authenticated receive DATA count excludes health: %+v enabled=%v",inbound,ok)
    }
    // Bypass outer TCP segment replay short-circuit to exercise the actual
    // independently authenticated tlsrecord duplicate branch on the SAME wire.
    direct,err:=sl.InboundPayload(records[0].Wire,t0.Add(6*time.Millisecond))
    if err!=nil {t.Fatal(err)}
    if len(direct.RecordErrors)!=1 {
        // exact duplicate error comes from tlsrecord; only one rejected record.
        t.Fatalf("expected exactly one replayed protected DATA record: %+v",direct)
    }
    inbound,ok=server.QualityReceiveWindow(d.Ref,t0.Add(7*time.Millisecond))
    if !ok || inbound.Unique!=1 || inbound.Duplicate!=1 {
        t.Fatalf("cryptographic replay must not increase first-unique count: %+v",inbound)
    }
    // Duplicate authenticated health is NOT a duplicate DATA record.
    healthDuplicate,err:=sl.InboundPayload(c2s[0].Payload,t0.Add(8*time.Millisecond))
    if err!=nil || len(healthDuplicate.RecordErrors)!=1 {
        t.Fatalf("health replay must fail cryptographic de-duplication: %+v, %v",healthDuplicate,err)
    }
    inbound,ok=server.QualityReceiveWindow(d.Ref,t0.Add(9*time.Millisecond))
    if !ok || inbound.Unique!=1 || inbound.Duplicate!=1 {
        t.Fatalf("health replay polluted DATA-only counters: %+v",inbound)
    }
    beforeTick:=len(c2s)
    if err:=client.Tick(t0.Add(2*time.Second));err!=nil {t.Fatal(err)}
    // The runtime 2s tick can also schedule same-Seq FakeTCP RTO recovery.
    // The *last* segment is not guaranteed to be the newest KindHealth v2;
    // deliver all fresh emissions in the same order as the real outer path.
    // A repair of prior ciphertext must remain idempotent at the decoder.
    for _,seg:=range c2s[beforeTick:] {
        if len(seg.Payload)==0 {continue}
        if err:=server.HandleSegment(d.Ref,seg,t0.Add(2*time.Second));err!=nil{t.Fatal(err)}
    }
    // The new v2 quality has a sender-success window but no matched
    // RX watermark and MUST remain explicitly INSUFFICIENT, not ESTIMATED.
    report,state,enabled:=server.QualityFeedback(d.Ref,t0.Add(2*time.Second),0)
    if !enabled || state!=datapath.QualityFeedbackInsufficient ||
        report.Flags&datapath.QualityFlagWindowValid==0 ||
        report.Flags&datapath.QualityFlagLossEstimated!=0 ||
        report.TxSuccess!=1 || report.LocalDrops!=1 ||
        report.WindowLastPN!=failedRecords[0].PN ||
        report.RxUnique!=0 || report.EstimatedMissing!=0 {
        t.Fatalf("no unpaired PN/WAN loss estimate allowed: report=%+v state=%d enabled=%v",report,state,enabled)
    }
}
