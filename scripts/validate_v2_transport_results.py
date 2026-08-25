#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
from pathlib import Path


def f(v):
    try:
        return float(v)
    except (TypeError, ValueError):
        return None


def validate_results(path: Path, *, expected_cases: int | None, require_baseline: bool = True) -> list[str]:
    with path.open(newline="") as fh:
        rows = list(csv.DictReader(fh))
    problems: list[str] = []
    if expected_cases is not None and len(rows) != expected_cases:
        problems.append(f"cases={len(rows)} want={expected_cases}")
    errors = [r for r in rows if r.get("case_status") == "harness_error"]
    if errors:
        sample = errors[0].get("harness_error", "")[:300]
        problems.append(f"harness_error_cases={len(errors)} sample={sample}")

    established = [r for r in rows if r.get("case_status") not in {"harness_error", "handshake_fail"}]
    for r in established:
        missing = [k for k in ("delivery_ratio", "p99_ms", "cpu_ms_total", "rss_peak_kb_total") if f(r.get(k)) is None]
        if missing:
            problems.append(f"{r.get('case')}: established case missing {','.join(missing)}")
            break

    if require_baseline:
        baseline = [r for r in rows if f(r.get("loss_pct_per_direction")) == 0.0]
        if not baseline:
            problems.append("no 0% loss baseline cases")
        for r in baseline:
            if r.get("case_status") != "pass":
                problems.append(f"{r.get('case')}: 0% loss baseline status={r.get('case_status')}")
                continue
            d = f(r.get("delivery_ratio"))
            if d is None or d < 0.99:
                problems.append(f"{r.get('case')}: 0% loss delivery={d}")

    positive_loss_measured = [
        r for r in rows
        if (f(r.get("loss_pct_per_direction")) or 0) > 0
        and r.get("case_status") not in {"harness_error", "handshake_fail"}
        and f(r.get("delivery_ratio")) is not None
    ]
    if any((f(r.get("loss_pct_per_direction")) or 0) > 0 for r in rows) and not positive_loss_measured:
        problems.append("no positive-loss case reached application measurement")
    return problems


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--results", type=Path, required=True)
    ap.add_argument("--expected-cases", type=int)
    ap.add_argument("--receipt", type=Path)
    a = ap.parse_args()
    problems = validate_results(a.results, expected_cases=a.expected_cases)
    if a.receipt and a.receipt.exists():
        receipt = json.loads(a.receipt.read_text())
        if a.expected_cases is not None and receipt.get("cases") != a.expected_cases:
            problems.append(f"receipt cases={receipt.get('cases')} want={a.expected_cases}")
    if problems:
        print("TRANSPORT_RESULTS_INVALID")
        for p in problems:
            print(f"- {p}")
        return 2
    print("TRANSPORT_RESULTS_VALID")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
