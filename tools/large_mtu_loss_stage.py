#!/usr/bin/env python3
"""One fixed 300-second dual-direction loss stage; one sample, no loop of scenarios."""
import argparse,json,subprocess,time
from pathlib import Path

def tc(ns,bin,dev,loss,seed):
    cmd=["ip","netns","exec",ns,bin,"qdisc","change","dev",dev,
         "root","netem","limit","200000","delay","300ms"]
    if loss:cmd+=["loss","random",str(loss)+"%","seed",str(seed)]
    p=subprocess.run(cmd,capture_output=True,text=True)
    if p.returncode:raise RuntimeError(repr((cmd,p.returncode,p.stdout,p.stderr)))

def snap(ns,bin,dev):
    p=subprocess.run(["ip","netns","exec",ns,bin,"-s","-j","qdisc","show","dev",dev],
                     capture_output=True,text=True,check=True)
    return json.loads(p.stdout)

def main():
    a=argparse.ArgumentParser()
    for x in ["namespace","c2s-dev","s2c-dev","tc-bin","output"]:a.add_argument("--"+x,required=True)
    a.add_argument("--fixed-loss",type=int,choices=[0,5,20,30],required=True)
    a.add_argument("--start-ns",type=int,required=True)
    a.add_argument("--seed",type=int,required=True)
    # Existing strict harness arguments accepted, but fixed-loss is authoritative.
    for x in ["pre-loss","stress-loss","post-loss"]:a.add_argument("--"+x,type=float)
    x=a.parse_args()
    Path(x.output).parent.mkdir(parents=True,exist_ok=True)
    with open(x.output,"w",buffering=1) as f:
        try:
            def event(name):
                row={"event":name,"monotonic_ns":time.monotonic_ns(),
                     "loss_percent":{"c2s":x.fixed_loss,"s2c":x.fixed_loss},
                     "qdisc":{"c2s":snap(x.namespace,x.tc_bin,x.c2s_dev),
                              "s2c":snap(x.namespace,x.tc_bin,x.s2c_dev)}}
                f.write(json.dumps(row)+"\n")
            while time.monotonic_ns()<x.start_ns:
                time.sleep(min(.1,(x.start_ns-time.monotonic_ns())/1e9))
            tc(x.namespace,x.tc_bin,x.c2s_dev,x.fixed_loss,x.seed*100+1)
            tc(x.namespace,x.tc_bin,x.s2c_dev,x.fixed_loss,x.seed*100+2)
            event("business_start")
            while time.monotonic_ns()<x.start_ns+300_000_000_000:time.sleep(.05)
            event("business_end")
            # Preserve the same impairment for the exact three-second drain, not fifteen.
            while time.monotonic_ns()<x.start_ns+303_000_000_000:time.sleep(.05)
            event("drain_end")
            tc(x.namespace,x.tc_bin,x.c2s_dev,0,x.seed*100+91)
            tc(x.namespace,x.tc_bin,x.s2c_dev,0,x.seed*100+92)
        except Exception as ex:
            f.write(json.dumps({"event":"harness_error","message":str(ex)})+"\n")
            raise

if __name__=="__main__":main()
