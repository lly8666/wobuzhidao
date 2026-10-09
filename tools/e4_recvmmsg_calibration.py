#!/usr/bin/env python3
"""One nonproduct, one-mode, single-job 12s receiver syscall calibration.

Each GitHub Actions workflow_dispatch runs exactly one mode OFF or ON.
Never compare modes within this driver or claim a qualified CPU gain.
No packet network interfaces, Go binaries, secrets, syscall arguments or
per-event BPF print are involved. A child C binary uses AF_UNIX socketpairs.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import resource
import subprocess
import tempfile
import time

from afpacket_recvmmsg_tracepoint import MAX_BPF_MAP_KEYS, program
from e4_recvmmsg_calibration_compare import host_psi_avg10

CALLS=48000
MARKER="WBD_E4_RECVMMSG_FIXTURE_TRACER_READY"
MAX_OUTPUT_BYTES=131072

def count_map(output,key):
    matches=re.findall(r"(?m)^@"+re.escape(key)+r":\s*(\d+)\s*$",output)
    if len(matches)!=1:
        raise ValueError("missing or repeated kernel aggregate "+key)
    return int(matches[0])

def validate_kernel_counts(text,fixture):
    if fixture.get("target_calls")!=CALLS or fixture.get("decoy_calls")!=CALLS:
        raise ValueError("fixture did not produce exactly 48000 calls per fd")
    entered=count_map(text,"enters")
    exited=count_map(text,"exits")
    extra=re.findall(r"(?m)^@unpaired_enter:\s*(\d+)\s*$",text)
    if entered!=CALLS or exited!=CALLS or sum(map(int,extra))!=0:
        raise ValueError("kernel target-fd counts missing/extra/unpaired")
    return {"entered":entered,"exited":exited,"unpaired":sum(map(int,extra)),
            "accounted_target_fraction":1.0,
            "tracepoint_output_size_bytes":len(text.encode())}

def cpu_total_s(usage):
    return round(usage.ru_utime+usage.ru_stime,6)

def read_limited(path):
    raw=path.read_bytes()
    if len(raw)>MAX_OUTPUT_BYTES:
        raise ValueError("bounded trace/log output exceeded 128 KiB")
    return raw.decode("utf-8",errors="replace")

def host_snapshot():
    model="UNKNOWN"
    try:
        for line in Path("/proc/cpuinfo").read_text().splitlines():
            if line.startswith("model name"):
                model=line.split(":",1)[1].strip()
                break
    except OSError:
        pass
    def read(path):
        try:
            return Path(path).read_text().strip()
        except OSError:
            return None
    cpu_pressure=read("/proc/pressure/cpu")
    quota=read("/sys/fs/cgroup/cpu.max")
    cpu_stat=read("/sys/fs/cgroup/cpu.stat")
    memory=read("/sys/fs/cgroup/memory.current")
    steal=None
    try:
        vals=Path("/proc/stat").read_text().splitlines()[0].split()
        steal=int(vals[8]) if len(vals)>8 else None
    except (OSError,ValueError,IndexError):
        pass
    return {"model":model,"vcpus":os.cpu_count(),"cgroup_cpu_max":quota,
            "cpu_pressure_first_line":cpu_pressure.splitlines()[0] if cpu_pressure else None,
            "cgroup_cpu_stat":cpu_stat,
            "cgroup_memory_current_bytes":int(memory) if memory and memory.isdigit() else None,
            "host_steal_ticks":steal}

def parse_nr_throttled(stat):
    if not stat:return None
    for line in stat.splitlines():
        fields=line.split()
        if len(fields)==2 and fields[0]=="nr_throttled" and fields[1].isdigit():
            return int(fields[1])
    return None

def wait_usage(child):
    pid,status,usage=os.wait4(child.pid,0)
    if pid!=child.pid:
        raise ValueError("unexpected child pid reaped")
    code=os.waitstatus_to_exitcode(status)
    child.returncode=code
    return code,usage

def assess_host_quality(receipt):
    issues=[]
    before=receipt.get("host_before") or {}
    after=receipt.get("host_after") or {}
    for label,host in (("before",before),("after",after)):
        psi=host_psi_avg10(host)
        if psi is None or psi>10.0:
            issues.append("CPU_PSI_UNKNOWN_OR_OVER_10_PCT_"+label.upper())
        if not host.get("cgroup_cpu_max"):
            issues.append("CPU_QUOTA_UNKNOWN_"+label.upper())
    for name in ("model","vcpus","cgroup_cpu_max"):
        if before.get(name)!=after.get(name):
            issues.append("HOST_STRATUM_CHANGED_"+name.upper())
    if receipt.get("cgroup_throttled_delta")!=0:
        issues.append("CPU_THROTTLE_NONZERO_OR_UNKNOWN")
    if receipt.get("host_steal_ticks_delta")!=0:
        issues.append("HOST_STEAL_NONZERO_OR_UNKNOWN")
    return {"eligible":not issues,"reasons":issues,"scope":"SYNTHETIC_HOST_QUALITY_ONLY"}

def run_one(fixture_binary,mode):
    if mode not in ("off","on"):
        raise ValueError("one and only one observer mode required")
    sha=hashlib.sha256(fixture_binary.read_bytes()).hexdigest()
    receipt={"schema":"wbd-e4-recvmmsg-single-mode-calibration/v1",
             "mode":mode,"status":"NOT_CALIBRATED",
             "fixture_sha256":sha,"product_used":False,"network_interface_used":False,
             "source_kind":"local AF_UNIX SOCK_DGRAM two-FD C fixture",
             "measurement_sec":12,"target_rate_hz":4000,"known_target_calls":CALLS,
             "known_noise_calls":CALLS,"observer":mode,
             "measurement_comparison":"NOT_PERFORMED_IN_THIS_RUN",
             "cross_host_normalization":"FORBIDDEN_WITHOUT_MATCHING_STRATA",
             "product_cpu_gain":"UNPROVEN","bpf_event_loss_under_product_load":"UNKNOWN",
             "tracer_overhead_qualified":False}
    with tempfile.TemporaryDirectory(prefix="wbd-e4-cal-") as temp:
        tmp=Path(temp)
        stdout=tmp/"fixture.out"
        stderr=tmp/"fixture.err"
        traceout=tmp/"trace.out"
        traceerr=tmp/"trace.err"
        baseline=host_snapshot()
        fixture=None
        tracer=None
        try:
            with stdout.open("wb") as out,stderr.open("wb") as err:
                fixture=subprocess.Popen([str(fixture_binary.resolve())],
                                         stdin=subprocess.PIPE,stdout=out,stderr=err)
            if mode=="on":
                env=os.environ.copy()
                env["BPFTRACE_MAX_MAP_KEYS"]=str(MAX_BPF_MAP_KEYS)
                # One fixture TGID; only FD30, not decoy FD31.
                script=program(fixture.pid,seconds=30,ready=True,recv_fd=30,
                               calibration=True)
                with traceout.open("wb") as out,traceerr.open("wb") as err:
                    tracer=subprocess.Popen(["bpftrace","-q","-e",script],
                                            stdout=out,stderr=err,env=env)
                deadline=time.monotonic()+12
                ready=False
                while time.monotonic()<deadline:
                    if MARKER in read_limited(traceout):
                        ready=True
                        break
                    if tracer.poll() is not None:
                        break
                    time.sleep(.05)
                if not ready:
                    raise ValueError("tracer not attached/ready before fixture starts")
            t0=time.monotonic()
            fixture.stdin.write(b"G")
            fixture.stdin.flush()
            fixture.stdin.close()
            fixture.stdin=None
            # 12s fixture with strict 20s bound, no second case.
            while fixture.poll() is None and time.monotonic()-t0<20:
                time.sleep(.05)
            if fixture.poll() is None:
                raise ValueError("single fixture timeout")
            code,usage=wait_usage(fixture) if fixture.returncode is None else (fixture.returncode,None)
            if usage is None:
                # Popen.poll() reaps; fallback to C's exact self rusage below.
                code=fixture.returncode
            if code!=0:
                raise ValueError("fixture nonzero exit; telemetry invalid")
            lines=read_limited(stdout).splitlines()
            if len(lines)!=1:
                raise ValueError("fixture must output exactly one JSON document")
            fixture_data=json.loads(lines[0])
            if fixture_data.get("target_calls")!=CALLS or fixture_data.get("decoy_calls")!=CALLS:
                raise ValueError("fixture counts invalid")
            elapsed=fixture_data.get("elapsed_ns")
            if type(elapsed) is not int or not 11_000_000_000<=elapsed<=18_000_000_000:
                raise ValueError("pacing not representative or fixture clock invalid")
            receipt["fixture"]={
                "elapsed_ns":elapsed,"target_calls":CALLS,"decoy_calls":CALLS,
                "recv_p99_ns":fixture_data["recv_p99_ns"],
                "recv_p999_ns":fixture_data["recv_p999_ns"],
                "recv_max_ns":fixture_data["recv_max_ns"],
                "late_over_250us":fixture_data["late_over_250us"],
                "maximum_lateness_ns":fixture_data["maximum_lateness_ns"],
                "cpu_user_s":round(fixture_data["cpu_user_us"]/1e6,6),
                "cpu_system_s":round(fixture_data["cpu_system_us"]/1e6,6),
                "cpu_total_s":round((fixture_data["cpu_user_us"]+fixture_data["cpu_system_us"])/1e6,6),
                "maxrss_kib":fixture_data["maxrss_kib"],
                "actual_target_rate_hz":round(CALLS/(elapsed/1e9),3)}
            if tracer:
                # BPF program is time-bounded to 30s. wait4 captures only
                # bpftrace userspace CPU (not in-kernel event overhead).
                trcode,trusage=wait_usage(tracer)
                receipt["tracer"]={
                    "exit_code":trcode,"userspace_cpu_s":cpu_total_s(trusage),
                    "maxrss_kib":trusage.ru_maxrss,
                    "stderr_sha256":hashlib.sha256(traceerr.read_bytes()).hexdigest()}
                if trcode!=0 or read_limited(traceerr).strip():
                    raise ValueError("tracer stderr or exit failure")
                trace=read_limited(traceout)
                receipt["kernel_counters"]=validate_kernel_counts(trace,fixture_data)
                receipt["tracer"]["stdout_sha256"]=hashlib.sha256(trace.encode()).hexdigest()
            else:
                receipt["kernel_counters"]="NOT_MEASURED_OBSERVER_OFF"
            receipt["status"]="ONE_SYNTHETIC_CASE_COMPLETE_NOT_CALIBRATED"
        except (OSError,ValueError,subprocess.SubprocessError,KeyError,
                json.JSONDecodeError) as exc:
            receipt["status"]="INCONCLUSIVE_SAMPLE_FAILURE"
            receipt["reason_type"]=type(exc).__name__
            receipt["reason"]=str(exc)[:180]
        finally:
            for p in (tracer,fixture):
                if p is not None and p.returncode is None:
                    p.kill()
                    try:p.wait(timeout=3)
                    except subprocess.TimeoutExpired:pass
            receipt["host_before"]=baseline
            receipt["host_after"]=host_snapshot()
            c1=parse_nr_throttled(receipt["host_before"]["cgroup_cpu_stat"])
            c2=parse_nr_throttled(receipt["host_after"]["cgroup_cpu_stat"])
            receipt["cgroup_throttled_delta"]=c2-c1 if c1 is not None and c2 is not None and c2>=c1 else None
            st=receipt["host_before"]["host_steal_ticks"]
            et=receipt["host_after"]["host_steal_ticks"]
            receipt["host_steal_ticks_delta"]=et-st if st is not None and et is not None and et>=st else None
            receipt["host_quality"]=assess_host_quality(receipt)
            if receipt["status"]=="ONE_SYNTHETIC_CASE_COMPLETE_NOT_CALIBRATED" and not receipt["host_quality"]["eligible"]:
                receipt["status"]="SYNTHETIC_EXECUTION_COMPLETE_HOST_QUALITY_INELIGIBLE"
    return receipt

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument("--fixture",type=Path,required=True)
    p.add_argument("--mode",choices=("off","on"),required=True)
    p.add_argument("--output",type=Path,required=True)
    args=p.parse_args()
    result=run_one(args.fixture,args.mode)
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print("E4_SYSCALL_CALIBRATION_ONE_CASE",result["status"],"mode",result["mode"])
    if result["status"]!="ONE_SYNTHETIC_CASE_COMPLETE_NOT_CALIBRATED":
        raise SystemExit(1)

if __name__=="__main__":main()
