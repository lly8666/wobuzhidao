#!/usr/bin/env python3
"""Exact frozen C10 GE clock trace and bounded eBPF map loader/readback.

Reference only; independent from product controller and closed FEC groups.
Use Python 3.12 stdlib, eBPF Linux syscall map-id IO. In an isolated
GitHub Actions runner, run load/read as root ONLY after attaching two
tc filters. Never map netem truth into the product.
"""
import argparse, ctypes, hashlib, json, os, platform, random, struct, subprocess
from pathlib import Path

MAX_EVENTS=2048
DURATION_MS=240_000
FMT="<QII"+str(MAX_EVENTS)+"IQQQ"
VALUE_SIZE=struct.calcsize(FMT)
VERSION="n2-ge-ctmc-event-trace-v1"
NAME="c10_ge_ctl"
LIBC=ctypes.CDLL(None,use_errno=True)
BPF_LOOKUP=1
BPF_UPDATE=2
BPF_FD_BY_ID=14

def checksum(b):
    return hashlib.sha256(b).hexdigest()

def make_trace(freeze):
    assert freeze["model_version"]==VERSION
    assert freeze["seed"]==2026101210 and freeze["total_business_ms"]==DURATION_MS
    assert freeze["good_mean_ms"]==450 and freeze["bad_mean_ms"]==50
    assert freeze["conditional_good_drop_percent"]==0 and freeze["conditional_bad_drop_percent"]==50
    r=random.Random(freeze["seed"])
    bad=int(r.random()<.1)
    initial_bad=bad
    last_ms=0
    t=0.0
    changes=[]
    while True:
        t+=r.expovariate(1./(50. if bad else 450.))
        ms=max(last_ms+1,round(t))
        if ms>=DURATION_MS:break
        changes.append(ms)
        last_ms=ms
        bad^=1
        if len(changes)>MAX_EVENTS:raise ValueError("GE transition map capacity exceeded; do NOT truncate")
    bad_ms=0
    last=0
    state=initial_bad
    for cut in changes+[DURATION_MS]:
        if state:bad_ms+=cut-last
        state^=1
        last=cut
    obj={"schema":1,"version":VERSION,"seed":freeze["seed"],
         "initial_bad":initial_bad,"active_ms":DURATION_MS,
         "boundaries_ms":changes,"good_mean_ms":450,"bad_mean_ms":50,
         "good_erase_p":0.0,"bad_erase_p":0.5}
    canonical=json.dumps(obj,sort_keys=True,separators=(",",":")).encode()
    if (len(changes)!=freeze["exact_accepted_transition_count"]
        or initial_bad!=freeze["starting_bad"]
        or bad_ms!=freeze["exact_bad_state_ms"]
        or checksum(canonical)!=freeze["canonical_trace_sha256"]):
        raise ValueError("frozen GE trace hash, count, dwell or initial-state mismatch")
    return obj

def smoke_trace():
    return {"schema":1,"version":"C10_EVENTMAP_CAPABILITY_ONLY_NOT_EXPONENTIAL",
            "seed":0,"initial_bad":0,"active_ms":DURATION_MS,
            "boundaries_ms":[100*i for i in range(1,65)],
            "good_mean_ms":None,"bad_mean_ms":None,"good_erase_p":0.0,"bad_erase_p":0.5}

def encode(trace,start_ns):
    transitions=trace["boundaries_ms"]
    if not (0<start_ns<2**64 and 0<=len(transitions)<=MAX_EVENTS and
            all(0<t<DURATION_MS for t in transitions) and
            all(a<b for a,b in zip(transitions,transitions[1:])) and
            trace["initial_bad"] in (0,1)):
        raise ValueError("invalid GE clock phase start/transition array")
    words=transitions+[0]*(MAX_EVENTS-len(transitions))
    data=struct.pack(FMT,start_ns,len(transitions),trace["initial_bad"],
                     *words,0,0,0)
    assert len(data)==VALUE_SIZE==8232
    return data

def decode(raw):
    vals=struct.unpack(FMT,raw)
    epoch,n,initial=vals[:3]
    if n>MAX_EVENTS:raise ValueError("corrupted GE map count")
    return {"start_ns":epoch,"transition_count":n,"initial_bad":initial,
            "boundaries_ms":list(vals[3:3+n]),
            "good_packets":vals[-3],"bad_packets":vals[-2],"bad_dropped":vals[-1]}

def bpf_syscall(command,attr):
    nr={"x86_64":321,"aarch64":280}.get(platform.machine())
    if nr is None:
        raise RuntimeError("Unsupported BPF syscall architecture, fail closed")
    LIBC.syscall.restype=ctypes.c_long
    result=LIBC.syscall(nr,command,ctypes.byref(attr),ctypes.sizeof(attr))
    if result<0:
        error=ctypes.get_errno()
        raise OSError(error,os.strerror(error))
    return result

def mapfd(map_id):
    attr=(ctypes.c_uint32*2)(map_id,0)
    return bpf_syscall(BPF_FD_BY_ID,attr)

def bpf_elem(fd,write,data=None):
    key=ctypes.c_uint32(0)
    val=ctypes.create_string_buffer(data if write else b"\0"*VALUE_SIZE,VALUE_SIZE)
    # bpf_attr BPF_MAP_LOOKUP_ELEM/UPDATE_ELEM: map_fd, pad, key, value, flags
    class Elem(ctypes.Structure):
        _fields_=[("map_fd",ctypes.c_uint32),("pad",ctypes.c_uint32),
                  ("key",ctypes.c_uint64),("value",ctypes.c_uint64),
                  ("flags",ctypes.c_uint64)]
    a=Elem(fd,0,ctypes.addressof(key),ctypes.addressof(val),0)
    bpf_syscall(BPF_UPDATE if write else BPF_LOOKUP,a)
    return val.raw

def maps():
    entries=json.loads(subprocess.check_output(["bpftool","-j","map","show"]))
    selected=[e for e in entries if e.get("name")==NAME and
              e.get("type")=="array" and e.get("value_size")==VALUE_SIZE and
              e.get("max_entries")==1]
    if len(selected)!=2:raise ValueError(f"Expected exactly 2 attached C10 GE maps, got {selected!r}")
    return sorted(int(e["id"]) for e in selected)

def run(mode,trace,start_ns,output):
    ids=maps()
    data=encode(trace,start_ns)
    observed=[]
    for identity in ids:
        fd=mapfd(identity)
        try:
            if mode=="load":
                bpf_elem(fd,True,data)
            got=bpf_elem(fd,False)
        finally:
            os.close(fd)
        decoded=decode(got)
        for key,want in (("start_ns",start_ns),
                         ("transition_count",len(trace["boundaries_ms"])),
                         ("initial_bad",trace["initial_bad"]),
                         ("boundaries_ms",trace["boundaries_ms"])):
            if decoded[key]!=want:
                raise ValueError(f"Map id {identity} readback mismatch {key}")
        observed.append({"map_id":identity,**decoded})
    result={"schema":1,"phase":"C10_GE_MAP_"+mode.upper(),
            "kernel_monotonic_start_ns":start_ns,
            "model_version":trace["version"],
            "trace_sha256":checksum(json.dumps(trace,sort_keys=True,separators=(",",":")).encode()),
            "maps":observed,
            "readback_2_of_2_identical_predeclared_time_and_state":True,
            "product_qualification":"NOT_QUALIFIED_FROM_MAP_ONLY"}
    output.write_text(json.dumps(result,sort_keys=True,indent=2)+"\n")
    if mode=="audit":
        live=[x for x in observed if x["good_packets"]>0 and x["bad_packets"]>0]
        if trace["version"]=="C10_EVENTMAP_CAPABILITY_ONLY_NOT_EXPONENTIAL":
            if not live:
                raise ValueError("smoke: neither map saw both time phases")
            for item in live:
                if item["bad_packets"]>=16 and not .25<=item["bad_dropped"]/item["bad_packets"]<=.75:
                    raise ValueError("smoke: BAD packet erasure outside finite-sample 25%-75%")
        else:
            if len(live)!=2:
                raise ValueError("GE real: both direction egress maps must see good AND bad packets")
            for item in live:
                ratio=item["bad_dropped"]/item["bad_packets"]
                if item["bad_packets"]<100 or not .40<=ratio<=.60:
                    raise ValueError(f"GE real: BAD state erasure outside 40%-60%: {ratio}")
    return result

def main():
    p=argparse.ArgumentParser()
    p.add_argument("command",choices=["freeze","unit","load","audit"])
    p.add_argument("--frozen",type=Path,default=Path("docs/evidence/adaptive-n2-c10-ctmc-event-trace-freeze-v1.json"))
    p.add_argument("--trace",type=Path)
    p.add_argument("--mode",choices=["real","smoke"],default="real")
    p.add_argument("--start-ns",type=int)
    p.add_argument("--output",type=Path)
    a=p.parse_args()
    freeze=json.loads(a.frozen.read_text())
    trace=smoke_trace() if a.mode=="smoke" else make_trace(freeze)
    if a.command=="unit":
        expected=make_trace(freeze)
        for src in (trace,expected):
            raw=encode(src,123456789123456)
            read=decode(raw)
            assert read["start_ns"]==123456789123456
            assert read["boundaries_ms"]==src["boundaries_ms"]
            assert read["transition_count"]==len(src["boundaries_ms"])
        print("WBD_C10_GE_FROZEN_TRACE_AND_8232B_MAP_PACKING_UNIT_PASS")
        return
    if a.command=="freeze":
        if not a.output:p.error("--output is required")
        a.output.write_text(json.dumps(trace,sort_keys=True,indent=2)+"\n")
        print("WBD_C10_GE_TRACE_FREEZE_SHA256",
              checksum(json.dumps(trace,sort_keys=True,separators=(",",":")).encode()))
        return
    if a.start_ns is None or a.output is None:
        p.error("load/audit require --start-ns and --output")
    if a.trace:
        actual=json.loads(a.trace.read_text())
        if actual!=trace:raise ValueError("predeclared exact event trace drift")
    run(a.command,trace,a.start_ns,a.output)

if __name__=="__main__":main()
