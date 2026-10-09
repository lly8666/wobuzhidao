#!/usr/bin/env python3
"""Small functional recvmmsg tracepoint witness in a local AF_UNIX socketpair.

No throughput test, product process or public network. This only qualifies
whether the kernel produced two exit counts and one >20ms intercall gap.
A pass does NOT qualify trace overhead for 300s workload.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import select
import subprocess
import time

from afpacket_recvmmsg_tracepoint import MARKER,MAX_BPF_MAP_KEYS,program

READY="WBD_E4_RECVMMSG_FIXTURE_TRACER_READY"
COUNT=re.compile(r"(?m)^@exits:\s*(\d+)\s*$")
ENTER=re.compile(r"(?m)^@enters:\s*(\d+)\s*$")
GAP=re.compile(r"(?m)^@long_inter_call_gap_bucket\[\d+\]:\s*(\d+)\s*$")
UNPAIRED=re.compile(r"(?m)^@unpaired_enter:\s*(\d+)\s*$")

def result(text,fixture_exit,two_fd=False):
    enters=ENTER.findall(text)
    exits=COUNT.findall(text)
    gaps=[int(x) for x in GAP.findall(text)]
    unpaired=[int(x) for x in UNPAIRED.findall(text)]
    if len(enters)!=1 or len(exits)!=1 or fixture_exit!=0:
        return "INCONCLUSIVE_MISSING_COUNTS"
    if (int(enters[0])!=2 or int(exits[0])!=2 or
            sum(gaps)!=1 or sum(unpaired)!=0):
        return "INCONCLUSIVE_EVENTS_NOT_MATCHING_FIXTURE"
    if two_fd:
        return "TWO_FD_TARGET_ONLY_FUNCTIONAL"
    return "TWO_SYSCALLS_ONE_GAP_FUNCTIONAL_ONLY"


def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--fixture",required=True,type=Path)
    ap.add_argument("--target-fd",type=int,default=None,help="for dual socketpair, exact numeric target descriptor")
    ap.add_argument("--output",required=True,type=Path)
    a=ap.parse_args()
    out={"schema":"wbd-e4-recvmmsg-functional-attach/v1",
         "status":"NOT_RUN","fixture":"local AF_UNIX SOCK_DGRAM socketpair",
         "product_source_used":False,"no_product_network_traffic":True,
         "duration_seconds":5,"intentionally_one_intercall_gap_ms":60,
         "kernel_tracepoint_min_gap_ms":20,
         "scope":"DUAL_FD_ADVERSARIAL" if a.target_fd is not None else "ONE_FD_BASELINE",
         "non_target_recvmmsg_calls":2 if a.target_fd is not None else 0,
         "performance_overhead_validated":False}
    if not a.fixture.is_file():
        out["status"]="UNSUPPORTED_FIXTURE_MISSING"
    else:
        tracer=None
        child=None
        try:
            child=subprocess.Popen([str(a.fixture)],stdin=subprocess.PIPE,
                                   stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
            env=os.environ.copy()
            env["BPFTRACE_MAX_MAP_KEYS"]=str(MAX_BPF_MAP_KEYS)
            tracer=subprocess.Popen(["bpftrace","-q","-e",program(child.pid,seconds=5,ready=True,recv_fd=a.target_fd)],
                                    stdout=subprocess.PIPE,stderr=subprocess.PIPE,
                                    text=True,bufsize=1,env=env)
            ready=False
            deadline=time.monotonic()+9
            lines=[]
            while time.monotonic()<deadline:
                rd,_,_=select.select([tracer.stdout],[],[],max(.1,deadline-time.monotonic()))
                if rd:
                    line=tracer.stdout.readline()
                    if not line:
                        break
                    lines.append(line)
                    if READY in line:
                        ready=True
                        break
                if tracer.poll() is not None:
                    break
            if not ready:
                out["status"]="UNSUPPORTED_OR_NO_TRACE_ATTACH_HANDSHAKE"
            else:
                child.stdin.write(b"G")
                child.stdin.flush()
                child.stdin.close()
                child.stdin=None
                exit_code=child.wait(timeout=5)
                stdout,stderr=tracer.communicate(timeout=12)
                all_text="".join(lines)+stdout
                out["status"]=result(all_text,exit_code,two_fd=a.target_fd is not None)
                out["fixture_exit"]=exit_code
                out["bpftrace_exit"]=tracer.returncode
                out["captured_output_sha256"]=hashlib.sha256(all_text.encode()).hexdigest()
                out["stderr_sha256"]=hashlib.sha256(stderr.encode()).hexdigest()
                out["slow_recvmmsg_observed_not_required"]=False
                if tracer.returncode!=0:
                    out["status"]="UNSUPPORTED_BPFTRACE_RUNTIME_FAILURE"
        except (OSError,subprocess.TimeoutExpired) as exc:
            out["status"]="UNSUPPORTED_OR_TIMEOUT"
            out["exception_type"]=type(exc).__name__
        finally:
            for proc in (tracer,child):
                if proc is not None and proc.poll() is None:
                    proc.kill()
                    proc.wait(timeout=3)
    a.output.parent.mkdir(parents=True,exist_ok=True)
    a.output.write_text(json.dumps(out,sort_keys=True,indent=2)+"\n")
    print("E4_RECVMMSG_FUNCTIONAL",out["status"])

if __name__=="__main__":
    main()
