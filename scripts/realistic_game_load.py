#!/usr/bin/env python3
import json
import random
import select
import socket
import struct
import sys
import time
import zlib

out = sys.argv[1]
duration = float(sys.argv[2])
rate_bps = int(sys.argv[3])
seed = int(sys.argv[4])
# Synthetic game-like mix: small packets dominate, with some state snapshots and
# rare near-MTU payloads. This is intentionally not attributed to any one game.
sizes = [128] * 35 + [256] * 25 + [512] * 20 + [1000] * 15 + [1320] * 5
rng = random.Random(seed)
rng.shuffle(sizes)
magic = b"WBDR"
fmt = "!4sBBHQQI"
hdr = struct.calcsize(fmt)
latency_stride = 8

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
# game-lane-client deliberately pins the first application UDP peer by IP:port.
# The realistic harness warm-up uses 47601, so the measured load must use the
# same source identity rather than being silently rejected as a second peer.
sock.bind(("127.0.0.1", 47601))
sock.setblocking(False)
seen = bytearray()
latency_us = []
interarrival_us = []
sent_by_size = {str(x): 0 for x in sorted(set(sizes))}
recv_by_size = {k: 0 for k in sent_by_size}
sent = recv = duplicates = bad = out_of_order = 0
sent_bytes = recv_bytes = 0
highest = -1
last_unique_ns = None
lat_min = None
lat_max = 0


def was_seen(seq):
    idx = seq >> 3
    if idx >= len(seen):
        seen.extend(b"\0" * (idx + 1 - len(seen)))
    mask = 1 << (seq & 7)
    old = seen[idx] & mask
    seen[idx] |= mask
    return bool(old)


def make_packet(seq, size, send_ns):
    body = bytes([(seq * 131 + size) & 255]) * (size - hdr)
    zero = struct.pack(fmt, magic, 1, 1, size, seq, send_ns, 0) + body
    crc = zlib.crc32(zero) & 0xFFFFFFFF
    return struct.pack(fmt, magic, 1, 1, size, seq, send_ns, crc) + body


def parse_packet(data):
    if len(data) < hdr:
        return None
    mg, ver, direction, size, seq, send_ns, crc = struct.unpack(fmt, data[:hdr])
    if mg != magic or ver != 1 or direction != 1 or size != len(data):
        return None
    zero = struct.pack(fmt, mg, ver, direction, size, seq, send_ns, 0) + data[hdr:]
    if (zlib.crc32(zero) & 0xFFFFFFFF) != crc:
        return None
    return seq, send_ns, size


def percentile(values, q):
    if not values:
        return None
    values.sort()
    x = (len(values) - 1) * q
    lo = int(x)
    hi = min(lo + 1, len(values) - 1)
    frac = x - lo
    return values[lo] * (1 - frac) + values[hi] * frac


# Fast fail before the 30-minute clock starts. This uses the exact same socket
# and payload framing as the measured traffic, so a peer-identity or ingress
# mismatch is detected in seconds rather than after a full long-run allocation.
preflight = make_packet(0, 128, time.monotonic_ns())
sock.sendto(preflight, ("127.0.0.1", 47500))
preflight_deadline = time.monotonic() + 20.0
preflight_ok = False
while time.monotonic() < preflight_deadline:
    ready, _, _ = select.select([sock], [], [], 0.25)
    if not ready:
        continue
    while True:
        try:
            data, _ = sock.recvfrom(65535)
        except BlockingIOError:
            break
        if data == preflight:
            preflight_ok = True
            break
    if preflight_ok:
        break
if not preflight_ok:
    raise SystemExit("WBD_REALISTIC_LOAD_PREFLIGHT_FAIL no exact echo on pinned app peer")
print("WBD_REALISTIC_LOAD_PREFLIGHT_PASS source=127.0.0.1:47601")

start = time.monotonic()
deadline = start + duration
next_tx = start
while True:
    now = time.monotonic()
    burst = 0
    while now >= next_tx and next_tx < deadline and burst < 256:
        size = sizes[sent % len(sizes)]
        send_ns = time.monotonic_ns()
        sock.sendto(make_packet(sent, size, send_ns), ("127.0.0.1", 47500))
        sent += 1
        sent_bytes += size
        sent_by_size[str(size)] += 1
        next_tx += size * 8.0 / rate_bps
        burst += 1
        now = time.monotonic()

    ready, _, _ = select.select([sock], [], [], 0.002 if now < deadline else 0.01)
    if ready:
        while True:
            try:
                data, _ = sock.recvfrom(65535)
            except BlockingIOError:
                break
            rx_ns = time.monotonic_ns()
            parsed = parse_packet(data)
            if parsed is None:
                bad += 1
                continue
            seq, send_ns, size = parsed
            if seq >= sent:
                bad += 1
                continue
            if was_seen(seq):
                duplicates += 1
                continue
            recv += 1
            recv_bytes += size
            recv_by_size[str(size)] += 1
            if seq < highest:
                out_of_order += 1
            else:
                highest = seq
            us = max(0, (rx_ns - send_ns) // 1000)
            lat_min = us if lat_min is None else min(lat_min, us)
            lat_max = max(lat_max, us)
            if seq % latency_stride == 0:
                latency_us.append(us)
            if last_unique_ns is not None and recv % 16 == 0:
                interarrival_us.append((rx_ns - last_unique_ns) // 1000)
            last_unique_ns = rx_ns

    now = time.monotonic()
    # Sending is exactly DURATION_SEC. The receive-only tail covers the product's
    # 60 s maximum FakeTCP RTO without converting unresolved tail traffic into a
    # premature loss verdict.
    if now >= deadline and (recv >= sent or now >= deadline + 65.0):
        break

lost = max(0, sent - recv)
summary = {
    "requested_duration_sec": int(duration),
    "requested_bps_each_direction": rate_bps,
    "payload_profile": {"128": 0.35, "256": 0.25, "512": 0.20, "1000": 0.15, "1320": 0.05},
    "profile_seed": seed,
    "sent": sent,
    "received_unique": recv,
    "lost": lost,
    "loss_ratio": (lost / sent if sent else 1.0),
    "duplicates": duplicates,
    "bad_payload": bad,
    "out_of_order_unique": out_of_order,
    "sent_payload_bytes": sent_bytes,
    "received_payload_bytes": recv_bytes,
    "offered_payload_bps": sent_bytes * 8 / duration,
    "goodput_payload_bps": recv_bytes * 8 / duration,
    "sent_by_size": sent_by_size,
    "received_by_size": recv_by_size,
    "latency_sample_stride": latency_stride,
    "latency_samples": len(latency_us),
    "rtt_us": {
        "min": lat_min,
        "p50": percentile(latency_us, 0.50),
        "p90": percentile(latency_us, 0.90),
        "p95": percentile(latency_us, 0.95),
        "p99": percentile(latency_us, 0.99),
        "p999": percentile(latency_us, 0.999),
        "max": lat_max,
    },
    "rx_interarrival_us": {
        "samples": len(interarrival_us),
        "p50": percentile(interarrival_us, 0.50),
        "p95": percentile(interarrival_us, 0.95),
        "p99": percentile(interarrival_us, 0.99),
    },
}
with open(out, "w", encoding="utf-8") as f:
    json.dump(summary, f, sort_keys=True, indent=2)
    f.write("\n")
print("WBD_REALISTIC_LOAD_RESULT " + json.dumps(summary, sort_keys=True))