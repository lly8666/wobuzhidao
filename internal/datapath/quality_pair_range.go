package datapath

import (
    "time"

    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// QualityReceiveRange is receiver-local proof for a *peer* sender PN span.
// Only authenticated KindLINK PNs are counted; KindHealth PNs never enter it.
// A marker PN is checked by the runtime pair evaluator, not inferred here.
type QualityReceiveRange struct {
    FirstPN, LastPN uint64
    Unique uint64
    Late, Duplicate uint32
    HaveAnchor bool
    RangeRetained bool
    CapacityLimited bool
    AgeMillis uint16
}

// qualityRangeLocked is O(256) and is called only by an explicit
// feedback query or maintenance tick; no new hot-path loop or allocation.
func (w *qualityRXWindow) rangeLocked(first, last uint64, now time.Time) QualityReceiveRange {
    result:=QualityReceiveRange{FirstPN:first,LastPN:last,AgeMillis:qualityAgeMillis(now,w.lastAt)}
    if first>last || last-first>=QualityPNWindowCapacity {
        result.CapacityLimited=true
        return result
    }
    if !w.haveHigh || w.high<first {return result}
    if w.high-first>=QualityPNWindowCapacity {
        result.CapacityLimited=true
        return result
    }
    result.RangeRetained=true
    result.CapacityLimited=w.outside
    for i:=range w.slots {
        slot:=w.slots[i]
        if !slot.used || slot.pn<first || slot.pn>last {continue}
        result.HaveAnchor=true
        result.Unique++
        if slot.late && result.Late<^uint32(0) {result.Late++}
        room:=uint64(^uint32(0))-uint64(result.Duplicate)
        if uint64(slot.duplicates)>room {result.Duplicate=^uint32(0)
        } else {result.Duplicate+=slot.duplicates}
    }
    return result
}

func (l *Lane) QualityReceiveRange(first,last uint64,now time.Time) QualityReceiveRange {
    if l==nil{return QualityReceiveRange{FirstPN:first,LastPN:last,AgeMillis:0xffff}}
    l.rxMu.Lock()
    defer l.rxMu.Unlock()
    if l.closed{return QualityReceiveRange{FirstPN:first,LastPN:last,AgeMillis:0xffff}}
    return l.qualityRX.rangeLocked(first,last,now)
}

// QualityReceiveRange fences the current authoritative lane before reading
// its own authenticated receiver PN space. It cannot cross an incarnation.
func (o *TunnelOwner) QualityReceiveRange(ref logicaltunnel.LaneRef, first,last uint64, now time.Time) (QualityReceiveRange,error) {
    if o==nil {return QualityReceiveRange{},ErrTunnelOwnerClosed}
    o.mu.Lock()
    if o.closed {o.mu.Unlock();return QualityReceiveRange{},ErrTunnelOwnerClosed}
    binding,ok:=o.active[ref.ID]
    if !ok {o.mu.Unlock();return QualityReceiveRange{},ErrLaneUnavailable}
    if binding.ref!=ref {current:=binding.ref;o.mu.Unlock();return QualityReceiveRange{},staleGeneration(ref,current)}
    lane:=binding.lane
    o.mu.Unlock()
    return lane.QualityReceiveRange(first,last,now),nil
}
