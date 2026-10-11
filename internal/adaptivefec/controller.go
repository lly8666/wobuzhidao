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
    // A conservative, source-correlated first-attempt interval must be narrow
    // before it contributes control samples. Five percentage points is a
    // confidence/identifiability gate, NOT a target line loss or a tuning knob.
    MaxQualifiedBoundWidth = 0.05
    confidenceHistory = 32 // bounded independent source windows, not lifetime counts
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
    // An authenticated, source-PN-correlated conservative interval. The caller
    // must separately attest CLOSED FEC-block shapes for exactly the same
    // first-transmission source cohort; current runtime does NOT yet do so.
    EvidenceFirstAttemptBounded
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
    Sent, FirstAttemptLost uint32 // lower bound for bounded evidence; exact for verified
    FirstAttemptLostMax uint32 // upper bound for bounded evidence
    // Counts of CLOSED, source-correlated groups, by size class (<=256,
    // <=512, >512) and actual systematic k=1..20. Each group contributes
    // k source records, not r parity records. Source/parity LINK PN windows
    // cannot be silently equated to these counts.
    ClosedGroups [3][DataShards+1]uint16
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
    HoldWideBound Reason = "HOLD_WIDE_FIRST_ATTEMPT_INTERVAL"
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
    BoundWidth float64
    ShapeRecords uint64
    ShapeBlocks uint64
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
    history [confidenceHistory]controlSample
    historyPos, historyCount int
    shapeTotals [3][DataShards+1]uint64
    mixedEvidence bool
    boundWidth float64
}

type controlSample struct {
    sent, upperLost uint32
    groups [3][DataShards+1]uint16
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
        AcceptedRecords:c.sinceDecision,
        BoundWidth:c.boundWidth,
        ShapeRecords:c.shapeRecords(), ShapeBlocks:c.shapeBlocks()}
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

// weightedResidual prices a source uniformly among actual CLOSED groups,
// so a k=1 partial is NOT silently valued like a full 20:20 group. Counts
// are bounded by the caller's authenticated, single-source 256-PN coverage.
func weightedResidual(groups [3][DataShards+1]uint64, parity int, p float64) float64 {
    var total uint64
    sum := 0.0
    for _, class := range groups {
        for k:=1; k<=DataShards; k++ {
            n:=class[k]
            if n==0 {continue}
            total+=uint64(k)*n
            sum+=float64(k)*float64(n)*ResidualReference(k,parity,p)
        }
    }
    if total==0 {return math.NaN()}
    return sum/float64(total)
}

func requiredMixedProfile(groups [3][DataShards+1]uint64,p float64,min,max int) int {
    for _,v:=range profiles {
        if v<min||v>max {continue}
        if weightedResidual(groups,v,p)<=ResidualTarget {return v}
    }
    return max
}

func (c *Controller) shapeRecords() uint64 {
    if !c.mixedEvidence {return 0}
    var n uint64
    for _, class:=range c.shapeTotals {
        for k:=1;k<=DataShards;k++ {n+=uint64(k)*class[k]}
    }
    return n
}
func (c *Controller) shapeBlocks() uint64 {
    if !c.mixedEvidence {return 0}
    var n uint64
    for _, class:=range c.shapeTotals {
        for k:=1;k<=DataShards;k++ {n+=class[k]}
    }
    return n
}

// Only accepted nonoverlapping source windows contribute. The last 32
// accepted intervals bound confidence and shape weighting; otherwise a
// lifetime of clean windows would mask a new damaged period indefinitely.
func (c *Controller) pushSample(s Input,upper uint32,bounded bool) {
    if c.historyCount==len(c.history) {
        old:=c.history[c.historyPos]
        c.confidentSent-=uint64(old.sent)
        c.confidentLost-=uint64(old.upperLost)
        for class:=range c.shapeTotals {
            for k:=1;k<=DataShards;k++ {
                c.shapeTotals[class][k]-=uint64(old.groups[class][k])
            }
        }
    } else {c.historyCount++}
    item:=controlSample{sent:s.Sent,upperLost:upper}
    if bounded {item.groups=s.ClosedGroups}
    c.history[c.historyPos]=item
    c.historyPos=(c.historyPos+1)%len(c.history)
    c.confidentSent+=uint64(s.Sent)
    c.confidentLost+=uint64(upper)
    if bounded {
        for class:=range c.shapeTotals {
            for k:=1;k<=DataShards;k++ {
                c.shapeTotals[class][k]+=uint64(s.ClosedGroups[class][k])
            }
        }
    }
}

func (c *Controller) Step(now time.Time,s Input) Decision {
    if c==nil {return Decision{Reason:HoldUnqualified}}
    bounded:=s.Evidence==EvidenceFirstAttemptBounded
    if !bounded && s.Evidence!=EvidenceFirstAttemptVerified {
        return c.current(HoldUnqualified)
    }
    stale:=10*time.Second
    if s.RTT>0 && s.RTT*4>stale {stale=s.RTT*4}
    if s.At.IsZero() || now.Before(s.At) || now.Sub(s.At)>stale ||
       s.Sequence==0 || s.Generation==0 || s.Sent==0 ||
       s.FirstAttemptLost>s.Sent || s.FirstPN>s.LastPN ||
       s.LastPN-s.FirstPN>=256 || uint64(s.Sent)>s.LastPN-s.FirstPN+1 ||
       s.RTT<0 {return c.current(HoldStale)}
    if s.Generation!=c.generation ||
       (c.seen && (s.Sequence<=c.lastSeq || s.FirstPN<=c.lastPN ||
                   !s.At.After(c.lastAt))) {
        return c.current(HoldReplay)
    }
    if s.CapacityOrCongestion {return c.current(HoldCapacity)}
    if !s.ShapeVerified {return c.current(HoldShape)}
    if c.seen && c.mixedEvidence!=bounded {return c.current(HoldUnqualified)}
    upper:=s.FirstAttemptLost
    if bounded {
        upper=s.FirstAttemptLostMax
        if upper<s.FirstAttemptLost || upper>s.Sent {
            return c.current(HoldStale)
        }
        var shapeSourceRecords uint64
        for _, class:=range s.ClosedGroups {
            for k:=1;k<=DataShards;k++ {
                shapeSourceRecords+=uint64(k)*uint64(class[k])
            }
        }
        // Never approximate source-group coverage from just a block-count
        // histogram, FEC parity PNs, or the most recent partial group.
        if shapeSourceRecords!=uint64(s.Sent) {return c.current(HoldShape)}
        if float64(upper-s.FirstAttemptLost)/float64(s.Sent)>MaxQualifiedBoundWidth {
            return c.current(HoldWideBound)
        }
    } else if s.ShardK<1 || s.ShardK>DataShards {
        return c.current(HoldShape)
    }
    p:=float64(upper)/float64(s.Sent)
    if !c.seen {
        c.fast,c.slow=p,p
        c.firstValid=s.At
        c.mixedEvidence=bounded
    } else {
        dt:=s.At.Sub(c.lastAt).Seconds()
        c.fast+=(p-c.fast)*(-math.Expm1(-dt/2))
        c.slow+=(p-c.slow)*(-math.Expm1(-dt/15))
    }
    c.boundWidth=float64(upper-s.FirstAttemptLost)/float64(s.Sent)
    c.lastSeq,c.lastPN,c.lastAt=s.Sequence,s.LastPN,s.At
    c.seen=true
    if c.sinceDecision<1<<20 {c.sinceDecision+=uint64(s.Sent)}
    c.pushSample(s,upper,bounded)
    if c.sinceDecision<MinDecisionRecords {return c.current(HoldSampleCount)}
    upTarget:=0
    if bounded {
        upTarget=requiredMixedProfile(c.shapeTotals,math.Max(c.fast,p),c.cfg.MinParity,c.cfg.MaxParity)
    } else {
        upTarget=requiredProfile(int(s.ShardK),math.Max(c.fast,p),c.cfg.MinParity,c.cfg.MaxParity)
    }
    if upTarget>c.ordinary {
        c.ordinary=upTarget
        c.lastChange=now
        c.sinceDecision=0
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
    confidence:=wilsonUpper(c.confidentLost,c.confidentSent)
    if bounded {
        if weightedResidual(c.shapeTotals,prior,confidence)>decreaseMargin ||
           requiredMixedProfile(c.shapeTotals,c.slow,c.cfg.MinParity,c.cfg.MaxParity)>prior {
            return c.current(HoldStable)
        }
    } else if ResidualReference(int(s.ShardK),prior,confidence)>decreaseMargin ||
              requiredProfile(int(s.ShardK),c.slow,c.cfg.MinParity,c.cfg.MaxParity)>prior {
        return c.current(HoldStable)
    }
    c.ordinary=prior
    c.lastLower=now
    c.lastChange=now
    c.sinceDecision=0
    return c.current(Lower)
}
