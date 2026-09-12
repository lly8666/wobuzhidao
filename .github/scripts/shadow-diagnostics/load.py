"""Paced UDP probe: actual timestamps, bounded catch-up and batched receive."""
import json
import math
import os
import select
import socket
import struct
import sys
import time

out, duration, rate, size = sys.argv[1:]
duration, rate, size = float(duration), int(rate), int(size)
assert duration > 0 and rate > 0 and size >= 20
interval = size * 8 / rate
s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
s.bind(('127.0.0.1', 47600))
s.setblocking(False)
target = ('127.0.0.1', int(os.environ.get('PROBE_PORT', '47500')))
start = time.monotonic()
deadline = start + duration
sent = skipped = errors = duplicates = bad = 0
slot = 0
unique = set()
rtts, lateness = [], []
pad = b'Z' * (size - 20)
while True:
    now = time.monotonic()
    # At most 32 sends per receive turn; omit stale slots instead of a huge burst.
    if now < deadline:
        due = int((now - start) / interval)
        if due - slot >= 32:
            skipped += due - slot - 31
            slot = due - 31
        for _ in range(32):
            now = time.monotonic()
            planned = start + slot * interval
            if now >= deadline or now < planned:
                break
            packet = b'WBD1' + struct.pack('!Qd', sent, now - start) + pad
            try:
                s.sendto(packet, target)
            except BlockingIOError:
                errors += 1
            else:
                sent += 1
                lateness.append((now - planned) * 1000)
            slot += 1
    for _ in range(256):
        try:
            data = s.recv(65535)
        except BlockingIOError:
            break
        received_at = time.monotonic()
        if len(data) != size or data[:4] != b'WBD1' or data[20:] != pad:
            bad += 1
            continue
        seq, tx = struct.unpack('!Qd', data[4:20])
        if seq >= sent or tx < 0 or tx > duration:
            bad += 1
        elif seq in unique:
            duplicates += 1
        else:
            unique.add(seq)
            rtts.append((received_at - start - tx) * 1000)
    now = time.monotonic()
    if now >= deadline and (len(unique) == sent or now >= deadline + 65):
        break
    wait = min(0.002, max(0, start + slot * interval - now)) if now < deadline else 0.002
    select.select([s], [], [], wait)

def percentile(values, p):
    return sorted(values)[max(0, math.ceil(len(values) * p) - 1)] if values else None

result = dict(duration_sec=duration, rate_target_bps=rate, payload_bytes=size,
              sent=sent, received_unique=len(unique), lost=sent-len(unique),
              duplicates=duplicates, bad_payload=bad, skipped_send_slots=skipped,
              send_errors=errors, up_payload_bps=sent*size*8/duration,
              down_payload_bps=len(unique)*size*8/duration,
              loss_ratio=1-len(unique)/sent if sent else 1,
              rtt_p50_ms=percentile(rtts, .5), rtt_p99_ms=percentile(rtts, .99),
              sender_lateness_p99_ms=percentile(lateness, .99),
              within_1000ms_ratio=sum(x <= 1000 for x in rtts)/sent if sent else 0,
              probe_rcvbuf=s.getsockopt(socket.SOL_SOCKET, socket.SO_RCVBUF),
              probe_sndbuf=s.getsockopt(socket.SOL_SOCKET, socket.SO_SNDBUF))
result['load_valid'] = sent >= duration/interval*.995 and not errors and not bad
with open(out, 'w') as f:
    json.dump(result, f, indent=2)
print(json.dumps(result))
if not result['load_valid']:
    raise SystemExit(11)
