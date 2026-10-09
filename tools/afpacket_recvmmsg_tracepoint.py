#!/usr/bin/env python3
"""E4 recvmmsg tracepoint feasibility only, never a product performance run.

The generated eBPF probes filter an exact target TGID and do not read packet
content, addresses, syscall arguments or thread IDs into user-visible data.
Two-second preflight deliberately uses nonexistent TGID 2147483647.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess

MARKER = "WBD_E4_RECVMMSG_TRACEPOINT_ATTACH_PASS"
MAX_BPF_MAP_KEYS = 4096
MIN_GAP_NS = 20_000_000
BUCKET_NS = 100_000_000

def program(pid, seconds=2):
    if type(pid) is not int or not 1 <= pid <= 2147483647:
        raise ValueError("positive numeric target tgid required")
    if type(seconds) is not int or seconds != 2:
        raise ValueError("only bounded 2s smoke is executable")
    return f"""tracepoint:syscalls:sys_enter_recvmmsg /pid == {pid}/ {{
  if (@previous_exit[pid] != 0) {{
    $gap_ns = nsecs - @previous_exit[pid];
    if ($gap_ns >= {MIN_GAP_NS}) {{
      @long_inter_call_gap_bucket[nsecs / {BUCKET_NS}] = count();
    }}
  }}
  @entered_ns[tid] = nsecs;
}}
tracepoint:syscalls:sys_exit_recvmmsg /pid == {pid} && @entered_ns[tid] != 0/ {{
  $duration_ns = nsecs - @entered_ns[tid];
  if ($duration_ns >= {MIN_GAP_NS}) {{
    @long_in_kernel_wait_bucket[nsecs / {BUCKET_NS}] = count();
  }}
  @exits = count();
  @previous_exit[pid] = nsecs;
  delete(@entered_ns[tid]);
}}
interval:s:{seconds} {{
  printf("{MARKER}\\n");
  exit();
}}
"""

def capabilities():
    root=Path("/sys/kernel/tracing/events/syscalls")
    return {
        "native_tracepoint_formats_visible": all(
            (root/(name+"/format")).is_file()
            for name in ("sys_enter_recvmmsg","sys_exit_recvmmsg")),
        "bpftrace_installed":shutil.which("bpftrace") is not None,
    }

def preflight():
    cap=capabilities()
    result={"schema":"wbd-e4-recvmmsg-trace-capability/v1",
            "status":"UNSUPPORTED","reason":"NOT_EVALUATED",
            "capability":cap,"scope":"TWO_SECOND_NO_PRODUCT_ATTACH_ONLY",
            "product_qualification":"NOT_RUN",
            "measurement_overhead_proven":False,
            "max_bpf_map_keys":MAX_BPF_MAP_KEYS,
            "threshold_ns":MIN_GAP_NS,"bucket_ns":BUCKET_NS,
            "no_perf_sysctl_or_tracefs_changes":True}
    if not cap["bpftrace_installed"]:
        result["reason"]="BPFTRACE_NOT_INSTALLED"
        return result
    env=os.environ.copy()
    env["BPFTRACE_MAX_MAP_KEYS"]=str(MAX_BPF_MAP_KEYS)
    try:
        run=subprocess.run(["bpftrace","-q","-e",program(2147483647)],
                           capture_output=True,text=True,check=False,
                           timeout=15,env=env)
    except (OSError,subprocess.TimeoutExpired) as exc:
        result["reason"]="TIMEOUT_OR_EXEC_FAILURE"
        result["exception_type"]=type(exc).__name__
        return result
    result["exit_code"]=run.returncode
    result["stderr_sha256"]=hashlib.sha256(run.stderr.encode()).hexdigest()
    if run.returncode==0 and MARKER in run.stdout:
        result["status"]="TRACEPOINT_ATTACH_ONLY"
        result["reason"]="NATIVE_PROBES_ATTACHED_NO_PRODUCT"
    else:
        result["reason"]="KERNEL_PERMISSION_OR_VERIFIER_REJECTED"
    return result

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument("--output",required=True,type=Path)
    parser.add_argument("--preflight",action="store_true")
    args=parser.parse_args()
    result=preflight() if args.preflight else {
        "schema":"wbd-e4-recvmmsg-trace-capability/v1",
        "status":"DESIGN_ONLY","product_qualification":"NOT_RUN",
        "capability":capabilities(),
        "program_sha256":hashlib.sha256(program(2147483647).encode()).hexdigest()}
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n")
    print("E4_RECVMMSG_CAPABILITY",result["status"],result.get("reason",""))

if __name__=="__main__":main()
