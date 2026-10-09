#!/usr/bin/env python3
"""Bounded Linux schedstat observer, numeric-only; not a Go goroutine trace."""
import os
import time
from pathlib import Path

MAX_THREADS = 64

def parse_starttime(text):
    i=text.rfind(")")
    if i<0: raise ValueError("proc stat missing comm")
    tail=text[i+1:].split()
    if len(tail)<=19 or not tail[19].isdigit():
        raise ValueError("proc stat missing starttime")
    return int(tail[19])

def parse_schedstat(text):
    parts=text.split()
    if len(parts)!=3 or any(not p.isdigit() for p in parts):
        raise ValueError("invalid schedstat")
    return tuple(int(p) for p in parts)

def snapshot(pid,side):
    if type(pid) is not int or pid<=0 or side not in ("client","server"):
        raise ValueError("invalid product process id or side")
    root=Path("/proc")/str(pid)
    try:
        if os.path.basename(os.readlink(root/"exe")) != "wbd-"+side:
            return {"status":"WRONG_EXECUTABLE","at_ns":time.monotonic_ns(),"threads":{}}
        birth=parse_starttime((root/"stat").read_text())
        tids=sorted(p.name for p in (root/"task").iterdir() if p.name.isdigit())
        if len(tids)>MAX_THREADS:
            return {"status":"OVER_THREAD_CAP","at_ns":time.monotonic_ns(),"threads":{}}
        values={}
        for tid in tids:
            root_tid=root/"task"/tid
            start=parse_starttime((root_tid/"stat").read_text())
            vals=parse_schedstat((root_tid/"schedstat").read_text())
            values[(int(tid),start)]=vals
        return {"status":"OK" if values else "NO_THREADS",
                "birth":birth,"at_ns":time.monotonic_ns(),"threads":values}
    except (OSError,ValueError):
        return {"status":"UNAVAILABLE","at_ns":time.monotonic_ns(),"threads":{}}

def aggregate(previous,current):
    """Only matching (tid, birth ticks) deltas; TIDs never written to JSONL."""
    out={"status":"UNAVAILABLE","observed_threads":len(current.get("threads",{})),
         "matched_threads":0,"new_threads":None,"exited_threads":None,
         "runtime_ns_delta":None,"runqueue_wait_ns_delta":None,
         "slices_delta":None,"from_monotonic_ns":None,
         "to_monotonic_ns":current.get("at_ns")}
    if current.get("status")!="OK":
        out["status"]=current.get("status","UNAVAILABLE")
        return out
    if previous is None:
        out["status"]="BASELINE_ONLY"
        return out
    if previous.get("status")!="OK" or previous.get("birth")!=current.get("birth"):
        out["status"]="IDENTITY_UNKNOWN"
        return out
    old,new=previous["threads"],current["threads"]
    shared=set(old)&set(new)
    out["matched_threads"]=len(shared)
    out["new_threads"]=len(set(new)-shared)
    out["exited_threads"]=len(set(old)-shared)
    out["from_monotonic_ns"]=previous["at_ns"]
    if not shared:
        out["status"]="NO_SURVIVING_THREADS"
        return out
    if any(new[k][i]<old[k][i] for k in shared for i in range(3)):
        out["status"]="COUNTER_RESET"
        return out
    d=[sum(new[k][i]-old[k][i] for k in shared) for i in range(3)]
    out["runtime_ns_delta"],out["runqueue_wait_ns_delta"],out["slices_delta"]=d
    out["status"]="MATCHED_COMPLETE" if not out["new_threads"] and not out["exited_threads"] else "MATCHED_PARTIAL"
    return out
