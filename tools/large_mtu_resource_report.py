#!/usr/bin/env python3
"""Read-only bounded synthesis of existing strict resource + diagnostic receipts.

Do not turn a slow hosted run into CAPACITY_LIMITED without measured capacity
pressure. This is a companion to the real-path analyzer, not a workload.
"""
import json
import os
import re
import runpy
from pathlib import Path

def _pct_stat(line, key):
    if not isinstance(line,str):return None
    match=re.search(r"\b"+re.escape(key)+r"=([0-9.]+)",line)
    return float(match.group(1)) if match else None

def _proc_cpu(prev,current):
    a=(prev or {}).get("proc_stat",{}).get("cpu")
    b=(current or {}).get("proc_stat",{}).get("cpu")
    if not a or not b or len(a)<8 or len(b)<8:return None
    d=[y-x for x,y in zip(a,b)]
    total=sum(d)
    if total<=0:return None
    return {"host_busy_percent":100*(total-d[3]-d[4])/total,
            "softirq_percent":100*d[6]/total,"steal_percent":100*d[7]/total}

def _rss(proc):
    status=(proc or {}).get("status") or {}
    def mib(s):
        match=re.search(r"(\d+)\s*kB",s or "")
        return int(match.group(1))/1024 if match else None
    return mib(status.get("VmRSS")),mib(status.get("VmHWM"))

def _diag(root,label):
    path=root/(label+"-diag.jsonl")
    if not path.exists():return {"present":False}
    first=last=None; count=0;errors=0
    with path.open(encoding="utf-8",errors="replace") as f:
        for line in f:
            try:
                obj=json.loads(line)
                if first is None:first=obj
                last=obj;count+=1
            except json.JSONDecodeError:errors+=1
    if not count:return {"present":False,"parse_errors":errors}
    def pick(row):
        out={}
        for key in ("owner","raw_io","lanes","process","resources"):
            if key in row:out[key]=row[key]
        return out
    return {"present":True,"samples":count,"parse_errors":errors,
            "first_state":pick(first),"last_state":pick(last)}

def report(root,start,end):
    root=Path(root)
    path=root/"resources.jsonl"
    with path.open(encoding="utf-8",errors="replace") as f:
        samples=[]
        for line in f:
            if line.strip():
                try:samples.append(json.loads(line))
                except json.JSONDecodeError:pass
    if not samples:
        raise ValueError("no resource samples")
    offset=samples[0]["unix_ns"]-samples[0]["monotonic_ns"]
    summary=runpy.run_path("tools/check_strict_weaknet.py")["resource_summary"](
        samples,start+offset,end+offset)
    during=[r for r in samples if start<=r.get("monotonic_ns",0)<=end]
    if len(during)<270: summary["errors"].append("fewer than 270 resource snapshots in 300s")
    busy=[];softirq=[];steal=[];psic=[];psim=[];rss={};hwm={}
    pressure_events=[]
    cpu_quota=None
    for a,b in zip(during,during[1:]):
        sample=_proc_cpu(a,b)
        if sample:
            busy.append(sample["host_busy_percent"])
            softirq.append(sample["softirq_percent"])
            steal.append(sample["steal_percent"])
        for key,dest in (("cpu",psic),("memory",psim)):
            val=_pct_stat(((b.get("pressure") or {}).get(key) or ""), "avg10")
            if val is not None:dest.append(val)
        for label in ("client","server"):
            proc=(b.get("processes") or {}).get(label) or {}
            current,peak=_rss(proc)
            if current is not None:rss[label]=max(rss.get(label,0),current)
            if peak is not None:hwm[label]=max(hwm.get(label,0),peak)
            if cpu_quota is None:
                cg=(proc.get("cgroup") or {}).get("cpu.max","")
                z=cg.strip().split()
                if len(z)>=2 and z[0].isdigit() and z[1].isdigit() and int(z[1]):
                    cpu_quota=int(z[0])/int(z[1])
        pressure=((b.get("pressure") or {}).get("cpu") or "")
        value=_pct_stat(pressure,"avg10")
        if value is not None and value>=10 and len(pressure_events)<30:
            pressure_events.append({"elapsed_s":(b["monotonic_ns"]-start)/1e9,
                                    "cpu_psi_some_avg10":value})
    # Cgroup quota saturation and contemporaneous CPU PSI are stronger
    # evidence than poor throughput or a busy host snapshot alone.
    process_core_usage=sum(summary.get("process_cpu_seconds",{}).values())/300
    sustained_pressure=len(pressure_events)>=10 or (psic and max(psic)>=25)
    quota_pressure=cpu_quota is not None and cpu_quota>0 and process_core_usage>=.85*cpu_quota
    capacity_evidence=[]
    if sustained_pressure and quota_pressure:
        capacity_evidence.append({
            "kind":"measured_cpu_quota_psi_pressure",
            "first_elapsed_s":pressure_events[0]["elapsed_s"] if pressure_events else None,
            "cgroup_cpu_quota_cores":cpu_quota,
            "process_cores_avg":process_core_usage,
            "max_cpu_psi_some_avg10":max(psic) if psic else None})
    if summary.get("socket_drop_max",0):
        capacity_evidence.append({"kind":"local_socket_drop_not_independently_capacity",
                                  "drops":summary["socket_drop_max"]})
    link_drop=summary.get("link_drop_delta",{})
    if sum(max(0,v.get(k,0)) for v in link_drop.values() for k in ("rx","tx")):
        capacity_evidence.append({"kind":"local_interface_drop_not_independently_capacity",
                                  "drops_by_ns":link_drop})
    return {"strict_resource":summary,
            "host_busy_max_percent":max(busy) if busy else None,
            "host_steal_max_percent":max(steal) if steal else None,
            "host_softirq_max_percent":max(softirq) if softirq else None,
            "cpu_psi_some_avg10_max":max(psic) if psic else None,
            "memory_psi_some_avg10_max":max(psim) if psim else None,
            "cgroup_cpu_quota_cores":cpu_quota,
            "process_cpu_cores_average":process_core_usage,
            "process_rss_max_mib":rss,"process_hwm_max_mib":hwm,
            "capacity_evidence":capacity_evidence,
            "capacity_limited_evidenced":bool(sustained_pressure and quota_pressure),
            "diagnostics":{s:_diag(root,s) for s in ("client","server")}}
