"""Compare the four audited loss-unit experiments without declaring a root cause.

Usage:
    python3 compare_loss_unit_audits.py <audit-or-dir> [<audit-or-dir> ...]

Each argument may be a loss-unit-audit.json file or an artifact directory that
contains one. Exactly one audited sample is required for each controlled case:
30/1500, 30/1600, 20/1500, and 20/1600.
"""

from __future__ import annotations

import json
from pathlib import Path
import sys
from typing import Any

EXPECTED = [(30, 1500), (30, 1600), (20, 1500), (20, 1600)]


def read_audit(arg: str) -> tuple[Path, dict[str, Any]]:
    p = Path(arg)
    if p.is_dir():
        p = p / "loss-unit-audit.json"
    if not p.exists():
        raise SystemExit(f"missing audit: {p}")
    return p, json.loads(p.read_text(errors="replace"))


def selected_case(d: dict[str, Any]) -> tuple[int, int]:
    case = d.get("observations", {}).get("provenance", {}).get("selected_case")
    if not isinstance(case, dict):
        raise SystemExit("audit lacks observations.provenance.selected_case")
    try:
        key = (int(case["loss_pct"]), int(case["carrier_mtu"]))
    except Exception as exc:
        raise SystemExit(f"bad selected_case {case!r}: {exc}")
    if key not in EXPECTED:
        raise SystemExit(f"unexpected controlled case {key}")
    return key


def aggregate_carrier(d: dict[str, Any]) -> dict[str, Any]:
    rows = d.get("observations", {}).get("carrier_fragmentation", {})
    datagrams = fragmented = frames = input_bytes = 0
    budgets: set[int] = set()
    for side in ("faketcp-1.log", "faketcp-mux.log"):
        s = rows.get(side, {}) if isinstance(rows, dict) else {}
        datagrams += int(s.get("datagrams", 0) or 0)
        fragmented += int(s.get("fragmented_datagrams", 0) or 0)
        frames += int(s.get("frames", 0) or 0)
        input_bytes += int(s.get("input_bytes", 0) or 0)
        for b in s.get("payload_budgets", []) or []:
            budgets.add(int(b))
    return {
        "payload_budgets": sorted(budgets),
        "datagrams": datagrams,
        "fragmented_datagrams": fragmented,
        "fragmented_ratio": fragmented / datagrams if datagrams else None,
        "frames": frames,
        "mean_frames_per_datagram": frames / datagrams if datagrams else None,
        "mean_input_bytes": input_bytes / datagrams if datagrams else None,
    }


def aggregate_fec(d: dict[str, Any]) -> dict[str, Any]:
    fec = d.get("observations", {}).get("fec", {})
    settled = under = missing = 0
    reconstruct_calls = reconstruct_success = 0
    for side in ("link-1.log", "link-server.log"):
        s = fec.get(side, {}) if isinstance(fec, dict) else {}
        settled += int(s.get("settled_blocks", 0) or 0)
        under += int(s.get("under_required", 0) or 0)
        missing += int(s.get("missing_required", 0) or 0)
        reconstruct_calls += int(s.get("reconstruct_calls", 0) or 0)
        reconstruct_success += int(s.get("reconstruct_success", 0) or 0)
    return {
        "settled_blocks": settled,
        "under_required": under,
        "under_required_ratio": under / settled if settled else None,
        "missing_required": missing,
        "mean_missing_among_under": missing / under if under else None,
        "reconstruct_calls": reconstruct_calls,
        "reconstruct_success": reconstruct_success,
    }


def pct(v: Any) -> str:
    return "n/a" if v is None else f"{100 * float(v):.3f}%"


def num(v: Any, digits: int = 3) -> str:
    return "n/a" if v is None else f"{float(v):.{digits}f}"


items: dict[tuple[int, int], dict[str, Any]] = {}
for arg in sys.argv[1:]:
    path, data = read_audit(arg)
    key = selected_case(data)
    if key in items:
        raise SystemExit(f"duplicate audit for {key}: {path}")
    items[key] = {"path": str(path), "audit": data}

missing = [k for k in EXPECTED if k not in items]
if missing:
    raise SystemExit(f"missing controlled cases: {missing}")

summary: dict[str, Any] = {
    "cases": {},
    "pairs": {},
    "guardrails": [
        "Only compare steady-state metrics when both samples in a pair are valid.",
        "A reduction in fragmentation is necessary but not sufficient evidence that fragmentation caused application loss.",
        "Do not use MTU1600 as a product recommendation; it is an in-lab causal control.",
    ],
}

for key in EXPECTED:
    d = items[key]["audit"]
    load = d.get("observations", {}).get("load", {})
    constant = d.get("observations", {}).get("constant", {})
    host = d.get("observations", {}).get("host_pressure", {})
    case = {
        "source": items[key]["path"],
        "valid": bool(d.get("steady_state_sample_valid")),
        "invalid_reasons": d.get("invalid_reasons", []),
        "size_error_kinds": sorted({x.get("kind") for x in d.get("size_errors", []) if x.get("kind")}),
        "app_loss_ratio": load.get("loss_ratio", constant.get("app_loss_ratio")),
        "byte_loss_ratio": load.get("byte_loss_ratio", constant.get("byte_loss_ratio")),
        "goodput_mbps": constant.get("goodput_mbps"),
        "rtt_p99_ms": load.get("rtt_ms_p99"),
        "host_cpu_busy_cores": constant.get(
            "host_cpu_busy_cores",
            host.get("host_cpu_busy_cores") if isinstance(host, dict) else None,
        ),
        "host_cpu_count": constant.get(
            "host_cpu_count",
            host.get("host_cpu_count") if isinstance(host, dict) else None,
        ),
        "carrier": aggregate_carrier(d),
        "fec": aggregate_fec(d),
    }
    summary["cases"][f"{key[0]}-{key[1]}"] = case

for loss in (30, 20):
    a = summary["cases"][f"{loss}-1500"]
    b = summary["cases"][f"{loss}-1600"]
    pair: dict[str, Any] = {"both_valid": a["valid"] and b["valid"]}
    metrics = {
        "fragmented_ratio": (a["carrier"]["fragmented_ratio"], b["carrier"]["fragmented_ratio"]),
        "app_loss_ratio": (a["app_loss_ratio"], b["app_loss_ratio"]),
        "byte_loss_ratio": (a["byte_loss_ratio"], b["byte_loss_ratio"]),
        "under_required_ratio": (a["fec"]["under_required_ratio"], b["fec"]["under_required_ratio"]),
        "mean_missing_among_under": (
            a["fec"]["mean_missing_among_under"],
            b["fec"]["mean_missing_among_under"],
        ),
        "goodput_mbps": (a["goodput_mbps"], b["goodput_mbps"]),
        "rtt_p99_ms": (a["rtt_p99_ms"], b["rtt_p99_ms"]),
    }
    pair["metrics_1500_vs_1600"] = metrics
    pair["delta_1500_minus_1600"] = {
        name: (x - y) if x is not None and y is not None else None
        for name, (x, y) in metrics.items()
    }
    summary["pairs"][str(loss)] = pair

out = Path("loss-unit-matrix.json")
out.write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")

print("| loss | MTU | valid | fragment | app loss | byte loss | FEC under | mean missing | goodput Mbps | RTT p99 ms |")
print("| ---: | ---: | :---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |")
for loss, mtu in EXPECTED:
    c = summary["cases"][f"{loss}-{mtu}"]
    print(
        f"| {loss}% | {mtu} | {'yes' if c['valid'] else 'NO'} | "
        f"{pct(c['carrier']['fragmented_ratio'])} | {pct(c['app_loss_ratio'])} | {pct(c['byte_loss_ratio'])} | "
        f"{pct(c['fec']['under_required_ratio'])} | {num(c['fec']['mean_missing_among_under'])} | "
        f"{num(c['goodput_mbps'])} | {num(c['rtt_p99_ms'])} |"
    )
print(f"wrote {out}")
