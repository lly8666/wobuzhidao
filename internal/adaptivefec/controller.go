// Package adaptivefec holds a Normal-only N2 decision core. It cannot send,
// change encoder profiles or bind admission: those safety boundaries are wired
// only after authenticated FIRST-ATTEMPT observations are qualified.
package adaptivefec

import (
    "errors"
    "math"
    "time"
)

const (
    DataShards = 20
    MinDecisionRecords = 512
    ResidualTarget = 0.001
    decreaseMargin = 0.0005
)

var ErrConfig = errors.New("adaptivefec: invalid Normal automatic profile")

type Mode uint8

const (
    Auto Mode = iota + 1
    Aggressive
)

type Evidence uint8

const (
    EvidenceUnknown Evidence = iota
    // Post-repair paired PN deficits, including the shipped N1 echo, cannot
    // be interpreted as physical first-transmission loss (even at 0%).
    EvidencePostRepair
    // Reserved for a future authenticated, source-correlated first-attempt
    // witness. No production N1 callback currently supplies this evidence.
    EvidenceFirstAttemptVerified
)

type Config struct {
    Mode Mode
    InitialParity int
    MinParity int
    MaxParity int
}

var profiles = [...]int{0, 4, 8, 10, 12, 16, 20}

func supported(r int) bool {
    for _, v := range profiles { if r == v { return true } }
    return false
}

// Only Normal auto modes are accepted. Game and fixed do not instantiate a
// controller. The V3 design has initial=max=20, independently bounded min.
func (c Config) Validate() error {
    if (c.Mode != Auto && c.Mode != Aggressive) ||
        c.InitialParity != 20 || c.MaxParity != 20 ||
        !supported(c.MinParity) || c.MinParity > c.InitialParity {
        return ErrConfig
    }
    return nil
}

type Input struct {
    // Must be an independently authenticated and source-correlated window
    // from the ACTIVE incarnation. A N1 post-repair echo is NOT enough.
    Evidence Evidence
    Generation uint64
    Sequence uint64
    FirstPN, LastPN uint64
    Sent, FirstAttemptLost uint32
    At time.Time
    RTT time.Duration
    // A sender/receiver FEC block-shape witness is needed to price partial
    // min(k,r) groups; guessing k=20 during sparse traffic is forbidden.
    ShapeVerified bool
    ShardK uint8
    CapacityOrCongestion bool
}

type Reason string

const (
    HoldUnqualified Reason = "HOLD_UNQUALIFIED_FIRST_ATTEMPT"
    HoldStale Reason = "HOLD_STALE_OR_INVALID"
    HoldReplay Reason = "HOLD_OLD_SOURCE_OR_OVERLAP"
    HoldCapacity Reason = "HOLD_CAPACITY_OR_CONGESTION"
    HoldShape Reason = "HOLD_UNVERIFIED_PARTIAL_K"
    HoldSampleCount Reason = "HOLD_INSUFFICIENT_512"
    HoldStable Reason = "HOLD_STABILITY_OR_CONFIDENCE"
    HoldOptimal Reason = "HOLD_CURRENT_PROFILE"
    Raise Reason = "RAISE"
    Lower Reason = "LOWER_ONE"
)

type Decision struct {
    Ordinary int
    Applied int
    Reason Reason
    Fast, Slow float64
    ConfidenceUpper float64
    AcceptedRecords uint64
}

type Controller struct {
    cfg Config
    generation uint64
    ordinary int
    lastSeq, lastPN uint64
    seen bool
    fast, slow float64
    lastAt, firstValid, lastChange, lastLower time.Time
    sinceDecision uint64
    confidentSent, confidentLost uint64
}

func New(c Config, generation uint64) (*Controller, error) {
    if err := c.Validate(); err != nil || generation == 0 { return nil, ErrConfig }
    return &Controller{cfg:c, generation:generation, ordinary:c.InitialParity}, nil
}

func (c *Controller) current(reason Reason) Decision {
    applied:=c.ordinary
    if c.cfg.Mode == Aggressive {
        for _, p := range profiles {
            if p>applied && p<=c.cfg.MaxParity { applied=p; break }
        }
    }
    return Decision{Ordinary:c.ordinary, Applied:applied, Reason:reason,
        Fast:c.fast, Slow:c.slow,
        ConfidenceUpper:wilsonUpper(c.confidentLost,c.confidentSent),
        AcceptedRecords:c.sinceDecision}
}

// ResidualReference is an independent IID Reed-Solomon model, never a real
// performance guarantee. A partial group uses r=min(k, configuredParity).
// At off it returns p. 1 <= k <= 20. Called on controller ticks, not packets.
func ResidualReference(k, configuredParity int, p float64) float64 {
    if k<1 || k>DataShards || !supported(configuredParity) || p<0 || p>1 || math.IsNaN(p) {
        return math.NaN()
    }
    r:=configuredParity
    if r>k { r=k }
    if r==0 || p==0 || p==1 { if p==1 && r>0 { return 1 }; return p }
    n:=k+r-1
    prob:=math.Pow(1-p,float64(n))
    tail:=0.0
    for i:=0;i<=n;i++ {
        if i>=r { tail+=prob }
        if i<n { prob*=float64(n-i)/float64(i+1)*p/(1-p) }
    }
    if tail>1 {tail=1}
    return p*tail
}

func requiredProfile(k int,p float64,min,max int) int {
    for _,v:=range profiles {
        if v<min || v>max {continue}
        if ResidualReference(k,v,p)<=ResidualTarget {return v}
    }
    return max
}
func priorProfile(current,min int) int {
    previous:=current
    for _,p:=range profiles {
        if p>=current { break }
        if p>=min { previous=p }
    }
    return previous
}

// Wilson 95% upper confidence prevents low-sample zeroes from triggering
// automatic downgrades. It is intentionally conservative under burst loss.
func wilsonUpper(lost,sent uint64) float64 {
    if sent==0 {return 1}
    n:=float64(sent)
    p:=float64(lost)/n
    z2:=1.96*1.96
    return (p+z2/(2*n)+1.96*math.Sqrt(p*(1-p)/n+z2/(4*n*n)))/(1+z2/n)
}

func (c *Controller) Step(now time.Time,s Input) Decision {
    if c==nil { return Decision{Reason:HoldUnqualified} }
    if s.Evidence!=EvidenceFirstAttemptVerified {return c.current(HoldUnqualified)}
    stale:=10*time.Second
    if s.RTT>0 && s.RTT*4>stale {stale=s.RTT*4}
    if s.At.IsZero() || now.Before(s.At) || now.Sub(s.At)>stale ||
       s.Sequence==0 || s.Generation==0 || s.Sent==0 ||
       s.FirstAttemptLost>s.Sent || s.FirstPN>s.LastPN ||
       s.LastPN-s.FirstPN>=256 || uint64(s.Sent)>s.LastPN-s.FirstPN+1 ||
       s.RTT<0 {return c.current(HoldStale)}
    if s.Generation!=c.generation ||
       (c.seen && (s.Sequence<=c.lastSeq || s.FirstPN<=c.lastPN || !s.At.After(c.lastAt))) {
        return c.current(HoldReplay)
    }
    // Source-local overload and unsupported shape never update the EWMAs or
    // consume evidence. Avoid inventing a low line-loss estimate.
    if s.CapacityOrCongestion {return c.current(HoldCapacity)}
    if !s.ShapeVerified || s.ShardK<1 || s.ShardK>DataShards {
        return c.current(HoldShape)
    }
    p:=float64(s.FirstAttemptLost)/float64(s.Sent)
    if !c.seen {
        c.fast,c.slow=p,p
        c.firstValid=s.At
    } else {
        dt:=s.At.Sub(c.lastAt).Seconds()
        c.fast+=(p-c.fast)*(-math.Expm1(-dt/2))
        c.slow+=(p-c.slow)*(-math.Expm1(-dt/15))
    }
    c.lastSeq,c.lastPN,c.lastAt=s.Sequence,s.LastPN,s.At
    c.seen=true
    c.sinceDecision+=uint64(s.Sent)
    c.confidentSent+=uint64(s.Sent)
    c.confidentLost+=uint64(s.FirstAttemptLost)
    if c.sinceDecision<MinDecisionRecords {return c.current(HoldSampleCount)}
    k:=int(s.ShardK)
    upTarget:=requiredProfile(k,math.Max(c.fast,p),c.cfg.MinParity,c.cfg.MaxParity)
    if upTarget>c.ordinary {
        c.ordinary=upTarget
        c.lastChange=now
        c.sinceDecision=0
        c.confidentSent,c.confidentLost=0,0
        c.firstValid=now
        return c.current(Raise)
    }
    prior:=priorProfile(c.ordinary,c.cfg.MinParity)
    if prior>=c.ordinary {return c.current(HoldOptimal)}
    stable:=30*time.Second
    if prior==0 {stable=60*time.Second}
    if now.Sub(c.firstValid)<stable ||
       (!c.lastChange.IsZero() && now.Sub(c.lastChange)<10*time.Second) ||
       (!c.lastLower.IsZero() && now.Sub(c.lastLower)<10*time.Second) {
        return c.current(HoldStable)
    }
    // Do not trust a low point estimate: the next lower profile must fit
    // with 2x safety margin using the conservative upper confidence bound.
    if ResidualReference(k,prior,wilsonUpper(c.confidentLost,c.confidentSent))>decreaseMargin ||
       requiredProfile(k,c.slow,c.cfg.MinParity,c.cfg.MaxParity)>prior {
        return c.current(HoldStable)
    }
    c.ordinary=prior
    c.lastLower=now
    c.lastChange=now
    c.sinceDecision=0
    return c.current(Lower)
}
