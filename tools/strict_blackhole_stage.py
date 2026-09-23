#!/usr/bin/env python3
import argparse, json, subprocess, time
from pathlib import Path

def wait_until(target_ns):
    while True:
        now=time.monotonic_ns()
        rem=target_ns-now
        if rem<=0: return
        if rem>2_000_000: time.sleep((rem-1_000_000)/1e9)

def tc_args(ns,tc_bin,dev,loss,seed):
    cmd=["ip","netns","exec",ns,tc_bin,"qdisc","change","dev",dev,"root","netem","limit","200000","delay","300ms"]
    if loss>0: cmd += ["loss","random",f"{loss:g}%","seed",str(seed)]
    return cmd

def change(args,dev,loss,seed):
    cmd=tc_args(args.namespace,args.tc_bin,dev,loss,seed)
    p=subprocess.run(cmd,check=False,text=True,capture_output=True)
    if p.returncode:
        raise RuntimeError(f"netem change failed rc={p.returncode} cmd={cmd!r} stdout={p.stdout!r} stderr={p.stderr!r}")

def qdisc(args,dev):
    p=subprocess.run(["ip","netns","exec",args.namespace,args.tc_bin,"-s","-j","qdisc","show","dev",dev],check=True,text=True,capture_output=True)
    return json.loads(p.stdout)

def emit(f,event,args,c_loss,s_loss,**extra):
    row={"schema":1,"event":event,"monotonic_ns":time.monotonic_ns(),"unix_ns":time.time_ns(),
         "loss_percent":{"c2s":c_loss,"s2c":s_loss},"tc_bin":args.tc_bin,
         "qdisc":{"c2s":qdisc(args,args.c2s_dev),"s2c":qdisc(args,args.s2c_dev)}}
    row.update(extra)
    f.write(json.dumps(row,sort_keys=True)+"\n"); f.flush()
    return row

def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--namespace",required=True); ap.add_argument("--c2s-dev",required=True); ap.add_argument("--s2c-dev",required=True)
    ap.add_argument("--tc-bin",default="tc"); ap.add_argument("--start-ns",type=int,required=True)
    ap.add_argument("--blackhole-ms",type=int,choices=(100,500),required=True); ap.add_argument("--seed",type=int,required=True)
    ap.add_argument("--output",required=True); args=ap.parse_args()
    Path(args.output).parent.mkdir(parents=True,exist_ok=True)
    with open(args.output,"w",encoding="utf-8",buffering=1) as f:
        try:
            wait_until(args.start_ns); emit(f,"pre_start",args,0,0)
            wait_until(args.start_ns+30_000_000_000); emit(f,"pre_end",args,0,0); emit(f,"stress_start",args,0,0)
            wait_until(args.start_ns+60_000_000_000)
            change(args,args.c2s_dev,100,args.seed*100+601); change(args,args.s2c_dev,100,args.seed*100+602)
            start=emit(f,"blackhole_start",args,100,100,blackhole_ms=args.blackhole_ms)
            wait_until(start["monotonic_ns"]+args.blackhole_ms*1_000_000)
            emit(f,"blackhole_end",args,100,100,blackhole_ms=args.blackhole_ms)
            change(args,args.c2s_dev,0,args.seed*100+603); change(args,args.s2c_dev,0,args.seed*100+604)
            emit(f,"blackhole_restored",args,0,0,blackhole_ms=args.blackhole_ms)
            wait_until(args.start_ns+90_000_000_000); emit(f,"stress_end",args,0,0); emit(f,"post_start",args,0,0)
            wait_until(args.start_ns+120_000_000_000); emit(f,"post_end",args,0,0); emit(f,"drain_start",args,0,0)
        except Exception as exc:
            f.write(json.dumps({"schema":1,"event":"harness_error","monotonic_ns":time.monotonic_ns(),"unix_ns":time.time_ns(),"error":str(exc)},sort_keys=True)+"\n")
            f.flush(); raise
if __name__=="__main__": main()
