#!/usr/bin/env python3
"""Read-only diagnostic audit of one existing GitHub Actions artifact.

No netem truth flows into a product: this script only consumes saved
diagnostic/summary JSON and publishes bounded summaries, never raw payloads.
"""
from __future__ import annotations

import argparse
from collections import Counter
import hashlib
import json
from pathlib import Path
import zipfile

VERSION = "n2-evidence-audit-v1"


def load(archive: zipfile.ZipFile, name: str):
    return json.loads(archive.read(name))


def lines(archive: zipfile.ZipFile, name: str):
    return [json.loads(s) for s in archive.read(name).splitlines() if s.strip()]


def lane_at(row: dict, direction: str):
    p = row["product"]
    if direction == "s2c":
        p = p.get("tunnel", {})
    lanes = p.get("lanes") or []
    if not lanes:
        return None
    return lanes[0]


def summarize_direction(rows: list[dict], start_ns: int, direction: str,
                        clean_start_s: float, clean_end_s: float) -> dict:
    observations = []
    distinct = {}
    for row in rows:
        lane = lane_at(row, direction)
        if lane is None:
            continue
        c = lane.get("control") or {}
        e = c.get("n2_source_evidence") or {}
        t = (row["unix_ns"] - start_ns) / 1e9
        item = {
            "t_s": round(t, 3),
            "actual_parity": lane.get("actual_tx_parity"),
            "source_seq": c.get("n2_source_report_seq") or 0,
            "last_qualified_seq": c.get("n2_lower_assessed_source_seq") or 0,
            "reason": c.get("n2_decision_reason"),
            "gate": c.get("n2_lower_gate"),
            "confidence_upper": c.get("n2_confidence_upper"),
            "history_windows": c.get("n2_confidence_history_windows"),
            "source_echo_age_ms": c.get("mature_echo_last_age_millis"),
            "lower_candidate": c.get("n2_lower_candidate"),
            "lower_residual": c.get("n2_lower_confidence_residual"),
            "evidence": {
                "source_seq": e.get("source_report_seq"),
                "M": e.get("lower_first_lost"),
                "M_plus_R": e.get("upper_first_lost"),
                "repair_distinct": e.get("distinct_repaired_pn"),
                "tx": e.get("window_tx_success"),
                "whole_blocks": e.get("shape_full_blocks"),
                "partial_blocks": e.get("shape_partial_blocks"),
                "cropped_source": e.get("shape_cropped_source"),
                "cropped_parity": e.get("shape_cropped_parity"),
                "invalid": e.get("shape_invalid"),
            },
        }
        observations.append(item)
        seq = item["evidence"]["source_seq"]
        if seq and seq not in distinct:
            distinct[seq] = item
    clean = [r for r in observations if clean_start_s <= r["t_s"] < clean_end_s]
    if not clean:
        raise ValueError("missing active clean diag; do not count drain")
    buckets = []
    for begin in range(int(clean_start_s), int(clean_end_s), 60):
        end = min(begin + 60, clean_end_s)
        subset = [r for r in clean if begin <= r["t_s"] < end]
        if not subset:
            continue
        a, b = subset[0], subset[-1]
        delta_reports = b["source_seq"] - a["source_seq"]
        delta_qualified_seq = b["last_qualified_seq"] - a["last_qualified_seq"]
        buckets.append({
            "interval_s": [begin, end],
            "report_seq_delta": delta_reports,
            "qualified_source_seq_delta": delta_qualified_seq,
            "qualified_to_report_seq_delta_ratio": (
                round(delta_qualified_seq / delta_reports, 4)
                if delta_reports > 0 else None),
            "confidence_upper_first_last": [a["confidence_upper"], b["confidence_upper"]],
            "profile_first_last": [a["actual_parity"], b["actual_parity"]],
            "history_first_last": [a["history_windows"], b["history_windows"]],
        })
    spikes = []
    for r in distinct.values():
        e = r["evidence"]
        if (clean_start_s <= r["t_s"] < clean_end_s and
                isinstance(e["M_plus_R"], int) and
                isinstance(e["M"], int) and e["M_plus_R"] > e["M"]):
            spikes.append({"t_s": r["t_s"], "seq": e["source_seq"],
                           "M": e["M"], "M_plus_R": e["M_plus_R"],
                           "repair_distinct": e["repair_distinct"],
                           "whole_blocks": e["whole_blocks"],
                           "cropped_source": e["cropped_source"],
                           "cropped_parity": e["cropped_parity"]})
    return {
        "clean_active_window_s": [clean_start_s, clean_end_s],
        "clean_active_first_last": [clean[0], clean[-1]],
        "minute_buckets": buckets,
        "clean_reason_counts": dict(Counter(r["reason"] for r in clean)),
        "clean_gate_counts": dict(Counter(r["gate"] for r in clean)),
        "first_attempt_upper_only_spikes": spikes,
        "distinct_evidence_reports_seen": len(distinct),
        "limits": [
            "per-second snapshots are not all emitted reports; seq delta is coverage proxy",
            "M+R upper bound includes same-wire repair attempts, not proven line loss",
            "256 PN group cropping is observational, not proof actual groups were unprotected",
            "wall-clock sample ages inside history cannot be reconstructed from snapshots",
        ],
    }


def audit(archive: zipfile.ZipFile) -> dict:
    stages = lines(archive, "stage-events.jsonl")
    first = next(e for e in stages if e.get("event") == "phase_start" and
                 e.get("offset_seconds") == 0)
    summary = load(archive, "summary.json")
    manifest = load(archive, "manifest.json")
    prep = load(archive, "harness-prep.json")
    clean_phase = next(s for s in prep["waveform"] if s["name"] == "recover0")
    clean_start, clean_end = clean_phase["start_s"], clean_phase["end_s"]
    if summary["source_sha"] != manifest["source_sha"]:
        raise ValueError("artifact source mismatch")
    if clean_start != 300 or clean_end != 540:
        raise ValueError("this audit is pinned to pre-existing recover540 fixture")
    return {
        "schema": 1, "audit_version": VERSION,
        "source_sha": summary["source_sha"],
        "original_run_id": 38161148715, "original_job_id": 114534148614,
        "original_artifact_id": 11710956407,
        "original_artifact_digest_sha256": "090a5bd2d5e4fda32309a5f37dc616c836c526c71b3adf7eddc3e87d7900ad3f",
        "summary_sha256": hashlib.sha256(archive.read("summary.json")).hexdigest(),
        "scoped_classification_unchanged": summary["classification"],
        "not_full_n2_pass": summary["not_full_n2_pass"],
        "capacity_flags": summary["capacity_flags"],
        "netem_rng": prep["netem_rng"],
        "configuration": manifest["config"],
        "directions": {
            "c2s": summarize_direction(lines(archive, "client-diag.jsonl"),
                                       first["time_wall_ns"], "c2s", clean_start, clean_end),
            "s2c": summarize_direction(lines(archive, "server-diag.jsonl"),
                                       first["time_wall_ns"], "s2c", clean_start, clean_end),
        },
        "limitations": [
            "read-only prior action; no new test, rerun or qualification",
            "diagnostics ON CPU not comparable with ordinary OFF",
            "actual first-attempt line loss and native FEC group closure outside PN crop unobserved",
            "probe p99 is returned-only; original missing unchanged",
            "historic tail and 80s downlink remain OPEN",
        ],
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--zip", type=Path, required=True)
    ap.add_argument("--output", type=Path, required=True)
    args = ap.parse_args()
    with zipfile.ZipFile(args.zip) as z:
        result = audit(z)
    args.output.write_text(json.dumps(result, sort_keys=True, indent=2) + "\n",
                           encoding="utf-8")


if __name__ == "__main__":
    main()
