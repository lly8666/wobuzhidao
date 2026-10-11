#!/usr/bin/env python3
"""off-weaknet-v2: independent, non-retroactive FEC-off impaired-screen assessment.

Not an alternate performance dataplane, and never a replacement for the
immutable loss-tolerant-v1 reference. Without qualified matched comparison
and no-HOL evidence the result is MEASURED_NOT_QUALIFIED, never PASS_SCREEN.
"""
import argparse
import hashlib
import json
import math
import statistics
from pathlib import Path

VERSION = "off-weaknet-v2"
DIRECTIONS = ("c2s", "s2c")
STAGES = ("pre", "stress", "post")
SCENARIOS = {"5205": (0.05, 0.20, 0.05), "5305": (0.05, 0.30, 0.05)}
GIB = float(1024 ** 3)
RECOVERY_WINDOW_SECONDS = 5
PRE5_SECONDS = tuple(range(15, 30))
POST_START = 90
RECOVERY_OBSERVE_END = 120
RECOVERY_INITIAL_REVIEW_SECONDS = 10


def eligible(kind, parity, scenario):
    return kind == "fec-profile-screen" and int(parity) == 0 and scenario in SCENARIOS


def _median(xs):
    return statistics.median(xs) if xs else None


def _mad(xs):
    m = _median(xs)
    return statistics.median(abs(x - m) for x in xs) if m is not None else None


def _rss(resources):
    out = {"client": None, "server": None}
    for sample in resources:
        for role in out:
            v = ((sample.get("processes") or {}).get(role) or {}).get("status") or {}
            try:
                kb = int(str(v.get("VmHWM", "")).split()[0])
            except (ValueError, IndexError):
                continue
            out[role] = max(out[role] or 0, kb * 1024)
    return out


def _raw_stage_measurements(legacy):
    rows = {}
    probe = ((legacy.get("latency") or {}).get("probe_stage") or {})
    for direction in DIRECTIONS:
        rows[direction] = {}
        for stage in STAGES:
            v = (((legacy.get("generator") or {}).get(direction) or {}).get(stage) or {})
            p = probe.get(stage) or {}
            sent = v.get("sent_packets")
            received = v.get("unique_delivered_packets")
            pb = v.get("sent_bytes")
            ub = v.get("unique_delivered_bytes")
            rows[direction][stage] = {
                "business_sent_packets": sent,
                "business_first_unique_packets": received,
                "business_packet_loss_percent": v.get("packet_loss_percent"),
                "business_sent_bytes": pb,
                "business_first_unique_bytes": ub,
                "business_byte_loss_percent": v.get("byte_loss_percent"),
                "timely_first_unique_wall_goodput_mbps": v.get("wall_delivered_mbps_path_delay_aligned"),
                "eventual_first_unique_goodput_mbps": v.get("eventual_goodput_mbps"),
                "stage_probe_sent": p.get("sent"),
                "stage_probe_received": p.get("received"),
                "stage_probe_missing": max(0, p["sent"] - p["received"]) if
                    isinstance(p.get("sent"), int) and isinstance(p.get("received"), int) else None,
                "stage_probe_returned_only_p50_ns": None,
                "stage_probe_returned_only_p95_ns": p.get("p95_ns"),
                "stage_probe_returned_only_p99_ns": p.get("p99_ns"),
                "probe_note": "Returned-only probe latency cannot hide missing/timeouts; no stage p50 instrumentation",
            }
    return rows


def fragmentation_reference(mapping, manifest, measured):
    """Optional trusted exact per-stage distribution; absence is NOT guessed."""
    if mapping is None:
        return {"state": "UNAVAILABLE", "reason": "No validated per-size LINK fragmentation count map; app payload size or average shard count cannot substitute."}
    if mapping.get("schema") != 1 or mapping.get("source_sha") != manifest.get("source_sha") or \
            mapping.get("harness_sha") != manifest.get("harness_sha") or \
            (mapping.get("evidence") or {}).get("kind") not in ("authenticated_link_counts", "verified_same_source_mtu_budget"):
        return {"state": "INVALID_MAP", "reason": "fragment evidence provenance/schema mismatch"}
    result = {}
    for direction in DIRECTIONS:
        result[direction] = {}
        for stage, p in zip(STAGES, SCENARIOS[manifest["scenario"]]):
            rows = (((mapping.get("size_distribution") or {}).get(direction) or {}).get(stage))
            if not isinstance(rows, list) or not rows:
                return {"state": "INVALID_MAP", "reason": "Missing exact size/fragment map for " + direction + "/" + stage}
            total_packets = total_bytes = 0
            weighted_packets = weighted_bytes = 0.0
            for row in rows:
                count, size, parts = (row.get("business_packets"), row.get("business_bytes_per_packet"), row.get("link_fragments_per_business_packet"))
                if not all(type(x) is int and x > 0 for x in (count, size, parts)) or parts > 64:
                    return {"state": "INVALID_MAP", "reason": "Invalid exact shard map integer range"}
                total_packets += count
                total_bytes += count * size
                q = 1.0 - (1.0 - p) ** parts
                weighted_packets += count * q
                weighted_bytes += count * size * q
            actual = measured[direction][stage]
            if total_packets != actual["business_sent_packets"] or total_bytes != actual["business_sent_bytes"]:
                return {"state": "INVALID_MAP", "reason": "Size map does not reconcile actual sent packets and bytes for " + direction + "/" + stage}
            result[direction][stage] = {
                "iid_no_repair_packet_loss_reference_percent": weighted_packets / total_packets * 100.0,
                "iid_no_repair_byte_loss_reference_percent": weighted_bytes / total_bytes * 100.0,
            }
    return {"state": "REFERENCE_ONLY", "source": mapping["evidence"]["kind"],
            "limitations": "Assumes independent equal-probability outer first-attempt shard erasures, no finite repair/reorder or correlated loss. Never a PASS/FAIL threshold.",
            "by_direction_stage": result}


def _five_second_relative_recovery(sources, queue_age_seconds=None):
    """Predeclared 15..29s reference, 5s moving post window; no per-1s luck gate."""
    out = {}
    for direction in DIRECTIONS:
        sender, receiver = ("biz", "target") if direction == "c2s" else ("target", "biz")
        sent = ((sources.get(sender) or {}).get("stats") or {}).get("sent_packets_by_second") or []
        recv = ((sources.get(receiver) or {}).get("stats") or {}).get("recv_packets_by_second") or []
        wall = ((sources.get(receiver) or {}).get("stats") or {}).get("recv_wall_bytes_by_second") or []
        if min(map(len, (sent, recv, wall))) < RECOVERY_OBSERVE_END:
            out[direction] = {"state": "INSUFFICIENT", "reason": "missing full 120s per-second offered/first-unique timeline"}
            continue
        def values(start, end):
            offered = sum(sent[start:end])
            arrived = sum(recv[start:end])
            return {"goodput_mbps": sum(wall[start:end]) * 8 / 1e6 / (end-start),
                    "missing_percent": 100.0 * (1.0 - arrived / offered) if offered else None}
        pre = [values(i, i+RECOVERY_WINDOW_SECONDS) for i in range(15, 26)]
        gb = [v["goodput_mbps"] for v in pre]
        ls = [v["missing_percent"] for v in pre if v["missing_percent"] is not None]
        if len(ls) < 5 or len(gb) < 5:
            out[direction] = {"state": "INSUFFICIENT", "reason": "less than five valid pre5 steady windows"}
            continue
        pre_good = _median(gb)
        pre_loss = _median(ls)
        # Frozen tolerances: 5% throughput/2 percentage point missing noise
        # floor, or three pre5 median absolute deviations, whichever higher.
        good_tol = max(0.05 * pre_good, 3 * _mad(gb))
        loss_tol = max(2.0, 3 * _mad(ls))
        last_queue = queue_age_seconds or []
        queue_baseline = [v for v in last_queue[15:30] if isinstance(v, (float, int)) and v >= 0]
        queue_pre = _median(queue_baseline) if len(queue_baseline) >= 5 else None
        queue_tol = max(200_000_000, 3 * _mad(queue_baseline)) if queue_pre is not None else None
        selected = None
        last = None
        for start in range(POST_START, RECOVERY_OBSERVE_END-RECOVERY_WINDOW_SECONDS+1):
            v = values(start, start + RECOVERY_WINDOW_SECONDS)
            qwindow = [k for k in last_queue[start:start+RECOVERY_WINDOW_SECONDS] if isinstance(k, (float, int)) and k >= 0]
            qmax = max(qwindow) if len(qwindow) == RECOVERY_WINDOW_SECONDS else None
            within = (v["goodput_mbps"] >= pre_good-good_tol and
                      v["missing_percent"] is not None and
                      v["missing_percent"] <= pre_loss+loss_tol)
            if queue_pre is not None:
                within = within and qmax is not None and qmax <= queue_pre+queue_tol
            v.update(start_second=start, queue_max_ns=qmax, within_pre5_band=within)
            last = v
            if within:
                selected = v
                break
        out[direction] = {
            "state": "RECOVERED_IN_INITIAL_10S" if selected is not None and
                selected["start_second"] - POST_START <= RECOVERY_INITIAL_REVIEW_SECONDS else
                ("LATE_RECOVERY" if selected is not None else "NOT_CONFIRMED_WITHIN_OBSERVATION"),
            "pre5_reference_seconds": [15, 30], "window_seconds": RECOVERY_WINDOW_SECONDS,
            "goodput_pre5_mbps": pre_good, "goodput_tolerance_mbps": good_tol,
            "missing_pre5_percent": pre_loss, "missing_tolerance_percentage_points": loss_tol,
            "queue_pre5_ns": queue_pre, "queue_tolerance_ns": queue_tol,
            "queue_observation": "OBSERVED" if queue_pre is not None else "UNAVAILABLE",
            "selected_post_window": selected, "last_post_window": last,
            "interpretation": "relative pre5 recovery screening; neither a guaranteed 5% business loss threshold nor an automatic qualification"}
    return out


def _comparison(current, baselines):
    if not baselines:
        return {"state": "UNAVAILABLE", "reason": "No independent same-harness, same-config, comparable-runner baseline receipts; lack of regression evidence cannot qualify."}
    peers = []
    issues = []
    for base in baselines:
        if base.get("policy_version") != VERSION or base.get("product_source_sha") == current["product_source_sha"] or \
           base.get("harness_sha") != current["harness_sha"] or \
           base.get("scenario") != current["scenario"] or base.get("mode") != current["mode"] or \
           base.get("seed") != current["seed"] or base.get("config") != current["config"] or \
           base.get("runner") != current["runner"]:
            issues.append("reference mismatch: source/harness/config/seed/runner/policy")
            continue
        peers.append(base)
        for direction in DIRECTIONS:
            for stage in STAGES:
                a = current["measurements"][direction][stage]["timely_first_unique_wall_goodput_mbps"]
                b = base["measurements"][direction][stage]["timely_first_unique_wall_goodput_mbps"]
                if isinstance(a,(int,float)) and isinstance(b,(int,float)) and b > 0 and a / b < 0.95:
                    issues.append("goodput >5% lower " + direction + "/" + stage)
        ac, bc = current["resource"]["cpu_seconds_per_delivered_gib"], base["resource"]["cpu_seconds_per_delivered_gib"]
        if ac is not None and bc is not None and bc > 0 and ac / bc > 1.10:
            issues.append("CPU/GiB >10% higher (needs repeat and noise adjudication)")
        ap = (current.get("latency") or {}).get("global_returned_probe_p99_ns")
        bp = (base.get("latency") or {}).get("global_returned_probe_p99_ns")
        if isinstance(ap,(int,float)) and isinstance(bp,(int,float)) and ap > bp+max(0.1*bp,5_000_000):
            issues.append("p99 >10% and >5ms higher (missing probe counts must also be reviewed)")
    if not peers:
        return {"state": "INCOMPARABLE", "issues": issues}
    return {"state": "PAIRED_SCREEN_REVIEW" if issues else "NO_SINGLE_PAIR_REGRESSION_FLAG",
            "paired_count": len(peers), "review_flags": sorted(set(issues)),
            "qualification_limit": "Require repeated independent comparable samples, bounded noise and no-HOL/queue evidence; a single pair is never PASS."}


def assess(legacy, manifest, biz, target, resources=(), baseline_receipts=(),
           fragment_mapping=None, queue_age_seconds=None):
    if not eligible("fec-profile-screen", (manifest.get("config") or {}).get("fec_parity", -1),
                    manifest.get("scenario")):
        raise ValueError("off-weaknet-v2 only accepts off + damaged profile-screen")
    if manifest.get("source_sha") != legacy.get("source_sha") or manifest.get("scenario") != legacy.get("scenario") or \
       manifest.get("mode") != legacy.get("mode"):
        raise ValueError("source/identity mismatch")
    measured = _raw_stage_measurements(legacy)
    cpu = (legacy.get("resource") or {}).get("process_cpu_seconds") or {}
    bytes_delivered = sum(
        (measured[d][s]["business_first_unique_bytes"] or 0)
        for d in DIRECTIONS for s in STAGES)
    total_cpu = sum(cpu.get(p,0) for p in ("client","server")) if all(isinstance(cpu.get(p),(int,float)) for p in ("client","server")) else None
    resource = {
        "process_cpu_seconds": cpu,
        "generator_cpu_seconds": {role: (data.get("generator_process_cpu_seconds")) for role,data in (("biz",biz),("target",target))},
        "cpu_seconds_per_delivered_gib": total_cpu / (bytes_delivered / GIB) if total_cpu is not None and bytes_delivered else None,
        "process_rss_peak_bytes": _rss(resources),
        "socket_drop_max": (legacy.get("resource") or {}).get("socket_drop_max"),
        "link_drop_delta": (legacy.get("resource") or {}).get("link_drop_delta"),
        "memory_note": "Process VmHWM via original resource samples, not shared cgroup memory.current",
    }
    raw_performance_errors = list((legacy.get("errors") or {}).get("performance") or [])
    c = legacy.get("classifications") or {}
    hard = {"correctness":c.get("CORRECTNESS"),"capture":c.get("CAPTURE"),
            "input":c.get("INPUT_VALIDITY"),"environment":c.get("ENVIRONMENT")}
    continuity = legacy.get("longest_zero_delivery_seconds") or {}
    severe_stalls = [role for role,v in continuity.items() if isinstance(v,(int,float)) and v >= 2]
    failures = []
    if c.get("CORRECTNESS") != "PASS" or c.get("CAPTURE") != "PASS" or legacy.get("profile_errors"):
        failures.append("CORRECTNESS_OR_CAPTURE_OR_EFFECTIVE_FEC_PROFILE")
    if severe_stalls:
        failures.append("AT_LEAST_TWO_FULL_SECONDS_NO_BUSINESS_DELIVERY")
    if c.get("INPUT_VALIDITY") != "PASS":
        failures.append("INPUT_INVALID")
    if c.get("ENVIRONMENT") != "PASS":
        failures.append("ENVIRONMENT_NOT_QUALIFIED")
    rec = _five_second_relative_recovery({"biz":biz,"target":target},queue_age_seconds)
    incomplete_rec = [k for k,v in rec.items() if v["state"]!="RECOVERED_IN_INITIAL_10S"]
    limit = {"CAPTURE":hard["capture"],"CORRECTNESS":hard["correctness"],
             "INPUT_VALIDITY":hard["input"],"ENVIRONMENT":hard["environment"],
             "CONTINUITY": "FAIL" if severe_stalls else ("PASS" if len(continuity)==2 else "INSUFFICIENT")}
    if "CORRECTNESS_OR_CAPTURE_OR_EFFECTIVE_FEC_PROFILE" in failures or severe_stalls:
        qualification="FAIL_HARD_GATE"
    elif "INPUT_INVALID" in failures:
        qualification="INVALID_INPUT"
    elif "ENVIRONMENT_NOT_QUALIFIED" in failures or resource["socket_drop_max"] not in (0,None):
        qualification="CAPACITY_LIMITED"
    elif incomplete_rec:
        qualification="REVIEW_REQUIRED"
    else:
        qualification="MEASURED_NOT_QUALIFIED"
    result = {
        "schema":1, "policy_version":VERSION, "qualification_status":qualification,
        "product_source_sha":manifest["source_sha"], "harness_sha":manifest.get("harness_sha"),
        "mode":manifest["mode"],"scenario":manifest["scenario"], "seed":manifest.get("seed"),
        "runner":manifest.get("runner"),"config":manifest.get("config"),
        "legacy_reference":{
            "analysis_version":legacy.get("analysis_version"),
            "classifications":c, "profile_errors":legacy.get("profile_errors"),
            "errors":legacy.get("errors"),
            "immutable_history_note":"This is a copy of frozen original result, not a reclassification or historical rerun.",
        },
        "hard_gate_classifications":limit,
        "critical_failures":failures,
        "measurements":measured,
        "continuity": {"longest_zero_delivery_seconds":continuity,
                       "severe_stall_roles":severe_stalls},
        "latency": {
            "global_returned_probe_p50_ns":((biz.get("stats") or {}).get("probe_rtt_p50_ns")),
            "global_returned_probe_p95_ns":((biz.get("stats") or {}).get("probe_rtt_p95_ns")),
            "global_returned_probe_p99_ns":((biz.get("stats") or {}).get("probe_rtt_p99_ns")),
            "total_probe_sent":((biz.get("stats") or {}).get("probe_sent")),
            "total_probe_timeouts":((biz.get("stats") or {}).get("probe_timeouts")),
            "note":"p99 conditional on returned probes; loss/timeout counts must accompany it. Missing per-business stage p50 is explicitly unmeasured.",
        },
        "resource":resource,
        "wire": {"legacy_cost_by_direction":legacy.get("cost"),
                 "note":"original recorded wire amplification, repair and control overhead, not added benefits"},
        "drain": legacy.get("drain"),
        "relative_pre5_recovery":rec,
        "fragmentation_reference":fragmentation_reference(fragment_mapping,manifest,measured),
        "threshold_revisions":{
            "removed_as_off_impairment_hard_gates":[
                "business_packet_loss <= configured_outer_loss",
                "timely_wall_goodput >= target*(1-p)*0.99",
                "post5 arbitrary three 1s loss<=5%, throughput>=target*0.95*0.99"],
            "preserved":"legacy original analysis, lossless/nonoff/formal20, correctness, capture, input, environmental, >=2s continuity, finite queues",
            "no_automatic_pass":"Valid unpaired measurements are MEASURED_NOT_QUALIFIED; no old FAIL is rewritten."},
    }
    result["comparison"]=_comparison(result,baseline_receipts)
    if result["comparison"].get("review_flags") and qualification=="MEASURED_NOT_QUALIFIED":
        result["qualification_status"]="REVIEW_REQUIRED"
    result["qualification_limitations"]=[
        "No standalone automatic performance PASS. Confirm no cross-business HOL and queue pressure separately.",
        "Missing trusted per-size LINK fragment mapping is UNAVAILABLE, not inferred from application payload.",
        "Post-repair deficit is not physical first-send loss. Returned-only probe latency never masks timeouts.",
    ]
    return result


def run_artifact(root, output, baselines=(), fragment_map=None):
    from check_strict_weaknet_loss_tolerant_v1 import read_jsonl, stage_bounds, queue_age_at
    legacy_path=root / "summary-loss-tolerant-v1.json"
    legacy=json.loads(legacy_path.read_text())
    manifest=json.loads((root/"manifest.json").read_text())
    biz=json.loads((root/"biz.json").read_text())
    target=json.loads((root/"target.json").read_text())
    resources=read_jsonl(root/"resources.jsonl")
    events=read_jsonl(root/"stage-events.jsonl")
    bounds=stage_bounds(events)
    diags={"client":read_jsonl(root/"client-diag.jsonl"),
           "server":read_jsonl(root/"server-diag.jsonl")}
    start_ns=bounds["pre"][0]
    queue=[queue_age_at(diags,start_ns+int((s+0.5)*1e9)) for s in range(120)]
    b=[json.loads(Path(x).read_text()) for x in baselines]
    mapping=json.loads(Path(fragment_map).read_text()) if fragment_map else None
    v=assess(legacy,manifest,biz,target,resources,b,mapping,queue)
    v["analyzer_sha256"]=hashlib.sha256(Path(__file__).read_bytes()).hexdigest()
    v["legacy_receipt_sha256"]=hashlib.sha256(legacy_path.read_bytes()).hexdigest()
    v["independent_v2_receipt"]=str(output.name)
    output.write_text(json.dumps(v,indent=2,sort_keys=True)+"\n")
    return v


def main():
    ap=argparse.ArgumentParser()
    ap.add_argument("--artifact-dir",required=True,type=Path)
    ap.add_argument("--output",required=True,type=Path)
    ap.add_argument("--baseline-v2",action="append",default=[])
    ap.add_argument("--verified-fragment-map",type=Path)
    args=ap.parse_args()
    result=run_artifact(args.artifact_dir,args.output,args.baseline_v2,args.verified_fragment_map)
    print("WBD_OFF_WEAKNET_V2 "+json.dumps({
        "source":result["product_source_sha"],"harness":result["harness_sha"],
        "qualification":result["qualification_status"],"hard":result["hard_gate_classifications"],
        "recovery":{k:v["state"] for k,v in result["relative_pre5_recovery"].items()}}))
    # Reporting a valid sample is not performance qualification. Exit nonzero
    # until a separate controlled evidence decision explicitly qualifies it.
    raise SystemExit(1)


if __name__=="__main__":
    main()
