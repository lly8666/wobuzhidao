package datapath

import (
 "encoding/binary"
 "time"

 "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
 "github.com/lly8666/wobuzhidao/internal/tlsrecord"
)

// ControlHeadroom returns exact single-record encrypted zero-padding room for
// the current ACTIVE sender direction. RecordWireMTU is already constrained by
// actual outer IPv4/TCP headers/options, peer MSS and negotiated record cap.
func (o *TunnelOwner) ControlHeadroom(ref logicaltunnel.LaneRef,bodyLen int) (int,error) {
 if o==nil{return 0,ErrTunnelOwnerClosed}
 o.mu.Lock()
 if o.closed{o.mu.Unlock();return 0,ErrTunnelOwnerClosed}
 b,ok:=o.active[ref.ID]
 if !ok{o.mu.Unlock();return 0,ErrLaneUnavailable}
 if b.ref!=ref{cur:=b.ref;o.mu.Unlock();return 0,staleGeneration(ref,cur)}
 lane:=b.lane
 o.mu.Unlock()
 if bodyLen<0{return 0,tlsrecord.ErrPayloadTooLarge}
 lane.mu.Lock()
 defer lane.mu.Unlock()
 if lane.closed{return 0,ErrLaneClosed}
 room:=lane.txBudget.RecordWireMTU-tlsrecord.FixedWireOverhead-bodyLen
 if room<0{return 0,tlsrecord.ErrPayloadTooLarge}
 return room,nil
}

// ControlHealthRecord is only for the three strictly specified KindHealth
// plaintext variants. The legacy 9B body is unchanged. Padded controls use
// the same TX sealer/PN and original ciphertext remains in FakeTCP repair
// storage; no FEC or LINK encoding and no packet fragmentation.
func (o *TunnelOwner) ControlHealthRecord(ref logicaltunnel.LaneRef,body []byte,padding int) (WireRecord,error) {
 if o==nil{return WireRecord{},ErrTunnelOwnerClosed}
 o.mu.Lock()
 if o.closed{o.mu.Unlock();return WireRecord{},ErrTunnelOwnerClosed}
 b,ok:=o.active[ref.ID]
 if !ok{o.mu.Unlock();return WireRecord{},ErrLaneUnavailable}
 if b.ref!=ref{cur:=b.ref;o.mu.Unlock();return WireRecord{},staleGeneration(ref,cur)}
 lane:=b.lane
 o.mu.Unlock()
 if padding<0{return WireRecord{},tlsrecord.ErrInvalidPadding}
 switch len(body) {
 case 9:
  if body[0]!=1{return WireRecord{},ErrQualityHealthV3}
  if binary.BigEndian.Uint64(body[1:])>uint64(MaxHealthIdle/time.Millisecond){return WireRecord{},ErrQualityHealthV3}
 case QualityHealthV3Size:
  if !lane.qualityHealthV3.Load(){return WireRecord{},ErrConfigMismatch}
  report,err:=DecodeQualityHealthV3(body)
  if err!=nil{return WireRecord{},err}
  if report.IncarnationNonce!=lane.cfg.IncarnationNonce||report.Generation!=ref.Generation{return WireRecord{},ErrConfigMismatch}
 case QualityEchoV4Size:
  if !lane.qualityHealthV3.Load(){return WireRecord{},ErrConfigMismatch}
  echo,err:=DecodeQualityEchoV4(body)
  if err!=nil{return WireRecord{},err}
  if echo.Report.IncarnationNonce!=lane.cfg.IncarnationNonce||echo.Report.Generation!=ref.Generation{return WireRecord{},ErrConfigMismatch}
 default:
  return WireRecord{},ErrQualityHealthV3
 }
 lane.mu.Lock()
 defer lane.mu.Unlock()
 if lane.closed{return WireRecord{},ErrLaneClosed}
 max:=lane.txBudget.RecordWireMTU-tlsrecord.FixedWireOverhead-len(body)
 if max<0{return WireRecord{},tlsrecord.ErrPayloadTooLarge}
 if padding>max{padding=max} // a smaller legal record is preferable to failure
 wire,pn,err:=lane.sealer.SealHealthWithPadding(body,padding)
 if err!=nil{return WireRecord{},err}
 if len(wire)>lane.txBudget.RecordWireMTU{return WireRecord{},ErrRecordOversize}
 return WireRecord{Ref:ref,PN:pn,Wire:wire,Control:true,PaddingBytes:padding},nil
}
