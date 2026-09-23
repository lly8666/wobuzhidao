#!/usr/bin/env python3
"""Artifact-only aggregate for loss-tolerant-v1.

This program never starts a workload. It combines summaries produced by separate
workflow runs and supplies the cross-run lossless RTT baseline gate.
"""
import argparse
import json
from collections import defaultdict
from pathlib import Path

P95_LIMIT_NS = 200_000_000
P99_LIMIT_NS = 500_000_000

def identity(row):
    return (
        row.get("source_sha"),
        row.get("mode"),
        str(row.get("seed")),
        float(row.get("target_mbps_each_direction", 0) or 0),
        int(row.get("lanes", 0) or 0),
    )

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", required=True)
    ap.add_argument("--output", required=True)
    ap.add_argument("--required-repeats", type=int, default=3)
    args = ap.parse_args()

    rows = []
    for path in Path(args.root).rglob("summary-loss-tolerant-v1.json"):
        row = json.loads(path.read_text(encoding="utf-8"))
        if row.get("analysis_version") != "loss-tolerant-v1":
            continue
        row["_artifact_summary"] = str(path)
        rows.append(row)

    by_id_scenario = {(identity(r), r.get("scenario")): r for r in rows}
    sample_results = []
    errors = []
    for row in rows:
        scenario = row.get("scenario")
        if scenario == "lossless":
            continue
        base = by_id_scenario.get((identity(row), "lossless"))
        item = {
            "identity": identity(row),
            "scenario": scenario,
            "summary": row["_artifact_summary"],
            "lossless_summary": base["_artifact_summary"] if base else None,
            "stages": {},
            "pass": True,
        }
        if base is None:
            item["pass"] = False
            errors.append(f"missing independent lossless baseline for {identity(row)} scenario={scenario}")
        else:
            for stage in ("pre", "stress", "post"):
                cur = ((row.get("latency") or {}).get("probe_stage") or {}).get(stage) or {}
                ref = ((base.get("latency") or {}).get("probe_stage") or {}).get(stage) or {}
                stage_row = {
                    "lossy_sent": cur.get("sent"),
                    "lossy_received": cur.get("received"),
                    "lossless_sent": ref.get("sent"),
                    "lossless_received": ref.get("received"),
                }
                stage_ok = True
                for key, limit in (("p95_ns", P95_LIMIT_NS), ("p99_ns", P99_LIMIT_NS)):
                    cv, bv = cur.get(key), ref.get(key)
                    delta = None if cv is None or bv is None else cv - bv
                    stage_row[key] = cv
                    stage_row["lossless_" + key] = bv
                    stage_row["delta_" + key] = delta
                    if delta is None or delta > limit:
                        stage_ok = False
                stage_row["pass"] = stage_ok
                item["stages"][stage] = stage_row
                item["pass"] = item["pass"] and stage_ok
        classes = row.get("classifications") or {}
        if any(classes.get(k) != "PASS" for k in ("CORRECTNESS", "INPUT_VALIDITY", "CAPTURE", "ENVIRONMENT", "PERFORMANCE")):
            item["pass"] = False
        if not item["pass"]:
            errors.append(f"sample gate failed identity={identity(row)} scenario={scenario}")
        sample_results.append(item)

    repeats = defaultdict(set)
    for row in rows:
        classes = row.get("classifications") or {}
        if all(classes.get(k) == "PASS" for k in ("CORRECTNESS", "INPUT_VALIDITY", "CAPTURE", "ENVIRONMENT", "PERFORMANCE")):
            repeats[(row.get("source_sha"), row.get("mode"), row.get("scenario"))].add(str(row.get("seed")))
    repeat_status = {}
    for key, seeds in sorted(repeats.items(), key=str):
        repeat_status[str(key)] = {"seeds": sorted(seeds), "count": len(seeds), "pass": len(seeds) >= args.required_repeats}

    out = {
        "schema": 1,
        "analysis_version": "loss-tolerant-v1",
        "kind": "artifact-only-aggregate",
        "sample_count": len(rows),
        "required_repeats": args.required_repeats,
        "sample_results": sample_results,
        "repeat_status": repeat_status,
        "errors": errors,
        "result": "PASS" if rows and not errors else "FAIL",
    }
    Path(args.output).write_text(json.dumps(out, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("WBD_LOSS_TOLERANT_V1_AGGREGATE " + json.dumps({"result": out["result"], "samples": len(rows)}, sort_keys=True))
    raise SystemExit(0 if out["result"] == "PASS" else 1)

if __name__ == "__main__":
    main()
