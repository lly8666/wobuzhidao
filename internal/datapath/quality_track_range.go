package datapath

import (
 "time"
 "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// Four closed 256-PN bitmaps. Quality-only metadata: no packet body,
// retransmission cache, ACK protocol or wait for fresh LINK delivery.
const QualityTrackedReceiveCapacity=4
const qualityTrackedMaxAge=10*time.Second

type qualityTrackedReceive struct {
 used bool
 seq,first,last uint64
 started time.Time
 bits [QualityPNWindowCapacity/64]uint64
 unique uint64
 late,duplicate uint32
 haveAnchor,retained,capacity bool
 age uint16
}

func (w *qualityRXWindow) trackRange(seq,first,last uint64,now time.Time) {
 s:=w.rangeLocked(first,last,now)
 p:=qualityTrackedReceive{used:true,seq:seq,first:first,last:last,started:now,
  unique:s.Unique,late:s.Late,duplicate:s.Duplicate,
  haveAnchor:s.HaveAnchor,retained:s.RangeRetained,capacity:s.CapacityLimited,age:s.AgeMillis}
 if p.retained&&!p.capacity {
  for _,v:=range w.slots {
   if !v.used||v.pn<first||v.pn>last {continue}
   offset:=v.pn-first
   p.bits[offset/64]|=uint64(1)<<(offset%64)
  }
 }
 w.tracked[seq%QualityTrackedReceiveCapacity]=p
}

// At most four scalar range checks per authenticated LINK. Only later
// first arrivals within a closed span modify the bitmap/late counter.
func (w *qualityRXWindow) observeTracked(pn uint64,duplicate bool,now time.Time) {
 for i:=range w.tracked {
  p:=&w.tracked[i]
  if !p.used||!p.retained||p.capacity||pn<p.first||pn>p.last||
   now.Before(p.started)||!now.Before(p.started.Add(qualityTrackedMaxAge)) {continue}
  offset:=pn-p.first
  mask:=uint64(1)<<(offset%64)
  word:=&p.bits[offset/64]
  if duplicate {
   if *word&mask==0 {p.capacity=true
   }else if p.duplicate<^uint32(0) {p.duplicate++}
   continue
  }
  if *word&mask!=0 {continue}
  *word|=mask
  p.haveAnchor=true
  p.unique++
  if p.late<^uint32(0) {p.late++}
 }
}
func (w *qualityRXWindow) trackedRange(seq uint64,now time.Time) QualityReceiveRange {
 p:=w.tracked[seq%QualityTrackedReceiveCapacity]
 if !p.used||p.seq!=seq {return QualityReceiveRange{CapacityLimited:true,AgeMillis:0xffff}}
 return QualityReceiveRange{FirstPN:p.first,LastPN:p.last,Unique:p.unique,
  Late:p.late,Duplicate:p.duplicate,HaveAnchor:p.haveAnchor,
  RangeRetained:p.retained,CapacityLimited:p.capacity,AgeMillis:p.age}
}
func (l *Lane) QualityTrackReceiveRange(seq,first,last uint64,now time.Time) bool {
 if l==nil{return false}
 l.rxMu.Lock()
 defer l.rxMu.Unlock()
 if l.closed{return false}
 l.qualityRX.trackRange(seq,first,last,now)
 return true
}
func (l *Lane) QualityTrackedReceiveRange(seq uint64,now time.Time) QualityReceiveRange {
 if l==nil{return QualityReceiveRange{CapacityLimited:true,AgeMillis:0xffff}}
 l.rxMu.Lock()
 defer l.rxMu.Unlock()
 if l.closed{return QualityReceiveRange{CapacityLimited:true,AgeMillis:0xffff}}
 return l.qualityRX.trackedRange(seq,now)
}
func (o *TunnelOwner) qualityTrackingLane(ref logicaltunnel.LaneRef) (*Lane,error) {
 if o==nil{return nil,ErrTunnelOwnerClosed}
 o.mu.Lock()
 if o.closed{o.mu.Unlock();return nil,ErrTunnelOwnerClosed}
 binding,ok:=o.active[ref.ID]
 if !ok{o.mu.Unlock();return nil,ErrLaneUnavailable}
 if binding.ref!=ref{current:=binding.ref;o.mu.Unlock();return nil,staleGeneration(ref,current)}
 lane:=binding.lane
 o.mu.Unlock()
 return lane,nil
}
func (o *TunnelOwner) QualityTrackReceiveRange(ref logicaltunnel.LaneRef,seq,first,last uint64,now time.Time) error {
 lane,err:=o.qualityTrackingLane(ref)
 if err!=nil{return err}
 if !lane.QualityTrackReceiveRange(seq,first,last,now){return ErrLaneUnavailable}
 return nil
}
func (o *TunnelOwner) QualityTrackedReceiveRange(ref logicaltunnel.LaneRef,seq uint64,now time.Time) (QualityReceiveRange,error) {
 lane,err:=o.qualityTrackingLane(ref)
 if err!=nil{return QualityReceiveRange{},err}
 return lane.QualityTrackedReceiveRange(seq,now),nil
}
