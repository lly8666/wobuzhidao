package runtimeowner

import (
 "errors"
 "testing"
 "time"

 "github.com/lly8666/wobuzhidao/internal/datapath"
 "github.com/lly8666/wobuzhidao/internal/faketcp"
 "github.com/lly8666/wobuzhidao/internal/pathmtu"
 "github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

func TestN1ControlMergedQualitySatisfiesHealthIdleSuppressesRepeatsAndBoundsPadding(t *testing.T) {
 lease:=runtimeLease(t)
 co,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil{t.Fatal(err)}
 so,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil{t.Fatal(err)}
 c,err:=New(co,nil);if err!=nil{t.Fatal(err)}
 defer c.Close()
 server,err:=New(so,nil);if err!=nil{t.Fatal(err)}
 defer server.Close()
 var out []faketcp.Segment
 cfg,scfg:=transportPair(func(seg faketcp.Segment)error{
   out=append(out,seg);return nil
 },func(faketcp.Segment)error{return nil},1,34551)
 cfg.ControlPaddingMaxBytes=79
 cl:=runtimeLane(t,datapath.RoleClient,lease,0,101)
 sl:=runtimeLane(t,datapath.RoleServer,lease,0,101)
 nonce:=cl.Config().IncarnationNonce
 cr,err:=c.AttachInitial(1,cl,cfg);if err!=nil{t.Fatal(err)}
 sr,err:=server.AttachInitial(1,sl,scfg);if err!=nil{t.Fatal(err)}
 if err:=c.ConfigureHealth(cr.Ref,15*time.Second,func(time.Time)time.Duration{return 2*time.Minute});err!=nil{t.Fatal(err)}
 if err:=c.ConfigureQualityV3(cr.Ref,nonce);err!=nil{t.Fatal(err)}
 if err:=server.ConfigureHealth(sr.Ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
 if err:=server.ConfigureQualityV3(sr.Ref,nonce);err!=nil{t.Fatal(err)}
 t0:=time.Unix(9001,0)
 if err:=c.Tick(t0);err!=nil{t.Fatal(err)}
 if len(out)!=1{t.Fatalf("bootstrap tick sent %d instead of exactly one merged quality/idle",len(out))}
 budget,err:=pathmtu.Derive(cl.Config().TxMTU);if err!=nil{t.Fatal(err)}
 if len(out[0].Payload)>budget.RecordWireMTU||len(out[0].Payload)>datapath.QualityHealthV3Size+tlsrecord.FixedWireOverhead+cfg.ControlPaddingMaxBytes {
  t.Fatalf("control exceeded negotiated outer MTU/cap: %d vs %+v",len(out[0].Payload),budget)
 }
 if err:=server.HandleSegment(sr.Ref,out[0],t0);err!=nil{t.Fatal(err)}
 if !server.PeerIdle(t0.Add(time.Second),time.Minute){t.Fatal("merged quality IdleFor was not consumed by ordinary PeerIdle")}
 stats,_:=c.ControlStats(cr.Ref)
 if stats.QualityEmitted!=1||stats.HealthEmitted!=0||stats.MergedHealth!=1 {
  t.Fatalf("bootstrap must merge two deadlines, stats=%+v",stats)
 }
 lengthCount:=uint64(0)
 for _,n:=range stats.ControlLengthBins{lengthCount+=n}
 if lengthCount!=1 || stats.ControlWireBytes!=uint64(len(out[0].Payload)) {
  t.Fatalf("one bounded wire size receipt required: %+v",stats)
 }
 for _,seconds:=range []int{2,4,6,8,10,12}{
  if err:=c.Tick(t0.Add(time.Duration(seconds)*time.Second));err!=nil{t.Fatal(err)}
 }
 if len(out)!=1{t.Fatalf("idle/no peer new measurement caused repetitive reports: %d",len(out))}
 stats,_=c.ControlStats(cr.Ref)
 if stats.SuppressedNoNewObservation==0{t.Fatalf("no-op quality decisions uncounted: %+v",stats)}
 if err:=c.Tick(t0.Add(17*time.Second));err!=nil{t.Fatal(err)}
 if len(out)!=2 {t.Fatalf("normal health must eventually carry real idle when quality suppressed, emitted=%d",len(out))}
 if len(out[1].Payload)>budget.RecordWireMTU{t.Fatalf("health padded beyond MTU: %d",len(out[1].Payload))}
 if err:=server.HandleSegment(sr.Ref,out[1],t0.Add(17*time.Second));err!=nil{t.Fatal(err)}
 if !server.PeerIdle(t0.Add(18*time.Second),time.Minute){t.Fatal("timed health did not refresh true business IdleFor")}
 stats,_=c.ControlStats(cr.Ref)
 if stats.QualityEmitted!=1||stats.HealthEmitted!=1 {
  t.Fatalf("idle quality not suppressed or timed health absent: %+v",stats)
 }
 lengthCount=0
 gapCount:=uint64(0)
 for _,n:=range stats.ControlLengthBins{lengthCount+=n}
 for _,n:=range stats.ControlGapBins{gapCount+=n}
 if lengthCount!=2||gapCount!=1 ||
  stats.ControlWireBytes!=uint64(len(out[0].Payload)+len(out[1].Payload)) {
  t.Fatalf("control-only length/gap histogram must be bounded and consistent: %+v",stats)
 }
 // Force idle remains mandatory and bypasses timing jitter.
 c.AdvertiseIdle(t0.Add(18*time.Second))
 if len(out)!=3{t.Fatal("force idle notification was throttled")}
}

func TestN1ControlFailedQualityCannotRefreshHealthAndNoHotRetry(t *testing.T) {
 lease:=runtimeLease(t)
 o,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil{t.Fatal(err)}
 c,err:=New(o,nil);if err!=nil{t.Fatal(err)}
 defer c.Close()
 sent:=0
 fail:=errors.New("synthetic original control segment drop before successful Emit")
 cfg,_:=transportPair(func(seg faketcp.Segment)error{
  if len(seg.Payload)==0{return nil}
  sent++
  if sent==1{return fail}
  return nil
 },func(faketcp.Segment)error{return nil},1,15667)
 cl:=runtimeLane(t,datapath.RoleClient,lease,0,102)
 cr,err:=c.AttachInitial(1,cl,cfg);if err!=nil{t.Fatal(err)}
 if err:=c.ConfigureHealth(cr.Ref,15*time.Second,func(time.Time)time.Duration{return 0});err!=nil{t.Fatal(err)}
 if err:=c.ConfigureQualityV3(cr.Ref,cl.Config().IncarnationNonce);err!=nil{t.Fatal(err)}
 t0:=time.Unix(9002,0)
 if err:=c.Tick(t0);!errors.Is(err,fail){t.Fatalf("initial control Emit failure not surfaced: %v",err)}
 if sent!=1{t.Fatalf("unexpected emit count %d",sent)}
 lane:=c.lanes[cr.Ref]
 lane.mu.Lock()
 nextHealth:=lane.health.nextSend
 retry:=lane.health.retryAfter
 lane.mu.Unlock()
 if !nextHealth.IsZero() || retry.Before(t0.Add(time.Second)) {
  t.Fatalf("failed control incorrectly refreshed health or lacked retry backoff: next=%v retry=%v",nextHealth,retry)
 }
 for _,ms:=range []int{100,300,700,999}{
  if err:=c.Tick(t0.Add(time.Duration(ms)*time.Millisecond));err!=nil{t.Fatal(err)}
 }
 if sent!=1{t.Fatalf("hot retry before one second: %d",sent)}
 if err:=c.Tick(t0.Add(time.Second));err!=nil{t.Fatal(err)}
 if sent!=2{t.Fatalf("control did not retry after independent backoff: %d",sent)}
 stats,_:=c.ControlStats(cr.Ref)
 if stats.EmissionFailures!=1||stats.QualityEmitted!=1||stats.MergedHealth!=1 {
  t.Fatalf("control failure/success counters wrong %+v",stats)
 }
}

func TestN1ControlPaddingExplicitOffAndSafeCap(t *testing.T){
 lease:=runtimeLease(t)
 o,err:=datapath.NewLeasedTunnelOwner(lease,1,8);if err!=nil{t.Fatal(err)}
 c,err:=New(o,nil);if err!=nil{t.Fatal(err)}
 defer c.Close()
 var got []faketcp.Segment
 cfg,_:=transportPair(func(s faketcp.Segment)error{got=append(got,s);return nil},
  func(faketcp.Segment)error{return nil},1,12577)
 cfg.ControlPaddingDisabled=true
 cl:=runtimeLane(t,datapath.RoleClient,lease,0,103)
 cr,err:=c.AttachInitial(1,cl,cfg);if err!=nil{t.Fatal(err)}
 if err:=c.ConfigureHealth(cr.Ref,15*time.Second,nil);err!=nil{t.Fatal(err)}
 if err:=c.ConfigureQualityV3(cr.Ref,cl.Config().IncarnationNonce);err!=nil{t.Fatal(err)}
 if err:=c.Tick(time.Unix(9010,0));err!=nil{t.Fatal(err)}
 if len(got)!=1||len(got[0].Payload)!=datapath.QualityHealthV3Size+tlsrecord.FixedWireOverhead{
  t.Fatalf("padding=false must preserve exact 104B health+fixed-overhead record: %d records first len=%d",len(got),func()int{if len(got)==0{return 0};return len(got[0].Payload)}())
 }
 head,err:=o.ControlHeadroom(cr.Ref,datapath.QualityHealthV3Size)
 if err!=nil{t.Fatal(err)}
 if head<0{t.Fatal("negative safe headroom")}
 report:=datapath.QualityHealthReport{Flags:datapath.QualityFlagInsufficient,ReportSeq:99,IncarnationNonce:cl.Config().IncarnationNonce,Generation:cr.Ref.Generation,AgeMillis:0xffff,RTTMillis:0xffff}
 body,err:=datapath.EncodeQualityHealthV3(report);if err!=nil{t.Fatal(err)}
 rec,err:=o.ControlHealthRecord(cr.Ref,body[:],head+100)
 if err!=nil||rec.PaddingBytes!=head || len(rec.Wire)!=tlsrecord.FixedWireOverhead+len(body)+head{
  t.Fatalf("oversize padding must be clipped to safe actual headroom: rec=%+v err=%v",rec,err)
 }
}
