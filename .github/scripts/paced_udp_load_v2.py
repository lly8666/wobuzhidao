#!/usr/bin/env python3
import json
import math
import os
import select
import socket
import struct
import sys
import time


def pct(xs, p):
    if not xs:
        return None
    ys = sorted(xs)
    i = max(0, min(len(ys) - 1, math.ceil(p * len(ys)) - 1))
    return ys[i]


def endpoint(raw):
    host, port = raw.rsplit(':', 1)
    return host, int(port)


def parse_phases(spec, duration):
    if not spec:
        return [("all", 0.0, duration)]
    out = []
    for item in spec.split(','):
        name, start, end = item.split(':')
        start = float(start)
        end = float(end)
        if not (0 <= start < end <= duration + 1e-9):
            raise SystemExit(f"invalid phase {item!r} for duration={duration}")
        out.append((name, start, end))
    if not out or abs(out[0][1]) > 1e-9 or abs(out[-1][2] - duration) > 1e-9:
        raise SystemExit("phase spec must cover the full send window")
    for a, b in zip(out, out[1:]):
        if abs(a[2] - b[1]) > 1e-9:
            raise SystemExit("phase spec must be contiguous")
    return out


def main():
    if len(sys.argv) != 5:
        raise SystemExit("usage: paced_udp_load_v2.py OUT_JSON DURATION_SEC RATE_BPS PAYLOAD_BYTES_UNUSED")
    out_path = sys.argv[1]
    duration = float(sys.argv[2])
    rate_bps = int(sys.argv[3])
    _payload_bytes_unused = int(sys.argv[4])
    if duration <= 0 or rate_bps <= 0:
        raise SystemExit("duration and rate must be positive")

    bind = endpoint(os.environ.get("WBD_LOAD_BIND", "127.0.0.1:47600"))
    dst = endpoint(os.environ.get("WBD_LOAD_DST", "127.0.0.1:47500"))
    max_payload = int(os.environ.get("WBD_LOAD_MAX_PAYLOAD", "1360"))
    if max_payload < 64:
        raise SystemExit("WBD_LOAD_MAX_PAYLOAD must be >=64")
    size_cycle = ([64] * 35 + [128] * 15 + [256] * 10 + [512] * 10 +
                  [1000] * 10 + [1200] * 5 + [max_payload] * 15)
    phases = parse_phases(os.environ.get("WBD_LOAD_PHASE_SPEC", ""), duration)
    drain_sec = float(os.environ.get("WBD_LOAD_DRAIN_SEC", "15"))
    max_send_burst = int(os.environ.get("WBD_LOAD_MAX_SEND_BURST", "64"))
    max_recv_burst = int(os.environ.get("WBD_LOAD_MAX_RECV_BURST", "256"))
    requested_rcvbuf = int(os.environ.get("WBD_LOAD_RCVBUF", str(4 << 20)))
    requested_sndbuf = int(os.environ.get("WBD_LOAD_SNDBUF", str(4 << 20)))

    def phase_for(rel):
        for name, a, b in phases:
            if a <= rel < b or (rel == duration and abs(b - duration) < 1e-9):
                return name
        return phases[-1][0]

    phase_state = {
        name: {
            "window_start_sec": a,
            "window_end_sec": b,
            "window_duration_sec": b - a,
            "attempted": 0,
            "successful": 0,
            "send_failures": 0,
            "sent_payload_bytes": 0,
            "received_unique": 0,
            "received_payload_bytes": 0,
            "latency_ms": [],
            "timely_1s": 0,
            "timely_2s": 0,
        }
        for name, a, b in phases
    }

    seconds = []
    for sec in range(int(math.ceil(duration))):
        start = float(sec)
        end = min(duration, sec + 1.0)
        seconds.append({
            "second": sec,
            "window_start_sec": start,
            "window_end_sec": end,
            "planned_payload_bytes": rate_bps * (end - start) / 8.0,
            "attempted": 0,
            "successful": 0,
            "send_failures": 0,
            "sent_payload_bytes": 0,
            "send_lag_ms": [],
            "received_realtime_payload_bytes": 0,
            "received_realtime_unique": 0,
        })
    drain_seconds = {}

    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF, requested_rcvbuf)
    s.setsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF, requested_sndbuf)
    s.bind(bind)
    s.setblocking(False)
    effective_rcvbuf = s.getsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF)
    effective_sndbuf = s.getsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF)

    start = time.monotonic()
    deadline = start + duration
    planned_bytes = 0
    seq = 0
    attempted = 0
    successful = 0
    successful_bytes = 0
    send_failures = 0
    send_lags_ms = []
    sent_meta = {}
    size_sent_counts = {}
    size_attempt_counts = {}
    size_recv_counts = {}
    unique = set()
    recv_bytes = 0
    dup = 0
    bad = 0
    unknown_seq = 0
    last_rx = start
    latency_ms = []
    timely_1s = 0
    timely_2s = 0
    timely_5s = 0

    def receive_some(limit):
        nonlocal recv_bytes, dup, bad, unknown_seq, last_rx, timely_1s, timely_2s, timely_5s
        got = 0
        while got < limit:
            try:
                data, _ = s.recvfrom(65535)
            except BlockingIOError:
                break
            got += 1
            now = time.monotonic()
            last_rx = now
            rel_rx = now - start
            if len(data) < 20 or not data.startswith(b'WBD1'):
                bad += 1
                continue
            rx_seq = struct.unpack('!Q', data[4:12])[0]
            tx_rel = struct.unpack('!d', data[12:20])[0]
            meta = sent_meta.get(rx_seq)
            if meta is None:
                unknown_seq += 1
                bad += 1
                continue
            expected_size, ph = meta
            if len(data) != expected_size:
                bad += 1
                continue
            if rx_seq in unique:
                dup += 1
                continue
            unique.add(rx_seq)
            recv_bytes += len(data)
            size_recv_counts[str(len(data))] = size_recv_counts.get(str(len(data)), 0) + 1
            age = max(0.0, rel_rx - tx_rel)
            age_ms = age * 1000.0
            latency_ms.append(age_ms)
            ps = phase_state[ph]
            ps["received_unique"] += 1
            ps["received_payload_bytes"] += len(data)
            ps["latency_ms"].append(age_ms)
            if age <= 1.0:
                timely_1s += 1
                ps["timely_1s"] += 1
            if age <= 2.0:
                timely_2s += 1
                ps["timely_2s"] += 1
            if age <= 5.0:
                timely_5s += 1
            bucket = int(rel_rx)
            if 0 <= bucket < len(seconds):
                seconds[bucket]["received_realtime_payload_bytes"] += len(data)
                seconds[bucket]["received_realtime_unique"] += 1
            else:
                d = drain_seconds.setdefault(str(bucket), {"received_payload_bytes": 0, "received_unique": 0})
                d["received_payload_bytes"] += len(data)
                d["received_unique"] += 1
        return got

    while True:
        now = time.monotonic()
        receive_some(max_recv_burst)
        now = time.monotonic()

        burst = 0
        while burst < max_send_burst:
            sched_rel = planned_bytes * 8.0 / rate_bps
            if sched_rel >= duration or now < start + sched_rel:
                break
            size = size_cycle[seq % len(size_cycle)]
            actual_rel = time.monotonic() - start
            if actual_rel >= duration:
                break
            lag_ms = max(0.0, (actual_rel - sched_rel) * 1000.0)
            ph = phase_for(actual_rel)
            ps = phase_state[ph]
            attempted += 1
            ps["attempted"] += 1
            size_attempt_counts[str(size)] = size_attempt_counts.get(str(size), 0) + 1
            sec = min(len(seconds) - 1, max(0, int(actual_rel)))
            seconds[sec]["attempted"] += 1
            seconds[sec]["send_lag_ms"].append(lag_ms)
            send_lags_ms.append(lag_ms)
            pad = b'Z' * (size - 20)
            payload = b'WBD1' + struct.pack('!Qd', seq, actual_rel) + pad
            try:
                n = s.sendto(payload, dst)
                if n != size:
                    raise OSError(f"short UDP send {n}/{size}")
            except (BlockingIOError, OSError):
                send_failures += 1
                ps["send_failures"] += 1
                seconds[sec]["send_failures"] += 1
            else:
                successful += 1
                successful_bytes += size
                ps["successful"] += 1
                ps["sent_payload_bytes"] += size
                seconds[sec]["successful"] += 1
                seconds[sec]["sent_payload_bytes"] += size
                size_sent_counts[str(size)] = size_sent_counts.get(str(size), 0) + 1
                sent_meta[seq] = (size, ph)
            planned_bytes += size
            seq += 1
            burst += 1
            now = time.monotonic()

        receive_some(max_recv_burst)
        now = time.monotonic()
        if now >= deadline:
            if (now - deadline >= 2.0 and now - last_rx >= 2.0) or now >= deadline + drain_sec:
                break
        next_sched = start + planned_bytes * 8.0 / rate_bps
        wait = 0.001
        if now < deadline:
            wait = max(0.0, min(0.001, next_sched - now))
        if wait > 0:
            select.select([s], [], [], wait)

    skipped_slots = 0
    skipped_bytes = 0
    sim_bytes = planned_bytes
    sim_seq = seq
    while sim_bytes * 8.0 / rate_bps < duration:
        size = size_cycle[sim_seq % len(size_cycle)]
        skipped_slots += 1
        skipped_bytes += size
        sim_bytes += size
        sim_seq += 1

    for sec in seconds:
        lags = sec.pop("send_lag_ms")
        win = max(1e-9, sec["window_end_sec"] - sec["window_start_sec"])
        sec["actual_injection_bps"] = sec["sent_payload_bytes"] * 8.0 / win
        sec["actual_injection_ratio"] = sec["actual_injection_bps"] / rate_bps
        sec["received_realtime_bps"] = sec["received_realtime_payload_bytes"] * 8.0 / win
        sec["send_lag_ms_p50"] = pct(lags, 0.50)
        sec["send_lag_ms_p99"] = pct(lags, 0.99)
        sec["send_lag_ms_max"] = max(lags) if lags else None

    phase_metrics = {}
    for name, _a, _b in phases:
        ps = phase_state[name]
        win = ps["window_duration_sec"]
        inj_bps = ps["sent_payload_bytes"] * 8.0 / max(win, 1e-9)
        sent_count = ps["successful"]
        recv_count = ps["received_unique"]
        sent_bytes = ps["sent_payload_bytes"]
        recv_phase_bytes = ps["received_payload_bytes"]
        lats = ps.pop("latency_ms")
        phase_metrics[name] = {
            **ps,
            "target_bps": rate_bps,
            "actual_injection_bps": inj_bps,
            "actual_injection_ratio": inj_bps / rate_bps,
            "injection_within_99_101pct": 0.99 <= inj_bps / rate_bps <= 1.01,
            "loss_ratio": ((sent_count - recv_count) / sent_count if sent_count else None),
            "byte_loss_ratio": ((sent_bytes - recv_phase_bytes) / sent_bytes if sent_bytes else None),
            "rtt_ms_p50": pct(lats, 0.50),
            "rtt_ms_p95": pct(lats, 0.95),
            "rtt_ms_p99": pct(lats, 0.99),
            "rtt_ms_max": max(lats) if lats else None,
            "timely_1s_ratio": (ps["timely_1s"] / sent_count if sent_count else None),
            "timely_2s_ratio": (ps["timely_2s"] / sent_count if sent_count else None),
        }

    overall_bps = successful_bytes * 8.0 / duration
    half = max(1, len(send_lags_ms) // 2)
    first_half_p99 = pct(send_lags_ms[:half], 0.99)
    second_half_p99 = pct(send_lags_ms[half:], 0.99)
    lag_growth = None if first_half_p99 is None or second_half_p99 is None else second_half_p99 - first_half_p99
    injection_ok = (
        0.99 <= overall_bps / rate_bps <= 1.01 and
        all(v["injection_within_99_101pct"] for v in phase_metrics.values()) and
        send_failures == 0 and skipped_slots == 0
    )

    summary = {
        "duration_sec": duration,
        "rate_target_bps": rate_bps,
        "traffic_profile": "realistic-mix-v2-actual-tx-time",
        "avg_payload_bytes": sum(size_cycle) / len(size_cycle),
        "max_payload_bytes": max_payload,
        "bind": f"{bind[0]}:{bind[1]}",
        "destination": f"{dst[0]}:{dst[1]}",
        "socket_buffers": {
            "requested_rcvbuf_bytes": requested_rcvbuf,
            "effective_rcvbuf_bytes": effective_rcvbuf,
            "requested_sndbuf_bytes": requested_sndbuf,
            "effective_sndbuf_bytes": effective_sndbuf,
        },
        "attempted": attempted,
        "sent": successful,
        "send_failures": send_failures,
        "skipped_send_slots": skipped_slots,
        "skipped_payload_bytes": skipped_bytes,
        "sent_payload_bytes": successful_bytes,
        "received_unique": len(unique),
        "received_payload_bytes": recv_bytes,
        "duplicates": dup,
        "bad_payload": bad,
        "unknown_seq": unknown_seq,
        "actual_injection_bps": overall_bps,
        "actual_injection_ratio": overall_bps / rate_bps,
        "up_payload_bps": overall_bps,
        "down_payload_bps": recv_bytes * 8.0 / duration,
        "loss_ratio": ((successful - len(unique)) / successful if successful else 1.0),
        "byte_loss_ratio": ((successful_bytes - recv_bytes) / successful_bytes if successful_bytes else 1.0),
        "send_lag_ms_p50": pct(send_lags_ms, 0.50),
        "send_lag_ms_p99": pct(send_lags_ms, 0.99),
        "send_lag_ms_max": max(send_lags_ms) if send_lags_ms else None,
        "send_lag_first_half_p99_ms": first_half_p99,
        "send_lag_second_half_p99_ms": second_half_p99,
        "send_lag_p99_growth_ms": lag_growth,
        "send_lag_sustained_increase": bool(lag_growth is not None and lag_growth > 5.0),
        "rtt_samples": len(latency_ms),
        "rtt_ms_p50": pct(latency_ms, 0.50),
        "rtt_ms_p95": pct(latency_ms, 0.95),
        "rtt_ms_p99": pct(latency_ms, 0.99),
        "rtt_ms_max": max(latency_ms) if latency_ms else None,
        "timely_1s_ratio": (timely_1s / successful if successful else None),
        "timely_2s_ratio": (timely_2s / successful if successful else None),
        "timely_5s_ratio": (timely_5s / successful if successful else None),
        "phase_metrics": phase_metrics,
        "per_second": seconds,
        "drain_realtime": drain_seconds,
        "size_attempt_counts": size_attempt_counts,
        "size_sent_counts": size_sent_counts,
        "size_received_counts": size_recv_counts,
        "load_valid_for_capacity": injection_ok,
        "load_invalid_reason": None if injection_ok else "actual injection outside 99-101% in full run/phase, send failure, or skipped slot",
    }
    with open(out_path, "w") as f:
        json.dump(summary, f, indent=2, sort_keys=True)
        f.write("\n")
    print("WBD_MEASURED_LOAD_RESULT " + json.dumps(summary, sort_keys=True))
    if bad or dup or unknown_seq:
        raise SystemExit(10)
    # Injection validity is reported, not used as process success. Invalid-load
    # samples must still retain their full diagnostics and artifacts.


if __name__ == "__main__":
    main()
