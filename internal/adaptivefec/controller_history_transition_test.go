package adaptivefec

import (
    "testing"
    "time"
)

// Qualified historical loss should leave a bounded 16-source-window history
// after sufficiently many fresh authenticated, non-overlapping replacements.
// This never asserts that netem's loss truth is available to production.
// M+R, sample min512, slow EWMA, stability and cooldown still gate changes.
func TestN2HistoryAgeOutKeepsNoForgedZeroOrLossFromRejectedSamples(t *testing.T) {
    start:=time.Unix(310000,0)
    ctl:=controller(t,Auto,4)
    for i:=1;i<=confidenceHistory;i++ {
        sample:=boundedSource(i,start,20,52,52)
        d:=ctl.Step(sample.At,sample)
        if d.ConfidenceHistoryWindows>confidenceHistory {
            t.Fatalf("unbounded qualified history at seq%d: %+v",i,d)
        }
    }
    if ctl.historyCount!=confidenceHistory ||
       ctl.confidentSent!=uint64(confidenceHistory*256) ||
       ctl.confidentLost!=uint64(confidenceHistory*52) {
        t.Fatalf("failed to collect true bounded high-loss windows: count%d sent%d upper%d",
            ctl.historyCount,ctl.confidentSent,ctl.confidentLost)
    }
    // Replace all damaged windows with different real source-correlated
    // M/R windows at about 5% lower bounds (not a fabricated zero).
    for i:=confidenceHistory+1;i<=2*confidenceHistory;i++ {
        s:=boundedSource(i,start,20,13,13)
        _=ctl.Step(s.At,s)
    }
    if ctl.historyCount!=confidenceHistory ||
       ctl.confidentSent!=uint64(confidenceHistory*256) ||
       ctl.confidentLost!=uint64(confidenceHistory*13) {
        t.Fatalf("historical 20pct stayed after sufficient NEW qualified 5pct source: count%d sent%d upper%d",
            ctl.historyCount,ctl.confidentSent,ctl.confidentLost)
    }
    last:=boundedSource(2*confidenceHistory,start,20,13,13)
    before:=ctl.confidentLost
    if d:=ctl.Step(last.At.Add(time.Second),last);d.Reason!=HoldReplay ||
       ctl.confidentLost!=before {
        t.Fatalf("duplicate source rewrote actual first attempt confidence: %+v upper%d",
            d,ctl.confidentLost)
    }
    capacity:=boundedSource(2*confidenceHistory+1,start,20,0,0)
    capacity.CapacityOrCongestion=true
    if d:=ctl.Step(capacity.At,capacity);d.Reason!=HoldCapacity ||
       ctl.confidentLost!=before {
        t.Fatalf("capacity record forged clean source: %+v",d)
    }
    for i:=2*confidenceHistory+1;i<=3*confidenceHistory;i++ {
        s:=boundedSource(i,start,20,0,0)
        _=ctl.Step(s.At,s)
    }
    if ctl.confidentLost!=0 ||
       ctl.confidentSent!=uint64(confidenceHistory*256) {
        t.Fatalf("fresh zero-loss evidence did not retire older damage: got lost%d sent%d",
            ctl.confidentLost,ctl.confidentSent)
    }
}
