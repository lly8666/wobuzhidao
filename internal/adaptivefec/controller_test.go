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
 // Since the last decision, many disjoint qualified windows already
 // accumulated >=512 records. A genuine sharp loss increase may rise
 // immediately after capacity pressure clears; no second 512-record delay.
 if d:=c.Step(s.At,s);d.Reason!=Raise || d.Ordinary<=4{t.Fatal(d)}
 prior:=c.ordinary
 next:=source(87,start);next.FirstAttemptLost=38
 if d:=c.Step(next.At,next);d.Reason!=HoldSampleCount || d.Ordinary!=prior {
  t.Fatal("repeated high loss must not re-decide before 512 new records",d)
 }
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


// These inputs are synthetic source-PN-correlated fixtures. Runtime must not
// use its cumulative encoder statistics or post-repair echoes as this witness.
func boundedSource(i int, start time.Time, k int, lower, upper uint32) Input {
    s:=source(i,start)
    s.Evidence=EvidenceFirstAttemptBounded
    s.FirstAttemptLost=lower
    s.FirstAttemptLostMax=upper
    s.ShardK=0 // cannot fall back to the single-full-block model
    // Physical sender PNs include BOTH k systematic and r=min(k,20)
    // parity first-emits. A 256-PN window cannot contain 128 k=2/r=2
    // groups (512 PNs); that old synthetic claim was not a real source.
    groups:=256/(2*k)
    s.ClosedGroups[2][k]=uint16(groups)
    s.ClosedR[k]=uint16(groups)
    s.ClosedBySizeKR[2][k][k]=uint16(groups)
    s.ParityRecords=uint32(groups*k)
    s.CroppedRecords=uint32(256-2*groups*k)
    return s
}

func TestBoundedMixedPartialProfilesAndWeightedModel(t *testing.T) {
    var mixed [3][DataShards+1]uint64
    mixed[0][1]=100
    mixed[1][4]=10
    mixed[2][20]=6
    const p=0.05
    want:=(100*ResidualReference(1,4,p)+40*ResidualReference(4,4,p)+120*ResidualReference(20,4,p))/260
    if v:=weightedResidual(mixed,4,p);math.Abs(v-want)>1e-12 {t.Fatalf("mixed %.9g expected %.9g",v,want)}
    if v:=weightedResidual(mixed,20,p);math.IsNaN(v) {t.Fatal("invalid full/partial mix")}
    if got:=requiredMixedProfile(mixed,p,4,20);got<requiredProfile(20,p,4,20) {
        t.Fatalf("sparse partial underestimated: mixed=%d full=%d",got,requiredProfile(20,p,4,20))
    }
}

func TestBoundedOverlapMissingCoverageAndWideIntervalsHold(t *testing.T) {
    start:=time.Unix(50000,0)
    c:=controller(t,Auto,4)
    s:=boundedSource(1,start,2,0,0)
    s.ClosedGroups[0][2]=1 // source/parity group counts now conflict
    if d:=c.Step(s.At,s);d.Reason!=HoldShape {t.Fatal(d)}
    s=boundedSource(1,start,2,0,20)
    if d:=c.Step(s.At,s);d.Reason!=HoldWideBound {t.Fatal(d)}
    s.FirstAttemptLostMax=0
    if d:=c.Step(s.At,s);d.Reason!=HoldSampleCount || d.ShapeRecords!=128 || d.ShapeBlocks!=64 {t.Fatal(d)}
    if d:=c.Step(s.At,s);d.Reason!=HoldReplay || d.AcceptedRecords!=256 {t.Fatal(d)}
    overlapping:=boundedSource(2,start,2,0,0)
    overlapping.FirstPN=s.LastPN
    overlapping.LastPN=s.LastPN+255 // exactly 256 PNs, not a malformed 257 span
    if d:=c.Step(overlapping.At,overlapping);d.Reason!=HoldReplay {t.Fatal(d)}
    if d:=c.Step(boundedSource(2,start,2,0,0).At,boundedSource(2,start,2,0,0));d.AcceptedRecords!=512 {t.Fatal(d)}
    if c.confidentSent!=512 {t.Fatal("overlap counted twice",c.confidentSent)}
}

func TestBoundedNormalAndAggressiveEventuallyMoveAndRaise(t *testing.T) {
    start:=time.Unix(60000,0)
    c:=controller(t,Auto,4)
    a:=controller(t,Aggressive,4)
    changed:=false
    for i:=1;i<=90;i++ {
        s:=boundedSource(i,start,2,0,0)
        n:=c.Step(s.At,s);g:=a.Step(s.At,s)
        if n.Ordinary!=g.Ordinary {t.Fatal("different ordinary advice",n,g)}
        if n.Ordinary<20 {changed=true}
        if n.Ordinary<20 && g.Applied<=n.Ordinary {t.Fatal("aggressive omitted one rung",n,g)}
    }
    if !changed || c.ordinary!=4 || a.ordinary!=4 {t.Fatal("controller stuck in HOLD",c.ordinary,a.ordinary)}
    s:=boundedSource(91,start,2,34,36)
    if d:=c.Step(s.At,s);d.Reason!=Raise || d.Ordinary<=4 {t.Fatal("trusted damage failed to raise",d)}
    if d:=c.Step(s.At,s);d.Reason!=HoldReplay {t.Fatal(d)}
}

func TestBoundedCapacityAgeGenerationAndConfidenceAreFinite(t *testing.T) {
    start:=time.Unix(70000,0)
    c:=controller(t,Auto,0)
    for i:=1;i<=100;i++ {
        s:=boundedSource(i,start,1,0,0)
        if i==2 {
            s.CapacityOrCongestion=true
            if d:=c.Step(s.At,s);d.Reason!=HoldCapacity {t.Fatal(d)}
            s.CapacityOrCongestion=false
        }
        d:=c.Step(s.At,s)
        if i==100 && (d.ConfidenceUpper<=0 || c.confidentSent>32*256) {t.Fatal("lifetime confidence or invalid Wilson",d,c.confidentSent)}
    }
    bad:=boundedSource(101,start,1,0,0)
    bad.Generation=6
    if d:=c.Step(bad.At,bad);d.Reason!=HoldReplay {t.Fatal(d)}
    bad.Generation=5
    if d:=c.Step(bad.At.Add(11*time.Second),bad);d.Reason!=HoldStale {t.Fatal(d)}
}


// A real full 20:20 block costs 40 separate successful LINK PNs. A 256-PN
// feedback cohort cannot always consist entirely of complete 40-PN groups:
// 6 complete blocks cost 240 records; the remaining 16 are partial edges.
// Those 16 are priced as unprotected, never as perfect full 20:20 blocks.
func TestN2CroppedReal20To20CohortsEventuallyLowerAndRaise(t *testing.T) {
	start:=time.Unix(80000,0)
	c:=controller(t,Auto,4)
	d0:=boundedSource(1,start,20,0,0)
	if d0.CroppedRecords!=16 || d0.ParityRecords!=120 || d0.ClosedGroups[2][20]!=6 {
		t.Fatalf("not a physical 256-PN k/r cohort %+v",d0)
	}
	bad:=d0
	bad.ParityRecords--
	if d:=c.Step(bad.At,bad);d.Reason!=HoldShape {t.Fatalf("missing parity admitted %+v",d)}
	bad=d0
	bad.InvalidRecords=1
	if d:=c.Step(bad.At,bad);d.Reason!=HoldShape {t.Fatalf("invalid PN trusted %+v",d)}
	d:=c.Step(d0.At,d0)
	if d.Reason!=HoldSampleCount||d.ShapeRecords!=120||d.ShapeBlocks!=6||
		d.CroppedRecords!=16||d.OffRecords!=0 {t.Fatalf("trimmed group not priced %+v",d)}
	changed:=false
	for i:=2;i<=90;i++ {
		s:=boundedSource(i,start,20,0,0)
		decision:=c.Step(s.At,s)
		if decision.Ordinary<20 {changed=true}
	}
	if !changed||c.ordinary!=4 {t.Fatalf("healthy partial-edge cohort never lowered %d",c.ordinary)}
	raised:=false
	for i:=91;i<=96;i++ {
		s:=boundedSource(i,start,20,45,46)
		d:=c.Step(s.At,s)
		if d.Reason==Raise&&d.Ordinary>4 {raised=true;break}
	}
	if !raised {t.Fatal("confident damaged physical PN cohort could not raise")}
}

func TestN2BoundedMixIncludesActualOffAndPartialEdges(t *testing.T) {
	start:=time.Unix(90000,0)
	c:=controller(t,Aggressive,0)
	s:=boundedSource(1,start,20,0,0)
	s.ClosedGroups=[3][DataShards+1]uint16{}
	s.ClosedR=[DataShards+1]uint16{}
	s.ClosedBySizeKR=[3][DataShards+1][DataShards+1]uint16{}
	s.ClosedGroups[2][20]=3 // 60 systematic
	s.ClosedGroups[0][4]=1 // 4 systematic
	s.ClosedR[20]=3
	s.ClosedR[4]=1
	s.ClosedBySizeKR[2][20][20]=3
	s.ClosedBySizeKR[0][4][4]=1
	s.ParityRecords=64
	s.OffRecords=64
	s.CroppedRecords=64
	if d:=c.Step(s.At,s);d.Reason!=HoldSampleCount||d.ShapeRecords!=64||
		d.ShapeBlocks!=4||d.OffRecords!=64||d.CroppedRecords!=64 {
		t.Fatalf("mixed off/20:R/cropped source rejected %+v",d)
	}
	// Parity and systematic are distinct unique source PNs. Replayed or
	// overlapping returned echoes cannot spend the remaining 256 again.
	if d:=c.Step(s.At,s);d.Reason!=HoldReplay||d.AcceptedRecords!=256 {
		t.Fatalf("same report counted twice %+v",d)
	}
}


func TestN2JointKRRejectsMarginallyMatchingImpossibleGroups(t *testing.T) {
	start:=time.Unix(97000,0)
	c:=controller(t,Auto,4)
	good:=boundedSource(1,start,20,0,0)
	// Separate k and r marginals cannot detect a false joint geometry.
	bad:=good
	bad.ClosedBySizeKR[2][20][20]--
	bad.ClosedBySizeKR[2][20][19]++ // 20:19 is unsupported
	if d:=c.Step(bad.At,bad);d.Reason!=HoldShape {
		t.Fatalf("forged 20:19 geometry admitted %+v",d)
	}
	bad=good
	bad.ClosedBySizeKR[2][20][20]--
	bad.ClosedBySizeKR[2][20][0]++ // a closed FEC group cannot send zero parity
	if d:=c.Step(bad.At,bad);d.Reason!=HoldShape {
		t.Fatalf("zero parity geometry admitted %+v",d)
	}
	if d:=c.Step(good.At,good);d.Reason!=HoldSampleCount {
		t.Fatalf("rejected samples consumed or poisoned cohort %+v",d)
	}
}
