package datapath

import (
    "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// EnableQualityHealthV3 is opt-in per authenticated V3 lane incarnation.
// A legacy V2/V1 lane is never enabled by default, so it still rejects every
// non-9-byte health body. The nonce returned is the admission-bound one; the
// caller must not substitute its own random or an installation ID.
func (o *TunnelOwner) EnableQualityHealthV3(ref logicaltunnel.LaneRef, expectedNonce [16]byte) error {
    if o == nil { return ErrTunnelOwnerClosed }
    o.mu.Lock()
    if o.closed { o.mu.Unlock();return ErrTunnelOwnerClosed }
    binding,ok:=o.active[ref.ID]
    if !ok {o.mu.Unlock();return ErrLaneUnavailable}
    if binding.ref!=ref {current:=binding.ref;o.mu.Unlock();return staleGeneration(ref,current)}
    lane:=binding.lane
    if expectedNonce==([16]byte{}) || lane.cfg.IncarnationNonce!=expectedNonce {
        o.mu.Unlock()
        return ErrConfigMismatch
    }
    lane.qualityHealthV3.Store(true)
    o.mu.Unlock()
    return nil
}

// QualityHealthRecord seals versioned quality control on the same owned
// direction-specific record PN / key schedule. No LINK/FEC, no startup
// padding, no business flow or additional repair queue. Only an active
// incarnation explicitly enabled after protected V3 admission may produce it.
func (o *TunnelOwner) QualityHealthRecord(ref logicaltunnel.LaneRef, report QualityHealthReport) (WireRecord,error) {
    if o == nil {return WireRecord{},ErrTunnelOwnerClosed}
    o.mu.Lock()
    if o.closed {o.mu.Unlock();return WireRecord{},ErrTunnelOwnerClosed}
    binding,ok:=o.active[ref.ID]
    if !ok {o.mu.Unlock();return WireRecord{},ErrLaneUnavailable}
    if binding.ref!=ref {current:=binding.ref;o.mu.Unlock();return WireRecord{},staleGeneration(ref,current)}
    lane:=binding.lane
    o.mu.Unlock()
    if !lane.qualityHealthV3.Load() || report.Generation!=ref.Generation ||
        report.IncarnationNonce!=lane.cfg.IncarnationNonce {
        return WireRecord{},ErrConfigMismatch
    }
    payload,err:=EncodeQualityHealthV3(report)
    if err!=nil {return WireRecord{},err}
    lane.mu.Lock()
    defer lane.mu.Unlock()
    if lane.closed {return WireRecord{},ErrLaneClosed}
    wire,pn,err:=lane.sealer.SealHealth(payload[:])
    if err!=nil {return WireRecord{},err}
    return WireRecord{Ref:ref,PN:pn,Control:true,Wire:wire},nil
}
