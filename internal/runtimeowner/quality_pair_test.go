package runtimeowner

import (
 "net/netip"
 "testing"
 "time"

 "github.com/lly8666/wobuzhidao/internal/datapath"
 "github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestN1QualityV3PairedWatermarkStrictInsufficientCapacityAndMaturity(t *testing.T) {
 t0:=time.Unix(500,0)
 m:=datapath.QualityHealthReport{
  Flags:datapath.QualityFlagInsufficient|datapath.QualityFlagWindowValid,
  ReportSeq:1,Generation:7,WindowFirstPN:0,WindowLastPN:20,
  TxSuccess:20,AgeMillis:1000,RTTMillis:0xffff,
 }
 m.IncarnationNonce[0]=77
 rx:=datapath.QualityReceiveRange{
  FirstPN:0,LastPN:20,Unique:19,HaveAnchor:true,RangeRetained:true,
  AgeMillis:1000,
 }
 c:=qualityPairCandidate{}
 if got:=evaluateQualityPair(c,rx,t0);got.State!=datapath.QualityFeedbackUnknown {t.Fatalf("without authenticated sender=%+v",got)}
 c.accept(m,21,t0)
 if got:=evaluateQualityPair(c,rx,t0.Add(2*time.Second));got.State!=datapath.QualityFeedbackInsufficient {
  t.Fatalf("not mature yet: %+v",got)
 }
 m.ReportSeq=2
 c.accept(m,22,t0.Add(2*time.Second))
 got:=evaluateQualityPair(c,rx,t0.Add(3*time.Second))
 if got.State!=datapath.QualityFeedbackEstimated || got.EstimatedMissing!=1 ||
  got.Fraction!=0.05 || got.SenderSuccess!=20 || got.ReceiverUnique!=19 ||
  got.Age!=3*time.Second {
  t.Fatalf("mature authenticated matching inbound PN estimate=%+v",got)
 }
 table:=[]struct {
  name string
  edit func(*qualityPairCandidate,*datapath.QualityReceiveRange)
  want datapath.QualityFeedbackState
 }{
  {"missing-end-marker",func(c *qualityPairCandidate,_ *datapath.QualityReceiveRange){c.controlPN=20},datapath.QualityFeedbackInsufficient},
  {"wrong-pn-envelope",func(c *qualityPairCandidate,_ *datapath.QualityReceiveRange){c.report.WindowLastPN=400},datapath.QualityFeedbackInsufficient},
  {"receiver-horizon-truncated",func(_ *qualityPairCandidate,r *datapath.QualityReceiveRange){r.RangeRetained=false;r.CapacityLimited=true},datapath.QualityFeedbackCapacity},
  {"record-window-capacity",func(c *qualityPairCandidate,_ *datapath.QualityReceiveRange){c.report.Flags|=datapath.QualityFlagCapacityLimited},datapath.QualityFeedbackCapacity},
  {"only-seven-received",func(_ *qualityPairCandidate,r *datapath.QualityReceiveRange){r.Unique=7},datapath.QualityFeedbackInsufficient},
  {"sender-under-16",func(c *qualityPairCandidate,_ *datapath.QualityReceiveRange){c.report.TxSuccess=15},datapath.QualityFeedbackInsufficient},
  {"receiver-more-than-sender",func(_ *qualityPairCandidate,r *datapath.QualityReceiveRange){r.Unique=21},datapath.QualityFeedbackInsufficient},
  {"no-receiver-anchor",func(_ *qualityPairCandidate,r *datapath.QualityReceiveRange){r.HaveAnchor=false},datapath.QualityFeedbackInsufficient},
  {"unknown-data-age",func(c *qualityPairCandidate,_ *datapath.QualityReceiveRange){c.report.AgeMillis=0xffff},datapath.QualityFeedbackInsufficient},
  {"sender-too-old",func(c *qualityPairCandidate,_ *datapath.QualityReceiveRange){c.report.AgeMillis=10001},datapath.QualityFeedbackInsufficient},
  {"sender-claims-estimate",func(c *qualityPairCandidate,_ *datapath.QualityReceiveRange){c.report.Flags|=datapath.QualityFlagLossEstimated},datapath.QualityFeedbackInsufficient},
  {"stale-report",func(c *qualityPairCandidate,_ *datapath.QualityReceiveRange){c.lastSeen=t0.Add(-8*time.Second)},datapath.QualityFeedbackStale},
 }
 for _,tc:=range table {
  t.Run(tc.name,func(t *testing.T){
   cc:=c;rr:=rx;tc.edit(&cc,&rr)
   got:=evaluateQualityPair(cc,rr,t0.Add(3*time.Second))
   if got.State!=tc.want {t.Fatalf("got %+v; want state=%v",got,tc.want)}
  })
 }
 newer:=m
 newer.ReportSeq=3
 newer.TxSuccess=21
 c.accept(newer,23,t0.Add(4*time.Second))
 if got:=evaluateQualityPair(c,rx,t0.Add(5*time.Second));got.State!=datapath.QualityFeedbackInsufficient {
  t.Fatalf("changed sender watermark must restart maturity: %+v",got)
 }
}

func TestN1QualityV3ActualAuthenticatedPeerDirectionMissingTailMatured(t *testing.T) {
 lease:=runtimeLease(t)
 addr,_:=lease.Config.LeaseIPv4()
 clientOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil{t.Fatal(err)}
 serverOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil{t.Fatal(err)}
 client,err:=New(clientOwner,nil);if err!=nil{t.Fatal(err)}
 defer client.Close()
 server,err:=New(serverOwner,nil);if err!=nil{t.Fatal(err)}
 defer server.Close()
 var wire []faketcp.Segment
 cc,sc:=transportPair(func(seg faketcp.Segment)error{wire=append(wire,seg);return nil},
  func(faketcp.Segment)error{return nil},1,43120)
 cl:=runtimeLane(t,datapath.RoleClient,lease,0,105)
 sl:=runtimeLane(t,datapath.RoleServer,lease,0,105)
 nonce:=cl.Config().IncarnationNonce
 cr,err:=client.AttachInitial(1,cl,cc);if err!=nil{t.Fatal(err)}
 sr,err:=server.AttachInitial(1,sl,sc);if err!=nil{t.Fatal(err)}
 if err:=client.ConfigureHealth(cr.Ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
 if err:=client.ConfigureQualityV3(cr.Ref,nonce);err!=nil{t.Fatal(err)}
 if err:=server.ConfigureHealth(sr.Ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
 if err:=server.ConfigureQualityV3(sr.Ref,nonce);err!=nil{t.Fatal(err)}
 now:=time.Unix(600,0)
 if err:=client.Tick(now);err!=nil{t.Fatal(err)}
 for _,seg:=range wire {
  if len(seg.Payload)>0 {if err:=server.HandleSegment(sr.Ref,seg,now);err!=nil{t.Fatal(err)}}
 }
 cursor:=len(wire)
 var omittedSeq uint32
 for i:=0;i<20;i++ {
  pkt:=runtimeIPv4(addr,netip.MustParseAddr("1.1.1.1"),[]byte{byte(i),100,101,102})
  recs,err:=clientOwner.NormalOutbound(pkt,now.Add(time.Millisecond))
  if err!=nil||len(recs)!=1{t.Fatalf("outbound %d records=%d err=%v",i,len(recs),err)}
  if err:=client.SendNormal(recs,now.Add(time.Millisecond));err!=nil{t.Fatal(err)}
 }
 if got:=len(wire)-cursor;got!=20 {t.Fatalf("fresh data count=%d",got)}
 omittedSeq=wire[cursor+19].Seq // tail DATA never delivered even through later RTOs
 deliver:=func(from int,at time.Time) {
  t.Helper()
  for _,seg:=range wire[from:] {
   if len(seg.Payload)==0||seg.Seq==omittedSeq {continue}
   if err:=server.HandleSegment(sr.Ref,seg,at);err!=nil{t.Fatal(err)}
  }
 }
 deliver(cursor,now.Add(100*time.Millisecond))
 if e,ok:=server.QualityInboundEstimate(sr.Ref,now.Add(200*time.Millisecond));!ok||
  e.State!=datapath.QualityFeedbackInsufficient {
  t.Fatalf("missing peer sender watermark must hold: %+v enabled=%v",e,ok)
 }
 before:=len(wire)
 if err:=client.Tick(now.Add(2*time.Second));err!=nil{t.Fatal(err)}
 deliver(before,now.Add(2*time.Second))
 e,ok:=server.QualityInboundEstimate(sr.Ref,now.Add(3*time.Second))
 if !ok||e.State!=datapath.QualityFeedbackInsufficient {t.Fatalf("prior to 3s maturity=%+v enabled=%v",e,ok)}
 before=len(wire)
 if err:=client.Tick(now.Add(4*time.Second));err!=nil{t.Fatal(err)}
 deliver(before,now.Add(4*time.Second))
 e,ok=server.QualityInboundEstimate(sr.Ref,now.Add(5*time.Second))
 if !ok||e.State!=datapath.QualityFeedbackEstimated || e.SenderSuccess!=20 ||
  e.ReceiverUnique!=19 || e.EstimatedMissing!=1 || e.Fraction!=0.05 ||
  e.ControlPN<=e.SenderLastPN {
  t.Fatalf("mature peer TX/control PN vs local auth RX estimate=%+v enabled=%v",e,ok)
 }
 // Existing 104B mailbox still only holds the most recent peer report,
 // marked INSUFFICIENT; N2 sees no loss-estimated control flag.
 report,state,_:=server.QualityFeedback(sr.Ref,now.Add(5*time.Second),0)
 if state!=datapath.QualityFeedbackInsufficient ||
  report.Flags&datapath.QualityFlagLossEstimated!=0 ||
  report.EstimatedMissing!=0 {t.Fatalf("N2 must remain HOLD: %+v state=%d",report,state)}
}
