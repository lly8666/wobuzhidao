package runtimeowner

import (
 "net/netip"
 "testing"
 "time"

 "github.com/lly8666/wobuzhidao/internal/datapath"
 "github.com/lly8666/wobuzhidao/internal/faketcp"
)

func TestN1ControlNewDataPNReportsDespiteStableInsufficientNumericState(t *testing.T) {
 lease:=runtimeLease(t)
 leaseAddr,_:=lease.Config.LeaseIPv4()
 clientOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
 if err!=nil {t.Fatal(err)}
 serverOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
 if err!=nil {t.Fatal(err)}
 client,err:=New(clientOwner,nil)
 if err!=nil {t.Fatal(err)}
 defer client.Close()
 server,err:=New(serverOwner,nil)
 if err!=nil {t.Fatal(err)}
 defer server.Close()
 var c2s []faketcp.Segment
 cc,sc:=transportPair(
  func(seg faketcp.Segment)error{c2s=append(c2s,seg);return nil},
  func(faketcp.Segment)error{return nil},
  1,54801)
 cl:=runtimeLane(t,datapath.RoleClient,lease,0,122)
 sl:=runtimeLane(t,datapath.RoleServer,lease,0,122)
 cr,err:=client.AttachInitial(1,cl,cc)
 if err!=nil{t.Fatal(err)}
 sr,err:=server.AttachInitial(1,sl,sc)
 if err!=nil{t.Fatal(err)}
 for _,side:=range []struct{r *Runtime;refIsClient bool}{{client,true},{server,false}}{
  ref:=sr.Ref
  if side.refIsClient{ref=cr.Ref}
  if err:=side.r.ConfigureHealth(ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
  if err:=side.r.ConfigureQualityV3(ref,cl.Config().IncarnationNonce);err!=nil{t.Fatal(err)}
 }
 t0:=time.Unix(14000,0)
 delivered:=0
 deliver:=func(at time.Time){
  t.Helper()
  // Deliver original and same-Seq retransmitted records in order. A Tick
  // can produce RTO ciphertext AFTER the new quality health record.
  for _,seg:=range c2s[delivered:]{
   if len(seg.Payload)==0{continue}
   if err:=server.HandleSegment(sr.Ref,seg,at);err!=nil{t.Fatal(err)}
  }
  delivered=len(c2s)
 }
 if err:=client.Tick(t0);err!=nil{t.Fatal(err)}
 deliver(t0)
 emitData:=func(i int,at time.Time){
  t.Helper()
  pkt:=runtimeIPv4(leaseAddr,netip.MustParseAddr("1.1.1.1"),[]byte{byte(i),71,72,73})
  records,err:=clientOwner.NormalOutbound(pkt,at)
  if err!=nil||len(records)!=1 {t.Fatalf("DATA %d: records=%d err=%v",i,len(records),err)}
  if err:=client.SendNormal(records,at);err!=nil{t.Fatal(err)}
 }
 for i:=0;i<20;i++{emitData(i,t0.Add(100*time.Millisecond))}
 deliver(t0.Add(100*time.Millisecond))
 if err:=client.Tick(t0.Add(2*time.Second));err!=nil{t.Fatal(err)}
 deliver(t0.Add(2*time.Second))
 first,state,ok:=server.QualityFeedback(sr.Ref,t0.Add(2*time.Second),0)
 if !ok||state!=datapath.QualityFeedbackInsufficient||first.ReportSeq!=2||first.TxSuccess!=20 {
  t.Fatalf("first DATA watermark report missing: %+v state=%v enabled=%v",first,state,ok)
 }
 if err:=client.Tick(t0.Add(4*time.Second));err!=nil{t.Fatal(err)}
 stats,ok:=client.ControlStats(cr.Ref)
 if !ok||stats.QualityEmitted!=2||stats.SuppressedNoNewObservation==0 {
  t.Fatalf("stale unchanged DATA may not keep reissuing old report: %+v enabled=%v",stats,ok)
 }
 emitData(21,t0.Add(4250*time.Millisecond))
 deliver(t0.Add(4250*time.Millisecond))
 if err:=client.Tick(t0.Add(6*time.Second));err!=nil{t.Fatal(err)}
 deliver(t0.Add(6*time.Second))
 newest,state,ok:=server.QualityFeedback(sr.Ref,t0.Add(6*time.Second),0)
 if !ok||state!=datapath.QualityFeedbackInsufficient ||
  newest.ReportSeq!=3||newest.WindowLastPN<=first.WindowLastPN||
  newest.TxSuccess<=first.TxSuccess||newest.Flags!=first.Flags {
  t.Fatalf("new PN with same quality STATE must report: old=%+v new=%+v state=%v",first,newest,state)
 }
 stats,_=client.ControlStats(cr.Ref)
 if stats.QualityNewWindow<2||stats.QualityEmitted!=3 {
  t.Fatalf("new source watermark not counted exactly once: %+v",stats)
 }
}

func TestN1ControlNoEchoPingPongOnPeerReportSequenceOnly(t *testing.T){
 lease:=runtimeLease(t)
 aOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
 if err!=nil{t.Fatal(err)}
 bOwner,err:=datapath.NewLeasedTunnelOwner(lease,1,8)
 if err!=nil{t.Fatal(err)}
 a,err:=New(aOwner,nil)
 if err!=nil{t.Fatal(err)}
 defer a.Close()
 b,err:=New(bOwner,nil)
 if err!=nil{t.Fatal(err)}
 defer b.Close()
 var a2b,b2a []faketcp.Segment
 ac,bc:=transportPair(
  func(seg faketcp.Segment)error{a2b=append(a2b,seg);return nil},
  func(seg faketcp.Segment)error{b2a=append(b2a,seg);return nil},
  1,54802)
 al:=runtimeLane(t,datapath.RoleClient,lease,0,123)
 bl:=runtimeLane(t,datapath.RoleServer,lease,0,123)
 ar,err:=a.AttachInitial(1,al,ac);if err!=nil{t.Fatal(err)}
 br,err:=b.AttachInitial(1,bl,bc);if err!=nil{t.Fatal(err)}
 for _,side:=range []struct{r *Runtime;isA bool}{{a,true},{b,false}}{
  ref:=br.Ref
  if side.isA{ref=ar.Ref}
  if err:=side.r.ConfigureHealth(ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
  if err:=side.r.ConfigureQualityV3(ref,al.Config().IncarnationNonce);err!=nil{t.Fatal(err)}
 }
 aDelivered,bDelivered:=0,0
 deliver:=func(now time.Time){
  t.Helper()
  for _,seg:=range a2b[aDelivered:]{
   if len(seg.Payload)>0 {
    if err:=b.HandleSegment(br.Ref,seg,now);err!=nil{t.Fatal(err)}
   }
  }
  aDelivered=len(a2b)
  for _,seg:=range b2a[bDelivered:]{
   if len(seg.Payload)>0 {
    if err:=a.HandleSegment(ar.Ref,seg,now);err!=nil{t.Fatal(err)}
   }
  }
  bDelivered=len(b2a)
 }
 t0:=time.Unix(14100,0)
 if err:=a.Tick(t0);err!=nil{t.Fatal(err)}
 if err:=b.Tick(t0);err!=nil{t.Fatal(err)}
 deliver(t0)
 if err:=a.Tick(t0.Add(2*time.Second));err!=nil{t.Fatal(err)}
 if err:=b.Tick(t0.Add(2*time.Second));err!=nil{t.Fatal(err)}
 deliver(t0.Add(2*time.Second))
 for _,sec:=range []int{4,6,8}{
  at:=t0.Add(time.Duration(sec)*time.Second)
  if err:=a.Tick(at);err!=nil{t.Fatal(err)}
  if err:=b.Tick(at);err!=nil{t.Fatal(err)}
  deliver(at)
 }
 for _,side:=range []struct{name string;r *Runtime;isA bool}{{"a",a,true},{"b",b,false}}{
  ref:=br.Ref
  if side.isA{ref=ar.Ref}
  stats,_:=side.r.ControlStats(ref)
  if stats.QualityEmitted!=2||stats.SuppressedNoNewObservation==0 {
   t.Fatalf("%s: peer echo reportSeq alone must not ping-pong; %+v",side.name,stats)
  }
 }
}
