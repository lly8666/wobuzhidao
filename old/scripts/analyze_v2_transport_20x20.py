#!/usr/bin/env python3
from __future__ import annotations

import argparse
import csv
import json
from collections import defaultdict
from pathlib import Path

EXPECTED_RTTS = [20, 50, 100, 200, 400, 600]
EXPECTED_LOSSES = [0.0, 1.0, 5.0, 10.0, 20.0, 30.0, 40.0]
EXPECTED_SEEDS = [260825, 260826, 260827]


def num(value):
    if value in (None, "", "-"):
        return None
    try:
        return float(value)
    except (TypeError, ValueError):
        return None


def load_medians(path: Path) -> list[dict]:
    rows = []
    with path.open(newline="") as f:
        for raw in csv.DictReader(f):
            row = dict(raw)
            for key in list(row):
                if key in {"rtt_ms", "cases", "handshake_success_cases", "tcp_like_stable_cases", "udp_like_stable_cases", "outer_rst_total", "gap_bypass_cases"}:
                    row[key] = int(float(row[key])) if row[key] not in (None, "") else 0
                elif key == "loss_pct_per_direction" or key.endswith("_median"):
                    row[key] = num(row[key])
            rows.append(row)
    return rows


def classify_point(row: dict, baseline: dict | None) -> dict:
    cases = int(row.get("cases") or 0)
    hs = int(row.get("handshake_success_cases") or 0)
    tcp = int(row.get("tcp_like_stable_cases") or 0)
    udp = int(row.get("udp_like_stable_cases") or 0)
    rst = int(row.get("outer_rst_total") or 0)
    delivery = num(row.get("delivery_ratio_median"))
    cpu = num(row.get("cpu_ms_per_delivered_mib_median"))
    rss = num(row.get("rss_peak_kb_total_median"))
    p99 = num(row.get("p99_ms_median"))
    loss = float(row["loss_pct_per_direction"])
    rtt = int(row["rtt_ms"])

    base_cpu = num(baseline.get("cpu_ms_per_delivered_mib_median")) if baseline else None
    base_rss = num(baseline.get("rss_peak_kb_total_median")) if baseline else None
    cpu_ratio = cpu / base_cpu if cpu is not None and base_cpu not in (None, 0) else None
    rss_ratio = rss / base_rss if rss is not None and base_rss not in (None, 0) else None

    labels = []
    if cases == 0 or hs == 0:
        labels.append("establishment_cliff")
    elif hs < cases:
        labels.append("establishment_degraded")
    if delivery is None:
        labels.append("no_delivery_measurement")
    elif delivery < 0.95:
        labels.append("delivery_cliff")
    elif delivery < 0.99:
        labels.append("delivery_degraded")
    if rst > 0 or tcp < cases:
        labels.append("tcp_like_degraded")
    if loss > 0 and udp == 0:
        labels.append("udp_like_evidence_degraded")
    if cpu_ratio is not None and cpu_ratio >= 1.5:
        labels.append("cpu_inflated_1_5x")
    if rss_ratio is not None and rss_ratio >= 1.25:
        labels.append("rss_inflated_1_25x")

    return {
        "rtt_ms": rtt,
        "loss_pct_per_direction": loss,
        "cases": cases,
        "handshake_success_cases": hs,
        "delivery_median": delivery,
        "p99_ms_median": p99,
        "tcp_like_stable_cases": tcp,
        "udp_like_stable_cases": udp,
        "outer_rst_total": rst,
        "gap_bypass_cases": int(row.get("gap_bypass_cases") or 0),
        "cpu_ms_per_delivered_mib_median": cpu,
        "cpu_ratio_vs_loss0_same_rtt": cpu_ratio,
        "rss_peak_kb_total_median": rss,
        "rss_ratio_vs_loss0_same_rtt": rss_ratio,
        "labels": labels,
    }


def first_boundary(points: list[dict], predicate) -> tuple[dict | None, dict | None]:
    previous = None
    for point in sorted(points, key=lambda x: x["loss_pct_per_direction"]):
        if predicate(point):
            return previous, point
        previous = point
    return previous, None


def midpoint_suggestion(previous: dict | None, bad: dict | None, reason: str) -> dict | None:
    if previous is None or bad is None:
        return None
    lo = float(previous["loss_pct_per_direction"])
    hi = float(bad["loss_pct_per_direction"])
    if hi - lo < 4.0:
        return None
    return {
        "rtt_ms": int(bad["rtt_ms"]),
        "loss_pct_per_direction": (lo + hi) / 2.0,
        "reason": reason,
        "bracket": [lo, hi],
    }


def analyze(rows: list[dict]) -> dict:
    by_rtt: dict[int, list[dict]] = defaultdict(list)
    for row in rows:
        by_rtt[int(row["rtt_ms"])].append(row)

    classified = []
    boundaries = []
    suggestions = []
    for rtt in sorted(by_rtt):
        raw_points = sorted(by_rtt[rtt], key=lambda x: float(x["loss_pct_per_direction"]))
        baseline = next((x for x in raw_points if float(x["loss_pct_per_direction"]) == 0.0), None)
        points = [classify_point(x, baseline) for x in raw_points]
        classified.extend(points)

        rules = {
            "establishment": lambda p: p["handshake_success_cases"] < p["cases"],
            "delivery_99pct": lambda p: p["delivery_median"] is None or p["delivery_median"] < 0.99,
            "delivery_95pct": lambda p: p["delivery_median"] is None or p["delivery_median"] < 0.95,
            "tcp_like": lambda p: p["outer_rst_total"] > 0 or p["tcp_like_stable_cases"] < p["cases"],
            "cpu_1_5x": lambda p: p["cpu_ratio_vs_loss0_same_rtt"] is not None and p["cpu_ratio_vs_loss0_same_rtt"] >= 1.5,
            "rss_1_25x": lambda p: p["rss_ratio_vs_loss0_same_rtt"] is not None and p["rss_ratio_vs_loss0_same_rtt"] >= 1.25,
        }
        item = {"rtt_ms": rtt}
        for name, pred in rules.items():
            prev, bad = first_boundary(points, pred)
            item[name] = {
                "last_better_loss": prev["loss_pct_per_direction"] if prev else None,
                "first_bad_loss": bad["loss_pct_per_direction"] if bad else None,
            }
            suggestion = midpoint_suggestion(prev, bad, name)
            if suggestion:
                suggestions.append(suggestion)
        boundaries.append(item)

    unique = []
    seen = set()
    for s in suggestions:
        key = (s["rtt_ms"], s["loss_pct_per_direction"])
        if key in seen:
            continue
        seen.add(key)
        reasons = sorted({x["reason"] for x in suggestions if (x["rtt_ms"], x["loss_pct_per_direction"]) == key})
        unique.append({**s, "reason": ",".join(reasons)})

    return {
        "schema": "wbd-v2-transport-20x20-cliff-analysis/v1",
        "points": classified,
        "boundaries": boundaries,
        "targeted_followups": unique,
    }


def render_markdown(result: dict, receipt: dict) -> str:
    lines = [
        "# V2 transport-only 20:20 cliff analysis",
        "",
        f"Matrix cases: **{receipt.get('cases', '?')}**; seeds: `{receipt.get('seeds', [])}`.",
        "",
        "| RTT ms | HS first bad loss | delivery <99% | delivery <95% | TCP-like first bad | CPU ≥1.5x | RSS ≥1.25x |",
        "| ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
    ]
    for b in result["boundaries"]:
        def v(name):
            x = b[name]["first_bad_loss"]
            return "-" if x is None else f"{x:g}%"
        lines.append(f"| {b['rtt_ms']} | {v('establishment')} | {v('delivery_99pct')} | {v('delivery_95pct')} | {v('tcp_like')} | {v('cpu_1_5x')} | {v('rss_1_25x')} |")
    lines += ["", "## Targeted follow-up candidates", ""]
    if not result["targeted_followups"]:
        lines.append("No midpoint follow-up is suggested by the current coarse grid.")
    else:
        lines.append("These are boundary refinements only; they are not a new full factorial sweep.")
        lines.append("")
        for s in result["targeted_followups"]:
            lines.append(f"- RTT {s['rtt_ms']} ms, loss {s['loss_pct_per_direction']:g}%/direction — bracket {s['bracket'][0]:g}–{s['bracket'][1]:g}%, reason: `{s['reason']}`")
    lines += ["", "## Interpretation rules", "", "- `delivery <99%` marks the first point where fixed 20:20 no longer gives near-complete delivery for the measured workload.", "- `delivery <95%` is treated as a stronger delivery cliff.", "- any RST or incomplete TCP-like stability classification is surfaced independently from application delivery.", "- CPU and RSS ratios are relative to the 0% loss point at the same RTT, so network-delay differences are not mixed into the baseline.", ""]
    return "\n".join(lines)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", type=Path, required=True, help="directory containing median.csv and receipt.json")
    ap.add_argument("--out", type=Path, required=True)
    a = ap.parse_args()
    rows = load_medians(a.input / "median.csv")
    receipt = json.loads((a.input / "receipt.json").read_text())
    result = analyze(rows)
    result["source_receipt"] = {
        "schema": receipt.get("schema"),
        "cases": receipt.get("cases"),
        "seeds": receipt.get("seeds"),
        "results_sha256": receipt.get("results_sha256"),
        "median_sha256": receipt.get("median_sha256"),
    }
    a.out.mkdir(parents=True, exist_ok=True)
    (a.out / "cliffs.json").write_text(json.dumps(result, indent=2, sort_keys=True) + "\n")
    (a.out / "cliffs.md").write_text(render_markdown(result, receipt))
    print((a.out / "cliffs.md").read_text())
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
