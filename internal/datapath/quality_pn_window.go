package datapath

import "time"

// QualityPNWindowCapacity bounds N1 diagnostics by record-PN span, not by
// allocations, wall-clock time, FakeTCP sequence or application packet count.
// It is deliberately smaller than the AEAD decoder's 65536-PN replay cache.
const QualityPNWindowCapacity = 256

// QualityReceiveWindow counts only authenticated tlsrecord.KindLINK records,
// including FEC parity records but excluding all KindHealth controls. It is
// receiver-local evidence, NOT a WAN loss rate or an aligned sender window.
type QualityReceiveWindow struct {
    FirstPN, LastPN uint64
    Unique uint64
    Late, Duplicate uint32
    HasData bool
    CapacityLimited bool
    AgeMillis uint16 // 0xffff means UNKNOWN, not zero age
}

type qualityRXSlot struct {
    pn uint64
    used bool
    late bool
    duplicates uint32
}

// qualityRXWindow is owned solely by Lane.rxMu. Each authenticated LINK
// decode performs O(1) work; only a 2s maintenance snapshot scans 256 slots.
// Neither sequence-gaps nor control record PNs count as received data.
type qualityRXWindow struct {
    slots [QualityPNWindowCapacity]qualityRXSlot
    high uint64
    haveHigh bool
    lastAt time.Time
    outside bool
}

func (w *qualityRXWindow) observe(pn uint64, duplicate bool, now time.Time) {
    if !w.haveHigh || pn>w.high {
        w.high=pn
        w.haveHigh=true
    }
    // Guard underflow at PN=0 and overflow near MaxUint64.
    if w.high>=pn && w.high-pn>=QualityPNWindowCapacity {
        w.outside=true
        return
    }
    slot:=&w.slots[pn%QualityPNWindowCapacity]
    if duplicate {
        if slot.used && slot.pn==pn && slot.duplicates<^uint32(0) {
            slot.duplicates++
        } else {
            w.outside=true // replay beyond our 256-PN diagnostic horizon
        }
        return
    }
    if slot.used && slot.pn==pn {
        // Decoder replay cache normally prevents this; never double-count.
        return
    }
    *slot=qualityRXSlot{pn:pn,used:true,late:pn<w.high}
    w.lastAt=now
}

func (w *qualityRXWindow) snapshot(now time.Time) QualityReceiveWindow {
    var s QualityReceiveWindow
    s.AgeMillis=qualityAgeMillis(now,w.lastAt)
    if !w.haveHigh {return s}
    s.HasData=true
    s.LastPN=w.high
    if w.high>=QualityPNWindowCapacity-1 {
        s.FirstPN=w.high-(QualityPNWindowCapacity-1)
    }
    s.CapacityLimited=w.outside
    for i:=range w.slots {
        v:=w.slots[i]
        if !v.used || v.pn<s.FirstPN || v.pn>s.LastPN {continue}
        s.Unique++
        if v.late {s.Late++}
        remaining:=uint64(^uint32(0))-uint64(s.Duplicate)
        if uint64(v.duplicates)>remaining {s.Duplicate=^uint32(0)} else {s.Duplicate+=v.duplicates}
    }
    return s
}

func qualityAgeMillis(now, sample time.Time) uint16 {
    if sample.IsZero() || now.Before(sample) {return 0xffff}
    delta:=now.Sub(sample)/time.Millisecond
    if delta>=0xffff {return 0xfffe}
    return uint16(delta)
}

// QualityReceiveWindow is a read-only bounded per-lane diagnostic snapshot.
// It is intentionally separate from the 104B report's sender-PN window:
// pairing opposite direction PN spaces without a matching watermark would
// falsely advertise physical loss.
func (l *Lane) QualityReceiveWindow(now time.Time) QualityReceiveWindow {
    if l==nil {return QualityReceiveWindow{AgeMillis:0xffff}}
    l.rxMu.Lock()
    defer l.rxMu.Unlock()
    if l.closed {return QualityReceiveWindow{AgeMillis:0xffff}}
    return l.qualityRX.snapshot(now)
}
