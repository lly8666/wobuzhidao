#!/usr/bin/env python3
"""One endpoint's concurrent real TCP+UDP business for a *single* C sample.

These two children are different protocols of the same mixed workload and
share its fixed 10Mbps per-direction aggregate; never separate test samples.
"""
import argparse
import json
import subprocess
import sys
from pathlib import Path

def specs(role, bind, peer, start_ns, seed, duration, drain, parent):
    tcp_path = str(Path(__file__).with_name("longmix_tcp_business.py"))
    udp_path = str(Path(__file__).with_name("longmix_udp_business.py"))
    port = 18080 if role == "target" else 28080
    udp_bind = bind.rsplit(":",1)[0]+":"+str(port)
    base = [sys.executable,"-u"]
    fixed = ["--role",role,"--seed",str(seed),
             "--start-ns",str(start_ns),"--duration",str(duration),
             "--drain",str(drain)]
    tcp = base+[tcp_path]+fixed+[
        "--workload","C","--bind",bind,
        "--output",str(parent/("tcp-"+role+".json"))]
    udp = base+[udp_path]+fixed+[
        "--workload","C","--bind",udp_bind,
        "--rate-mbps","5",
        "--output",str(parent/("udp-"+role+".json"))]
    if role=="biz":
        if peer is None:
            raise ValueError("client requires explicit private target peer")
        tcp+=["--peer",peer]
        udp+=["--peer",peer.rsplit(":",1)[0]+":18080"]
    elif peer:
        raise ValueError("target must not have direct peer")
    return tcp,udp

def main():
    a=argparse.ArgumentParser()
    a.add_argument("--role",choices=("biz","target"),required=True)
    a.add_argument("--workload",choices=("C",),required=True)
    a.add_argument("--bind",required=True)
    a.add_argument("--peer")
    a.add_argument("--seed",type=int,required=True)
    a.add_argument("--start-ns",type=int,required=True)
    a.add_argument("--duration",type=int,required=True)
    a.add_argument("--drain",type=int,required=True)
    a.add_argument("--output",required=True)
    args=a.parse_args()
    if args.seed!=2608103 or args.duration!=300 or args.drain!=10:
        a.error("C demands paired seed2608103 and one 300s+10s sample")
    root=Path(args.output).parent
    tcp,udp=specs(args.role,args.bind,args.peer,args.start_ns,
                  args.seed,args.duration,args.drain,root)
    children=[subprocess.Popen(x) for x in (tcp,udp)]
    results={}
    try:
        for name,p in zip(("tcp","udp"),children):
            try:
                results[name]=p.wait(timeout=340)
            except subprocess.TimeoutExpired:
                p.kill()
                results[name]=124
    finally:
        for p in children:
            if p.poll() is None:
                p.kill()
            p.wait()
    receipt={"schema":"wbd-mixed-endpoint/v1","workload":"C","role":args.role,
             "tcp_mbps_each_direction":5,"udp_mbps_each_direction":5,
             "no_quota_reallocation":True,"tcp_children":1,"udp_children":1,
             "two_protocols_one_sample":True,"exit_codes":results}
    Path(args.output).write_text(json.dumps(receipt,sort_keys=True)+"\n")
    if any(c!=0 for c in results.values()):
        raise SystemExit("one or more real protocol sources failed")

if __name__=="__main__":
    main()
