package adaptivefec

import (
    "testing"
    "time"
)

// Product controller tests run in GitHub Actions only. The independent
// math.comb oracle lives in tools/, never imported into the production code.
func TestN2UnattainableDoesNotBlindlyRaiseTo20(t *testing.T) {
    var k1 [3][DataShards+1]uint64
    k1[0][1]=128
    for _,floor:=range []int{0,4} {
        got:=requiredProfileWithEdges(k1,0,0,.05,floor,20)
        // From off, adding its first parity shard improves p to p^2;
        // after any enabled rung, min(k,R)=1 and all grades tie.
        want:=floor
        if floor==0 {want=4}
        if got!=want {
            t.Fatalf("k=1 partial after first enabled rung must saturate: floor%d got%d want%d",floor,got,want)
        }
    }
    var closed [3][DataShards+1]uint64
    closed[2][20]=6 // six real complete 20:20 groups, 120 systematic
    // Eight source shards at a 256-PN report edge are unverified and keep
    // a real 0.02*8/128 uncertainty floor; their parity siblings are not
    // counted as extra business loss.
    if v:=mixedResidualWithEdges(closed,0,8,20,.02);v<=ResidualTarget {
        t.Fatalf("fixture must be unattainable even with maximum parity: %g",v)
    }
    if got:=requiredProfileWithEdges(closed,0,8,.02,4,20);got!=4 {
        t.Fatalf("max20 parity cannot fix cropped uncertainty at 2pct: got %d",got)
    }
    // High loss still needs substantial parity on actual CLOSED groups:
    // large gains are not waived just because the cropped absolute bound
    // remains above the nominal 0.1 percent target.
    if got:=requiredProfileWithEdges(closed,0,8,.20,4,20);got<12 {
        t.Fatalf("high physical bounded loss underprotected: %d",got)
    }
}

func TestN2MarginalReliefNeverPretendsDamageIsZero(t *testing.T) {
    var full [3][DataShards+1]uint64
    full[2][20]=6
    if !negligibleMarginalProtection(full,0,8,.02,16,20) {
        t.Fatal("repair-only bounds should not demand costly indistinguishable rungs")
    }
    if negligibleMarginalProtection(full,0,8,.20,4,20) {
        t.Fatal("20 percent damage needs additional parity, not a full drop")
    }
    if negligibleMarginalProtection(full,0,8,.02,20,16) {
        t.Fatal("cannot call rising profile a downgrade")
    }
}

func TestN2BoundedRepairOnlyCleanAdaptationCanLowerGradually(t *testing.T) {
    start:=time.Unix(260000,0)
    c:=controller(t,Auto,4)
    firstLower:=-1
    for i:=1;i<=90;i++ {
        s:=boundedSource(i,start,20,0,5)
        // M is zero while R makes the first-attempt upper bound 5/256:
        // not exact physical zero and never marked VERIFIED evidence.
        d:=c.Step(s.At,s)
        if firstLower<0 && d.Reason==Lower {firstLower=i}
        if i==15 && d.Ordinary!=20 {
            t.Fatalf("skipped stable 30 seconds: %+v",d)
        }
        if d.Ordinary<4 || d.Ordinary>20 {
            t.Fatalf("outside negotiated grades: %+v",d)
        }
    }
    if firstLower<15 || c.ordinary>8 {
        t.Fatalf("endless max20 despite verified k20 and negligible extra protection: firstLower=%d grade=%d",
            firstLower,c.ordinary)
    }
}
