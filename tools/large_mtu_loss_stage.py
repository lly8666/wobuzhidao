#!/usr/bin/env python3
"""One 300s fullstack impairment sample, fixed loss OR one 5205 waveform.

5205 is a SINGLE business scenario with pre/stress/post stages of
75/150/75 seconds and an exactly 3s impaired drain; it is not an
in-run A/B or a set of independent throughput samples.
"""
import argparse
import json
import subprocess
import time
from pathlib import Path

def tc(ns, tc_bin, dev, loss, seed, delay_ms=300):
    cmd = ["ip", "netns", "exec", ns, tc_bin, "qdisc", "change",
           "dev", dev, "root", "netem", "limit", "200000", "delay", str(delay_ms)+"ms"]
    if loss:
        cmd += ["loss", "random", str(loss)+"%", "seed", str(seed)]
    p = subprocess.run(cmd, capture_output=True, text=True)
    if p.returncode:
        raise RuntimeError(repr((cmd, p.returncode, p.stdout, p.stderr)))

def snap(ns, tc_bin, dev):
    p = subprocess.run(["ip", "netns", "exec", ns, tc_bin, "-s", "-j",
                        "qdisc", "show", "dev", dev],
                       capture_output=True, text=True, check=True)
    return json.loads(p.stdout)

def sleep_until(ns):
    while True:
        remaining = ns - time.monotonic_ns()
        if remaining <= 0:
            return
        time.sleep(min(0.05, remaining / 1e9))

def main():
    a = argparse.ArgumentParser()
    for key in ["namespace","c2s-dev","s2c-dev","tc-bin","output"]:
        a.add_argument("--"+key, required=True)
    a.add_argument("--fixed-loss", type=int, choices=[0,1,5,10,20,30,5205], required=True)
    a.add_argument("--start-ns", type=int, required=True)
    a.add_argument("--seed", type=int, required=True)
    a.add_argument("--duration-s",type=int,default=300,choices=[15,120,300])
    a.add_argument("--drain-s",type=int,default=3,choices=[3])
    a.add_argument("--delay-ms",type=int,default=300,choices=[15,50,100,150,300])
    for key in ["pre-loss","stress-loss","post-loss"]:
        a.add_argument("--"+key, type=float)
    x = a.parse_args()
    waveform = x.fixed_loss == 5205
    if waveform and (x.duration_s!=300 or x.delay_ms!=300):
        raise ValueError('5205 formal stage must remain 300s/300ms')
    # 5205 is 5%->20%->5% across exactly one 300-second business run.
    phases = [("pre",0,5),("stress",75,20),("post",225,5)] if waveform else [
        ("fixed",0,x.fixed_loss)]
    Path(x.output).parent.mkdir(parents=True, exist_ok=True)
    with open(x.output, "w", buffering=1) as f:
        active = phases[0][2]
        def event(name, loss):
            row = {
                "event":name,"configured_duration_s":x.duration_s,"configured_delay_ms":x.delay_ms,
                "monotonic_ns":time.monotonic_ns(),
                "loss_percent":{"c2s":loss,"s2c":loss},
                "qdisc":{
                    "c2s":snap(x.namespace,x.tc_bin,x.c2s_dev),
                    "s2c":snap(x.namespace,x.tc_bin,x.s2c_dev),
                },
            }
            f.write(json.dumps(row,sort_keys=True)+"\n")
        try:
            for idx,(name,offset,loss) in enumerate(phases):
                sleep_until(x.start_ns + offset*1_000_000_000)
                if idx:
                    event(phases[idx-1][0]+"_end",active)
                tc(x.namespace,x.tc_bin,x.c2s_dev,loss,x.seed*100+idx*10+1,x.delay_ms)
                tc(x.namespace,x.tc_bin,x.s2c_dev,loss,x.seed*100+idx*10+2,x.delay_ms)
                active=loss
                if waveform:
                    event(name+"_start",loss)
                if idx==0:
                    event("business_start",loss)
            sleep_until(x.start_ns+x.duration_s*1_000_000_000)
            if waveform:
                event("post_end",active)
            event("business_end",active)
            # Real drain remains under the final-stage impairment.
            sleep_until(x.start_ns+(x.duration_s+x.drain_s)*1_000_000_000)
            event("drain_end",active)
            tc(x.namespace,x.tc_bin,x.c2s_dev,0,x.seed*100+91,x.delay_ms)
            tc(x.namespace,x.tc_bin,x.s2c_dev,0,x.seed*100+92,x.delay_ms)
        except Exception as exc:
            f.write(json.dumps({"event":"harness_error","message":str(exc)})+"\n")
            raise

if __name__=="__main__":
    main()
