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
    // At most half of an independently authenticated 256-PN cohort may
    // consist of physically observed but edge-cropped FEC groups. Such
    // records are priced as NO FEC protection, not assumed k=20.
    MaxCroppedShare = 0.5
    confidenceHistory = 16 // recent FEC-active model: don't retain long-ago damage
    offConfidenceHistory = 32 // off needs >=8192 clean source PNs for unchanged 0.0005 Wilson margin
    ResidualTarget = 0.001
    decreaseMargin = 0.0005
    // If conservative edge/repair ambiguity makes the absolute target
    // unreachable, spending more parity for <=10% of the source-residual
    // target's marginal improvement is not an effective protection trade.
    // Only relative known-shape MDS model gain is compared: never rewrite
    // M+R to zero, assume cropped groups are complete, or drop fresh data.
    unattainableMarginalSlack = ResidualTarget / 10
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
    // Initial off is an explicit opt-in Normal AUTO experiment, never
    // an implicit change to legacy/default20 or aggressive's +1 mapping.
    initialOK:=c.InitialParity==20 ||
        (c.InitialParity==0 && c.Mode==Auto && c.MinParity==0)
    if (c.Mode != Auto && c.Mode != Aggressive) ||
        !initialOK || c.MaxParity != 20 ||
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
    // ALL successfully first-emitted KindLINK PNs are in Sent. This
    // includes FEC parity and FEC-off, not just k systematic shards.
    // ClosedR is the count of actually complete blocks by r=min(k,R).
    ClosedR [DataShards+1]uint16
    // Actual joint k/r geometry, bounded by the authenticated single source
    // PN window. The marginal k/r histograms alone are not a pairing proof.
    ClosedBySizeKR [3][DataShards+1][DataShards+1]uint16
    ParityRecords, OffRecords, CroppedRecords, InvalidRecords uint32
    // Exact sender-local masks for edge-truncated FEC groups. Parity can
    // reveal network loss but is NOT an application first-arrival source.
    CroppedSourceRecords, CroppedParityRecords uint32
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
    OffRecords, CroppedRecords uint64
    // Low-frequency read-only evidence for WHY stability/confidence HOLDS.
    // Last 16 accepted, disjoint windows only; rejected source windows do
    // not advance these counters or reset EWMA/stable clocks.
    ConfidenceSent, ConfidenceLost uint64
    ConfidenceHistoryWindows uint32
    FirstValidUnixNS, LastQualifiedUnixNS int64
    LastChangeUnixNS, LastLowerUnixNS int64
    // The LAST QUALIFIED source's lower-profile assessment. Rejected/echo-
    // duplicate sources do not recalculate it. These fields explain the
    // existing 30s/60s, Wilson and slow EWMA gates; they cannot cause a
    // grade transition, and are never written into the wire protocol.
    LowerAssessedSeq uint64
    LowerCandidate int
    LowerGate string
    LowerConfidenceResidual float64
    LowerSlowRequired int
    LowerWaitingStable, LowerConfidenceBlocked, LowerSlowBlocked bool
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
    confidentSent, confidentLost, confidentLowerLost uint64
    history [confidenceHistory]controlSample
    historyPos, historyCount int
    // The off decision still needs 32 disjoint certified windows: with
    // only 16*256 clean PNs, Wilson95 upper > decreaseMargin. This fixed
    // extra ring holds counters/shapes only, never actual packet data.
    offHistory [offConfidenceHistory]controlSample
    offHistoryPos, offHistoryCount int
    shapeTotals [3][DataShards+1]uint64
    mixedEvidence bool
    boundWidth float64
    offTotal,croppedTotal,croppedSourceTotal uint64
    lowerAssessedSeq uint64
    lowerCandidate int
    lowerGate string
    lowerConfidenceResidual float64
    lowerSlowRequired int
    lowerWaitingStable, lowerConfidenceBlocked, lowerSlowBlocked bool
}

type controlSample struct {
    sent, upperLost, lowerLost uint32
    off,cropped,croppedSource uint32
    groups [3][DataShards+1]uint16
}


func New(c Config, generation uint64) (*Controller, error) {
    if err := c.Validate(); err != nil || generation == 0 { return nil, ErrConfig }
    return &Controller{cfg:c, generation:generation, ordinary:c.InitialParity}, nil
}

func controllerUnixNano(t time.Time) int64 {
    if t.IsZero() {return 0}
    return t.UnixNano()
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
        ShapeRecords:c.shapeRecords(), ShapeBlocks:c.shapeBlocks(),
        OffRecords:c.offTotal, CroppedRecords:c.croppedTotal,
        ConfidenceSent:c.confidentSent, ConfidenceLost:c.confidentLost,
        ConfidenceHistoryWindows:uint32(c.historyCount),
        FirstValidUnixNS:controllerUnixNano(c.firstValid),
        LastQualifiedUnixNS:controllerUnixNano(c.lastAt),
        LastChangeUnixNS:controllerUnixNano(c.lastChange),
        LastLowerUnixNS:controllerUnixNano(c.lastLower),
        LowerAssessedSeq:c.lowerAssessedSeq,LowerCandidate:c.lowerCandidate,
        LowerGate:c.lowerGate,LowerConfidenceResidual:c.lowerConfidenceResidual,
        LowerSlowRequired:c.lowerSlowRequired,
        LowerWaitingStable:c.lowerWaitingStable,
        LowerConfidenceBlocked:c.lowerConfidenceBlocked,
        LowerSlowBlocked:c.lowerSlowBlocked}
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
// A 95% LOWER line-damage bound is the opposite of downgrade's upper
// residual risk bound. Only a credibly damaged, actual FEC-off cohort
// triggers a small pilot; one loss in a short sample cannot drive a blind
// jump to maximum redundancy.
func wilsonLower(lost,sent uint64) float64 {
    if sent==0 || lost==0 {return 0}
    n:=float64(sent)
    p:=float64(lost)/n
    z2:=1.96*1.96
    v:=(p+z2/(2*n)-1.96*math.Sqrt(p*(1-p)/n+z2/(4*n*n)))/(1+z2/n)
    if v<0 {return 0}
    return v
}

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

// Edge groups have real emitted PN coverage but no authoritative whole
// k/r in THIS report. Only their observed SYSTEMATIC PNs represent business
// first-arrivals. Treat these unverified k sources conservatively unprotected;
// observed parity PNs still count in sender exposure/confidence/width but
// NEVER as invented unprotected business. No guessed k=20 groups.
// Counts of parity from complete groups are NEVER counted as business sources.
func mixedResidualWithEdges(groups [3][DataShards+1]uint64,off,croppedSource uint64,parity int,p float64) float64 {
	var systematic uint64
	var weighted float64
	for _,class:=range groups {
		for k:=1;k<=DataShards;k++ {
			n:=class[k]
			if n==0 {continue}
			systematic+=uint64(k)*n
			weighted+=float64(k)*float64(n)*ResidualReference(k,parity,p)
		}
	}
	unknown:=off+croppedSource
	if systematic+unknown==0 {return math.NaN()}
	return (weighted+float64(unknown)*p)/float64(systematic+unknown)
}

// Conservative absolute target may be unattainable because a 256-PN
// source window has UNKNOWN boundary groups or historical FEC-off source.
// Do not convert "unattainable" to a blind R=20. Instead choose the
// least expensive rung within a very small absolute source-residual
// distance of the best *modeled* protection (<=10% of the 0.1% target).
// This retains all meaningful known-group protection, and avoids treating
// unverifiable systematic edge source as either zero loss or full k20.
func requiredProfileWithEdges(groups [3][DataShards+1]uint64,off,croppedSource uint64,p float64,min,max int) int {
    var selected [len(profiles)]float64
    best:=math.Inf(1)
    for i,v:=range profiles {
        if v<min||v>max {selected[i]=math.NaN(); continue}
        q:=mixedResidualWithEdges(groups,off,croppedSource,v,p)
        selected[i]=q
        if !math.IsNaN(q) && q<best {best=q}
        if q<=ResidualTarget {return v}
    }
    if math.IsInf(best,1) {return max}
    for i,v:=range profiles {
        if v>=min&&v<=max&&!math.IsNaN(selected[i]) &&
           selected[i]<=best+unattainableMarginalSlack {return v}
    }
    return max
}

// An absolute historical confidence bound can be dominated by edge crop
// and ambiguous same-wire repair attempts; while such uncertainty remains,
// permit a *stable, one-rung* decrease only if the proposed rung loses at
// most 10% of the target residual protection relative to the CURRENT
// profile on the very same conservative input. It does not manufacture a
// clean sample, bypass 512 records, slow EWMA, 30/60s stability, or 10s
// change limits. A materially protective rung still cannot be skipped.
func negligibleMarginalProtection(groups [3][DataShards+1]uint64,
    off,croppedSource uint64,p float64,prior,current int) bool {
    if prior>=current || prior<0 || current>20 || math.IsNaN(p) {return false}
    lower:=mixedResidualWithEdges(groups,off,croppedSource,prior,p)
    upper:=mixedResidualWithEdges(groups,off,croppedSource,current,p)
    return !math.IsNaN(lower) && !math.IsNaN(upper) &&
        lower>=upper && lower-upper<=unattainableMarginalSlack
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

// Off has strictly more exact zero-loss confidence requirements than
// a reduction between enabled FEC rungs. Aggregate up to 32 independently
// authenticated qualified windows, including their actual closed k/R and
// cropped source, without treating observed repairs or UNKNOWN as clean.
// On every initial off pilot, this history resets with the profile history.
// No packet storage or extra network sampling is introduced.
type offConfidenceTotals struct {
    sent, upperLost uint64
    off, croppedSource uint64
    groups [3][DataShards+1]uint64
}

func (c *Controller) offProofTotals() offConfidenceTotals {
    var total offConfidenceTotals
    for i:=0;i<c.offHistoryCount;i++ {
        x:=c.offHistory[i]
        total.sent+=uint64(x.sent)
        total.upperLost+=uint64(x.upperLost)
        total.off+=uint64(x.off)
        total.croppedSource+=uint64(x.croppedSource)
        for class:=range total.groups {
            for k:=1;k<=DataShards;k++ {
                total.groups[class][k]+=uint64(x.groups[class][k])
            }
        }
    }
    return total
}

// Only accepted nonoverlapping source windows contribute. The last 16
// accepted intervals bound confidence and shape weighting; otherwise a
// lifetime of clean windows would mask a new damaged period indefinitely.
func (c *Controller) pushSample(s Input,upper uint32,bounded bool) {
    if c.historyCount==len(c.history) {
        old:=c.history[c.historyPos]
        c.confidentSent-=uint64(old.sent)
        c.confidentLost-=uint64(old.upperLost)
        c.confidentLowerLost-=uint64(old.lowerLost)
        c.offTotal-=uint64(old.off)
        c.croppedTotal-=uint64(old.cropped)
        c.croppedSourceTotal-=uint64(old.croppedSource)
        for class:=range c.shapeTotals {
            for k:=1;k<=DataShards;k++ {
                c.shapeTotals[class][k]-=uint64(old.groups[class][k])
            }
        }
    } else {c.historyCount++}
    item:=controlSample{sent:s.Sent,upperLost:upper,lowerLost:s.FirstAttemptLost}
    if bounded {
        item.groups=s.ClosedGroups
        item.off=s.OffRecords
        item.cropped=s.CroppedRecords
        item.croppedSource=s.CroppedSourceRecords
    }
    c.history[c.historyPos]=item
    c.historyPos=(c.historyPos+1)%len(c.history)
    c.offHistory[c.offHistoryPos]=item
    c.offHistoryPos=(c.offHistoryPos+1)%len(c.offHistory)
    if c.offHistoryCount<len(c.offHistory) {c.offHistoryCount++}
    c.confidentSent+=uint64(s.Sent)
    c.confidentLost+=uint64(upper)
    c.confidentLowerLost+=uint64(s.FirstAttemptLost)
    if bounded {
        c.offTotal+=uint64(s.OffRecords)
        c.croppedTotal+=uint64(s.CroppedRecords)
        c.croppedSourceTotal+=uint64(s.CroppedSourceRecords)
    }
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
        var shapeSourceRecords,wholeGroups,wholeRGroups,wholeParity uint64
        for _,class:=range s.ClosedGroups {
            for k:=1;k<=DataShards;k++ {
                n:=uint64(class[k])
                shapeSourceRecords+=uint64(k)*n
                wholeGroups+=n
            }
        }
        for r:=1;r<=DataShards;r++ {
            n:=uint64(s.ClosedR[r])
            wholeRGroups+=n
            wholeParity+=uint64(r)*n
        }
        if s.ClosedR[0]!=0 {return c.current(HoldShape)}
        for class:=0;class<3;class++ {
            if s.ClosedGroups[class][0]!=0 {return c.current(HoldShape)}
        }
        var jointK [3][DataShards+1]uint64
        var jointR [DataShards+1]uint64
        for class:=0;class<3;class++ {
            for k:=0;k<=DataShards;k++ {
                for r:=0;r<=DataShards;r++ {
                    n:=uint64(s.ClosedBySizeKR[class][k][r])
                    if n==0 {continue}
                    // r=min(k,actual configured supported parity).
                    // Any r<k must be an explicitly supported fixed R;
                    // r==k is possible when configured R>=k.
                    if k==0 || r==0 || r>k || (r<k && !supported(r)) {
                        return c.current(HoldShape)
                    }
                    jointK[class][k]+=n
                    jointR[r]+=n
                }
            }
        }
        for class:=0;class<3;class++ {
            for k:=1;k<=DataShards;k++ {
                if jointK[class][k]!=uint64(s.ClosedGroups[class][k]) {
                    return c.current(HoldShape)
                }
            }
        }
        for r:=1;r<=DataShards;r++ {
            if jointR[r]!=uint64(s.ClosedR[r]) {
                return c.current(HoldShape)
            }
        }
        // All KindLINK first-success records share one 256-PN window,
        // including parity and off. Match actual r=min(k,R) closure count.
        // A closed block with k sources has AT LEAST one parity and at most
        // k; no "full k=20" approximation can fill a cropped edge.
        if s.InvalidRecords!=0 ||
           uint64(s.CroppedSourceRecords)+uint64(s.CroppedParityRecords)!=uint64(s.CroppedRecords) ||
           wholeRGroups!=wholeGroups ||
           wholeParity!=uint64(s.ParityRecords) ||
           wholeParity>shapeSourceRecords ||
           shapeSourceRecords+wholeParity+uint64(s.OffRecords)+uint64(s.CroppedRecords)!=uint64(s.Sent) ||
           (shapeSourceRecords+uint64(s.OffRecords))==0 ||
           float64(s.CroppedRecords)/float64(s.Sent)>MaxCroppedShare {
            return c.current(HoldShape)
        }
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
    // Reset the detail on each accepted source. A later UNKNOWN/HOLD due
    // to invalid source retains the last QUALIFIED assessment+its seq.
    c.lowerAssessedSeq=s.Sequence
    c.lowerCandidate=0
    c.lowerGate="WAIT_SAMPLES"
    c.lowerConfidenceResidual=0
    c.lowerSlowRequired=0
    c.lowerWaitingStable=false
    c.lowerConfidenceBlocked=false
    c.lowerSlowBlocked=false
    if c.sinceDecision<MinDecisionRecords {return c.current(HoldSampleCount)}
    // Off-start cohorts contain only bare LINK records. Even while FEC
    // is off, SAME-WIRE retransmissions may have been attempted: use only
    // the conservative LOWER physical first-loss bound M (post-repair
    // receiver deficit), NEVER the possible M+R upper bound to prove damage.
    // Their observed k is UNKNOWN, however. Using these off records as
    // if no future parity can protect them would always pick max20.
    // Probe the first allowed rung (20:4) ONLY when 95% lower line-damage
    // confidence exceeds the predeclared 0.1% first-loss target. Permit
    // finite initial first-loss, then collect >=512 NEW post-probe records
    // with their true full/partial k/r before further model-based escalation.
    // UNKNOWN, capacity, stale and mixed/cropped cohorts never use this.
    if bounded && c.ordinary==0 && c.offTotal==c.confidentSent &&
        c.croppedTotal==0 && c.shapeBlocks()==0 {
        if wilsonLower(c.confidentLowerLost,c.confidentSent)<=ResidualTarget {
            c.lowerGate="OFF_WAIT_CONFIDENT_DAMAGE"
            return c.current(HoldOptimal)
        }
        c.ordinary=4
        c.lastChange=now
        c.firstValid=now
        c.lastLower=time.Time{}
        c.sinceDecision=0
        // Off had no verifiable FEC k; discard ONLY the profile-model
        // history, not the monotonic source identity or EWMA. This stops
        // an old unprotectable off cohort from falsely forcing 20:20 after
        // the pilot. Future grade advice requires fresh authenticated k/r.
        c.history=[confidenceHistory]controlSample{}
        c.historyPos,c.historyCount=0,0
        c.offHistory=[offConfidenceHistory]controlSample{}
        c.offHistoryPos,c.offHistoryCount=0,0
        c.confidentSent,c.confidentLost,c.confidentLowerLost=0,0,0
        c.shapeTotals=[3][DataShards+1]uint64{}
        c.offTotal,c.croppedTotal,c.croppedSourceTotal=0,0,0
        c.lowerGate="OFF_PILOT_WITH_WILSON_DAMAGE"
        return c.current(Raise)
    }
    upTarget:=0
    if bounded {
        upTarget=requiredProfileWithEdges(c.shapeTotals,c.offTotal,c.croppedSourceTotal,math.Max(c.fast,p),c.cfg.MinParity,c.cfg.MaxParity)
    } else {
        upTarget=requiredProfile(int(s.ShardK),math.Max(c.fast,p),c.cfg.MinParity,c.cfg.MaxParity)
    }
    if upTarget>c.ordinary {
        c.ordinary=upTarget
        c.lastChange=now
        c.sinceDecision=0
        c.firstValid=now
        c.lowerGate="RAISED"
        return c.current(Raise)
    }
    prior:=priorProfile(c.ordinary,c.cfg.MinParity)
    if prior>=c.ordinary {
        c.lowerGate="AT_FLOOR"
        return c.current(HoldOptimal)
    }
    stable:=30*time.Second
    if prior==0 {stable=60*time.Second}
    // Diagnose every soft downgrade blocker, even if the 30/60s stable
    // clock still blocks. Enabled FEC rungs use the recent 16 qualified
    // windows; a proposal to switch FEC fully OFF instead uses 32 *actual*
    // qualified windows to preserve the unchanged 0.05% Wilson margin.
    confidence:=wilsonUpper(c.confidentLost,c.confidentSent)
    modelGroups:=c.shapeTotals
    modelOff,modelCroppedSource:=c.offTotal,c.croppedSourceTotal
    if prior==0 {
        proof:=c.offProofTotals()
        confidence=wilsonUpper(proof.upperLost,proof.sent)
        modelGroups=proof.groups
        modelOff,modelCroppedSource=proof.off,proof.croppedSource
    }
    residual:=0.0
    slowRequired:=0
    if bounded {
        residual=mixedResidualWithEdges(modelGroups,modelOff,modelCroppedSource,prior,confidence)
        slowRequired=requiredProfileWithEdges(modelGroups,modelOff,modelCroppedSource,c.slow,c.cfg.MinParity,c.cfg.MaxParity)
    } else {
        residual=ResidualReference(int(s.ShardK),prior,confidence)
        slowRequired=requiredProfile(int(s.ShardK),c.slow,c.cfg.MinParity,c.cfg.MaxParity)
    }
    waitStable:=now.Sub(c.firstValid)<stable ||
       (!c.lastChange.IsZero() && now.Sub(c.lastChange)<10*time.Second) ||
       (!c.lastLower.IsZero() && now.Sub(c.lastLower)<10*time.Second)
    confidenceBlocked:=residual>decreaseMargin
    // In bounded mode only, historical M+R/cropped uncertainty must not
    // force endless max parity when the next smaller rung loses essentially
    // no model protection. This is NOT an assertion of physical zero loss.
    if bounded && prior!=0 && confidenceBlocked &&
       negligibleMarginalProtection(modelGroups,modelOff,
           modelCroppedSource,confidence,prior,c.ordinary) {
        confidenceBlocked=false
    }
    slowBlocked:=slowRequired>prior
    c.lowerCandidate=prior
    c.lowerConfidenceResidual=residual
    c.lowerSlowRequired=slowRequired
    c.lowerWaitingStable=waitStable
    c.lowerConfidenceBlocked=confidenceBlocked
    c.lowerSlowBlocked=slowBlocked
    if waitStable {
        c.lowerGate="WAIT_STABLE"
        return c.current(HoldStable)
    }
    if confidenceBlocked {
        c.lowerGate="CONFIDENCE_RESIDUAL"
        return c.current(HoldStable)
    }
    if slowBlocked {
        c.lowerGate="SLOW_EWMA"
        return c.current(HoldStable)
    }
    c.lowerGate="LOWERED"
    c.ordinary=prior
    c.lastLower=now
    c.lastChange=now
    c.sinceDecision=0
    return c.current(Lower)
}
