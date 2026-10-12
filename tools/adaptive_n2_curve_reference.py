#!/usr/bin/env python3
"""Independent N2 acceptance oracle. Test-only: no product controller imports.

Model v1 is fixed before any new performance run. Actual completed FEC group
shapes can be substituted after measurement without changing thresholds.
This oracle never supplies decisions to the product.
"""
from __future__ import annotations

import argparse
import json
import math
from pathlib import Path

PROFILES = (0, 4, 8, 10, 12, 16, 20)
TARGET = 0.001
LOWER_MARGIN = 0.0005
MODEL_VERSION = "n2-independent-iid-timeband-v1"


def residual(k: int, configured_r: int, p: float) -> float:
    """Exact MDS source erasure probability via independent binomial terms.

    This intentionally uses math.comb, not the production recurrence.
    A partial block emits min(k, configured_r) parity shards.
    """
    if not (1 <= k <= 20 and configured_r in PROFILES and
            math.isfinite(p) and 0 <= p <= 1):
        raise ValueError("bad k, R or p")
    r = min(k, configured_r)
    if r == 0:
        return p
    n = k + r - 1
    return p * sum(math.comb(n, j) * p ** j * (1 - p) ** (n - j)
                   for j in range(r, n + 1))


def mixed_residual(groups: list[dict], parity: int, p: float,
                   byte_weighted: bool = False) -> float:
    """Use actual CLOSED group k; do not equate 256-PN crop with a lost group.

    Each entry is {k, count, source_bytes?}. Byte-weighted mode needs
    a source_bytes total over its group's systematic payloads.
    """
    numerator = denominator = 0.0
    for item in groups:
        k, count = int(item["k"]), int(item["count"])
        if not 1 <= k <= 20 or count <= 0:
            raise ValueError("invalid group shape")
        weight = (float(item["source_bytes"]) if byte_weighted
                  else k * count)
        if weight <= 0:
            raise ValueError("invalid systematic byte weight")
        numerator += weight * residual(k, parity, p)
        denominator += weight
    if denominator == 0:
        raise ValueError("actual completed group shape unavailable")
    return numerator / denominator


def ideal(groups: list[dict], p: float, floor: int, ceiling: int = 20,
          target: float = TARGET) -> dict:
    if floor not in PROFILES or ceiling not in PROFILES or floor > ceiling:
        raise ValueError("invalid floor or ceiling")
    curve = {str(r): mixed_residual(groups, r, p)
             for r in PROFILES if floor <= r <= ceiling}
    sufficient = [int(r) for r, v in curve.items() if v <= target]
    return {"ideal_parity": sufficient[0] if sufficient else None,
            "target_unattainable": not bool(sufficient),
            "residual_by_parity": curve}


def next_profile(p: int, ceiling: int = 20) -> int:
    return next((r for r in PROFILES if r > p and r <= ceiling), p)


def time_band(*, rtt_s: float, from_r: int, to_r: int,
              report_s: float, maturity_s: float,
              effective_reports_per_s: float, history_windows: int,
              min_records: int, pn_window: int, fast_tau_s: float,
              slow_tau_s: float, stable_s: float, off_stable_s: float,
              cooldown_s: float) -> dict:
    """Independent observable scheduling budget; not product Step replay.

    Source coverage, feedback and maturity are pipelined. Samples and history
    are conservative design-rate limits, not fitted from a measured failure.
    Unreliable feedback / no valid source -> QUALITY_LIMITED, not a fake bound.
    """
    if effective_reports_per_s <= 0 or report_s <= 0 or pn_window <= 0:
        raise ValueError("no declared qualified input rate")
    if from_r not in PROFILES or to_r not in PROFILES:
        raise ValueError("invalid profile")
    count = abs(PROFILES.index(from_r) - PROFILES.index(to_r))
    sample_s = math.ceil(min_records / pn_window) / effective_reports_per_s
    observation = maturity_s + rtt_s + report_s + sample_s
    history_exit_s = history_windows / effective_reports_per_s
    falling = to_r < from_r
    # 15s slow filter and historical eviction are concurrent, not additive.
    settle = max(slow_tau_s, history_exit_s, stable_s) if falling else fast_tau_s
    if falling and to_r == 0:
        settle = max(settle, off_stable_s)
    lower = observation + ((max(stable_s, sample_s) if falling else fast_tau_s)
                           + max(0, count - 1) * cooldown_s if count else 0)
    expected = observation + (settle + max(0, count - 1) * cooldown_s
                              if count else 0)
    latest_screen = max(1.5 * expected, expected + max(10., 4. * rtt_s))
    return {"earliest_s": round(lower, 3),
            "expected_s": round(expected, 3),
            "latest_screen_s": round(latest_screen, 3),
            "report_coverage_s": round(sample_s, 3),
            "history_exit_s": round(history_exit_s, 3),
            "rate_assumption": "predeclared design estimate, not post-fit"}


def generate(manifest: dict) -> dict:
    if manifest["model_version"] != MODEL_VERSION:
        raise ValueError("wrong frozen model version")
    config = manifest["timing"]
    groups = manifest["shape_assumption"]["groups"]
    records = []
    for scenario in manifest["scenarios"]:
        prev = scenario["initial_parity"]
        stage_results = []
        for stage in scenario["stages"]:
            p = stage["loss_percent"] / 100.
            result = ideal(groups, p, scenario["min_parity"])
            if result["ideal_parity"] != stage["frozen_ideal_parity"]:
                raise ValueError("frozen ideal parity mismatch: " + scenario["id"] +
                                 "/" + stage["name"])
            goal = result["ideal_parity"]
            if goal is not None:
                tb = time_band(rtt_s=scenario["rtt_s"], from_r=prev,
                               to_r=goal, **config)
                if tb["earliest_s"] > stage["duration_s"] and (
                    stage.get("must_settle", False)):
                    raise ValueError("stage shorter than earliest expected bound")
                prev = goal
            else:
                tb = None
            stage_results.append({"name": stage["name"], "loss_percent": stage["loss_percent"],
                                  "ideal": goal, "unattainable": result["target_unattainable"],
                                  "residual": result["residual_by_parity"],
                                  "grade_screen": stage["frozen_grade_screen"],
                                  "time_since_stage_start": tb})
        records.append({"id": scenario["id"], "stages": stage_results})
    return {"schema": 1, "model_version": MODEL_VERSION,
            "conditions": "IID complete-group hypothesis only; real k/bursts recalculate",
            "note": "Not an empirical result; quality and capacity invalidations separate",
            "scenarios": records}


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    args = parser.parse_args()
    manifest = json.loads(args.manifest.read_text(encoding="utf-8"))
    result = generate(manifest)
    args.output.write_text(json.dumps(result, indent=2, sort_keys=True) + "\n",
                           encoding="utf-8")


if __name__ == "__main__":
    main()
