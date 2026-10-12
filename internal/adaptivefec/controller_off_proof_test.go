package adaptivefec

import (
    "testing"
    "time"
)

// 16 disjoint clean 256-PN source windows do not justify disabling FEC
// under the pre-existing 0.0005 Wilson residual margin. Keep only
// 16 windows for enabled-rung decisions but independently retain the
// authenticated 32-window proof needed for OFF. No QoS/netem oracle.
func TestN2EnabledHistory16OffRequires32CleanQualifiedWindows(t *testing.T) {
    start:=time.Unix(330000,0)
    c:=controller(t,Auto,0)
    for i:=1;i<=16;i++ {
        s:=source(i,start)
        c.Step(s.At,s)
    }
    if c.historyCount!=confidenceHistory || c.offHistoryCount!=16 {
        t.Fatalf("wrong dual window counts recent=%d off=%d",c.historyCount,c.offHistoryCount)
    }
    before:=c.offProofTotals()
    if before.sent!=16*256 || before.upperLost!=0 {
        t.Fatalf("invalid clean off proof counters: %+v",before)
    }
    if wilsonUpper(before.upperLost,before.sent)<=decreaseMargin {
        t.Fatal("insufficient 16-window clean proof unexpectedly disables full FEC")
    }
    original:=c.offHistoryCount
    invalid:=source(17,start)
    invalid.Evidence=EvidencePostRepair
    if d:=c.Step(invalid.At,invalid); d.Reason!=HoldUnqualified {
        t.Fatalf("post repair evidence misused: %+v",d)
    }
    if c.offHistoryCount!=original {
        t.Fatal("unqualified/repaired records must not enter off proof")
    }
    for i:=17;i<=32;i++ {
        s:=source(i,start)
        c.Step(s.At,s)
    }
    enough:=c.offProofTotals()
    if c.offHistoryCount!=offConfidenceHistory || enough.sent!=32*256 ||
       enough.upperLost!=0 || wilsonUpper(enough.upperLost,enough.sent)>=decreaseMargin {
        t.Fatalf("32 real disjoint clean source windows should be eligible to prove off: %+v",enough)
    }
    for i:=33;i<=90;i++ {
        s:=source(i,start)
        c.Step(s.At,s)
    }
    if c.ordinary!=0 {
        t.Fatalf("the preserved 60s evidence rule cannot lower all the way to off: grade=%d",c.ordinary)
    }
}

// OFF proof must age out historical high loss only after 32 *new*
// qualified windows even though enabled-grade advice can forget it
// after 16. The replay and ambiguous source inputs cannot forge clean.
func TestN2OffConfidenceKeepsSeparateOlderVerifiedDamage(t *testing.T) {
    start:=time.Unix(340000,0)
    c:=controller(t,Auto,0)
    for i:=1;i<=32;i++ {
        s:=source(i,start)
        s.FirstAttemptLost=52
        c.Step(s.At,s)
    }
    for i:=33;i<=48;i++ {
        s:=source(i,start)
        c.Step(s.At,s)
    }
    if c.confidentLost!=0 || c.confidentSent!=16*256 {
        t.Fatalf("short enabled ring did not discard old loss: %d/%d",c.confidentLost,c.confidentSent)
    }
    still:=c.offProofTotals()
    if still.upperLost!=16*52 || still.sent!=32*256 {
        t.Fatalf("off guard prematurely forgot authentic damage: %+v",still)
    }
    if wilsonUpper(still.upperLost,still.sent)<=decreaseMargin {
        t.Fatal("mixed loss and clean upper confidence cannot justify off")
    }
    for i:=49;i<=64;i++ {
        s:=source(i,start)
        c.Step(s.At,s)
    }
    clean:=c.offProofTotals()
    if clean.upperLost!=0 || clean.sent!=32*256 {
        t.Fatalf("off proof didn't retire damaged windows with NEW source: %+v",clean)
    }
}
