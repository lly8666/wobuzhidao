#!/usr/bin/env python3
"""Independent continuous-time Gilbert-Elliott MDS source erasure oracle.

TEST ONLY. Never import adaptivefec/product controller or feed netem truth
into production. This reference models source-first, parity-after-closure
timings with exact two-state CTMC probabilities, not IID p averaged to 5%.
"""
from __future__ import annotations
import argparse
import json
import math
from pathlib import Path

VERSION="n2-independent-ge-ctmc-v1"
PROFILES=(0,4,8,10,12,16,20)
TARGET=0.001


def params(d):
    a=float(d["good_mean_ms"]); b=float(d["bad_mean_ms"])
    p_g=float(d["good_loss"]); p_b=float(d["bad_loss"])
    if not (a>0 and b>0 and 0<=p_g<=1 and 0<=p_b<=1):
        raise ValueError("invalid GE model")
    rate_g=1000/a
    rate_b=1000/b
    pi_bad=rate_g/(rate_g+rate_b)
    return rate_g,rate_b,p_g,p_b,pi_bad


def state_transition(rate_g,rate_b,dt_s):
    if dt_s<0 or not math.isfinite(dt_s):
        raise ValueError("nonmonotonic source/parity event times")
    den=rate_g+rate_b
    bad=rate_g/den
    good=1-bad
    x=math.exp(-den*dt_s)
    return ((good+bad*x,bad*(1-x)),
            (good*(1-x),bad+good*x))


def group_timestamps(k,r,sys_gap_ms,parity_gap_ms):
    if not (1<=k<=20 and r in PROFILES and sys_gap_ms>0 and parity_gap_ms>=0):
        raise ValueError("invalid group/timing")
    effective=min(k,r)
    systematic=[i*sys_gap_ms/1000 for i in range(k)]
    parity=[systematic[-1]+(i+1)*parity_gap_ms/1000 for i in range(effective)]
    return systematic+parity


def exact_source_residual(k,r,ge,sys_gap_ms,parity_gap_ms):
    """Average per-source probability that its shard is erased and group
    total erasures exceed emitted min(k,R); MDS source failure exact.
    """
    rate_g,rate_b,p_g,p_b,pi_bad=params(ge)
    times=group_timestamps(k,r,sys_gap_ms,parity_gap_ms)
    eff=min(k,r)
    n=len(times)
    prob_loss=(p_g,p_b)
    total=0.
    for target_idx in range(k):
        # keyed by (state, erasure_count, tagged_source_lost)
        dist={(0,0,False):1-pi_bad,(1,0,False):pi_bad}
        last=0.
        for idx,t in enumerate(times):
            P=state_transition(rate_g,rate_b,t-last)
            last=t
            updated={}
            for (state,count,tag),weight in dist.items():
                for next_state in (0,1):
                    middle=weight*P[state][next_state]
                    p=prob_loss[next_state]
                    for erased,part in ((0,1-p),(1,p)):
                        c=min(eff+1,count+erased)
                        tagged=tag or (idx==target_idx and erased==1)
                        key=(next_state,c,tagged)
                        updated[key]=updated.get(key,0.)+middle*part
            dist=updated
        total+=sum(v for (state,c,tag),v in dist.items()
                   if tag and c>eff)
    result=total/k
    if not (-1e-12<=result<=1+1e-12):
        raise AssertionError("GE probability not normalized")
    return max(0.,min(1.,result))


def mixture(groups,r,ge,sys_gap_ms,parity_gap_ms):
    weighted=den=0.
    for group in groups:
        k=int(group["k"])
        count=int(group["count"])
        if not 1<=k<=20 or count<=0:
            raise ValueError("bad closed group shape")
        w=k*count
        weighted+=w*exact_source_residual(k,r,ge,sys_gap_ms,parity_gap_ms)
        den+=w
    if not den:
        raise ValueError("no valid groups")
    return weighted/den


def freeze(manifest):
    if manifest.get("model_version")!=VERSION:
        raise ValueError("model freeze mismatch")
    ge=manifest["ge"]
    rate_g,rate_b,pg,pb,pi_bad=params(ge)
    observed=(1-pi_bad)*pg+pi_bad*pb
    if abs(observed-manifest["stationary_mean_loss"])>1e-10:
        raise ValueError("GE stationary mean loss changed")
    results={}
    for sc in manifest["assumptions"]:
        timing=sc["timing_ms"]
        curve={str(r):mixture(sc["groups"],r,ge,
            timing["source_spacing"],timing["parity_spacing"]) for r in PROFILES}
        sufficient=[int(r) for r,p in curve.items() if p<=manifest["target_source_residual"]]
        results[sc["id"]]={
            "hypothesis_only":True,"per_source_residual":curve,
            "least_sufficient_parity":sufficient[0] if sufficient else None,
            "target_unattainable_under_declared_geometry":not bool(sufficient),
            "shape":sc["groups"],"timing_ms":timing}
    return {"schema":1,"model_version":VERSION,"stationary_mean_loss":observed,
            "note":"one exactly specified CTMC (G mean450ms, B mean50ms), source-first parity-after-closure, not actual emitted wire packet time; recompute using actual verified group shapes and event timing where supported",
            "results":results,
            "production_selection_from_netem_truth":False,
            "measurement_status":"NOT_RUN_AT_REFERENCE_FREEZE"}


def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--manifest",type=Path,required=True)
    ap.add_argument("--output",type=Path,required=True)
    args=ap.parse_args()
    result=freeze(json.loads(args.manifest.read_text(encoding="utf8")))
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n",encoding="utf8")


if __name__=="__main__":
    main()
