package runtimeowner

import (
 crand "crypto/rand"
 "encoding/binary"
 "math/big"
 "time"

 "github.com/lly8666/wobuzhidao/internal/datapath"
 "github.com/lly8666/wobuzhidao/internal/logicaltunnel"
)

// ControlStats is bounded per ACTIVE incarnation, collected only when an
// actual control decision is due. No per-business packet log or goroutine.
type ControlStats struct {
 QualityNewWindow uint64 `json:"quality_new_window"`
 QualityEchoChange uint64 `json:"quality_echo_change"`
 QualityStatusChange uint64 `json:"quality_status_change"`
 QualityRefreshDeadline uint64 `json:"quality_refresh_deadline"`
 QualityBootstrap uint64 `json:"quality_bootstrap"`
 SuppressedNoNewObservation uint64 `json:"suppressed_no_new_observation"`
 MergedHealth uint64 `json:"merged_health"`
 HealthEmitted uint64 `json:"health_emitted"`
 QualityEmitted uint64 `json:"quality_emitted"`
 EmissionFailures uint64 `json:"emission_failures"`
 PaddingBytes uint64 `json:"padding_bytes"`
 PaddingBudgetSkips uint64 `json:"padding_budget_skips"`
 FeedbackAgeMillis uint64 `json:"feedback_age_millis"` // read-only last computed qualified age
 // Bounded per-direction encrypted TLS-like record length and successful
 // control-to-control interval histograms. No raw packet body or pcap.
 ControlWireBytes uint64 `json:"control_wire_bytes"`
 ControlLengthBins [8]uint64 `json:"control_length_bins"`
 ControlGapBins [8]uint64 `json:"control_gap_bins"`
 lastControlAt time.Time
}

func controlLengthBucket(bytes int) int {
 for i,limit:=range [...]int{64,128,256,512,768,1024,1280} {
  if bytes<=limit{return i}
 }
 return 7
}

func controlGapBucket(delay time.Duration) int {
 for i,limit:=range [...]time.Duration{
  500*time.Millisecond,time.Second,1500*time.Millisecond,2*time.Second,
  2500*time.Millisecond,3500*time.Millisecond,5*time.Second} {
  if delay<=limit{return i}
 }
 return 7
}

func controlRandomInt(n int64) int64 {
 if n<=1{return 0}
 v,err:=crand.Int(crand.Reader,big.NewInt(n))
 if err!=nil{return 0} // no extra control traffic on RNG failure
 return v.Int64()
}

func qualityNextDelay() time.Duration {
 return 1500*time.Millisecond+time.Duration(controlRandomInt(501))*time.Millisecond
}

func healthNextDelay(interval time.Duration) time.Duration {
 if interval<=0{return interval}
 delta:=interval/10
 if delta==0{return interval}
 return interval-delta+time.Duration(controlRandomInt(int64(2*delta+1)))
}

// Select only once per control record. The effective headroom is the exact
// pathmtu-derived negotiated *outer* carrier record limit (incl real
// configured IPv4 and TCP options, MSS, local PMTU). No inner TUN MTU used.
func (t *laneTransport) controlPadding(ref logicaltunnel.LaneRef,body []byte) (int,error) {
 if t.cfg.ControlPaddingDisabled {return 0,nil}
 headroom,err:=t.owner.ControlHeadroom(ref,len(body))
 if err!=nil{return 0,err}
 max:=headroom
 if t.cfg.ControlPaddingMaxBytes>0 && max>t.cfg.ControlPaddingMaxBytes {
  max=t.cfg.ControlPaddingMaxBytes
 }
 if max<=0{return 0,nil}
 return int(controlRandomInt(int64(max)+1)),nil
}

func (t *laneTransport) sealControl(ref logicaltunnel.LaneRef,body []byte) (datapath.WireRecord,error) {
 padding,err:=t.controlPadding(ref,body)
 if err!=nil{return datapath.WireRecord{},err}
 record,err:=t.owner.ControlHealthRecord(ref,body,padding)
 if err!=nil{return datapath.WireRecord{},err}
 // A legitimate random choice of zero padding is not a skip.
 if !t.cfg.ControlPaddingDisabled {
  if headroom,e:=t.owner.ControlHeadroom(ref,len(body));e==nil && headroom==0 {
   t.mu.Lock();t.control.PaddingBudgetSkips++;t.mu.Unlock()
  }
 }
 return record,nil
}

func (t *laneTransport) noteControlSuccess(now time.Time, record datapath.WireRecord, quality bool) {
 t.mu.Lock()
 interval:=t.health.interval
 t.mu.Unlock()
 delay:=healthNextDelay(interval) // RNG only in control path, outside TX lock
 t.mu.Lock()
 defer t.mu.Unlock()
 t.control.PaddingBytes+=uint64(record.PaddingBytes)
 t.control.ControlWireBytes+=uint64(len(record.Wire))
 t.control.ControlLengthBins[controlLengthBucket(len(record.Wire))]++
 if !t.control.lastControlAt.IsZero() && !now.Before(t.control.lastControlAt) {
  t.control.ControlGapBins[controlGapBucket(now.Sub(t.control.lastControlAt))]++
 }
 t.control.lastControlAt=now
 if quality {
  t.control.QualityEmitted++
  if t.health.interval>0 {
   t.control.MergedHealth++
  }
 }else {
  t.control.HealthEmitted++
 }
 if t.health.interval>0 {
  t.health.nextSend=now.Add(delay)
 }
}

func (t *laneTransport) noteControlFailure(now time.Time,quality bool) {
 t.mu.Lock()
 t.control.EmissionFailures++
 if quality {
  t.quality.nextSend=now.Add(time.Second)
  t.health.retryAfter=now.Add(time.Second)
 }else {
  t.health.retryAfter=now.Add(time.Second)
 }
 t.mu.Unlock()
}

func (t *laneTransport) tickControl(now time.Time) error {
 t.mu.Lock()
 enabled:=t.quality.enabled
 t.mu.Unlock()
 if !enabled{return t.tickHealth(now)}
 attempted,err:=t.tickQuality(now)
 if attempted{return err}
 return t.tickHealth(now)
}

func (r *Runtime) ControlStats(ref logicaltunnel.LaneRef) (ControlStats,bool) {
 if r==nil{return ControlStats{},false}
 r.mu.Lock()
 lane:=r.lanes[ref]
 active:=!r.closed && r.active[ref.ID]==ref
 r.mu.Unlock()
 if !active||lane==nil{return ControlStats{},false}
 lane.mu.Lock()
 v:=lane.control
 lane.mu.Unlock()
 return v,true
}

func legacyIdleBody(d time.Duration) [9]byte {
 if d<0{d=0}
 if d>datapath.MaxHealthIdle{d=datapath.MaxHealthIdle}
 var p [9]byte
 p[0]=1
 binary.BigEndian.PutUint64(p[1:],uint64(d/time.Millisecond))
 return p
}
