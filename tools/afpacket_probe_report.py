#!/usr/bin/env python3
"""Fail-closed numerical reducer for diagnostic-ON AF_PACKET 100ms probes.

Does not replace the formal application analyzer. No packet content or real
addresses are read. A malformed / missing socket receipt is NEVER zero drops.
"""
import argparse
import hashlib
import json
from pathlib import Path

MAX_ROWS = 4096
EXPECTED_GAP_NS = 250_000_000
MIN_GOOD_300S = 2700


def summarize(rows, side, start_ns, end_ns):
    if side not in ("client", "server") or start_ns <= 0 or end_ns <= start_ns:
        raise ValueError("invalid side or business monotonic bounds")
    if len(rows) > MAX_ROWS:
        raise ValueError("numerical socket probe exceeded bounded row cap")
    out = {
        "side": side, "samples": len(rows), "valid_samples": 0,
        "invalid_samples": 0, "first_drops": None, "last_drops": None,
        "observed_positive_drop_delta": None, "sampled_rmem_peak_bytes": None,
        "sampled_rb_values_bytes": [], "max_valid_bracket_gap_s": None,
        "gaps_over_250ms": 0, "drop_intervals": [],
        "quality": "UNUSABLE", "business_window_covered": False,
        "original_business_gate": "NOT_EVALUATED",
    }
    prev_start = None
    prev_valid = None
    first_valid = last_valid = None
    rb = set()
    peak = 0
    positive = 0
    largest_gap = 0
    for row in rows:
        if row.get("schema") != 1 or row.get("side") != side:
            raise ValueError("unexpected trace schema or side")
        a = row.get("ss_start_monotonic_ns")
        z = row.get("ss_end_monotonic_ns")
        if type(a) is not int or type(z) is not int or a <= 0 or z < a:
            raise ValueError("invalid socket observation monotonic bracket")
        if prev_start is not None and a <= prev_start:
            raise ValueError("non-monotonic or duplicate socket probe observation")
        prev_start = a
        sock = row.get("packet_socket")
        if not row.get("ss_ok") or not isinstance(sock, dict):
            out["invalid_samples"] += 1
            continue
        if not all(type(sock.get(k)) is int for k in ("r", "rb", "d")):
            raise ValueError("valid socket row missing numerical r/rb/d")
        if sock["r"] < 0 or sock["rb"] <= 0 or sock["d"] < 0:
            raise ValueError("invalid packet-socket resource counters")
        out["valid_samples"] += 1
        rb.add(sock["rb"])
        peak = max(peak, sock["r"])
        if first_valid is None:
            first_valid = (a, z, sock)
        if prev_valid is not None:
            p_a, p_z, p_sock = prev_valid
            if sock["d"] < p_sock["d"]:
                raise ValueError("packet-socket drop counter reset / identity ambiguous")
            gap_ns = a - p_a
            largest_gap = max(largest_gap, gap_ns)
            if gap_ns > EXPECTED_GAP_NS:
                out["gaps_over_250ms"] += 1
            delta = sock["d"] - p_sock["d"]
            if delta:
                positive += delta
                out["drop_intervals"].append({
                    "drop_increment": delta,
                    "counter_from": p_sock["d"], "counter_to": sock["d"],
                    # Previous ss counters were sampled somewhere inside the
                    # invocation, not necessarily at its end; conservative.
                    "earliest_possible_s": round((p_a-start_ns)/1e9, 9),
                    "latest_possible_s": round((z-start_ns)/1e9, 9),
                    "previous_ss_duration_ms": round((p_z-p_a)/1e6, 3),
                    "current_ss_duration_ms": round((z-a)/1e6, 3),
                    "missed_sample_bracket": gap_ns > EXPECTED_GAP_NS,
                    "rmem_observed_before_after": [p_sock["r"], sock["r"]],
                    "rb_observed_before_after": [p_sock["rb"], sock["rb"]],
                    "cpu_psi_avg10_before_after": [
                        prev_valid_psi, row.get("cpu_psi_some_avg10_percent")],
                })
        prev_valid_psi = row.get("cpu_psi_some_avg10_percent")
        prev_valid = (a, z, sock)
        last_valid = prev_valid
    if first_valid is not None:
        out["first_drops"] = first_valid[2]["d"]
        out["last_drops"] = last_valid[2]["d"]
        out["observed_positive_drop_delta"] = positive
        out["sampled_rmem_peak_bytes"] = peak
        out["sampled_rb_values_bytes"] = sorted(rb)
        out["max_valid_bracket_gap_s"] = round(largest_gap / 1e9, 9)
        out["business_window_covered"] = first_valid[0] <= start_ns and last_valid[1] >= end_ns
        if (out["business_window_covered"] and
                out["valid_samples"] >= MIN_GOOD_300S and
                out["invalid_samples"] == 0 and out["gaps_over_250ms"] == 0):
            out["quality"] = "CONTINUOUS_NUMERIC_ONLY"
        else:
            out["quality"] = "PARTIAL_NUMERIC_ONLY"
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--client", type=Path, required=True)
    ap.add_argument("--server", type=Path, required=True)
    ap.add_argument("--stage-events", type=Path, required=True)
    ap.add_argument("--output", type=Path, required=True)
    args = ap.parse_args()
    stages = [json.loads(row) for row in args.stage_events.read_text().splitlines() if row.strip()]
    starts = [x for x in stages if x.get("event") == "business_start"]
    ends = [x for x in stages if x.get("event") == "business_end"]
    if len(starts) != 1 or len(ends) != 1:
        raise ValueError("one complete business monotonic window required")
    start_ns, end_ns = int(starts[0]["monotonic_ns"]), int(ends[0]["monotonic_ns"])
    reports = {}
    hashes = {}
    for side in ("client", "server"):
        raw = getattr(args, side).read_bytes()
        rows = [json.loads(line) for line in raw.splitlines() if line.strip()]
        reports[side] = summarize(rows, side, start_ns, end_ns)
        hashes[side+"-packet-probe.jsonl"] = hashlib.sha256(raw).hexdigest()
    out = {
        "schema": "wbd-packet-socket-numerical-probe-reducer/v1",
        "source_sha256": hashes, "business_start_monotonic_ns": start_ns,
        "business_end_monotonic_ns": end_ns, "diagnostic_only": True,
        "original_business_gate": "UNCHANGED_NOT_EVALUATED",
        "capacity_limited_evidenced": False, "causal_root": "NOT_ESTABLISHED",
        "sides": reports,
        "limitations": [
            "A missing ss row is unknown, never a zero socket-drop sample.",
            "Drop time is bracketed between separate ss invocations, not kernel timestamped.",
            "CPU PSI is 10s-averaged, not a cause attribution.",
            "No trace from a different host can close the previous profile-OFF failure.",
            "Only profile-ON observer data; no CPU cost or speedup comparison.",
        ],
    }
    args.output.write_text(json.dumps(out, indent=2, sort_keys=True) + "\n")
    print("NUMERIC_AF_PACKET_REDUCER", {side: reports[side]["quality"] for side in reports})


if __name__ == "__main__":
    main()
