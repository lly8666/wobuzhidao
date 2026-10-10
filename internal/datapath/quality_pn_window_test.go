package datapath

import (
    "testing"
    "time"
)

func TestN1QualityReceivePNBoundedAuthenticatedDataOnlyAccounting(t *testing.T) {
    t0:=time.Unix(100,0)
    var w qualityRXWindow
    if got:=w.snapshot(t0);got.HasData || got.Unique!=0 || got.AgeMillis!=0xffff {
        t.Fatalf("missing DATA is UNKNOWN, not zero-loss sample: %+v",got)
    }
    w.observe(0,false,t0)
    w.observe(4,false,t0.Add(time.Millisecond))
    w.observe(2,false,t0.Add(2*time.Millisecond)) // first unique late
    w.observe(2,true,t0.Add(2*time.Millisecond))  // duplicate is not unique
    got:=w.snapshot(t0.Add(3*time.Millisecond))
    if !got.HasData || got.FirstPN!=0 || got.LastPN!=4 || got.Unique!=3 ||
        got.Late!=1 || got.Duplicate!=1 || got.AgeMillis!=1 || got.CapacityLimited {
        t.Fatalf("sparse auth LINK PNs must not count holes as loss: %+v",got)
    }
    // A 256-PN bound contains only matching recent records, not all history.
    w.observe(300,false,t0.Add(4*time.Millisecond))
    got=w.snapshot(t0.Add(5*time.Millisecond))
    if got.FirstPN!=45 || got.LastPN!=300 || got.Unique!=1 || got.Late!=0 ||
        got.Duplicate!=0 || got.CapacityLimited {
        t.Fatalf("stale entries counted as recent: %+v",got)
    }
    w.observe(2,false,t0.Add(6*time.Millisecond)) // long late: no overwrite
    if got=w.snapshot(t0.Add(7*time.Millisecond));!got.CapacityLimited || got.Unique!=1 {
        t.Fatalf("late beyond bounded receive window should mark capacity: %+v",got)
    }
}

func TestN1QualityReceivePNNearUint64ExhaustionNoOverflow(t *testing.T) {
    var w qualityRXWindow
    t0:=time.Unix(200,0)
    pn:=^uint64(0)
    w.observe(pn-2,false,t0)
    w.observe(pn,false,t0)
    w.observe(pn-1,false,t0) // late but unique
    got:=w.snapshot(t0)
    if got.FirstPN!=pn-(QualityPNWindowCapacity-1) || got.LastPN!=pn ||
        got.Unique!=3 || got.Late!=1 || got.CapacityLimited {
        t.Fatalf("near-exhausted PN window wrapped: %+v",got)
    }
    if age:=qualityAgeMillis(t0.Add(70*time.Second),t0);age!=0xfffe {
        t.Fatalf("age must saturate, not wrap: %d",age)
    }
    if age:=qualityAgeMillis(t0.Add(-time.Second),t0);age!=0xffff {
        t.Fatalf("future sample must be UNKNOWN: %d",age)
    }
}
