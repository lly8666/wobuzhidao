#!/usr/bin/env python3
"""Exactly one N2 Normal 0->1->5->20->5->0 packet-loss waveform.

Five 120s stages, then 180s clean recovery: 780 seconds of one unchanged product/source/
workload/seed, not six configurations or an A/B benchmark. Each direction
uses its own router netem qdisc. The ubuntu-24.04 packaged tc netem
parser has NO seed option; the loss RNG is therefore UNSEEDED. Fixed
business seeds and actually observed per-stage qdisc drops are retained;
never claim that a requested kernel loss seed was applied. Captures
only bounded tc JSONL aggregates; no packet bodies or application payload.
"""
import argparse
import json
import subprocess
import time
from pathlib import Path

STAGES = (("clean0",0,0), ("loss1",120,1), ("loss5",240,5),
          ("loss20",360,20), ("recover5",480,5), ("recover0",600,0))
DURATION = 780
DRAIN = 10
DELAY = 300

def snapshot(ns,dev):
    p=subprocess.run(["ip","netns","exec",ns,"tc","-s","-j","qdisc",
                      "show","dev",dev],capture_output=True,text=True,check=True)
    value=json.loads(p.stdout)
    if not isinstance(value,list) or not value:
        raise ValueError("missing tc qdisc snapshot for "+dev)
    return value

def change(ns,dev,pct):
    cmd=["ip","netns","exec",ns,"tc","qdisc","change","dev",dev,
         "root","netem","limit","200000","delay","300ms"]
    if pct:
        cmd+=["loss","random",str(pct)+"%"]
    p=subprocess.run(cmd,capture_output=True,text=True)
    if p.returncode:
        raise RuntimeError("netem change rejected: "+repr((cmd,p.returncode,p.stderr)))

def until(epoch):
    while True:
        left=epoch-time.monotonic_ns()
        if left<=0:return
        time.sleep(min(0.05,left/1e9))

def main():
    p=argparse.ArgumentParser()
    p.add_argument("--namespace",required=True)
    p.add_argument("--c2s-dev",required=True)
    p.add_argument("--s2c-dev",required=True)
    p.add_argument("--start-ns",required=True,type=int)
    p.add_argument("--seed",required=True,type=int)
    p.add_argument("--output",required=True,type=Path)
    x=p.parse_args()
    if x.start_ns<=0 or x.seed<=0 or x.c2s_dev==x.s2c_dev:
        p.error("invalid single-case start/seed/dev identity")
    x.output.parent.mkdir(parents=True,exist_ok=True)
    with x.output.open("w",encoding="utf-8",buffering=1) as f:
        def event(event_name,name,pct,offset):
            now=time.monotonic_ns()
            row={"event":event_name,"name":name,"loss_percent":pct,
                 "offset_seconds":offset,"duration_seconds":DURATION,
                 "one_way_delay_ms":DELAY,"time_monotonic_ns":now,"time_wall_ns":time.time_ns(),
                 "requested_netem_seed":x.seed,"netem_seed_applied":False,
                 "netem_rng":"KERNEL_RANDOM_UNSEEDED_TC_OPTION_UNSUPPORTED",
                 "clock_delta_from_target_ns":now-(x.start_ns+offset*1000000000),
                 "c2s":snapshot(x.namespace,x.c2s_dev),
                 "s2c":snapshot(x.namespace,x.s2c_dev)}
            f.write(json.dumps(row,sort_keys=True)+"\n")
        try:
            for i,(name,offset,loss) in enumerate(STAGES):
                until(x.start_ns+offset*1000000000)
                if i:
                    prev=STAGES[i-1]
                    event("phase_end",prev[0],prev[2],offset)
                change(x.namespace,x.c2s_dev,loss)
                change(x.namespace,x.s2c_dev,loss)
                event("phase_start",name,loss,offset)
            until(x.start_ns+DURATION*1000000000)
            event("phase_end",STAGES[-1][0],STAGES[-1][2],DURATION)
            until(x.start_ns+(DURATION+DRAIN)*1000000000)
            event("drain_end",STAGES[-1][0],0,DURATION+DRAIN)
            change(x.namespace,x.c2s_dev,0)
            change(x.namespace,x.s2c_dev,0)
        except Exception as exc:
            f.write(json.dumps({"event":"harness_error","error":str(exc)})+"\n")
            raise

if __name__=="__main__":
    main()
