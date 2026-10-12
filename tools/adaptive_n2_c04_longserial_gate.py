#!/usr/bin/env python3
"""Read-only C04 long-lived receive-range regression acceptance.

Runs only AFTER one real GitHub Actions performance job on the same SOURCE.
Never supplies netem truth or phase state to the product controller. This
does not certify the full N2 acceptance matrix, CPU optimization, or physical.
"""
from __future__ import annotations

import argparse
import json
import hashlib
from pathlib import Path

VERSION = "n2-c04-longserial-first-delivery-gate-v1"
FINAL_CLEAN_ACTIVE_INTERVAL = (720, 840)  # Frozen C04's last 120 active s.
MAX_FINAL_CLEAN_ZERO_GAP_S = 4  # existing >=5s uninterrupted-gap red line
MIN_FINAL_CLEAN_DELIVERY_RATIO = .90  # not a high-loss zero-packet demand
PROFILES = (0, 4, 8, 10, 12, 16, 20)


def longest_zero_span(values: list[int], first: int) -> dict:
    max_span = 0
    position = None
    current = 0
    begin = first
    for idx, count in enumerate(values, first):
        if count < 0:
            raise ValueError("negative delivery count")
        if count == 0:
            if current == 0:
                begin = idx
            current += 1
            if current > max_span:
                max_span, position = current, begin
        else:
            current = 0
    return {"length_seconds": max_span,
            "first_second": position,
            "last_second": position + max_span - 1 if position is not None else None}


def score(summary: dict, manifest: dict, biz: dict, target: dict,
          source_sha: str) -> dict:
    errors = []
    if summary.get("source_sha") != source_sha or manifest.get("source_sha") != source_sha:
        errors.append("SOURCE_MISMATCH")
    if summary.get("errors") or summary.get("correctness_errors"):
        errors.append("ORIGINAL_ANALYZER_ERRORS_OR_INTEGRITY")
    if summary.get("capacity_flags"):
        errors.append("CAPACITY_OR_DELIVERY_GAP_FLAGS_REQUIRE_REVIEW")
    config = manifest.get("config") or {}
    if any(config.get(k) != v for k, v in {
            "v3_client_fec_mode": "auto", "v3_client_fec_min_parity": 0,
            "v3_client_fec_initial_parity": 0,
            "duration_s": 840, "application_mbps_each_direction": 10.0,
            "one_way_delay_ms_each_direction": 300, "mtu": 1400
    }.items()):
        errors.append("NOT_ORIGINAL_SINGLE_C04_CONFIGURATION")
    if not summary.get("c04_loss20_grade_screen_pass"):
        errors.append("HIGH_20PCT_PROTECTION_GRADE_SCREEN_NOT_OBSERVED")
    if not summary.get("c04_final_recovery_grade_screen_pass"):
        errors.append("FINAL_CLEAN_GRADE_SCREEN_NOT_OBSERVED")
    final = next((s for s in summary.get("waveform", [])
                  if s.get("name") == "recover0"), None)
    if final is None or (final.get("start_s"), final.get("end_s")) != (600, 840):
        errors.append("C04_FINAL_STAGE_NOT_OBSERVED")
    else:
        for direction in ("c2s", "s2c"):
            q = final.get(direction) or {}
            if q.get("drop_counter_delta") != 0 or q.get("packets_counter_delta", 0) <= 0:
                errors.append("FINAL_CLEAN_NETEM_NOT_ZERO_" + direction)
    # In addition to the frozen [0,4] allowed band, observe actual off
    # within the last 60 active seconds; otherwise saving qualification OPEN.
    last60 = summary.get("c04_stage_last60_grade", {}).get("recover0", {})
    for direction in ("c2s", "s2c"):
        observed = last60.get(direction) or {}
        if observed.get("status") != "OBSERVED" or 0 not in observed.get("actual_r", []):
            errors.append(direction + "_OFF_NOT_OBSERVED_IN_FINAL_ACTIVE_60S")
    payload = {}
    lo, hi = FINAL_CLEAN_ACTIVE_INTERVAL
    for direction, src, dst in (("c2s", biz, target), ("s2c", target, biz)):
        sender = src.get("stats") or {}
        receiver = dst.get("stats") or {}
        sent = sender.get("sent_packets_by_second") or []
        wall = receiver.get("recv_wall_packets_by_second") or []
        if len(sent) < hi or len(wall) < hi:
            errors.append(direction + "_INCOMPLETE_BUSINESS_SECOND_SERIES")
            continue
        emitted = sum(sent[lo:hi])
        unique = sum(wall[lo:hi])
        ratio = unique / emitted if emitted > 0 else None
        gap = longest_zero_span(wall[lo:hi], lo)
        payload[direction] = {
            "active_second_interval": [lo, hi],
            "sent": emitted, "wall_first_unique": unique,
            "wall_first_unique_to_sent_ratio": ratio,
            "longest_wall_zero_first_delivery": gap,
            "entire_run_sent": sender.get("sent_packets"),
            "entire_run_received_first_unique": receiver.get("recv_unique_packets"),
            "entire_run_missing": max(0, (sender.get("sent_packets") or 0) -
                                    (receiver.get("recv_unique_packets") or 0)),
            "corrupt": receiver.get("corrupt"),
            "duplicates": receiver.get("recv_duplicates"),
            "send_failures": sender.get("send_failures"),
            "returned_business_oneway_p99_ns": receiver.get("oneway_p99_ns"),
        }
        if gap["length_seconds"] > MAX_FINAL_CLEAN_ZERO_GAP_S:
            errors.append(direction + "_FINAL_CLEAN_CONTINUOUS_DELIVERY_OUTAGE")
        if ratio is None or ratio < MIN_FINAL_CLEAN_DELIVERY_RATIO:
            errors.append(direction + "_FINAL_CLEAN_FIRST_DELIVERY_RATE_TOO_LOW")
        if sender.get("send_failures") or receiver.get("corrupt") or receiver.get("recv_duplicates"):
            errors.append(direction + "_INTEGRITY_OR_SEND_FAILURE")
    return {
        "schema": 1, "gate_version": VERSION, "source_sha": source_sha,
        "scope": "ONE_C04_LONG_LIVED_TWO_GIB_SERIAL_REGRESSION_NOT_N2_FINAL",
        "classification": ("SCOPED_C04_LONG_LIVED_FIRST_DELIVERY_AND_GRADE_PASS"
                           if not errors else "NOT_QUALIFIED_C04_LONG_LIVED_FIRST_DELIVERY"),
        "errors": errors,
        "predeclared_final_clean": {
            "active_seconds": [lo, hi],
            "maximum_allowed_contiguous_no_delivery_seconds": MAX_FINAL_CLEAN_ZERO_GAP_S,
            "minimum_first_unique_receive_to_sent_ratio": MIN_FINAL_CLEAN_DELIVERY_RATIO,
            "requires_high_20pct_and_final_clean_grade_screens": True,
        },
        "business": payload,
        "old_analyzer_classification": summary.get("classification"),
        "old_analyzer_capacity_flags": summary.get("capacity_flags"),
        "not_full_n2_pass": True,
        "physical": "NOT_RUN",
        "limitations": [
            "No zero business loss or probe full returns imposed at 20% stage",
            "No claims about burst/reorder/feedback/sparse/idle/mixed TCP UDP/MTU/Game",
            "CPU diagnostic ON not a same-CPU diagnostic OFF improvement",
            "No raw PCAP or business plaintext output",
        ],
    }


def main() -> None:
    p = argparse.ArgumentParser()
    p.add_argument("--artifact-dir", required=True, type=Path)
    p.add_argument("--source-sha", required=True)
    p.add_argument("--output", required=True, type=Path)
    args = p.parse_args()
    errors = []
    try:
        read = lambda name: json.loads((args.artifact_dir / name).read_text(encoding="utf-8"))
        out = score(read("summary.json"), read("manifest.json"),
                    read("biz.json"), read("target.json"), args.source_sha)
    except Exception as ex:
        out = {"schema": 1, "gate_version": VERSION,
               "source_sha": args.source_sha,
               "classification": "INVALID_C04_ANALYSIS",
               "errors": [str(ex)], "not_full_n2_pass": True}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(out, indent=2, sort_keys=True) + "\n",
                           encoding="utf-8")
    print("WBD_C04_LONG_SERIAL_" + out["classification"])
    if out["errors"]:
        raise SystemExit(1)


if __name__ == "__main__":
    main()
