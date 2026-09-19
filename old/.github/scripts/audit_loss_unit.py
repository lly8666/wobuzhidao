"""Audit one loss-unit artifact without turning diagnostics into root-cause claims.

The audit has two jobs:
1. decide whether the sample is valid for steady-state comparison; and
2. emit a compact, provenance-preserving summary for the 1500/1600 carrier-MTU A/B.

It intentionally does not infer that fragmentation, FEC pressure, CPU, or any
other mechanism is the root cause. Those are conclusions for cross-run review.
"""

from __future__ import annotations

import json
from pathlib import Path
import re
import sys
from typing import Any


root = Path(sys.argv[1])
failures: list[str] = []
warnings: list[str] = []
observations: dict[str, Any] = {}
size_errors: list[dict[str, Any]] = []


def load_json(name: str) -> Any:
    path = root / name
    if not path.exists():
        return None
    try:
        return json.loads(path.read_text(errors="replace"))
    except Exception as exc:  # artifact diagnostics must survive malformed JSON
        warnings.append(f"{name}: invalid json: {exc}")
        return None


def number_fields(line: str) -> dict[str, int]:
    out: dict[str, int] = {}
    for key, value in re.findall(r"\b([a-zA-Z][a-zA-Z0-9_]*)=(\d+)\b", line):
        out[key] = int(value)
    return out


def classify_size_error(name: str, line: str) -> dict[str, Any]:
    if "linkdata encode:" in line:
        kind = "encode_input_exceeds_link_mtu"
        direction = "encode"
    elif "linkdata decode:" in line or "linkdata decode off:" in line:
        kind = "decode_size_or_declared_length_mismatch"
        direction = "decode"
    else:
        kind = "legacy_unclassified_packet_too_large"
        direction = "unknown"
    return {
        "log": name,
        "kind": kind,
        "direction": direction,
        "fields": number_fields(line),
        "line": line,
    }


def last_fec_diag(name: str, text: str) -> dict[str, Any] | None:
    rows = []
    for line in text.splitlines():
        if "WBD_LINK_FEC_DIAG " not in line:
            continue
        try:
            rows.append(json.loads(line.split("WBD_LINK_FEC_DIAG ", 1)[1]))
        except Exception as exc:
            warnings.append(f"{name}: malformed WBD_LINK_FEC_DIAG: {exc}")
    if not rows:
        return None
    d = rows[-1]
    settled = int(d.get("rx_horizon_settled_blocks", 0) or 0)
    under = int(d.get("rx_horizon_under_required", 0) or 0)
    missing = int(d.get("rx_horizon_missing_required", 0) or 0)
    return {
        "snapshot_count": len(rows),
        "settled_blocks": settled,
        "under_required": under,
        "under_required_ratio": under / settled if settled else None,
        "missing_required": missing,
        "mean_missing_among_under": missing / under if under else None,
        "no_final_metadata": d.get("rx_horizon_no_final_metadata"),
        "reconstruct_calls": d.get("reconstruct_calls"),
        "reconstruct_success": d.get("reconstruct_success"),
        "stale_would_decode_after_retire": d.get("stale_would_decode_after_retire"),
        "fallback_would_decode_compact": d.get("fallback_would_decode_compact"),
        "peak_in_flight": d.get("peak_in_flight"),
        "pressure_retire_events": d.get("pressure_retire_events"),
        "last_snapshot": d,
        "caveat": "Last periodic snapshot only; TX/RX sides are not synchronized final totals.",
    }


lane_fail_total = 0
dormant_drop_total = 0
log_names = ("link-1.log", "link-server.log", "game-client.log", "game-server.log")
for name in log_names:
    path = root / name
    if not path.exists():
        failures.append("missing " + name)
        continue
    text = path.read_text(errors="replace")

    fatal = [line for line in text.splitlines() if re.search(r"WBD_\w*FAIL\b", line)]
    failures.extend(name + ": " + line for line in fatal)

    for line in text.splitlines():
        if "fec: packet too large" in line:
            size_errors.append(classify_size_error(name, line))

    lane_values = [int(x) for x in re.findall(r"\blane_fail=(\d+)", text)]
    dormant_values = [int(x) for x in re.findall(r"\bdormant_drop=(\d+)", text)]
    lane_fail_total += max(lane_values, default=0)
    dormant_drop_total += max(dormant_values, default=0)
    if any(lane_values):
        failures.append(name + ": lane_fail is nonzero")
    if any(dormant_values):
        failures.append(name + ": dormant_drop is nonzero")

    diag = last_fec_diag(name, text)
    if diag is not None:
        observations.setdefault("fec", {})[name] = diag


carrier: dict[str, Any] = {}
for name in ("faketcp-1.log", "faketcp-mux.log"):
    path = root / name
    rows: list[dict[str, Any]] = []
    if path.exists():
        for line in path.read_text(errors="replace").splitlines():
            if "WBD_CARRIER_FRAGMENT_STATS " not in line:
                continue
            try:
                rows.append(json.loads(line.split("WBD_CARRIER_FRAGMENT_STATS ", 1)[1]))
            except Exception as exc:
                warnings.append(f"{name}: malformed WBD_CARRIER_FRAGMENT_STATS: {exc}")
    if not rows:
        failures.append(name + ": missing carrier fragmentation stats")
        carrier[name] = {"rows": []}
        continue

    datagrams = sum(int(d.get("datagrams", 0) or 0) for d in rows)
    fragmented = sum(int(d.get("fragmented_datagrams", 0) or 0) for d in rows)
    frames = sum(int(d.get("frames", 0) or 0) for d in rows)
    input_bytes = sum(int(d.get("input_bytes", 0) or 0) for d in rows)
    budgets = sorted({d.get("payload_budget") for d in rows if d.get("payload_budget") is not None})
    carrier[name] = {
        "session_count": len(rows),
        "payload_budgets": budgets,
        "datagrams": datagrams,
        "fragmented_datagrams": fragmented,
        "fragmented_ratio": fragmented / datagrams if datagrams else None,
        "frames": frames,
        "mean_frames_per_datagram": frames / datagrams if datagrams else None,
        "extra_frames": frames - datagrams,
        "input_bytes": input_bytes,
        "mean_input_bytes": input_bytes / datagrams if datagrams else None,
        "rows": rows,
        "caveat": "Counts fragmentation decisions before raw send/repair; not actual frame delivery.",
    }
observations["carrier_fragmentation"] = carrier


load_result = load_json("load-result.json")
constant_result = load_json("constant-result.json")
host_pressure = load_json("host-pressure.json")
if load_result is not None:
    observations["load"] = {
        key: load_result.get(key)
        for key in (
            "sent", "received_unique", "lost", "loss_ratio", "byte_loss_ratio",
            "down_payload_bps", "rtt_ms_p50", "rtt_ms_p95", "rtt_ms_p99",
            "timely_1s_ratio", "traffic_profile", "avg_payload_bytes", "rate_target_bps",
        )
    }
if constant_result is not None:
    observations["constant"] = {
        key: constant_result.get(key)
        for key in (
            "candidate", "product_source_sha", "loss_model", "loss_pct_each_direction",
            "rate_bps_each_direction", "goodput_mbps", "app_loss_ratio", "byte_loss_ratio",
            "peak_pending", "peak_buffered_oo", "fresh_blocked_by_repair", "repair_evicted",
            "forgiven_gaps", "repair_to_fresh_bytes", "avg_cpu_cores", "host_cpu_busy_cores",
            "host_cpu_count", "softnet_dropped_delta", "softnet_time_squeeze_delta",
        )
    }
if host_pressure is not None:
    observations["host_pressure"] = host_pressure

selected_case = load_json("selected-case.json")
source_sha_path = root / "source-sha.txt"
source_sha = source_sha_path.read_text(errors="replace").strip() if source_sha_path.exists() else None
observations["provenance"] = {
    "selected_case": selected_case,
    "source_sha": source_sha,
    "test_overlay_present": (root / "test-overlay.patch").exists(),
}
if selected_case is None:
    warnings.append("selected-case.json missing; expected for the new loss-unit workflow")
if not source_sha:
    warnings.append("source-sha.txt missing; exact-source provenance unavailable")

# Do not conflate a failed validity gate with the mechanism that caused it.
invalid_reasons: list[str] = []
if size_errors:
    invalid_reasons.append("size_error")
if lane_fail_total:
    invalid_reasons.append("lane_failure")
if dormant_drop_total:
    invalid_reasons.append("dormant_drop")
if any(msg.startswith("missing ") for msg in failures):
    invalid_reasons.append("missing_required_log")
if any("missing carrier fragmentation stats" in msg for msg in failures):
    invalid_reasons.append("missing_fragmentation_stats")

result = {
    "steady_state_sample_valid": not failures,
    "invalid_reasons": sorted(set(invalid_reasons)),
    "failures": failures,
    "warnings": warnings,
    "size_errors": size_errors,
    "lane_fail_total": lane_fail_total,
    "dormant_drop_total": dormant_drop_total,
    "observations": observations,
    "interpretation_guardrails": [
        "A dead lane is not a steady-state FEC-loss sample.",
        "A fragmentation ratio is not proof that fragmentation caused application loss.",
        "Periodic FEC snapshots and endpoint TX/RX counters are not synchronized final totals.",
        "Compare 1500 vs 1600 only when business size, FEC, rate, delay, helper overlay, and source lineage are otherwise identical.",
    ],
}
(root / "loss-unit-audit.json").write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
print(json.dumps({
    "steady_state_sample_valid": result["steady_state_sample_valid"],
    "invalid_reasons": result["invalid_reasons"],
    "size_error_kinds": sorted({x["kind"] for x in size_errors}),
    "lane_fail_total": lane_fail_total,
    "dormant_drop_total": dormant_drop_total,
}))
raise SystemExit(0 if not failures else 1)
