#!/usr/bin/env python3
"""Conservative same-run E4 AF_PACKET drop/read-activity/cgroup time-witness.

Crosses clock domains ONLY with paired 1s resource samples (wall+monotonic);
the product ON diagnostics have wall Unix times, not kernel event timestamps.
Correlation is observational. No source changes, packets, addresses, or claims
that a Go goroutine was runnable or blocked at the kernel event.
"""
import argparse
import hashlib
import json
from pathlib import Path
from afpacket_probe_report import summarize

MAX_DIAG_ROWS = 512
MAX_RESOURCE_ROWS = 512
MAX_DROP_INTERVALS = 64
MAX_CLOCK_DRIFT_NS = 20_000_000


def clock_offset(resources):
    if not 2 <= len(resources) <= MAX_RESOURCE_ROWS:
        raise ValueError("bounded paired resources required")
    offsets = []
    last_mono = 0
    for row in resources:
        mono, wall = row.get("monotonic_ns"), row.get("unix_ns")
        if type(mono) is not int or type(wall) is not int or mono <= last_mono or wall <= 0:
            raise ValueError("invalid or unordered resource clock pair")
        last_mono = mono
        offsets.append(mono-wall)
    if max(offsets)-min(offsets) > MAX_CLOCK_DRIFT_NS:
        raise ValueError("resource wall/monotonic drift >20ms; cannot align clocks")
    return sorted(offsets)[len(offsets)//2], max(offsets)-min(offsets)


def diagnostic_buckets(rows, side, offset):
    if len(rows) > MAX_DIAG_ROWS:
        raise ValueError("unbounded diagnostic snapshots")
    ordered = []
    prev = None
    for row in rows:
        wall = row.get("observed_unix_ns", row.get("unix_ns"))
        if type(wall) is not int or wall <= 0:
            raise ValueError("diagnostic lacks wall timestamp")
        at = wall + offset
        if prev is not None and at <= prev["at"]:
            raise ValueError("diagnostic clock not increasing")
        product = row.get("product") or {}
        io = product.get("raw_io") or {}
        calls = io.get("receive_calls")
        messages = io.get("receive_messages")
        if type(calls) is not int or type(messages) is not int:
            raise ValueError("diagnostic lacks real raw receive counters")
        read_gap = None
        if side == "server":
            tunnel = product.get("tunnel") or {}
            if isinstance(tunnel, dict):
                pipeline = tunnel.get("server_pipeline") or {}
                if pipeline.get("enabled") is True:
                    read_gap = pipeline.get("read_gap")
        if read_gap is not None:
            if (not isinstance(read_gap, dict) or
                    any(type(read_gap.get(k)) is not int for k in ("over_10ms", "max_ns"))):
                raise ValueError("invalid server read-gap counters")
        current = {"at":at,"calls":calls,"messages":messages,"gap":read_gap}
        if prev is not None:
            if calls < prev["calls"] or messages < prev["messages"]:
                raise ValueError("raw receive counter reset / process identity ambiguous")
            gap_events = max_rise = None
            if read_gap is not None and prev["gap"] is not None:
                if (read_gap["over_10ms"] < prev["gap"]["over_10ms"] or
                        read_gap["max_ns"] < prev["gap"]["max_ns"]):
                    raise ValueError("read-gap counters reset")
                gap_events = read_gap["over_10ms"] - prev["gap"]["over_10ms"]
                max_rise = read_gap["max_ns"] > prev["gap"]["max_ns"]
            ordered.append({
                "start_ns":prev["at"],"end_ns":at,
                "receive_calls_delta":calls-prev["calls"],
                "receive_messages_delta":messages-prev["messages"],
                "read_gap_over10ms_count_delta":gap_events,
                "read_gap_new_record_max":max_rise,
            })
        prev = current
    return ordered


def cgroup_throttling(resources, side):
    out = []
    for a, z in zip(resources, resources[1:]):
        group1 = (((a.get("processes") or {}).get(side) or {}).get("cgroup") or {})
        group2 = (((z.get("processes") or {}).get(side) or {}).get("cgroup") or {})
        text1, text2 = group1.get("cpu.stat"), group2.get("cpu.stat")
        delta = None
        if group1.get("path") == group2.get("path") and isinstance(text1, str) and isinstance(text2, str):
            def parse(text):
                fields = {}
                for line in text.splitlines():
                    parts = line.split()
                    if len(parts) == 2 and parts[1].isdigit():
                        fields[parts[0]] = int(parts[1])
                return fields
            d1,d2=parse(text1),parse(text2)
            if "nr_throttled" in d1 and "nr_throttled" in d2:
                if d2["nr_throttled"] < d1["nr_throttled"]:
                    raise ValueError("cgroup throttle counter reset / membership change")
                delta = d2["nr_throttled"]-d1["nr_throttled"]
        out.append({"start_ns":int(a["monotonic_ns"]),"end_ns":int(z["monotonic_ns"]),
                    "nr_throttled_delta":delta})
    return out


def overlaps(w, a, z):
    return w["start_ns"] < z and w["end_ns"] > a


def correlate(probes, diag, resources, stages):
    starts=[e for e in stages if e.get("event")=="business_start"]
    ends=[e for e in stages if e.get("event")=="business_end"]
    if len(starts)!=1 or len(ends)!=1:
        raise ValueError("exact business start and end required")
    start,end=int(starts[0]["monotonic_ns"]),int(ends[0]["monotonic_ns"])
    if start <= 0 or end <= start:
        raise ValueError("invalid monotonic business window")
    offset, drift=clock_offset(resources)
    sides={}
    for side in ("client","server"):
        base=summarize(probes[side],side,start,end)
        buckets=diagnostic_buckets(diag[side],side,offset)
        throttle=cgroup_throttling(resources,side)
        witness=[]
        intervals=base["drop_intervals"]
        if len(intervals)>MAX_DROP_INTERVALS:
            raise ValueError("too many socket drop intervals for bounded witness")
        for interval in intervals:
            a=start+round(interval["earliest_possible_s"]*1e9)
            z=start+round(interval["latest_possible_s"]*1e9)
            overlapping=[w for w in buckets if overlaps(w,a,z)]
            cpu=[w for w in throttle if overlaps(w,a,z)]
            calls=sum(w["receive_calls_delta"] for w in overlapping) if overlapping else None
            messages=sum(w["receive_messages_delta"] for w in overlapping) if overlapping else None
            max_rises=[w["read_gap_new_record_max"] for w in overlapping
                       if w["read_gap_new_record_max"] is not None]
            gap_events=[w["read_gap_over10ms_count_delta"] for w in overlapping
                       if w["read_gap_over10ms_count_delta"] is not None]
            throttle_values=[w["nr_throttled_delta"] for w in cpu
                            if w["nr_throttled_delta"] is not None]
            witness.append({
                "drop_increment":interval["drop_increment"],
                "business_elapsed_bound_s":[interval["earliest_possible_s"],interval["latest_possible_s"]],
                "kernel_time_exact":False,
                "ss_gap_over_250ms":interval["missed_sample_bracket"],
                "read_buckets_1s_overlapping":len(overlapping),
                "raw_receive_calls_in_overlapping_buckets":calls,
                "raw_receive_messages_in_overlapping_buckets":messages,
                "server_read_gap_over10ms_bucket_delta":sum(gap_events) if gap_events else None,
                "server_read_gap_record_max_rose":any(max_rises) if max_rises else None,
                "resource_cgroup_1s_buckets_overlapping":len(cpu),
                "cgroup_throttle_delta_in_overlapping_buckets":sum(throttle_values) if throttle_values else None,
                "verdict":"TEMPORAL_COINCIDENCE_ONLY" if overlapping and cpu else "INSUFFICIENT_TIME_CONTEXT",
            })
        sides[side]={
            "probe_quality":base["quality"],
            "numerical_positive_drop_lower_bound":base["observed_positive_drop_delta"],
            "invalid_100ms_probe_rows":base["invalid_samples"],
            "raw_receive_snapshot_buckets":len(buckets),
            "cgroup_1s_buckets":len(throttle),
            "socket_drop_witnesses":witness,
            "interpretation":("NO_VALID_SOCKET_TRACE" if base["quality"]=="UNUSABLE" else
                              "NO_REPRODUCED_SOCKET_DROP_NOT_ROOT_CLOSED" if not witness else
                              "DROP_OBSERVED_CAUSAL_ROOT_NOT_ESTABLISHED"),
        }
    return {
        "schema":"wbd-e4-same-run-drop-read-gap-correlator/v1",
        "clock_calibration":{"source":"resources.jsonl paired unix_ns/monotonic_ns",
                            "max_offset_drift_ns":drift,"wall_to_monotonic_offset_ns":offset},
        "one_business_start_monotonic_ns":start,"one_business_end_monotonic_ns":end,
        "original_analyzer":"UNTOUCHED", "diagnostic_only":True,
        "product_source":"UNTOUCHED","cpu_gain":"NOT_EVALUATED","causal_root":"NOT_ESTABLISHED",
        "limits":[
            "raw_io receive_calls are cumulative and snapshots are ~1s: positives cannot exclude a 100ms recv starvation",
            "server read_gap is inter-successful-read high watermark and over10ms count, not recvmmsg syscall stall timestamps",
            "cgroup CPU throttling may be shared across processes/host; 1s overlap does not prove socket-drop cause",
            "wall-to-monotonic alignment via paired resources is approximate; raw ss kernel drop time not timestamped",
            "absent and malformed numeric probe data are UNUSABLE, not zero packet loss",
            "profile ON overhead and different CPUs forbid formal CPU efficiency comparison",
        ],
        "sides":sides,
    }


def main():
    p=argparse.ArgumentParser(description=__doc__)
    for side in ("client","server"):
        p.add_argument("--"+side+"-packet-probe",type=Path,required=True)
        p.add_argument("--"+side+"-diag",type=Path,required=True)
    p.add_argument("--resources",type=Path,required=True)
    p.add_argument("--stage-events",type=Path,required=True)
    p.add_argument("--output",type=Path,required=True)
    args=p.parse_args()
    hashes={}
    def load(name,path):
        raw=path.read_bytes()
        hashes[name]=hashlib.sha256(raw).hexdigest()
        return [json.loads(x) for x in raw.splitlines() if x.strip()]
    probes={side:load(side+"-packet-probe.jsonl",getattr(args,side+"_packet_probe"))
            for side in ("client","server")}
    diag={side:load(side+"-diag.jsonl",getattr(args,side+"_diag"))
          for side in ("client","server")}
    resources=load("resources.jsonl",args.resources)
    stages=load("stage-events.jsonl",args.stage_events)
    result=correlate(probes,diag,resources,stages)
    result["source_sha256"]=hashes
    args.output.write_text(json.dumps(result,indent=2,sort_keys=True)+"\n",encoding="utf-8")
    print("AF_PACKET_SAME_RUN_OBSERVATIONAL_WITNESS",
          {side:d["interpretation"] for side,d in result["sides"].items()})

if __name__=="__main__": main()
