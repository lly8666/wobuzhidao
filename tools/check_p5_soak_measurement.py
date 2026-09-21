#!/usr/bin/env python3
import argparse
import json
import math
import os
import statistics
import struct
import sys
from collections import defaultdict

SCHEMA = "wbd-p5-https-measurement/v1"
DURATION_S = 31 * 60
DURATION_NS = DURATION_S * 1_000_000_000
ONE_WAY_DELAY_NS = 300_000_000
DROP_PERCENT = 5
REQUEST_PERIOD_NS = 10_000_000_000
EXPECTED_REQUESTS = DURATION_S // 10
EXPECTED_LANES = 2
MIN_ROTATION_ADVANCES = 15
BASE_SEED = 20260935


def fail(msg):
    print(f"WBD_P5_SOAK_FAIL {msg}", file=sys.stderr)
    raise SystemExit(1)


def load_json(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def load_jsonl(path):
    out = []
    with open(path, "r", encoding="utf-8") as f:
        for lineno, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                out.append(json.loads(line))
            except json.JSONDecodeError as e:
                fail(f"{os.path.basename(path)}:{lineno}: {e}")
    return out


def percentile_nearest_rank(values, pct):
    if not values:
        return 0
    values = sorted(values)
    rank = max(1, math.ceil((pct / 100.0) * len(values)))
    return values[rank - 1]


def pcap_packet_count(path):
    with open(path, "rb") as f:
        gh = f.read(24)
        if len(gh) != 24:
            fail("pcap global header missing")
        magic = struct.unpack("<I", gh[:4])[0]
        if magic not in (0xA1B2C3D4, 0xA1B23C4D):
            fail(f"pcap magic={magic:#x}")
        count = 0
        while True:
            hdr = f.read(16)
            if not hdr:
                break
            if len(hdr) != 16:
                fail("pcap truncated record header")
            _, _, incl_len, orig_len = struct.unpack("<IIII", hdr)
            if incl_len != orig_len:
                fail("pcap capture/original length mismatch")
            pkt = f.read(incl_len)
            if len(pkt) != incl_len:
                fail("pcap truncated packet")
            count += 1
        return count


def drop_score(seed, direction, ordinal):
    direction_salt = 17 if direction == "s2c" else 0
    return ((ordinal - 1) * 37 + seed % 100 + direction_salt) % 100


def cpu_busy(first, last):
    if not isinstance(first, list) or not isinstance(last, list) or len(first) < 4 or len(last) < 4:
        return None
    n = min(len(first), len(last))
    delta = [int(last[i]) - int(first[i]) for i in range(n)]
    if any(x < 0 for x in delta):
        return None
    total = sum(delta)
    if total <= 0:
        return None
    idle = delta[3] + (delta[4] if n > 4 else 0)
    return (total - idle) / total


def minute_floor_medians(samples):
    by_minute = defaultdict(list)
    for x in samples:
        t_ns = int(x.get("t_ns", 0) or 0)
        if t_ns < 0 or t_ns > DURATION_NS + 10_000_000_000:
            continue
        minute = min(30, t_ns // 60_000_000_000)
        value = int(x.get("heap_inuse_bytes", 0) or 0)
        if value > 0:
            by_minute[int(minute)].append(value)
    floors = {m: min(vals) for m, vals in by_minute.items() if vals}
    middle = [floors[m] for m in range(10, 20) if m in floors]
    late = [floors[m] for m in range(21, 31) if m in floors]
    if len(middle) < 8 or len(late) < 8:
        fail(f"memory plateau windows incomplete middle={len(middle)} late={len(late)}")
    middle_median = statistics.median(middle)
    late_median = statistics.median(late)
    allowed_growth = max(8 * 1024 * 1024, middle_median * 0.50)
    if late_median > middle_median + allowed_growth:
        fail(
            f"memory plateau failed middle_floor_median={middle_median} "
            f"late_floor_median={late_median} allowed_growth={allowed_growth}"
        )
    return floors, middle_median, late_median, allowed_growth


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--manifest", required=True)
    ap.add_argument("--events", required=True)
    ap.add_argument("--pcap", required=True)
    ap.add_argument("--requests", required=True)
    ap.add_argument("--network", required=True)
    ap.add_argument("--resources", required=True)
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    if len(args.source_sha) != 40:
        fail("source SHA must be exact 40 hex")

    manifest = load_json(args.manifest)
    events = load_jsonl(args.events)
    requests_raw = load_jsonl(args.requests)
    network = load_jsonl(args.network)
    resources = load_jsonl(args.resources)

    if manifest.get("schema") != SCHEMA or manifest.get("source_sha") != args.source_sha or manifest.get("harness_sha") != args.source_sha:
        fail("manifest schema/source/harness SHA")
    scenario = manifest.get("scenario", {})
    run = int(scenario.get("run_ordinal", 0) or 0)
    seed = int(scenario.get("seed", 0) or 0)
    if run not in (1, 2) or seed != BASE_SEED + run - 1:
        fail(f"run/seed provenance run={run} seed={seed}")
    if scenario.get("duration_seconds") != DURATION_S or scenario.get("one_way_delay_ms") != 300 or scenario.get("drop_percent") != DROP_PERCENT:
        fail("31m/300ms/5pct soak provenance")
    if scenario.get("desired_lanes") != EXPECTED_LANES or scenario.get("rotation_interval_seconds") != 60:
        fail("soak lane/rotation provenance")
    if scenario.get("fec_profile") != "off" or scenario.get("production_default_fec") != "off" or scenario.get("padding") != "off":
        fail("soak FEC/padding policy")

    transitions = [x for x in network if x.get("event") == "phase_transition"]
    if len(transitions) != 2:
        fail(f"phase transition count={len(transitions)}")
    start, end = transitions
    if start.get("phase") != "sustained5" or start.get("t_ns") != 0 or start.get("drop_percent") != 5 or start.get("one_way_delay_ns") != ONE_WAY_DELAY_NS or start.get("seed") != seed:
        fail("sustained5 transition provenance")
    if end.get("phase") != "end" or end.get("t_ns") != DURATION_NS or end.get("drop_percent") != 0:
        fail("soak end transition provenance")

    decisions = [x for x in network if x.get("event") == "network_decision" and x.get("phase") == "sustained5"]
    deliveries = [x for x in network if x.get("event") == "network_delivery" and x.get("phase") == "sustained5"]
    if not decisions:
        fail("no soak network decisions")
    delivered_by_key = {}
    residency = defaultdict(list)
    for d in deliveries:
        key = (d.get("direction"), d.get("packet_id"))
        if key in delivered_by_key:
            fail(f"duplicate network delivery {key}")
        delivered_by_key[key] = d
        r = int(d.get("queue_residency_ns", 0) or 0)
        if r < ONE_WAY_DELAY_NS:
            fail(f"delivery residency below 300ms key={key} residency={r}")
        residency[d.get("direction")].append(r)

    attempts = defaultdict(int)
    drops = defaultdict(int)
    wire_bytes = defaultdict(int)
    ordinals = defaultdict(list)
    for d in decisions:
        direction = d.get("direction")
        if direction not in ("c2s", "s2c"):
            fail(f"network direction={direction}")
        ordinal = int(d.get("phase_ordinal", 0) or 0)
        if ordinal <= 0:
            fail("network phase ordinal")
        score = drop_score(seed, direction, ordinal)
        if d.get("seed") != seed or d.get("drop_percent") != DROP_PERCENT or d.get("drop_score") != score:
            fail(f"drop provenance {direction}/{ordinal}")
        expected_drop = score < DROP_PERCENT
        if d.get("dropped") is not expected_drop:
            fail(f"drop decision mismatch {direction}/{ordinal}")
        if d.get("planned_delay_ns") != ONE_WAY_DELAY_NS:
            fail("planned delay provenance")
        attempts[direction] += 1
        wire_bytes[direction] += int(d.get("outer_packet_len", 0) or 0)
        ordinals[direction].append(ordinal)
        key = (direction, d.get("packet_id"))
        if expected_drop:
            drops[direction] += 1
            if key in delivered_by_key:
                fail(f"dropped packet was delivered {key}")
        else:
            delivered = delivered_by_key.get(key)
            if delivered is None:
                fail(f"non-dropped packet lacks delivery {key}")
            if delivered.get("delivery_error"):
                fail(f"network delivery error {key}: {delivered.get('delivery_error')}")
    for direction in ("c2s", "s2c"):
        if attempts[direction] < 1000 or drops[direction] <= 0:
            fail(f"insufficient sustained-loss evidence {direction} attempts={attempts[direction]} drops={drops[direction]}")
        if sorted(ordinals[direction]) != list(range(1, attempts[direction] + 1)):
            fail(f"network ordinal sequence {direction}")
        expected_drops = sum(1 for i in range(1, attempts[direction] + 1) if drop_score(seed, direction, i) < DROP_PERCENT)
        if drops[direction] != expected_drops:
            fail(f"drop count recomputation {direction}")

    outer_events = [x for x in events if x.get("event") == "outer_packet"]
    record_events = [x for x in events if x.get("event") == "tlslike_record"]
    pcap_packets = pcap_packet_count(args.pcap)
    if pcap_packets != len(outer_events):
        fail(f"capture loss pcap={pcap_packets} outer_events={len(outer_events)}")
    if not record_events:
        fail("no TLS-like record events")

    request_samples = [x for x in requests_raw if x.get("event") == "https_request"]
    inventories = [x for x in requests_raw if x.get("event") == "application_inventory"]
    if len(inventories) != 1:
        fail("application inventory count")
    if len(request_samples) < EXPECTED_REQUESTS - 1:
        fail(f"too few scheduled soak requests={len(request_samples)} expected={EXPECTED_REQUESTS}")
    rtts = []
    send_lags = []
    successes = 0
    failure_streak = 0
    max_failure_streak = 0
    response_bytes = 0
    inner_c2s = 0
    inner_s2c = 0
    flow_ids = set()
    for index, r in enumerate(request_samples, 1):
        if int(r.get("ordinal", 0) or 0) != index:
            fail(f"request ordinal={index}")
        scheduled = int(r.get("scheduled_start_ns", -1))
        if scheduled != (index - 1) * REQUEST_PERIOD_NS:
            fail(f"request schedule {index} got={scheduled}")
        send_lags.append(max(0, int(r.get("send_lag_ns", 0) or 0)))
        inner_c2s += int(r.get("inner_tls_wire_bytes_c2s", 0) or 0)
        inner_s2c += int(r.get("inner_tls_wire_bytes_s2c", 0) or 0)
        flow_id = int(r.get("business_flow_id", 0) or 0)
        if flow_id:
            flow_ids.add(flow_id)
        if r.get("success"):
            successes += 1
            failure_streak = 0
            rtt = int(r.get("rtt_ns", 0) or 0)
            if rtt <= 0:
                fail(f"successful RTT missing request={index}")
            rtts.append(rtt)
            response_bytes += int(r.get("response_bytes", 0) or 0)
            if not r.get("body_sha256"):
                fail(f"successful body hash missing request={index}")
        else:
            failure_streak += 1
            max_failure_streak = max(max_failure_streak, failure_streak)
            if not r.get("error_stage"):
                fail(f"failed request lacks stage {index}")
    max_failure_streak = max(max_failure_streak, failure_streak)
    if max_failure_streak > 3:
        fail(f"unexpected sustained business interruption max_failure_streak={max_failure_streak}")
    if successes == 0 or len(flow_ids) < max(1, successes // 2):
        fail(f"insufficient independent business-flow exercise success={successes} flow_ids={len(flow_ids)}")

    inv = inventories[0]
    duplicates = int(inv.get("application_duplicates", -1))
    if duplicates != 0:
        fail(f"application duplicate count={duplicates}")
    if int(inv.get("attempted_requests", -1)) != len(request_samples) or int(inv.get("successful_requests", -1)) != successes:
        fail("application inventory request counts")

    finals = [x for x in resources if x.get("event") == "final_inventory"]
    samples = [x for x in resources if x.get("event") == "resource_sample"]
    if len(finals) != 1:
        fail("final inventory count")
    final = finals[0]
    if final.get("fec_profile") != "off" or int(final.get("padding_bytes", -1)) != 0:
        fail("final FEC/padding policy")
    if int(final.get("client_tcp_flows", -1)) != 0 or int(final.get("server_tcp_flows", -1)) != 0:
        fail("stop-flow TCP retirement")
    cowner = final.get("client_owner", {})
    sowner = final.get("server_owner", {})
    for side, owner in (("client", cowner), ("server", sowner)):
        if int(owner.get("BusinessFlows", owner.get("business_flows", -1))) != 0:
            fail(f"{side} business flows did not retire")
        active = int(owner.get("ActiveLogicalLanes", owner.get("active_logical_lanes", -1)))
        physical = int(owner.get("PhysicalLanes", owner.get("physical_lanes", -1)))
        retiring = int(owner.get("Retiring", owner.get("retiring", -1)))
        if active != EXPECTED_LANES or physical != EXPECTED_LANES or retiring != 0:
            fail(f"{side} final lane state active={active} physical={physical} retiring={retiring}")
    client_rotations = int(final.get("client_rotation_advances", 0) or 0)
    server_rotations = int(final.get("server_rotation_advances", 0) or 0)
    if client_rotations < MIN_ROTATION_ADVANCES or server_rotations < MIN_ROTATION_ADVANCES:
        fail(f"rotation coverage client={client_rotations} server={server_rotations}")

    if len(samples) < 800:
        fail(f"resource samples too sparse={len(samples)}")
    samples.sort(key=lambda x: int(x.get("t_ns", 0) or 0))
    floors, middle_floor, late_floor, plateau_allowance = minute_floor_medians(samples)
    max_heap_alloc = max(int(x.get("heap_alloc_bytes", 0) or 0) for x in samples)
    max_heap_inuse = max(int(x.get("heap_inuse_bytes", 0) or 0) for x in samples)
    max_sys = max(int(x.get("sys_bytes", 0) or 0) for x in samples)
    gc_delta = int(samples[-1].get("num_gc", 0) or 0) - int(samples[0].get("num_gc", 0) or 0)
    pause_delta = int(samples[-1].get("pause_total_ns", 0) or 0) - int(samples[0].get("pause_total_ns", 0) or 0)
    if gc_delta < 0 or pause_delta < 0:
        fail("GC counters regressed")
    first_cpu = samples[0].get("host_cpu", {})
    last_cpu = samples[-1].get("host_cpu", {})
    host_busy = cpu_busy(first_cpu.get("cpu"), last_cpu.get("cpu"))
    if host_busy is None:
        fail("aggregate host CPU counters missing")
    per_core = {}
    for key in sorted(set(first_cpu) & set(last_cpu)):
        if key == "cpu":
            continue
        v = cpu_busy(first_cpu[key], last_cpu[key])
        if v is not None:
            per_core[key] = v
    if not per_core:
        fail("per-core CPU counters missing")

    queue = {}
    for direction in ("c2s", "s2c"):
        vals = residency[direction]
        if not vals:
            fail(f"no queue residency samples {direction}")
        queue[direction] = {
            "samples": len(vals),
            "p50_ns": percentile_nearest_rank(vals, 50),
            "p95_ns": percentile_nearest_rank(vals, 95),
            "p99_ns": percentile_nearest_rank(vals, 99),
            "max_ns": max(vals),
            "peak_depth": max(int(x.get(f"injection_queue_peak_{direction}", 0) or 0) for x in samples),
        }

    total_attempts = attempts["c2s"] + attempts["s2c"]
    total_drops = drops["c2s"] + drops["s2c"]
    outer_wire = wire_bytes["c2s"] + wire_bytes["s2c"]
    inner_total = inner_c2s + inner_s2c
    wire_amp = outer_wire / inner_total if inner_total else 0.0
    manifest_cost = manifest.get("cost", {})
    if int(manifest_cost.get("wire_amplification_numerator_bytes", -1)) != outer_wire or int(manifest_cost.get("wire_amplification_denominator_bytes", -1)) != inner_total:
        fail("manifest wire amplification bytes")
    if not math.isclose(float(manifest_cost.get("wire_amplification", -1)), wire_amp, rel_tol=1e-9, abs_tol=1e-12):
        fail("manifest wire amplification ratio")

    rtt_summary = {
        "samples": len(rtts),
        "p50_ns": percentile_nearest_rank(rtts, 50),
        "p95_ns": percentile_nearest_rank(rtts, 95),
        "p99_ns": percentile_nearest_rank(rtts, 99),
        "max_ns": max(rtts) if rtts else 0,
    }
    send_lag_summary = {
        "p50_ns": percentile_nearest_rank(send_lags, 50),
        "p95_ns": percentile_nearest_rank(send_lags, 95),
        "p99_ns": percentile_nearest_rank(send_lags, 99),
        "max_ns": max(send_lags) if send_lags else 0,
    }

    summary = {
        "schema": SCHEMA,
        "source_sha": args.source_sha,
        "scenario": "p5-new-version-31m-sustained-loss-rotation-retirement-memory",
        "run_ordinal": run,
        "seed": seed,
        "duration_seconds": DURATION_S,
        "network": {
            "attempts": total_attempts,
            "drops": total_drops,
            "drop_ratio": total_drops / total_attempts,
            "c2s": {"attempts": attempts["c2s"], "drops": drops["c2s"]},
            "s2c": {"attempts": attempts["s2c"], "drops": drops["s2c"]},
            "aggregate_pps": total_attempts / DURATION_S,
            "outer_wire_mbps": outer_wire * 8 / DURATION_S / 1_000_000,
            "queue": queue,
        },
        "application": {
            "scheduled": EXPECTED_REQUESTS,
            "attempted": len(request_samples),
            "success": successes,
            "loss": len(request_samples) - successes,
            "duplicates": duplicates,
            "max_failure_streak": max_failure_streak,
            "response_bytes": response_bytes,
            "goodput_mbps": response_bytes * 8 / DURATION_S / 1_000_000,
            "inner_tls_mbps": inner_total * 8 / DURATION_S / 1_000_000,
            "distinct_business_flow_ids": len(flow_ids),
            "rtt": rtt_summary,
            "send_lag": send_lag_summary,
        },
        "lifecycle": {
            "client_rotation_advances": client_rotations,
            "server_rotation_advances": server_rotations,
            "lifecycle_error_count": int(final.get("lifecycle_error_count", 0) or 0),
            "client_tcp_flows_final": 0,
            "server_tcp_flows_final": 0,
            "business_flows_final": 0,
        },
        "memory_plateau": {
            "minute_floor_heap_inuse": floors,
            "middle_minutes_10_19_median_bytes": middle_floor,
            "late_minutes_21_30_median_bytes": late_floor,
            "allowed_growth_bytes": plateau_allowance,
            "late_minus_middle_bytes": late_floor - middle_floor,
            "verdict": "PASS",
        },
        "resources": {
            "samples": len(samples),
            "host_busy_ratio": host_busy,
            "per_core_busy_ratio": per_core,
            "max_heap_alloc_bytes": max_heap_alloc,
            "max_heap_inuse_bytes": max_heap_inuse,
            "max_sys_bytes": max_sys,
            "gc_cycles_delta": gc_delta,
            "gc_pause_ns_delta": pause_delta,
            "scope": "observational-only-no-cross-vm-absolute-gate",
        },
        "capture": {
            "pcap_packets": pcap_packets,
            "outer_events": len(outer_events),
            "tlslike_record_events": len(record_events),
            "capture_loss_packets": 0,
        },
        "wire_amplification": wire_amp,
        "verdict": "PASS",
    }
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(summary, f, indent=2, sort_keys=True)
        f.write("\n")

    print(
        f"WBD_P5_SOAK_PASS source_sha={args.source_sha} run={run} seed={seed} duration={DURATION_S}s "
        f"attempts={total_attempts} drops={total_drops} app_success={successes} app_loss={len(request_samples)-successes} "
        f"duplicate=0 max_failure_streak={max_failure_streak} rtt_p95_ns={rtt_summary['p95_ns']} "
        f"rtt_p99_ns={rtt_summary['p99_ns']} rotations={client_rotations} lifecycle_errors={summary['lifecycle']['lifecycle_error_count']} "
        f"heap_middle={int(middle_floor)} heap_late={int(late_floor)} host_busy={host_busy:.6f} "
        f"inner_tls_mbps={summary['application']['inner_tls_mbps']:.6f} outer_mbps={summary['network']['outer_wire_mbps']:.6f} "
        f"wire_amp={wire_amp:.6f} fec=off padding=off"
    )


if __name__ == "__main__":
    main()
