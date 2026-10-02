#!/usr/bin/env python3
"""Read independent summaries; never start a measurement workload."""
import argparse
import json
import math
import re
from collections import defaultdict
from pathlib import Path

P95_LIMIT_NS = 200_000_000
P99_LIMIT_NS = 500_000_000
CLASSES = ("CORRECTNESS", "INPUT_VALIDITY", "CAPTURE", "ENVIRONMENT", "PERFORMANCE")
CONFIGS = {"normal": (10.0, 1), "game": (3.0, 4)}
SCENARIOS = ("lossless", "5205", "5305")


def identity(row):
    return (row.get("source_sha"), row.get("mode"), str(row.get("seed")),
            float(row.get("target_mbps_each_direction", 0) or 0),
            int(row.get("lanes", 0) or 0))


def probe_valid(probe):
    sent, received = probe.get("sent"), probe.get("received")
    if (type(sent) is not int or type(received) is not int
            or not 0 < received <= sent):
        return False
    values = [probe.get(k) for k in ("p95_ns", "p99_ns")]
    return (all(type(v) in (int, float) and math.isfinite(v) and v >= 0 for v in values)
            and values[0] <= values[1])


def aggregate(rows, required_repeats=3, source_sha=None, seeds=None,
              require_run_receipts=False, load_errors=()):
    errors = list(load_errors)
    if required_repeats < 1:
        errors.append("required_repeats must be positive")
    seed_set = set(map(str, seeds)) if seeds is not None else None
    if seed_set is not None and len(seed_set) != required_repeats:
        errors.append("expected seed count must equal required_repeats")
    sources = {r.get("source_sha") for r in rows}
    if len(sources) != 1:
        errors.append("campaign must contain exactly one source SHA")
    sha = source_sha if source_sha is not None else next(iter(sources), None)
    if not isinstance(sha, str) or not re.fullmatch(r"[0-9a-f]{40}", sha):
        errors.append("campaign source SHA must be a full lowercase SHA")
    if sources != {sha}:
        errors.append("summary source does not match campaign source")

    by_id_scenario = {}
    repeats = defaultdict(set)
    seen_runs = set()
    for row in rows:
        path = row.get("_artifact_summary", "<memory>")
        try:
            ident = identity(row)
        except (TypeError, ValueError, OverflowError):
            errors.append(f"invalid identity: {path}")
            continue
        scenario = row.get("scenario")
        key = (ident, scenario)
        if key in by_id_scenario:
            errors.append(f"duplicate sample identity: {key}")
        else:
            by_id_scenario[key] = row
        if (row.get("analysis_version") != "loss-tolerant-v1"
                or row.get("mode") not in CONFIGS or scenario not in SCENARIOS):
            errors.append(f"unexpected analyzer/mode/scenario: {path}")
        elif (ident[3], ident[4]) != CONFIGS[row["mode"]]:
            errors.append(f"unexpected target rate/lanes: {path}")
        if seed_set is not None and ident[2] not in seed_set:
            errors.append(f"unexpected seed: {path}")
        classes = row.get("classifications") or {}
        valid = all(classes.get(k) == "PASS" for k in CLASSES)
        if not valid:
            errors.append(f"sample classifications failed: {path}")
        for stage in ("pre", "stress", "post"):
            probe = ((row.get("latency") or {}).get("probe_stage") or {}).get(stage) or {}
            if not probe_valid(probe):
                valid = False
                errors.append(f"invalid/missing probe stage={stage}: {path}")
        if valid:
            repeats[(row.get("mode"), scenario)].add(ident[2])
        if require_run_receipts:
            receipt = row.get("_run_receipt") or {}
            run_id = receipt.get("id")
            if (type(run_id) is not int or run_id <= 0 or run_id in seen_runs
                    or receipt.get("head_sha") != sha
                    or receipt.get("event") != "workflow_dispatch"
                    or receipt.get("run_attempt") != 1
                    or receipt.get("status") != "completed"
                    or receipt.get("conclusion") != "success"):
                errors.append(f"invalid/non-independent run receipt: {path}")
            seen_runs.add(run_id)

    sample_results = []
    for (ident, scenario), row in by_id_scenario.items():
        if scenario == "lossless":
            continue
        base = by_id_scenario.get((ident, "lossless"))
        item = {"identity": ident, "scenario": scenario,
                "summary": row.get("_artifact_summary"),
                "lossless_summary": base.get("_artifact_summary") if base else None,
                "stages": {}, "pass": True}
        if base is None:
            item["pass"] = False
            errors.append(f"missing independent lossless baseline: {ident} scenario={scenario}")
        else:
            for stage in ("pre", "stress", "post"):
                cur = ((row.get("latency") or {}).get("probe_stage") or {}).get(stage) or {}
                ref = ((base.get("latency") or {}).get("probe_stage") or {}).get(stage) or {}
                stage_row = {"lossy_sent": cur.get("sent"), "lossy_received": cur.get("received"),
                             "lossless_sent": ref.get("sent"), "lossless_received": ref.get("received")}
                stage_ok = probe_valid(cur) and probe_valid(ref)
                for key, limit in (("p95_ns", P95_LIMIT_NS), ("p99_ns", P99_LIMIT_NS)):
                    cv, bv = cur.get(key), ref.get(key)
                    delta = cv - bv if probe_valid(cur) and probe_valid(ref) else None
                    stage_row.update({key: cv, "lossless_" + key: bv, "delta_" + key: delta})
                    if delta is None or delta > limit:
                        stage_ok = False
                stage_row["pass"] = stage_ok
                item["stages"][stage] = stage_row
                item["pass"] = item["pass"] and stage_ok
        if not item["pass"]:
            errors.append(f"RTT baseline gate failed: {ident} scenario={scenario}")
        sample_results.append(item)

    repeat_status = {}
    for mode in CONFIGS:
        for scenario in SCENARIOS:
            found = repeats[(mode, scenario)]
            ok = len(found) >= required_repeats and (seed_set is None or found == seed_set)
            repeat_status[f"{mode}/{scenario}"] = {"seeds": sorted(found), "count": len(found), "pass": ok}
            if not ok:
                errors.append(f"incomplete passing repeats: {mode}/{scenario} seeds={sorted(found)}")
    expected_count = len(CONFIGS) * len(SCENARIOS) * required_repeats
    if seed_set is not None and len(rows) != expected_count:
        errors.append(f"expected {expected_count} independent samples, found {len(rows)}")
    return {"schema": 1, "analysis_version": "loss-tolerant-v1", "aggregate_revision": 2,
            "kind": "artifact-only-aggregate", "source_sha": sha, "sample_count": len(rows),
            "required_repeats": required_repeats, "expected_seeds": sorted(seed_set) if seed_set else None,
            "require_run_receipts": require_run_receipts, "sample_results": sample_results,
            "repeat_status": repeat_status, "errors": errors,
            "result": "PASS" if rows and not errors else "FAIL"}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--root", required=True)
    ap.add_argument("--output", required=True)
    ap.add_argument("--required-repeats", type=int, default=3)
    ap.add_argument("--source-sha")
    ap.add_argument("--seeds", help="exact comma-separated campaign seeds")
    ap.add_argument("--require-run-receipts", action="store_true")
    args = ap.parse_args()
    rows, load_errors = [], []
    for path in sorted(Path(args.root).rglob("summary-loss-tolerant-v1.json")):
        try:
            row = json.loads(path.read_text(encoding="utf-8"))
            if not isinstance(row, dict):
                raise ValueError("summary must be an object")
            row["_artifact_summary"] = str(path)
            if args.require_run_receipts:
                row["_run_receipt"] = json.loads((path.parent / "run-receipt.json").read_text(encoding="utf-8"))
            rows.append(row)
        except (ValueError, OSError) as exc:
            load_errors.append(f"unreadable summary/receipt {path}: {exc}")
    out = aggregate(rows, args.required_repeats, args.source_sha,
                    args.seeds.split(",") if args.seeds else None,
                    args.require_run_receipts, load_errors)
    Path(args.output).write_text(json.dumps(out, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("WBD_LOSS_TOLERANT_V1_AGGREGATE " + json.dumps({"result": out["result"], "samples": len(rows)}, sort_keys=True))
    raise SystemExit(0 if out["result"] == "PASS" else 1)


if __name__ == "__main__":
    main()
