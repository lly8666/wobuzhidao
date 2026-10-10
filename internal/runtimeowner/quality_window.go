package runtimeowner

import (
    "time"
    "github.com/lly8666/wobuzhidao/internal/datapath"
    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// QualityTransmitWindow is actual post-Emit outcome, not Seal() success,
// FakeTCP retransmission, queued records, parity inference or raw WAN loss.
// PN window is local outbound KindLINK only, including systematic/FEC parity.
// A missing PN is NOT lost: it can be a control, failed seal or local drop.
type QualityTransmitWindow struct {
    FirstPN, LastPN uint64
    Success uint64
    LocalDrops uint32
    HasAttempts bool
    AgeMillis uint16
    CapacityLimited bool
}

type qualityTXSlot struct {
    pn uint64
    used bool
    success bool
}

type qualityTXWindow struct {
    slots [datapath.QualityPNWindowCapacity]qualityTXSlot
    high uint64
    haveHigh bool
    lastSuccessAt time.Time
    outside bool
}

// note runs inside laneTransport.mu upon FakeTCP Emit / EmitBatch completion.
// A failed callback is counted as local drop only, never as WAN packet loss.
// Retried bytes at original TCP Seq do not pass this fresh completion hook.
func (w *qualityTXWindow) note(pn uint64, success bool, now time.Time) {
    if !w.haveHigh || pn>w.high {
        w.high=pn
        w.haveHigh=true
    }
    if w.high>=pn && w.high-pn>=datapath.QualityPNWindowCapacity {
        w.outside=true
        return
    }
    slot:=&w.slots[pn%datapath.QualityPNWindowCapacity]
    if slot.used && slot.pn==pn {
        // Defensive against an accidentally doubled completion callback:
        // success is immutable per fresh PN. Never double count.
        return
    }
    *slot=qualityTXSlot{pn:pn,used:true,success:success}
    if success {w.lastSuccessAt=now}
}

func (w *qualityTXWindow) snapshot(now time.Time) QualityTransmitWindow {
    s:=QualityTransmitWindow{AgeMillis:0xffff,CapacityLimited:w.outside}
    if !w.haveHigh {return s}
    s.HasAttempts=true
    s.LastPN=w.high
    if w.high>=datapath.QualityPNWindowCapacity-1 {
        s.FirstPN=w.high-(datapath.QualityPNWindowCapacity-1)
    }
    if !w.lastSuccessAt.IsZero() && !now.Before(w.lastSuccessAt) {
        age:=now.Sub(w.lastSuccessAt)/time.Millisecond
        if age>=0xffff {s.AgeMillis=0xfffe}else{s.AgeMillis=uint16(age)}
    }
    for i:=range w.slots {
        v:=w.slots[i]
        if !v.used || v.pn<s.FirstPN || v.pn>s.LastPN {continue}
        if v.success {s.Success++} else if s.LocalDrops<^uint32(0) {s.LocalDrops++}
    }
    if s.Success==0 {s.AgeMillis=0xffff}
    return s
}

// QualityTransmitWindow is an O(256) snapshot of the most recent bounded
// DATA-PN span. It is read only on a configured V3 ACTIVE lane, not by every
// application packet or a global per-packet lock.
func (r *Runtime) QualityTransmitWindow(ref logicaltunnel.LaneRef, now time.Time) (QualityTransmitWindow,bool) {
    if r==nil {return QualityTransmitWindow{},false}
    r.mu.Lock()
    lane:=r.lanes[ref]
    active:=!r.closed && r.active[ref.ID]==ref
    r.mu.Unlock()
    if !active || lane==nil {return QualityTransmitWindow{},false}
    lane.mu.Lock()
    enabled:=lane.quality.enabled
    out:=lane.quality.tx.snapshot(now)
    lane.mu.Unlock()
    if !enabled {return QualityTransmitWindow{},false}
    return out,true
}

// QualityReceiveWindow is the independent authenticated peer KindLINK-only
// view of the active lane. It must not be paired against this endpoint's own
// outgoing PN space to claim loss; the peer must supply the matching sender
// watermark and matured observation window first.
func (r *Runtime) QualityReceiveWindow(ref logicaltunnel.LaneRef, now time.Time) (datapath.QualityReceiveWindow,bool) {
    if r==nil {return datapath.QualityReceiveWindow{},false}
    r.mu.Lock()
    lane:=r.lanes[ref]
    active:=!r.closed && r.active[ref.ID]==ref
    r.mu.Unlock()
    if !active || lane==nil {return datapath.QualityReceiveWindow{},false}
    lane.mu.Lock()
    enabled:=lane.quality.enabled
    lane.mu.Unlock()
    if !enabled {return datapath.QualityReceiveWindow{},false}
    result,err:=r.owner.QualityReceiveWindow(ref,now)
    return result,err==nil
}
