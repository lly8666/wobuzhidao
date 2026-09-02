#!/usr/bin/env python3
import argparse
import json
import socket
import struct
import time

HDR = struct.Struct("!IQ")


def percentile(xs, q):
    if not xs:
        return None
    ys = sorted(xs)
    if len(ys) == 1:
        return ys[0]
    x = (len(ys) - 1) * q
    lo = int(x)
    hi = min(lo + 1, len(ys) - 1)
    frac = x - lo
    return ys[lo] * (1 - frac) + ys[hi] * frac


def server(a):
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    host, port = a.bind.rsplit(":", 1)
    sock.bind((host, int(port)))
    sock.settimeout(0.05)
    first = {}
    start = time.monotonic()
    deadline = start + a.window
    while time.monotonic() < deadline and len(first) < a.expect:
        try:
            data, _ = sock.recvfrom(65535)
        except socket.timeout:
            continue
        now = time.monotonic_ns()
        if len(data) < HDR.size:
            continue
        ident, sent = HDR.unpack_from(data)
        if ident not in first:
            first[ident] = (now - sent) / 1e6
    vals = list(first.values())
    out = {
        "expected": a.expect,
        "received_first": len(vals),
        "delivery_ratio": len(vals) / a.expect,
        "p50_ms": percentile(vals, 0.50),
        "p75_ms": percentile(vals, 0.75),
        "p90_ms": percentile(vals, 0.90),
        "p95_ms": percentile(vals, 0.95),
        "p99_ms": percentile(vals, 0.99),
        "max_ms": max(vals) if vals else None,
        "measurement_wall_s": time.monotonic() - start,
    }
    with open(a.out, "w") as f:
        f.write(json.dumps(out, sort_keys=True) + "\n")
    print("FIRST_ARRIVAL " + json.dumps(out, sort_keys=True), flush=True)


def client(a):
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    host, port = a.dest.rsplit(":", 1)
    payload_size = max(a.size, HDR.size)
    body = bytes(payload_size - HDR.size)
    for ident in range(a.count):
        sent = time.monotonic_ns()
        sock.sendto(HDR.pack(ident, sent) + body, (host, int(port)))
        if a.interval_ms > 0:
            time.sleep(a.interval_ms / 1000.0)


def main():
    p = argparse.ArgumentParser()
    sub = p.add_subparsers(dest="mode", required=True)
    sp = sub.add_parser("server")
    sp.add_argument("--bind", required=True)
    sp.add_argument("--expect", type=int, required=True)
    sp.add_argument("--window", type=float, default=3.0)
    sp.add_argument("--out", required=True)
    cp = sub.add_parser("client")
    cp.add_argument("--dest", required=True)
    cp.add_argument("--count", type=int, required=True)
    cp.add_argument("--size", type=int, default=1200)
    cp.add_argument("--interval-ms", type=float, default=2.0)
    a = p.parse_args()
    if a.mode == "server":
        server(a)
    else:
        client(a)


if __name__ == "__main__":
    main()
