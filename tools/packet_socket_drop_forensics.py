#!/usr/bin/env python3
"""Read-only attribution of AF_PACKET skmem drop increments to business monotonic time.

This is NOT a new measurement or evidence of why a kernel packet socket dropped.
It consumes numerical Actions receipts; never reads private packet payloads.
"""
import argparse
import hashlib
import json
import re
from pathlib import Path

# iproute2 ss -0 prints skmem either inline (actual Actions) or indented next line.
RAW_RE = re.compile(r"(?m)^p_raw\b[^\n]*?(?:[ \t]+|\n[ \t]*)skmem:\(([^)]*)\)")
KV_RE = re.compile(r"(?<![A-Za-z])(rb|tb|bl|r|d|o|f|w|t)(\d+)(?!\w)")
PSI_RE = re.compile(r"(?m)^some\s+[^\n]*\bavg10=([0-9.]+)")


def packet_socket(row, side):
    """Return exact single packet receive socket, or None if absent.

    Never aggregate multiple AF_PACKET sockets: ss totals cannot then be
    attributed to the product. A socket changing identity mid-run is unknown.
    """
    entry = ((row.get("namespaces") or {}).get(side) or {}).get("ss_packet") or {}
    if entry.get("returncode") != 0:
        return None
    matches = RAW_RE.findall(entry.get("stdout") or "")
    if not matches:
        return None
    if len(matches) != 1:
        raise ValueError("%s: multiple packet sockets; attribution ambiguous" % side)
    fields = {k: int(v) for k, v in KV_RE.findall(matches[0])}
    if not {"r", "rb", "d"}.issubset(fields):
        raise ValueError("%s: skmem lacks r/rb/d fields" % side)
    return {k: fields[k] for k in ("r", "rb", "d")}


def psi_avg10(row):
    text = ((row.get("pressure") or {}).get("cpu") or "")
    m = PSI_RE.search(text)
    return float(m.group(1)) if m else None


def cpu_ticks(row, side):
    stat = (((row.get("processes") or {}).get(side) or {}).get("stat") or {})
    if "utime_ticks" not in stat or "stime_ticks" not in stat:
        return None
    return int(stat["utime_ticks"]) + int(stat["stime_ticks"])


def analyze(resources, stages):
    events = [e for e in stages if e.get("event") == "business_start"]
    if len(events) != 1 or "monotonic_ns" not in events[0]:
        raise ValueError("exactly one monotonic business_start is required")
    start = int(events[0]["monotonic_ns"])
    ends = [e for e in stages if e.get("event") == "business_end"]
    if len(ends) != 1 or int(ends[0]["monotonic_ns"]) <= start:
        raise ValueError("exactly one later monotonic business_end is required")
    end = int(ends[0]["monotonic_ns"])
    if not resources:
        raise ValueError("resource samples required")
    samples = sorted(resources, key=lambda x: int(x["monotonic_ns"]))
    if any(int(b["monotonic_ns"]) == int(a["monotonic_ns"]) for a, b in zip(samples, samples[1:])):
        raise ValueError("duplicate resource monotonic timestamp")
    result = {
        "schema": "wbd-packet-socket-business-clock-forensics/v1",
        "business_start_monotonic_ns": start,
        "business_end_monotonic_ns": end,
        "resource_first_monotonic_ns": int(samples[0]["monotonic_ns"]),
        "resource_first_to_business_start_s": round((start - int(samples[0]["monotonic_ns"])) / 1e9, 9),
        "resource_samples": len(samples),
        "sides": {},
        "original_classifier_unchanged": True,
        "limitations": [
            "Drops are located only within two neighboring sampling timestamps, not timed at the kernel event.",
            "A zero rmem reading on both sides of an interval cannot exclude a brief full ring/queue in between.",
            "CPU PSI avg10 and process CPU ticks bracket a time interval; neither identifies the cause of socket drops.",
            "Kernel packet socket drops are a separate resource gate even if multi-lane business recovery is complete.",
            "No inferred capacity-limited, host fault, product bug, or CPU gain from this read-only report.",
        ],
    }
    for side in ("client", "server"):
        parsed = [(row, packet_socket(row, side)) for row in samples]
        missing = sum(p is None for _, p in parsed)
        valid = [(row, p) for row, p in parsed if p is not None]
        deltas = []
        for (a, pa), (b, pb) in zip(parsed, parsed[1:]):
            if pa is None or pb is None:
                continue
            delta = pb["d"] - pa["d"]
            if delta < 0:
                raise ValueError("%s: drop counter reset; socket identity changed" % side)
            if delta == 0:
                continue
            l, h = int(a["monotonic_ns"]), int(b["monotonic_ns"])
            entry = {
                "drop_increment": delta,
                "counter_from": pa["d"],
                "counter_to": pb["d"],
                "monotonic_interval_ns": [l, h],
                "business_elapsed_interval_s": [round((l-start)/1e9, 9), round((h-start)/1e9, 9)],
                "business_window_overlap": l < end and h > start,
                "rmem_before_after_bytes": [pa["r"], pb["r"]],
                "rb_effective_before_after_bytes": [pa["rb"], pb["rb"]],
                "cpu_psi_some_avg10_before_after_percent": [psi_avg10(a), psi_avg10(b)],
                "process_cpu_ticks_delta": (cpu_ticks(b, side) - cpu_ticks(a, side)
                                            if cpu_ticks(a, side) is not None and cpu_ticks(b, side) is not None else None),
            }
            deltas.append(entry)
        result["sides"][side] = {
            "observed_packet_socket_samples": len(valid),
            "missing_packet_socket_samples": missing,
            "first_observed_drops": valid[0][1]["d"] if valid else None,
            "last_observed_drops": valid[-1][1]["d"] if valid else None,
            "all_adjacent_drop_increments": sum(d["drop_increment"] for d in deltas),
            "sampled_rmem_peak_bytes": max((p["r"] for _, p in valid), default=None),
            "sampled_rb_effective_bytes": sorted({p["rb"] for _, p in valid}),
            "drop_intervals": deltas,
            "time_axis_complete": missing == 0 and len(valid) == len(samples),
        }
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--resources", required=True, type=Path)
    p.add_argument("--stage-events", required=True, type=Path)
    p.add_argument("--output", required=True, type=Path)
    a = p.parse_args()
    raw_resources = a.resources.read_bytes()
    raw_stages = a.stage_events.read_bytes()
    resources = [json.loads(x) for x in raw_resources.splitlines() if x.strip()]
    stages = [json.loads(x) for x in raw_stages.splitlines() if x.strip()]
    result = analyze(resources, stages)
    result["source_sha256"] = {
        "resources.jsonl": hashlib.sha256(raw_resources).hexdigest(),
        "stage-events.jsonl": hashlib.sha256(raw_stages).hexdigest(),
    }
    a.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("AF_PACKET_FORENSIC_ONLY original_gate_unchanged=1 drop_delta=%s" %
          {side: r["all_adjacent_drop_increments"] for side, r in result["sides"].items()})


if __name__ == "__main__":
    main()
