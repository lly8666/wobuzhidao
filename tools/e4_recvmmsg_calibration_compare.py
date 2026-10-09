#!/usr/bin/env python3
"""Fail-closed cross-Actions E4 *synthetic* calibration comparator.

No benchmarks are run here. One independent workflow_dispatch produces ONE
OFF or ONE ON JSON. Never combine measurements from different CPU/vCPU/quota
strata or treat synthetic overhead as product qualifications.
"""
import argparse
import json
from pathlib import Path
from statistics import median

MIN_INDEPENDENT_CASES_PER_MODE = 3
MAX_CPU_RATIO = 1.05
MAX_RECV_P99_RATIO = 1.10
MAX_TRACER_RSS_KIB = 512 * 1024
MAX_HOST_CPU_PSI_AVG10_PERCENT = 10.0

def host_psi_avg10(host):
    line=(host or {}).get("cpu_pressure_first_line")
    if not isinstance(line,str) or not line.startswith("some "):
        return None
    for part in line.split():
        if part.startswith("avg10="):
            try:
                value=float(part.split("=",1)[1])
            except ValueError:
                return None
            return value if 0<=value<=100 else None
    return None


def stratum(row):
    host=row.get("host_before") or {}
    model,vcpus,quota=host.get("model"),host.get("vcpus"),host.get("cgroup_cpu_max")
    fixture_sha=row.get("fixture_sha256")
    if (not isinstance(model,str) or model=="UNKNOWN" or
            type(vcpus) is not int or vcpus<1 or
            not isinstance(quota,str) or not quota or
            not isinstance(fixture_sha,str) or len(fixture_sha)!=64):
        return None
    return (model,vcpus,quota,fixture_sha)

def valid_case(row,mode):
    if not isinstance(row,dict) or row.get("mode")!=mode:
        return False
    if row.get("status")!="ONE_SYNTHETIC_CASE_COMPLETE_NOT_CALIBRATED":
        return False
    if row.get("measurement_sec")!=12 or row.get("target_rate_hz")!=4000:
        return False
    if row.get("product_used") is not False or row.get("tracer_overhead_qualified") is not False:
        return False
    fixture=row.get("fixture") or {}
    if fixture.get("target_calls")!=48000 or fixture.get("decoy_calls")!=48000:
        return False
    for name in ("cpu_total_s","recv_p99_ns","recv_p999_ns"):
        value=fixture.get(name)
        if type(value) not in (int,float) or not 0<value<1e12:
            return False
    if row.get("cgroup_throttled_delta") != 0 or row.get("host_steal_ticks_delta") != 0:
        return False
    base=row.get("host_before")
    after=row.get("host_after")
    if not stratum(row) or not isinstance(after,dict):
        return False
    if (base.get("model")!=after.get("model") or
            base.get("vcpus")!=after.get("vcpus") or
            base.get("cgroup_cpu_max")!=after.get("cgroup_cpu_max")):
        return False
    for host in (base,after):
        pressure=host_psi_avg10(host)
        if pressure is None or pressure>MAX_HOST_CPU_PSI_AVG10_PERCENT:
            return False
    if mode=="on":
        counters=row.get("kernel_counters")
        tracer=row.get("tracer")
        if not isinstance(counters,dict) or not isinstance(tracer,dict):
            return False
        if (counters.get("entered"),counters.get("exited"),counters.get("unpaired"))!=(48000,48000,0):
            return False
        if tracer.get("exit_code")!=0 or type(tracer.get("maxrss_kib")) is not int or tracer["maxrss_kib"]>MAX_TRACER_RSS_KIB:
            return False
    else:
        if row.get("kernel_counters")!="NOT_MEASURED_OBSERVER_OFF":
            return False
    return True

def compare(off,on):
    result={"schema":"wbd-e4-recvmmsg-synthetic-cross-actions-comparison/v1",
            "status":"NOT_CALIBRATED",
            "min_cases_per_mode":MIN_INDEPENDENT_CASES_PER_MODE,
            "cpu_ratio_budget":MAX_CPU_RATIO,
            "recv_p99_ratio_budget":MAX_RECV_P99_RATIO,
            "max_host_cpu_psi_avg10_percent":MAX_HOST_CPU_PSI_AVG10_PERCENT,
            "product_qualification":"NOT_RUN",
            "product_cpu_gain":"UNPROVEN"}
    good_off=[x for x in off if valid_case(x,"off")]
    good_on=[x for x in on if valid_case(x,"on")]
    result["total_submitted"]={"off":len(off),"on":len(on)}
    result["quality_eligible"]={"off":len(good_off),"on":len(good_on)}
    overlap=set(stratum(x) for x in good_off)&set(stratum(x) for x in good_on)
    result["matching_strata"]=len(overlap)
    if len(off)!=len(good_off) or len(on)!=len(good_on):
        result["reason"]="RED_OR_UNKNOWN_SAMPLES_CANNOT_BE_DISCARDED"
        return result
    if not overlap:
        result["reason"]="NO_MATCHING_CPU_VCPU_QUOTA_FIXTURE_STRATUM"
        return result
    # A cross-stratum aggregate would require explicit uncertainty and
    # coverage weighting, so this first gate requires ONE matched stratum.
    if len(overlap)!=1 or any(stratum(x) not in overlap for x in good_off+good_on):
        result["reason"]="ONLY_ONE_EXACT_STRATUM_SUPPORTED"
        return result
    if len(good_off)<MIN_INDEPENDENT_CASES_PER_MODE or len(good_on)<MIN_INDEPENDENT_CASES_PER_MODE:
        result["reason"]="AT_LEAST_THREE_INDEPENDENT_ACTIONS_EACH_MODE_REQUIRED"
        return result
    f=lambda rows,k:[float(r["fixture"][k]) for r in rows]
    cpu_off,cpu_on=f(good_off,"cpu_total_s"),f(good_on,"cpu_total_s")
    p99_off,p99_on=f(good_off,"recv_p99_ns"),f(good_on,"recv_p99_ns")
    ratio_cpu=median(cpu_on)/median(cpu_off)
    ratio_p99=median(p99_on)/median(p99_off)
    result["scope"]="12S_LOCAL_AF_UNIX_4000_TARGET_CALLS_PER_SECOND"
    result["median_cpu_ratio_on_off"]=round(ratio_cpu,6)
    result["median_recv_p99_ratio_on_off"]=round(ratio_p99,6)
    result["worst_case_cpu_ratio_on_off"]=round(max(cpu_on)/min(cpu_off),6)
    result["worst_case_p99_ratio_on_off"]=round(max(p99_on)/min(p99_off),6)
    # No optimistic bootstrap p-values with only 3 samples. Strict
    # worst-case across independent Actions is intentionally conservative.
    if (max(cpu_on)/min(cpu_off)<=MAX_CPU_RATIO and
            max(p99_on)/min(p99_off)<=MAX_RECV_P99_RATIO):
        result["status"]="SYNTHETIC_SCOPE_ONLY_WITHIN_CONSERVATIVE_BUDGET"
        result["reason"]="SYNTHETIC_ONLY_NOT_PRODUCT_OVERHEAD_CERTIFIED"
    else:
        result["status"]="SYNTHETIC_OVERHEAD_BUDGET_NOT_PROVEN"
        result["reason"]="WORST_CASE_ON_OFF_EXCEEDS_PREDECLARED_LIMIT"
    return result

def main():
    p=argparse.ArgumentParser(description=__doc__)
    p.add_argument("--off",type=Path,required=True,nargs="+")
    p.add_argument("--on",type=Path,required=True,nargs="+")
    p.add_argument("--output",type=Path,required=True)
    args=p.parse_args()
    out=compare([json.loads(x.read_text()) for x in args.off],
                [json.loads(x.read_text()) for x in args.on])
    args.output.parent.mkdir(parents=True,exist_ok=True)
    args.output.write_text(json.dumps(out,indent=2,sort_keys=True)+"\n")
    print("E4_SYNTHETIC_CROSS_ACTIONS",out["status"],out.get("reason",""))

if __name__=="__main__": main()
