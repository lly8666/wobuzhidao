#!/usr/bin/env python3
import argparse
import json
import math
import os
import struct
from collections import defaultdict

SCHEMA = "wbd-p5-https-measurement/v1"
BASE_SEED = 20260921
ONE_WAY_DELAY_NS = 300_000_000
SCENARIO_NS = 120_000_000_000
REQUEST_PERIOD_NS = 3_000_000_000
PHASES = [
    ("low5-initial", 0, 30_000_000_000, 5),
    ("high20", 30_000_000_000, 90_000_000_000, 20),
    ("low5-recovery", 90_000_000_000, 120_000_000_000, 5),
]
PHASE_BY_NAME = {name: (start, end, drop) for name, start, end, drop in PHASES}


def fail(msg):
    raise SystemExit(f"P5 weaknet validation failed: {msg}")


def load_json(path):
    with open(path, "r", encoding="utf-8") as f:
        return json.load(f)


def load_jsonl(path):
    out = []
    with open(path, "r", encoding="utf-8") as f:
        for lineno, raw in enumerate(f, 1):
            raw = raw.strip()
            if not raw:
                continue
            try:
                out.append(json.loads(raw))
            except json.JSONDecodeError as e:
                fail(f"{os.path.basename(path)}:{lineno}: {e}")
    return out


def percentile_nearest_rank(values, p):
    if not values:
        return None
    xs = sorted(values)
    rank = max(1, math.ceil((p / 100.0) * len(xs)))
    return xs[rank - 1]


def drop_score(seed, direction, phase, ordinal):
    direction_salt = 17 if direction == "s2c" else 0
    phase_salt = {"high20": 31, "low5-recovery": 61, "post": 83}.get(phase, 0)
    if ordinal <= 0:
        return 0
    return ((ordinal - 1) * 37 + seed % 100 + direction_salt + phase_salt) % 100


def phase_for_offset(ns):
    for name, start, end, _ in PHASES:
        if start <= ns < end:
            return name
    return "post"


def count_pcap_packets(path):
    with open(path, "rb") as f:
        header = f.read(24)
        if len(header) != 24:
            fail("pcap global header missing")
        magic = struct.unpack("<I", header[:4])[0]
        if magic != 0xA1B2C3D4:
            fail(f"pcap magic={magic:#x}")
        count = 0
        byte_total = 0
        while True:
            rec = f.read(16)
            if not rec:
                break
            if len(rec) != 16:
                fail("pcap truncated record header")
            _sec, _usec, incl, orig = struct.unpack("<IIII", rec)
            if incl != orig or incl == 0:
                fail("pcap capture/original length mismatch")
            payload = f.read(incl)
            if len(payload) != incl:
                fail("pcap truncated packet")
            count += 1
            byte_total += incl
        return count, byte_total


def cpu_busy(first, last):
    if not first or not last or len(first) != len(last):
        return None
    delta = [b - a for a, b in zip(first, last)]
    if any(x < 0 for x in delta):
        return None
    total = sum(delta)
    if total <= 0:
        return None
    idle = delta[3] if len(delta) > 3 else 0
    iowait = delta[4] if len(delta) > 4 else 0
    return max(0.0, min(1.0, (total - idle - iowait) / total))


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
    run_ordinal = scenario.get("run_ordinal")
    seed = scenario.get("seed")
    if run_ordinal not in (1, 2):
        fail(f"run ordinal={run_ordinal}")
    if seed != BASE_SEED + run_ordinal - 1:
        fail(f"seed={seed} run={run_ordinal}")
    if scenario.get("one_way_delay_ms") != 300 or scenario.get("duration_seconds") != 120:
        fail("300ms/120s scenario provenance")
    if scenario.get("phase_seconds") != [30, 60, 30] or scenario.get("drop_percent") != [5, 20, 5]:
        fail("30/60/30 or 5/20/5 scenario provenance")
    if (
        scenario.get("fec_profile") != "20:20"
        or scenario.get("fec_data_shards") != 20
        or scenario.get("fec_parity_shards") != 20
        or scenario.get("fec_flush_after_ns") != 8_000_000
        or scenario.get("fec_max_blocks") != 8
        or scenario.get("fec_profile_role") != "explicit-controlled-weaknet-capability-cost-scenario-not-production-default"
        or scenario.get("production_default_fec") != "off"
        or scenario.get("padding") != "off"
    ):
        fail("fixed FEC 20:20/padding-off scenario policy")

    transitions = [x for x in network if x.get("event") == "phase_transition"]
    expected_transitions = [
        ("low5-initial", 0, 5),
        ("high20", 30_000_000_000, 20),
        ("low5-recovery", 90_000_000_000, 5),
        ("end", 120_000_000_000, 0),
    ]
    if len(transitions) != 4:
        fail(f"phase transition count={len(transitions)}")
    for event, (name, t_ns, drop) in zip(transitions, expected_transitions):
        if event.get("schema") != SCHEMA or event.get("phase") != name or event.get("t_ns") != t_ns:
            fail(f"phase transition {name}")
        if event.get("drop_percent") != drop or event.get("one_way_delay_ns") != ONE_WAY_DELAY_NS or event.get("seed") != seed:
            fail(f"phase transition config {name}")
        if not isinstance(event.get("phase_start_unix_ns"), int) or event.get("phase_start_unix_ns") <= 0:
            fail(f"phase transition timestamp {name}")

    decisions = [x for x in network if x.get("event") == "network_decision" and x.get("phase") in PHASE_BY_NAME]
    deliveries = [x for x in network if x.get("event") == "network_delivery"]
    if not decisions:
        fail("no network decisions")
    delivery_by_key = {}
    for d in deliveries:
        key = (d.get("direction"), d.get("packet_id"))
        if key in delivery_by_key:
            fail(f"duplicate network delivery {key}")
        delivery_by_key[key] = d

    attempts = defaultdict(int)
    drops = defaultdict(int)
    wire_bytes = defaultdict(int)
    phase_ordinals = defaultdict(list)
    residency = defaultdict(list)
    max_decision_queue = defaultdict(int)
    for d in decisions:
        direction = d.get("direction")
        phase = d.get("phase")
        if direction not in ("c2s", "s2c"):
            fail(f"network direction={direction}")
        start, end, drop_pct = PHASE_BY_NAME[phase]
        t_ns = d.get("t_ns")
        if not isinstance(t_ns, int) or not (start <= t_ns < end):
            fail(f"decision phase timestamp {phase}/{direction}: {t_ns}")
        ordinal = d.get("phase_ordinal")
        if not isinstance(ordinal, int) or ordinal <= 0:
            fail("network phase ordinal")
        phase_ordinals[(phase, direction)].append(ordinal)
        expected_score = drop_score(seed, direction, phase, ordinal)
        if d.get("seed") != seed or d.get("drop_score") != expected_score or d.get("drop_percent") != drop_pct:
            fail(f"drop provenance {phase}/{direction}/{ordinal}")
        expected_drop = expected_score < drop_pct
        if d.get("dropped") is not expected_drop:
            fail(f"drop decision mismatch {phase}/{direction}/{ordinal}")
        if d.get("planned_delay_ns") != ONE_WAY_DELAY_NS or d.get("queue_enter_ns") != t_ns:
            fail("delay queue provenance")
        pkt_len = d.get("outer_packet_len")
        if not isinstance(pkt_len, int) or pkt_len < 40:
            fail("outer packet length in network raw data")
        attempts[(phase, direction)] += 1
        wire_bytes[direction] += pkt_len
        max_decision_queue[direction] = max(max_decision_queue[direction], d.get("queue_depth_after", 0))
        key = (direction, d.get("packet_id"))
        delivered = delivery_by_key.get(key)
        if expected_drop:
            drops[(phase, direction)] += 1
            if delivered is not None:
                fail(f"dropped packet was delivered {key}")
        else:
            if delivered is None:
                fail(f"non-dropped packet lacks delivery {key}")
            if delivered.get("delivery_error") not in ("", None):
                fail(f"network delivery error {key}: {delivered.get('delivery_error')}")
            if delivered.get("phase") != phase or delivered.get("phase_ordinal") != ordinal:
                fail(f"delivery identity {key}")
            r = delivered.get("queue_residency_ns")
            if not isinstance(r, int) or r < ONE_WAY_DELAY_NS:
                fail(f"delivery residency {key}={r}")
            if delivered.get("queue_leave_ns", 0) < delivered.get("queue_enter_ns", 0):
                fail(f"delivery queue timestamps {key}")
            residency[direction].append(r)

    for name, _, _, _ in PHASES:
        for direction in ("c2s", "s2c"):
            ords = phase_ordinals[(name, direction)]
            # network_decision records may be written out of JSONL order because
            # multiple runtime emitters can race after each link has assigned its
            # monotonic phase_ordinal. File order is not packet identity; require
            # the ordinal set to be exactly contiguous instead.
            if not ords or sorted(ords) != list(range(1, len(ords) + 1)):
                fail(f"phase ordinal sequence {name}/{direction}")
            if drops[(name, direction)] <= 0:
                fail(f"no actual injected drops {name}/{direction}")

    pcap_packets, _ = count_pcap_packets(args.pcap)
    outer_events = [x for x in events if x.get("event") == "outer_packet"]
    record_events = [x for x in events if x.get("event") == "tlslike_record"]
    if pcap_packets != len(outer_events) or pcap_packets <= 0:
        fail(f"capture loss evidence pcap={pcap_packets} outer_events={len(outer_events)}")
    for direction in ("c2s", "s2c"):
        if not any(x.get("direction") == direction for x in outer_events):
            fail(f"outer packet direction {direction}")
        if not any(x.get("direction") == direction for x in record_events):
            fail(f"TLS-like record direction {direction}")
    # The recorder samples t_ns before taking its shared writer mutex. Concurrent
    # Emit calls in the same direction can therefore be serialized in JSONL a
    # few microseconds out of sampled-time order, which makes the convenience
    # inter_arrival_ns field negative even though the raw timestamps are valid.
    # Treat t_ns as the timing authority and independently reconstruct
    # chronological intervals per direction. The serialized interval remains
    # schema/type evidence only; omitempty still represents exact zero.
    outer_times = defaultdict(list)
    record_times = defaultdict(list)
    for file_index, e in enumerate(outer_events):
        if not isinstance(e.get("outer_packet_len"), int) or e.get("outer_packet_len") <= 0:
            fail("outer packet length raw evidence")
        if "burst_id" not in e or "burst_wire_bytes" not in e:
            fail("outer burst raw evidence")
        direction = e.get("direction")
        if direction not in ("c2s", "s2c"):
            fail("outer packet direction raw evidence")
        t_ns = e.get("t_ns")
        if not isinstance(t_ns, int) or t_ns < 0:
            fail("outer timestamp raw evidence")
        serialized_gap = e.get("inter_arrival_ns", 0)
        if not isinstance(serialized_gap, int):
            fail("outer interval schema evidence")
        outer_times[direction].append((t_ns, file_index))

    for file_index, e in enumerate(record_events):
        if not isinstance(e.get("record_len"), int) or e.get("record_len") <= 0:
            fail("TLS-like record raw evidence")
        direction = e.get("direction")
        if direction not in ("c2s", "s2c"):
            fail("TLS-like record direction raw evidence")
        t_ns = e.get("t_ns")
        if not isinstance(t_ns, int) or t_ns < 0:
            fail("TLS-like record timestamp raw evidence")
        serialized_gap = e.get("inter_arrival_ns", 0)
        if not isinstance(serialized_gap, int):
            fail("TLS-like record interval schema evidence")
        record_times[direction].append((t_ns, file_index))

    for kind, by_direction in (("outer", outer_times), ("record", record_times)):
        for direction in ("c2s", "s2c"):
            samples = sorted(by_direction[direction])
            if not samples:
                fail(f"{kind} chronological interval samples {direction}")
            previous = None
            positive_gap = False
            for t_ns, _ in samples:
                if previous is not None:
                    gap = t_ns - previous
                    if gap < 0:
                        fail(f"{kind} chronological interval {direction}")
                    if gap > 0:
                        positive_gap = True
                previous = t_ns
            if len(samples) > 1 and not positive_gap:
                fail(f"{kind} chronological timing collapsed {direction}")

    request_samples = [x for x in requests_raw if x.get("event") == "https_request"]
    inventories = [x for x in requests_raw if x.get("event") == "application_inventory"]
    expected_request_count = SCENARIO_NS // REQUEST_PERIOD_NS
    if len(request_samples) != expected_request_count or len(inventories) != 1:
        fail(f"request sample/inventory count={len(request_samples)}/{len(inventories)}")
    request_samples.sort(key=lambda x: x.get("ordinal", 0))
    success_by_phase = defaultdict(int)
    rtts = []
    response_bytes = 0
    inner_c2s = inner_s2c = 0
    for index, r in enumerate(request_samples, 1):
        if r.get("schema") != SCHEMA or r.get("ordinal") != index:
            fail(f"request ordinal={index}")
        scheduled = (index - 1) * REQUEST_PERIOD_NS
        expected_phase = phase_for_offset(scheduled)
        if r.get("scheduled_start_ns") != scheduled or r.get("scheduled_phase") != expected_phase:
            fail(f"request schedule {index}")
        inner_c2s += int(r.get("inner_tls_wire_bytes_c2s", 0) or 0)
        inner_s2c += int(r.get("inner_tls_wire_bytes_s2c", 0) or 0)
        if r.get("success") is True:
            if r.get("http_status") != 200 or r.get("response_bytes") != r.get("expected_response_bytes"):
                fail(f"successful response identity {index}")
            if not r.get("body_sha256") or len(r.get("body_sha256")) != 64:
                fail(f"successful body hash {index}")
            rtt = r.get("rtt_ns")
            complete = r.get("request_complete_ns")
            if not isinstance(rtt, int) or rtt <= 0 or not isinstance(complete, int) or complete <= 0:
                fail(f"successful RTT timing {index}")
            rtts.append(rtt)
            success_by_phase[expected_phase] += 1
            response_bytes += r.get("response_bytes", 0)
        else:
            if not r.get("error_stage"):
                fail(f"failed request lacks stage {index}")

    inv = inventories[0]
    successes = sum(1 for r in request_samples if r.get("success") is True)
    app_loss = len(request_samples) - successes
    if inv.get("scheduled_requests") != len(request_samples) or inv.get("successful_requests") != successes or inv.get("application_loss") != app_loss:
        fail("application inventory loss recomputation")
    counts = inv.get("server_request_counts")
    if not isinstance(counts, dict):
        fail("server request counts missing")
    duplicates = sum(max(int(v) - 1, 0) for v in counts.values())
    if inv.get("application_duplicates") != duplicates or duplicates != 0:
        fail(f"application duplicate count={duplicates}")
    for name, _, _, _ in PHASES:
        if success_by_phase[name] <= 0:
            fail(f"no successful HTTPS request in phase {name}")

    streak = 0
    post5_complete_ns = None
    for r in request_samples:
        if r.get("scheduled_phase") != "low5-recovery":
            continue
        if r.get("success") is True:
            streak += 1
            if streak == 3:
                post5_complete_ns = r.get("request_complete_ns")
                break
        else:
            streak = 0
    if post5_complete_ns is None or post5_complete_ns < 90_000_000_000 or post5_complete_ns >= SCENARIO_NS:
        fail(f"post5 recovery could not be recomputed: {post5_complete_ns}")
    post5_recovery_ns = post5_complete_ns - 90_000_000_000

    final_inventory = [x for x in resources if x.get("event") == "final_inventory"]
    resource_samples = [x for x in resources if x.get("event") == "resource_sample"]
    if len(final_inventory) != 1:
        fail("final inventory count")
    final = final_inventory[0]
    if (
        final.get("fec_profile") != "20:20"
        or final.get("fec_data_shards") != 20
        or final.get("fec_parity_config") != 20
        or final.get("fec_flush_after_ns") != 8_000_000
        or final.get("fec_max_blocks") != 8
        or final.get("client_fec_enabled") is not True
        or final.get("server_fec_enabled") is not True
    ):
        fail("final FEC 20:20 policy")
    client_sources = int(final.get("client_source_shards", 0) or 0)
    client_parity = int(final.get("client_parity_shards", 0) or 0)
    server_sources = int(final.get("server_source_shards", 0) or 0)
    server_parity = int(final.get("server_parity_shards", 0) or 0)
    if min(client_sources, client_parity, server_sources, server_parity) <= 0:
        fail("FEC 20:20 shard inventory empty")
    if client_parity != client_sources or server_parity != server_sources:
        fail(f"settled 20:20 parity/source mismatch client={client_parity}/{client_sources} server={server_parity}/{server_sources}")
    if int(final.get("client_pending_sources", -1)) != 0 or int(final.get("server_pending_sources", -1)) != 0:
        fail("FEC encoder did not settle")
    if int(final.get("client_decoder_max_blocks", 0) or 0) != 8 or int(final.get("server_decoder_max_blocks", 0) or 0) != 8:
        fail("FEC decoder max-block bound")
    for key in ("client_decoder_in_flight", "server_decoder_in_flight"):
        value = int(final.get(key, -1))
        if value < 0 or value > 8:
            fail(f"FEC decoder in-flight bound {key}={value}")
    client_reconstruct = int(final.get("client_reconstruction_events", 0) or 0)
    client_recovered = int(final.get("client_recovered_sources", 0) or 0)
    server_reconstruct = int(final.get("server_reconstruction_events", 0) or 0)
    server_recovered = int(final.get("server_recovered_sources", 0) or 0)
    if min(client_reconstruct, client_recovered, server_reconstruct, server_recovered) < 0:
        fail("negative FEC recovery inventory")
    if client_reconstruct > client_recovered or server_reconstruct > server_recovered:
        fail("FEC reconstruction event/source relation")
    recovered_total = client_recovered + server_recovered
    reconstruct_total = client_reconstruct + server_reconstruct
    if recovered_total <= 0 or reconstruct_total <= 0:
        fail("weak-network run did not exercise FEC reconstruction")
    expired_missing_total = int(final.get("client_recovery_expired_missing_sources", 0) or 0) + int(final.get("server_recovery_expired_missing_sources", 0) or 0)
    if final.get("padding_bytes") != 0 or final.get("capture_loss_packets") != 0:
        fail("padding/capture loss separation")
    repair = final.get("repair_retransmits")
    if not isinstance(repair, int) or repair < 0:
        fail("repair inventory")

    numerator = sum(wire_bytes.values())
    denominator = inner_c2s + inner_s2c
    if numerator <= 0 or denominator <= 0:
        fail("wire amplification inventory empty")
    if final.get("outer_wire_bytes_c2s") != wire_bytes["c2s"] or final.get("outer_wire_bytes_s2c") != wire_bytes["s2c"]:
        fail("outer wire bytes do not match raw network decisions")
    if final.get("inner_tls_wire_bytes_c2s") != inner_c2s or final.get("inner_tls_wire_bytes_s2c") != inner_s2c:
        fail("inner TLS bytes do not match raw requests")
    if final.get("wire_amplification_numerator_bytes") != numerator or final.get("wire_amplification_denominator_bytes") != denominator:
        fail("wire amplification byte inventory")
    wire_amp = numerator / denominator
    if not math.isclose(final.get("wire_amplification", -1), wire_amp, rel_tol=1e-9, abs_tol=1e-12):
        fail("wire amplification ratio")

    manifest_app = manifest.get("application", {})
    if manifest_app.get("scheduled_requests") != len(request_samples) or manifest_app.get("successful_requests") != successes or manifest_app.get("loss") != app_loss or manifest_app.get("duplicates") != duplicates:
        fail("manifest application inventory")
    if manifest_app.get("inner_tls_wire_bytes_c2s") != inner_c2s or manifest_app.get("inner_tls_wire_bytes_s2c") != inner_s2c:
        fail("manifest inner TLS wire inventory")
    manifest_fec = manifest.get("fec", {})
    if manifest_fec.get("profile") != "20:20":
        fail("manifest FEC profile")
    manifest_fec_checks = {
        "client_source_shards": client_sources,
        "client_parity_shards": client_parity,
        "client_reconstruction_events": client_reconstruct,
        "client_recovered_sources": client_recovered,
        "server_source_shards": server_sources,
        "server_parity_shards": server_parity,
        "server_reconstruction_events": server_reconstruct,
        "server_recovered_sources": server_recovered,
    }
    for key, value in manifest_fec_checks.items():
        if int(manifest_fec.get(key, -1)) != value:
            fail(f"manifest FEC inventory {key}")
    capture = manifest.get("capture", {})
    if capture.get("capture_loss_packets") != 0:
        fail("manifest capture loss inventory")
    # The manifest is written while the runtime/tick goroutines are still alive
    # and before deferred shutdown closes the client/server. Its packet/record
    # counters are therefore an atomic live snapshot, not the final file size.
    # Final capture completeness is independently proved above by exact
    # PCAP-packet == outer-event equality. Require the manifest snapshot to be
    # a valid prefix count and require every later captured tail event to be
    # outside the prescribed 120s measurement window.
    manifest_outer = capture.get("outer_packets")
    manifest_records = capture.get("tlslike_record_events")
    if not isinstance(manifest_outer, int) or manifest_outer <= 0 or manifest_outer > len(outer_events):
        fail("manifest outer capture snapshot")
    if not isinstance(manifest_records, int) or manifest_records <= 0 or manifest_records > len(record_events):
        fail("manifest record capture snapshot")
    if capture.get("client_initial_syns") != 1:
        fail("manifest initial SYN inventory")
    for e in outer_events[manifest_outer:]:
        if not isinstance(e.get("t_ns"), int) or e.get("t_ns") < SCENARIO_NS:
            fail("outer capture tail entered measurement window")
    for e in record_events[manifest_records:]:
        if not isinstance(e.get("t_ns"), int) or e.get("t_ns") < SCENARIO_NS:
            fail("record capture tail entered measurement window")
    cost = manifest.get("cost", {})
    if cost.get("wire_amplification_numerator_bytes") != numerator or cost.get("wire_amplification_denominator_bytes") != denominator or not math.isclose(cost.get("wire_amplification", -1), wire_amp, rel_tol=1e-9, abs_tol=1e-12):
        fail("manifest wire cost")

    if len(resource_samples) < 90:
        fail(f"resource samples too sparse: {len(resource_samples)}")
    resource_samples.sort(key=lambda x: x.get("t_ns", 0))
    max_mem_alloc = max(int(x.get("mem_alloc_bytes", 0) or 0) for x in resource_samples)
    max_heap_alloc = max(int(x.get("heap_alloc_bytes", 0) or 0) for x in resource_samples)
    max_heap_inuse = max(int(x.get("heap_inuse_bytes", 0) or 0) for x in resource_samples)
    max_sys = max(int(x.get("sys_bytes", 0) or 0) for x in resource_samples)
    gc_delta = int(resource_samples[-1].get("num_gc", 0) or 0) - int(resource_samples[0].get("num_gc", 0) or 0)
    gc_pause_delta = int(resource_samples[-1].get("pause_total_ns", 0) or 0) - int(resource_samples[0].get("pause_total_ns", 0) or 0)
    if gc_delta < 0 or gc_pause_delta < 0:
        fail("GC counters regressed")
    resource_queue_peak = {
        "c2s": max(int(x.get("injection_queue_peak_c2s", 0) or 0) for x in resource_samples),
        "s2c": max(int(x.get("injection_queue_peak_s2c", 0) or 0) for x in resource_samples),
    }
    for direction in ("c2s", "s2c"):
        if resource_queue_peak[direction] < max_decision_queue[direction]:
            fail(f"resource queue peak under-reports decisions: {direction}")

    host_first = resource_samples[0].get("host_cpu", {})
    host_last = resource_samples[-1].get("host_cpu", {})
    if "cpu" not in host_first or "cpu" not in host_last:
        fail("aggregate host CPU counters missing")
    host_busy = cpu_busy(host_first["cpu"], host_last["cpu"])
    if host_busy is None:
        fail("aggregate host CPU busy cannot be recomputed")
    per_core_busy = {}
    for name, first in host_first.items():
        if name == "cpu" or name not in host_last:
            continue
        busy = cpu_busy(first, host_last[name])
        if busy is not None:
            per_core_busy[name] = busy
    if not per_core_busy:
        fail("per-core CPU counters missing")

    rtt_summary = {
        "samples": len(rtts),
        "p50_ns": percentile_nearest_rank(rtts, 50),
        "p95_ns": percentile_nearest_rank(rtts, 95),
        "p99_ns": percentile_nearest_rank(rtts, 99),
    }
    if not rtts:
        fail("no successful RTT samples")
    phase_drop = {}
    phase_pps = {}
    for name, start, end, _ in PHASES:
        phase_drop[name] = {}
        duration_s = (end - start) / 1e9
        phase_pps[name] = {}
        for direction in ("c2s", "s2c"):
            a = attempts[(name, direction)]
            d = drops[(name, direction)]
            phase_drop[name][direction] = {"attempts": a, "drops": d, "actual_drop_ratio": d / a}
            phase_pps[name][direction] = a / duration_s

    queue_residency = {}
    for direction in ("c2s", "s2c"):
        vals = residency[direction]
        if not vals:
            fail(f"no queue residency samples {direction}")
        queue_residency[direction] = {
            "samples": len(vals),
            "p50_ns": percentile_nearest_rank(vals, 50),
            "p95_ns": percentile_nearest_rank(vals, 95),
            "p99_ns": percentile_nearest_rank(vals, 99),
            "max_ns": max(vals),
            "peak_depth": resource_queue_peak[direction],
        }

    total_drop = sum(drops.values())
    total_attempts = sum(attempts.values())
    summary = {
        "schema": SCHEMA,
        "source_sha": args.source_sha,
        "scenario": "fec20x20-weaknet-300ms-120s-5-20-5",
        "run_ordinal": run_ordinal,
        "seed": seed,
        "phase_drop": phase_drop,
        "network_attempts": total_attempts,
        "network_drops": total_drop,
        "actual_drop_ratio": total_drop / total_attempts,
        "capture_loss_packets": 0,
        "fec_profile": "20:20",
        "fec": {
            "data_shards": 20,
            "parity_shards": 20,
            "client_source_shards": client_sources,
            "client_parity_shards": client_parity,
            "server_source_shards": server_sources,
            "server_parity_shards": server_parity,
            "reconstruction_events": reconstruct_total,
            "recovered_sources": recovered_total,
            "expired_missing_sources": expired_missing_total,
            "parity_to_source_ratio": (client_parity + server_parity) / (client_sources + server_sources),
            "recovered_per_100_sources": recovered_total * 100.0 / (client_sources + server_sources),
        },
        "repair_retransmits": repair,
        "padding_bytes": 0,
        "application": {
            "scheduled": len(request_samples),
            "success": successes,
            "loss": app_loss,
            "duplicates": duplicates,
            "response_bytes": response_bytes,
            "goodput_bps": response_bytes * 8 / 120.0,
        },
        "rtt": rtt_summary,
        "post5_recovery_ns": post5_recovery_ns,
        "wire_amplification_numerator_bytes": numerator,
        "wire_amplification_denominator_bytes": denominator,
        "wire_amplification": wire_amp,
        "pps": phase_pps,
        "queue": queue_residency,
        "resources": {
            "samples": len(resource_samples),
            "host_busy_ratio": host_busy,
            "per_core_busy_ratio": per_core_busy,
            "max_mem_alloc_bytes": max_mem_alloc,
            "max_heap_alloc_bytes": max_heap_alloc,
            "max_heap_inuse_bytes": max_heap_inuse,
            "max_sys_bytes": max_sys,
            "gc_cycles_delta": gc_delta,
            "gc_pause_ns_delta": gc_pause_delta,
        },
        "raw_capture": {"pcap_packets": pcap_packets, "outer_events": len(outer_events), "tlslike_record_events": len(record_events)},
        "verdict": "PASS",
    }
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(summary, f, indent=2, sort_keys=True)
        f.write("\n")

    print(
        f"WBD_P5_FEC2020_WEAKNET_5205_PASS source_sha={args.source_sha} run={run_ordinal} seed={seed} "
        f"duration=120s delay=300ms drop=5-20-5 attempts={total_attempts} drops={total_drop} "
        f"app_success={successes} app_loss={app_loss} app_duplicate=0 "
        f"rtt_p95_ns={rtt_summary['p95_ns']} post5_recovery_ns={post5_recovery_ns} "
        f"fec=20:20 recovered={recovered_total} recon={reconstruct_total} expired_missing={expired_missing_total} "
        f"fec_sources={client_sources + server_sources} fec_parity={client_parity + server_parity} "
        f"repair={repair} padding=off wire_amp={wire_amp:.6f}"
    )


if __name__ == "__main__":
    main()
