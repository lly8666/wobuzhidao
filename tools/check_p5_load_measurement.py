#!/usr/bin/env python3
import argparse
import json
import math
import struct
from collections import defaultdict

SCHEMA = "wbd-p5-https-measurement/v1"
DURATION_NS = 15_000_000_000
RATES = [5, 10, 15, 20, 40]
SIZES = [1024, 8 * 1024, 32 * 1024]
BASE_SEED = 20260925
MIN_ACHIEVEMENT_RATIO = 0.95


def fail(msg):
    raise SystemExit(f"WBD_P5_LOAD_FAIL {msg}")


def read_jsonl(path):
    out = []
    with open(path, "r", encoding="utf-8") as f:
        for line_no, line in enumerate(f, 1):
            line = line.strip()
            if not line:
                continue
            try:
                out.append(json.loads(line))
            except json.JSONDecodeError as e:
                fail(f"json {path}:{line_no}: {e}")
    return out


def count_pcap_packets(path):
    count = 0
    total_bytes = 0
    with open(path, "rb") as f:
        hdr = f.read(24)
        if len(hdr) != 24:
            fail("pcap header")
        magic = struct.unpack("<I", hdr[:4])[0]
        if magic != 0xA1B2C3D4:
            fail(f"pcap magic={magic:#x}")
        while True:
            rec = f.read(16)
            if not rec:
                break
            if len(rec) != 16:
                fail("pcap record header")
            _, _, incl, orig = struct.unpack("<IIII", rec)
            if incl != orig or incl <= 0:
                fail("pcap packet length")
            payload = f.read(incl)
            if len(payload) != incl:
                fail("pcap packet truncated")
            count += 1
            total_bytes += incl
    return count, total_bytes


def percentile_nearest_rank(values, pct):
    if not values:
        return None
    vals = sorted(values)
    rank = max(1, math.ceil(len(vals) * pct / 100))
    return vals[rank - 1]


def cpu_busy(first, last):
    if not isinstance(first, list) or not isinstance(last, list):
        return None
    n = min(len(first), len(last))
    if n < 4:
        return None
    deltas = [last[i] - first[i] for i in range(n)]
    if any(x < 0 for x in deltas):
        return None
    total = sum(deltas)
    if total <= 0:
        return None
    idle = deltas[3] + (deltas[4] if n > 4 else 0)
    return (total - idle) / total


def expected_seed(rate, run):
    return BASE_SEED + RATES.index(rate) * 2 + run - 1


def build_plan(rate):
    target_bps = rate * 1_000_000
    cumulative_bits = 0
    ordinal = 1
    out = []
    while True:
        scheduled_ns = cumulative_bits * 1_000_000_000 // target_bps
        if scheduled_ns >= DURATION_NS:
            break
        size = SIZES[(ordinal - 1) % len(SIZES)]
        out.append((ordinal, size, scheduled_ns))
        cumulative_bits += size * 2 * 8
        ordinal += 1
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--manifest", required=True)
    ap.add_argument("--events", required=True)
    ap.add_argument("--pcap", required=True)
    ap.add_argument("--load", required=True)
    ap.add_argument("--resources", required=True)
    ap.add_argument("--source-sha", required=True)
    ap.add_argument("--output", required=True)
    args = ap.parse_args()

    with open(args.manifest, "r", encoding="utf-8") as f:
        manifest = json.load(f)
    events = read_jsonl(args.events)
    load = read_jsonl(args.load)
    resources = read_jsonl(args.resources)

    if manifest.get("schema") != SCHEMA or manifest.get("source_sha") != args.source_sha or manifest.get("harness_sha") != args.source_sha:
        fail("manifest schema/source/harness SHA")
    scenario = manifest.get("scenario", {})
    rate = scenario.get("target_mbps")
    run = scenario.get("run_ordinal")
    seed = scenario.get("seed")
    if rate not in RATES or run not in (1, 2):
        fail(f"rate/run {rate}/{run}")
    if seed != expected_seed(rate, run):
        fail(f"seed={seed} expected={expected_seed(rate, run)}")
    if scenario.get("duration_seconds") != 15 or scenario.get("target_scope") != "aggregate-inner-application-payload-c2s-plus-s2c":
        fail("duration/target scope")
    if scenario.get("payload_size_cycle_bytes") != SIZES:
        fail("payload size cycle")
    if scenario.get("fec_profile") != "off" or scenario.get("production_default_fec") != "off" or scenario.get("padding") != "off":
        fail("FEC/padding policy")
    if scenario.get("network_injection") != "none":
        fail("unexpected network injection")

    start_ns = scenario.get("load_start_event_ns")
    window_end_ns = scenario.get("load_window_end_event_ns")
    complete_ns = scenario.get("load_complete_event_ns")
    if not isinstance(start_ns, int) or not isinstance(window_end_ns, int) or not isinstance(complete_ns, int):
        fail("load event timestamps")
    if window_end_ns - start_ns != DURATION_NS or complete_ns < start_ns:
        fail("load event timing")

    plan = build_plan(rate)
    offered_bytes = sum(size * 2 for _, size, _ in plan)
    tx = [x for x in load if x.get("event") == "load_transaction"]
    inventory = [x for x in load if x.get("event") == "application_inventory"]
    if len(inventory) != 1:
        fail("application inventory count")
    inv = inventory[0]
    if inv.get("offered_transactions") != len(plan) or inv.get("offered_aggregate_payload_bytes") != offered_bytes:
        fail("offered plan inventory")

    tx.sort(key=lambda x: x.get("ordinal", 0))
    rtts = []
    lags = []
    inner_c2s = 0
    inner_s2c = 0
    actual_c2s_payload = 0
    actual_s2c_payload = 0
    successes = 0
    send_failures = 0
    for i, sample in enumerate(tx):
        if sample.get("schema") != SCHEMA:
            fail("load sample schema")
        ordinal = sample.get("ordinal")
        if not isinstance(ordinal, int) or ordinal != i + 1:
            fail(f"accepted ordinal sequence {ordinal} at {i+1}")
        expected_ordinal, expected_size, expected_sched = plan[i]
        if ordinal != expected_ordinal or sample.get("size_bytes") != expected_size or sample.get("scheduled_start_ns") != expected_sched:
            fail(f"load schedule sample {ordinal}")
        if sample.get("accepted") is not True:
            fail(f"accepted flag {ordinal}")
        lag = sample.get("send_lag_ns", 0)
        if not isinstance(lag, int) or lag < 0:
            fail(f"send lag {ordinal}")
        lags.append(lag)
        inner_c2s += int(sample.get("inner_tls_wire_bytes_c2s", 0) or 0)
        inner_s2c += int(sample.get("inner_tls_wire_bytes_s2c", 0) or 0)
        if sample.get("send_failure") is True:
            send_failures += 1
        if sample.get("request_bytes"):
            actual_c2s_payload += int(sample.get("request_bytes"))
        if sample.get("success") is True:
            successes += 1
            if sample.get("http_status") != 200 or sample.get("response_bytes") != expected_size:
                fail(f"successful response identity {ordinal}")
            actual_s2c_payload += int(sample.get("response_bytes"))
            rtt = sample.get("rtt_ns")
            complete = sample.get("request_complete_ns")
            if not isinstance(rtt, int) or rtt <= 0 or not isinstance(complete, int) or complete <= 0:
                fail(f"RTT sample {ordinal}")
            rtts.append(rtt)
        elif not sample.get("error_stage"):
            fail(f"failed transaction lacks stage {ordinal}")

    accepted = len(tx)
    app_loss = accepted - successes
    skipped = len(plan) - accepted
    if inv.get("accepted_transactions") != accepted or inv.get("successful_transactions") != successes:
        fail("accepted/success inventory")
    if inv.get("application_loss") != app_loss or inv.get("send_failures") != send_failures or inv.get("skipped_slots") != skipped:
        fail("loss/send-failure/skipped inventory")
    if inv.get("actual_c2s_payload_bytes") != actual_c2s_payload or inv.get("actual_s2c_payload_bytes") != actual_s2c_payload:
        fail("actual payload inventory")
    actual_aggregate = actual_c2s_payload + actual_s2c_payload
    if inv.get("actual_aggregate_payload_bytes") != actual_aggregate:
        fail("aggregate payload inventory")

    counts = inv.get("server_request_counts")
    if not isinstance(counts, dict):
        fail("server request counts")
    duplicates = sum(max(int(v) - 1, 0) for v in counts.values())
    if inv.get("application_duplicates") != duplicates:
        fail("duplicate inventory")
    if duplicates != 0:
        fail(f"application duplicates={duplicates}")
    if send_failures != 0 or app_loss != 0:
        fail(f"application failure send={send_failures} loss={app_loss}")

    achievement_ratio = actual_aggregate / offered_bytes
    accepted_ratio = int(inv.get("accepted_aggregate_payload_bytes", 0)) / offered_bytes
    if achievement_ratio < MIN_ACHIEVEMENT_RATIO:
        fail(f"CAPACITY_LIMITED target={rate} achievement={achievement_ratio:.6f}")
    if accepted_ratio < MIN_ACHIEVEMENT_RATIO:
        fail(f"INJECTION_UNDERRUN target={rate} accepted={accepted_ratio:.6f}")

    app = manifest.get("application", {})
    manifest_checks = {
        "offered_transactions": len(plan),
        "accepted_transactions": accepted,
        "successful_transactions": successes,
        "loss": app_loss,
        "duplicates": duplicates,
        "send_failures": send_failures,
        "skipped_slots": skipped,
        "offered_aggregate_payload_bytes": offered_bytes,
        "actual_c2s_payload_bytes": actual_c2s_payload,
        "actual_s2c_payload_bytes": actual_s2c_payload,
        "actual_aggregate_payload_bytes": actual_aggregate,
        "inner_tls_wire_bytes_c2s": inner_c2s,
        "inner_tls_wire_bytes_s2c": inner_s2c,
    }
    for key, value in manifest_checks.items():
        if app.get(key) != value:
            fail(f"manifest application {key}: {app.get(key)} != {value}")

    pcap_packets, pcap_bytes = count_pcap_packets(args.pcap)
    outer_events = [x for x in events if x.get("event") == "outer_packet"]
    record_events = [x for x in events if x.get("event") == "tlslike_record"]
    if pcap_packets != len(outer_events) or pcap_packets <= 0:
        fail(f"capture completeness pcap={pcap_packets} events={len(outer_events)}")
    capture = manifest.get("capture", {})
    if capture.get("capture_loss_packets") != 0 or capture.get("client_initial_syns") != 1:
        fail("capture loss/outer SYN")
    if not isinstance(capture.get("outer_packets"), int) or capture.get("outer_packets") <= 0 or capture.get("outer_packets") > len(outer_events):
        fail("capture outer snapshot")
    if not isinstance(capture.get("tlslike_record_events"), int) or capture.get("tlslike_record_events") <= 0 or capture.get("tlslike_record_events") > len(record_events):
        fail("capture record snapshot")

    workload_outer = []
    workload_records = []
    for e in outer_events:
        t_ns = e.get("t_ns")
        if isinstance(t_ns, int) and start_ns <= t_ns <= complete_ns:
            workload_outer.append(e)
    for e in record_events:
        t_ns = e.get("t_ns")
        if isinstance(t_ns, int) and start_ns <= t_ns <= complete_ns:
            workload_records.append(e)
    if not workload_outer or not workload_records:
        fail("workload raw evidence empty")
    if not any(x.get("direction") == "c2s" for x in workload_outer) or not any(x.get("direction") == "s2c" for x in workload_outer):
        fail("outer direction evidence")
    if not any(x.get("direction") == "c2s" for x in workload_records) or not any(x.get("direction") == "s2c" for x in workload_records):
        fail("record direction evidence")
    outer_wire = sum(int(x.get("outer_packet_len", 0) or 0) for x in workload_outer)
    if outer_wire <= 0 or inner_c2s + inner_s2c <= 0:
        fail("wire amplification inventory")
    wire_amp = outer_wire / (inner_c2s + inner_s2c)

    finals = [x for x in resources if x.get("event") == "final_inventory"]
    samples = [x for x in resources if x.get("event") == "resource_sample"]
    if len(finals) != 1:
        fail("final inventory count")
    final = finals[0]
    if final.get("fec_profile") != "off" or final.get("client_fec_enabled") is not False or final.get("server_fec_enabled") is not False:
        fail("final FEC policy")
    zero_fec = [
        "client_source_shards", "client_parity_shards", "client_reconstruction_events", "client_recovered_sources",
        "server_source_shards", "server_parity_shards", "server_reconstruction_events", "server_recovered_sources",
    ]
    if any(int(final.get(k, 0) or 0) != 0 for k in zero_fec):
        fail("FEC off inventory")
    if final.get("padding_bytes") != 0 or final.get("capture_loss_packets") != 0:
        fail("padding/capture inventory")
    if final.get("inner_tls_wire_bytes_c2s") != inner_c2s or final.get("inner_tls_wire_bytes_s2c") != inner_s2c:
        fail("final inner TLS wire inventory")
    repair = final.get("repair_retransmits")
    if not isinstance(repair, int) or repair < 0:
        fail("repair inventory")

    if len(samples) < 30:
        fail(f"resource samples too sparse={len(samples)}")
    samples.sort(key=lambda x: x.get("t_ns", 0))
    max_heap_alloc = max(int(x.get("heap_alloc_bytes", 0) or 0) for x in samples)
    max_heap_inuse = max(int(x.get("heap_inuse_bytes", 0) or 0) for x in samples)
    max_sys = max(int(x.get("sys_bytes", 0) or 0) for x in samples)
    max_mem_alloc = max(int(x.get("mem_alloc_bytes", 0) or 0) for x in samples)
    gc_delta = int(samples[-1].get("num_gc", 0) or 0) - int(samples[0].get("num_gc", 0) or 0)
    pause_delta = int(samples[-1].get("pause_total_ns", 0) or 0) - int(samples[0].get("pause_total_ns", 0) or 0)
    if gc_delta < 0 or pause_delta < 0:
        fail("GC counters regressed")
    host_first = samples[0].get("host_cpu", {})
    host_last = samples[-1].get("host_cpu", {})
    host_busy = cpu_busy(host_first.get("cpu"), host_last.get("cpu"))
    if host_busy is None:
        fail("host CPU unavailable")
    per_core = {}
    for name, first in host_first.items():
        if name == "cpu" or name not in host_last:
            continue
        busy = cpu_busy(first, host_last[name])
        if busy is not None:
            per_core[name] = busy
    if not per_core:
        fail("per-core CPU unavailable")

    peak_client_outstanding = max(int(x.get("client_transport", {}).get("outstanding", 0) or 0) for x in samples)
    peak_server_outstanding = max(int(x.get("server_transport", {}).get("outstanding", 0) or 0) for x in samples)
    peak_client_ooo = max(int(x.get("client_transport", {}).get("out_of_order", 0) or 0) for x in samples)
    peak_server_ooo = max(int(x.get("server_transport", {}).get("out_of_order", 0) or 0) for x in samples)

    rtt_summary = {
        "samples": len(rtts),
        "p50_ns": percentile_nearest_rank(rtts, 50),
        "p95_ns": percentile_nearest_rank(rtts, 95),
        "p99_ns": percentile_nearest_rank(rtts, 99),
    }
    lag_summary = {
        "samples": len(lags),
        "p50_ns": percentile_nearest_rank(lags, 50),
        "p95_ns": percentile_nearest_rank(lags, 95),
        "p99_ns": percentile_nearest_rank(lags, 99),
        "max_ns": max(lags) if lags else None,
    }
    actual_mbps = actual_aggregate * 8 / 15.0 / 1_000_000
    goodput_bps = actual_aggregate * 8 / 15.0
    fixed_window_outer = [x for x in outer_events if isinstance(x.get("t_ns"), int) and start_ns <= x.get("t_ns") < window_end_ns]
    pps = len(fixed_window_outer) / 15.0

    summary = {
        "schema": SCHEMA,
        "source_sha": args.source_sha,
        "scenario": "controlled-real-https-load",
        "target_mbps": rate,
        "target_scope": "aggregate-inner-application-payload-c2s-plus-s2c",
        "run_ordinal": run,
        "seed": seed,
        "duration_seconds": 15,
        "offered_transactions": len(plan),
        "accepted_transactions": accepted,
        "successful_transactions": successes,
        "send_failures": send_failures,
        "skipped_slots": skipped,
        "application_loss": app_loss,
        "application_duplicates": duplicates,
        "offered_aggregate_payload_bytes": offered_bytes,
        "actual_aggregate_payload_bytes": actual_aggregate,
        "achievement_ratio": achievement_ratio,
        "accepted_ratio": accepted_ratio,
        "actual_mbps": actual_mbps,
        "goodput_bps": goodput_bps,
        "rtt": rtt_summary,
        "scheduler_queue_residency": lag_summary,
        "pps": pps,
        "outer_wire_bytes_workload": outer_wire,
        "inner_tls_wire_bytes": inner_c2s + inner_s2c,
        "wire_amplification": wire_amp,
        "repair_retransmits": repair,
        "fec_profile": "off",
        "padding_bytes": 0,
        "capture_loss_packets": 0,
        "resources": {
            "samples": len(samples),
            "host_busy_ratio": host_busy,
            "per_core_busy_ratio": per_core,
            "max_mem_alloc_bytes": max_mem_alloc,
            "max_heap_alloc_bytes": max_heap_alloc,
            "max_heap_inuse_bytes": max_heap_inuse,
            "max_sys_bytes": max_sys,
            "gc_cycles_delta": gc_delta,
            "gc_pause_ns_delta": pause_delta,
            "peak_client_transport_outstanding": peak_client_outstanding,
            "peak_server_transport_outstanding": peak_server_outstanding,
            "peak_client_out_of_order": peak_client_ooo,
            "peak_server_out_of_order": peak_server_ooo,
        },
        "raw_capture": {
            "pcap_packets": pcap_packets,
            "pcap_bytes": pcap_bytes,
            "outer_events": len(outer_events),
            "tlslike_record_events": len(record_events),
            "workload_outer_events": len(workload_outer),
        },
        "resource_policy": "observational-only-no-cross-vm-absolute-gate",
        "verdict": "PASS",
    }
    with open(args.output, "w", encoding="utf-8") as f:
        json.dump(summary, f, indent=2, sort_keys=True)
        f.write("\n")

    print(
        f"WBD_P5_LOAD_PASS source_sha={args.source_sha} target_mbps={rate} scope=aggregate-inner "
        f"run={run} seed={seed} duration=15s offered_tx={len(plan)} accepted_tx={accepted} success={successes} "
        f"loss={app_loss} duplicate={duplicates} send_failures={send_failures} skipped={skipped} "
        f"actual_mbps={actual_mbps:.6f} achievement={achievement_ratio:.6f} rtt_p95_ns={rtt_summary['p95_ns']} "
        f"send_lag_p95_ns={lag_summary['p95_ns']} pps={pps:.3f} fec=off repair={repair} padding=off wire_amp={wire_amp:.6f}"
    )


if __name__ == "__main__":
    main()
