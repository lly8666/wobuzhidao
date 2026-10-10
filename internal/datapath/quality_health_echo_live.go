package datapath

import "github.com/lly8666/wobuzhidao/internal/logicaltunnel"

// QualityHealthEchoRecord seals one 128-byte authenticated feedback envelope
// in place of (never in addition to) this lane's next 2s health slot.
// No LINK/FEC, separate sealer/PN, timer, packet data field, or extra flow.
func (o *TunnelOwner) QualityHealthEchoRecord(ref logicaltunnel.LaneRef, echo QualityEchoV4) (WireRecord,error) {
    if o==nil{return WireRecord{},ErrTunnelOwnerClosed}
    o.mu.Lock()
    if o.closed {o.mu.Unlock();return WireRecord{},ErrTunnelOwnerClosed}
    binding,ok:=o.active[ref.ID]
    if !ok {o.mu.Unlock();return WireRecord{},ErrLaneUnavailable}
    if binding.ref!=ref {cur:=binding.ref;o.mu.Unlock();return WireRecord{},staleGeneration(ref,cur)}
    lane:=binding.lane
    o.mu.Unlock()
    if !lane.qualityHealthV3.Load() || echo.Report.Generation!=ref.Generation ||
        echo.Report.IncarnationNonce!=lane.cfg.IncarnationNonce {
        return WireRecord{},ErrConfigMismatch
    }
    payload,err:=EncodeQualityEchoV4(echo)
    if err!=nil{return WireRecord{},err}
    lane.mu.Lock()
    defer lane.mu.Unlock()
    if lane.closed{return WireRecord{},ErrLaneClosed}
    wire,pn,err:=lane.sealer.SealHealth(payload[:])
    if err!=nil{return WireRecord{},err}
    return WireRecord{Ref:ref,PN:pn,Control:true,Wire:wire},nil
}
