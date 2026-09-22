#!/usr/bin/env python3
import argparse
import json
from pathlib import Path


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    rows = []
    for path in Path(args.root).rglob("summary.json"):
        try:
            row = json.loads(path.read_text(encoding="utf-8"))
        except Exception:
            continue
        if row.get("schema") == 1 and row.get("mode") in ("normal", "game"):
            row["_path"] = str(path)
            rows.append(row)

    index = {(r["mode"], r["scenario"], int(r["seed"])): r for r in rows}
    errors = []
    expected = [(m, s, seed) for m in ("normal", "game")
                for s in ("lossless", "5205", "5305")
                for seed in (101, 202, 303)]
    missing = [x for x in expected if x not in index]
    if missing:
        errors.append(f"missing strict samples: {missing}")

    rtt = []
    for mode in ("normal", "game"):
        for seed in (101, 202, 303):
            base = index.get((mode, "lossless", seed))
            if not base:
                continue
            b = base["latency"]["probe_stage"]["stress"]
            for scenario in ("5205", "5305"):
                row = index.get((mode, scenario, seed))
                if not row:
                    continue
                x = row["latency"]["probe_stage"]["stress"]
                p95_delta = None if b["p95_ns"] is None or x["p95_ns"] is None else x["p95_ns"] - b["p95_ns"]
                p99_delta = None if b["p99_ns"] is None or x["p99_ns"] is None else x["p99_ns"] - b["p99_ns"]
                ok = (
                    p95_delta is not None and p95_delta <= 200_000_000 and
                    p99_delta is not None and p99_delta <= 500_000_000 and
                    row["latency"]["probe_loss_percent"] <= 1.0
                )
                item = {
                    "mode": mode, "scenario": scenario, "seed": seed,
                    "baseline_p95_ns": b["p95_ns"], "baseline_p99_ns": b["p99_ns"],
                    "p95_ns": x["p95_ns"], "p99_ns": x["p99_ns"],
                    "p95_delta_ns": p95_delta, "p99_delta_ns": p99_delta,
                    "probe_loss_percent": row["latency"]["probe_loss_percent"], "pass": ok,
                }
                rtt.append(item)
                if not ok:
                    errors.append(f"RTT gate failed: {item}")

    closures = {}
    for mode in ("normal", "game"):
        closures[mode] = {}
        for scenario in ("lossless", "5205", "5305"):
            sample_rows = [index.get((mode, scenario, seed)) for seed in (101, 202, 303)]
            sample_ok = all(
                r is not None and all(v == "PASS" for v in r.get("classifications", {}).values())
                for r in sample_rows
            )
            rtt_ok = True
            if scenario != "lossless":
                relevant = [x for x in rtt if x["mode"] == mode and x["scenario"] == scenario]
                rtt_ok = len(relevant) == 3 and all(x["pass"] for x in relevant)
            closures[mode][scenario] = {
                "sample_count": sum(r is not None for r in sample_rows),
                "all_sample_gates_pass": sample_ok,
                "rtt_gate_pass": rtt_ok,
                "pass": sample_ok and rtt_ok,
            }
            if not closures[mode][scenario]["pass"]:
                errors.append(f"scenario not closed: {mode}/{scenario} {closures[mode][scenario]}")

    result = {
        "schema": 1,
        "sample_count": len(index),
        "missing": missing,
        "rtt_comparisons": rtt,
        "scenario_closure": closures,
        "result": "PASS" if not errors else "FAIL",
        "errors": errors,
        "note": "PASS here closes only the 18 strict main-sample gate; target-rate 30min soak, specialty damage, patch A/B, module matrix and physical qualification remain separate.",
    }
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    Path(args.output).write_text(json.dumps(result, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("WBD_STRICT_WEAKNET_AGGREGATE " + json.dumps({
        "result": result["result"], "sample_count": result["sample_count"],
        "scenario_closure": closures, "errors": errors,
    }, sort_keys=True))
    raise SystemExit(0 if not errors else 1)


if __name__ == "__main__":
    main()
