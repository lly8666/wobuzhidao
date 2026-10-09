#!/usr/bin/env python3
"""Profile-ON-only bounded AF_PACKET numerical probe in one network namespace.

Do not share these measurements with profile-OFF CPU gain claims. The observer
uses extra CPU and never accesses data packets or socket payloads.
"""
import argparse
import json
import signal
import subprocess
import time
from pathlib import Path
from packet_socket_drop_forensics import packet_socket, psi_avg10

def numeric_row(side, began, finished, ss_out, rc, pressure):
    sock=packet_socket({"namespaces":{side:{"ss_packet":{"returncode":rc,"stdout":ss_out}}}},side)
    return {"schema":1,"side":side,"ss_start_monotonic_ns":began,
            "ss_end_monotonic_ns":finished,"ss_duration_ns":finished-began,
            "packet_socket":sock,"ss_ok":rc==0 and sock is not None,
            "cpu_psi_some_avg10_percent":psi_avg10({"pressure":{"cpu":pressure}})}

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--side",choices=("client","server"),required=True)
    ap.add_argument("--business-start-ns",type=int,required=True)
    ap.add_argument("--interval-ms",type=int,default=100)
    ap.add_argument("--duration-s",type=int,default=303)
    ap.add_argument("--output",required=True,type=Path)
    args=ap.parse_args()
    if args.business_start_ns<=0 or args.interval_ms!=100 or args.duration_s!=303:
        ap.error("fixed bounded probe requires positive business start, 100ms and 303s")
    stop=False
    def stop_handler(_signum,_frame):
        nonlocal stop
        stop=True
    signal.signal(signal.SIGTERM,stop_handler)
    signal.signal(signal.SIGINT,stop_handler)
    count=0; next_ns=time.monotonic_ns()
    deadline=args.business_start_ns+303_000_000_000
    args.output.parent.mkdir(parents=True,exist_ok=True)
    with args.output.open("w",encoding="utf-8",buffering=1) as f:
        while not stop and count<4096 and time.monotonic_ns()<deadline:
            began=time.monotonic_ns()
            try:
                p=subprocess.run(["ss","-0","-a","-m","-n"],capture_output=True,
                                 text=True,timeout=0.4,check=False)
                out,rc=p.stdout,p.returncode
            except (OSError,subprocess.TimeoutExpired):
                out,rc="",-1
            finished=time.monotonic_ns()
            try:
                pressure=Path("/proc/pressure/cpu").read_text(encoding="utf-8")
            except OSError:
                pressure=""
            row=numeric_row(args.side,began,finished,out,rc,pressure)
            row["business_elapsed_bracket_s"]=[
                round((began-args.business_start_ns)/1e9,9),
                round((finished-args.business_start_ns)/1e9,9)]
            # Persist numerical counters, not ss stdout/addresses.
            f.write(json.dumps(row,sort_keys=True,separators=(",",":"))+"\n")
            count+=1
            next_ns+=100_000_000
            now=time.monotonic_ns()
            if now>next_ns+100_000_000:
                next_ns=now+100_000_000
            if next_ns>now:
                time.sleep((next_ns-now)/1e9)
    print("AF_PACKET_NUMERICAL_PROBE side=%s samples=%d" %(args.side,count))

if __name__=="__main__":
    main()
