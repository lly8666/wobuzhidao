package adaptivefec

import (
 "math"
 "testing"
 "time"
)

func TestConfigAndIIDPartial(t *testing.T) {
 for _,c:=range []Config{
  {Mode:0,InitialParity:20,MaxParity:20,MinParity:4},
  {Mode:Auto,InitialParity:4,MaxParity:20,MinParity:4},
  {Mode:Auto,InitialParity:20,MaxParity:16,MinParity:4},
  {Mode:Auto,InitialParity:20,MaxParity:20,MinParity:6},
 } {
  if _,err:=New(c,1);err==nil{t.Fatalf("accepted %+v",c)}
 }
 if _,err:=New(Config{Mode:Auto,InitialParity:20,MaxParity:20,MinParity:4},0);err==nil{t.Fatal("accepted zero generation")}
 if v:=ResidualReference(20,0,.05);math.Abs(v-.05)>1e-12{t.Fatalf("off %.9g",v)}
 if v:=ResidualReference(20,4,.05);math.Abs(v-.0012907)>.00001{t.Fatalf("20:4 %.9g",v)}
 if v:=ResidualReference(20,8,.05);v>.00001{t.Fatalf("20:8 %.9g",v)}
 if v:=ResidualReference(1,20,.05);math.Abs(v-.0025)>1e-12{t.Fatalf("partial min(k,r) %.9g",v)}
 if v:=ResidualReference(20,20,1);v!=1{t.Fatal(v)}
 if v:=ResidualReference(0,20,.05);!math.IsNaN(v){t.Fatal(v)}
}

func source(i int, start time.Time) Input {
 return Input{
  Evidence:EvidenceFirstAttemptVerified,Generation:5,Sequence:uint64(i),
  FirstPN:uint64(i-1)*256+1,LastPN:uint64(i)*256,Sent:256,
  At:start.Add(time.Duration(i)*2*time.Second),ShapeVerified:true,ShardK:20,
 }
}
func controller(t *testing.T,mode Mode,min int) *Controller {
 t.Helper()
 c,err:=New(Config{Mode:mode,InitialParity:20,MinParity:min,MaxParity:20},5)
 if err!=nil{t.Fatal(err)}
 return c
}

func TestPostRepairAndUnverifiedHoldEvenAtZero(t *testing.T) {
 start:=time.Unix(10000,0)
 c:=controller(t,Auto,4)
 for i:=1;i<=90;i++{
  s:=source(i,start);s.Evidence=EvidencePostRepair
  d:=c.Step(s.At,s)
  if d.Ordinary!=20 || d.Reason!=HoldUnqualified{t.Fatalf("unsafe down with post-repair 0%% at %d: %+v",i,d)}
 }
 for i:=1;i<=90;i++{
  s:=source(i,start);s.ShapeVerified=false
  d:=c.Step(s.At,s)
  if d.Ordinary!=20 || d.Reason!=HoldShape{t.Fatalf("unknown k down at %d: %+v",i,d)}
 }
 if c.seen{t.Fatal("unqualified windows were consumed")}
}

func TestBoundedSlowDownAndAggressiveNotCumulative(t *testing.T) {
 start:=time.Unix(10000,0)
 normal:=controller(t,Auto,4)
 aggressive:=controller(t,Aggressive,4)
 lowers:=0
 for i:=1;i<=100;i++{
  s:=source(i,start)
  a:=normal.Step(s.At,s)
  b:=aggressive.Step(s.At,s)
  if a.Ordinary!=b.Ordinary {t.Fatalf("different ordinary recommendation at %d: %+v %+v",i,a,b)}
  if b.Applied!=20 && b.Applied<=b.Ordinary {t.Fatalf("aggressive did not add exactly next profile %+v",b)}
  if a.Reason==Lower {
   lowers++
   if i<15 {t.Fatalf("fell too early at %d",i)}
   if a.Ordinary!=16 && a.Ordinary!=12 && a.Ordinary!=10 && a.Ordinary!=8 && a.Ordinary!=4 {
    t.Fatalf("invalid lowering step %+v",a)
   }
  }
 }
 if lowers!=5 || normal.ordinary!=4 || aggressive.ordinary!=4 {t.Fatalf("lower count=%d ordinary=%d aggressive=%d",lowers,normal.ordinary,aggressive.ordinary)}
 if aggressive.current(HoldOptimal).Applied!=8 {t.Fatalf("aggressive repeated increments %+v",aggressive.current(HoldOptimal))}
}

func TestInvalidStaleReplayGenerationAndPressureDoNotChange(t *testing.T){
 start:=time.Unix(20000,0)
 c:=controller(t,Auto,4)
 first:=source(1,start)
 if d:=c.Step(first.At,first);d.Reason!=HoldSampleCount{t.Fatal(d)}
 for _,mutate:=range []func(*Input){
  func(s *Input){s.Generation=6},
  func(s *Input){s.Sequence=1},
  func(s *Input){s.FirstPN=1},
  func(s *Input){s.FirstAttemptLost=257},
  func(s *Input){s.At=s.At.Add(-time.Minute)},
  func(s *Input){s.CapacityOrCongestion=true},
 }{
  s:=source(2,start);mutate(&s)
  d:=c.Step(source(2,start).At,s)
  if d.Reason!=HoldReplay && d.Reason!=HoldStale && d.Reason!=HoldCapacity{t.Fatalf("unsafe accepted invalid: %+v",d)}
 }
 s:=source(2,start)
 if d:=c.Step(s.At,s);d.Reason!=HoldStable{t.Fatalf("did not resume valid stream %+v",d)}
 if c.lastSeq!=2 || c.sinceDecision!=512{t.Fatal(c.lastSeq,c.sinceDecision)}
}

func TestHighPhysicalLossEscalatesButOverloadFreezes(t *testing.T) {
 start:=time.Unix(30000,0)
 c:=controller(t,Auto,4)
 for i:=1;i<=85;i++{s:=source(i,start);c.Step(s.At,s)}
 if c.ordinary!=4{t.Fatalf("startup to conservative small parity failed: %d",c.ordinary)}
 s:=source(86,start);s.FirstAttemptLost=38;s.CapacityOrCongestion=true
 if d:=c.Step(s.At,s);d.Reason!=HoldCapacity || d.Ordinary!=4{t.Fatal(d)}
 s.CapacityOrCongestion=false
 if d:=c.Step(s.At,s);d.Ordinary!=4{t.Fatal("must accumulate 512 samples",d)}
 next:=source(87,start);next.FirstAttemptLost=38
 if d:=c.Step(next.At,next);d.Reason!=Raise || d.Ordinary<=4{t.Fatal(d)}
}
func TestOffNeedsLongEvidence(t *testing.T){
 start:=time.Unix(40000,0)
 c:=controller(t,Auto,0)
 firstOff:=-1
 for i:=1;i<=100;i++{
  s:=source(i,start);d:=c.Step(s.At,s)
  if d.Ordinary==0{firstOff=i;break}
 }
 if firstOff<30 {t.Fatalf("off entered without 60sec evidence: %d",firstOff)}
 if firstOff<0 {t.Fatal("never reached opt-in off on proven zero loss")}
}
