#!/usr/bin/env python3
import argparse
import json
import math
import subprocess
import sys
import time


def percentile(values, q):
    values = sorted(values)
    if not values:
        return None
    if q <= 0:
        return values[0]
    if q >= 1:
        return values[-1]
    pos = q * (len(values) - 1)
    lo = math.floor(pos)
    hi = math.ceil(pos)
    if lo == hi:
        return values[lo]
    return values[lo] + (values[hi] - values[lo]) * (pos - lo)


def run_one(diag, addr, server_name, timeout):
    cp = subprocess.run(
        [diag, "-addr", addr, "-server-name", server_name, "-count", "1", "-interval", "0s", "-timeout", timeout],
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        check=False,
    )
    if cp.returncode != 0:
        return {"ok": False, "runner_error": cp.stderr.strip() or cp.stdout.strip(), "returncode": cp.returncode}
    try:
        summary = json.loads(cp.stdout)
        row = dict(summary["results"][0])
        row["runner_returncode"] = cp.returncode
        return row
    except Exception as exc:
        return {"ok": False, "runner_error": f"invalid diagnostic JSON: {exc}", "stdout": cp.stdout[-2000:]}


def summarize(rows):
    good = [r for r in rows if r.get("ok")]
    tcp = [float(r["tcp_connect_ms"]) for r in good if "tcp_connect_ms" in r]
    tls = [float(r["tls_handshake_ms"]) for r in good if "tls_handshake_ms" in r]
    return {
        "attempts": len(rows),
        "successes": len(good),
        "failures": len(rows) - len(good),
        "success_ratio": len(good) / len(rows) if rows else 0.0,
        "tcp_connect_p50_ms": percentile(tcp, 0.50),
        "tcp_connect_p95_ms": percentile(tcp, 0.95),
        "tls_handshake_p50_ms": percentile(tls, 0.50),
        "tls_handshake_p95_ms": percentile(tls, 0.95),
    }


def delta(a, b):
    if a is None or b is None:
        return None
    return a - b


def main():
    p = argparse.ArgumentParser(description="Alternate genuine direct TLS and Reality-mirror TLS handshakes to reduce short-term drift.")
    p.add_argument("--diag", default="./wbd-tls-diag", help="path to wbd-tls-diag")
    p.add_argument("--direct", required=True, help="genuine target host:port")
    p.add_argument("--mirror", required=True, help="WBD mirror server host:port")
    p.add_argument("--server-name", required=True, help="genuine target TLS hostname/SNI")
    p.add_argument("--pairs", type=int, default=20)
    p.add_argument("--timeout", default="5s", help="Go duration passed to wbd-tls-diag")
    p.add_argument("--pause", type=float, default=0.10, help="seconds between individual attempts")
    args = p.parse_args()
    if args.pairs <= 0 or args.pairs > 1000 or args.pause < 0:
        p.error("--pairs must be 1..1000 and --pause must be non-negative")

    pairs = []
    direct_rows = []
    mirror_rows = []
    for i in range(args.pairs):
        order = ["direct", "mirror"] if i % 2 == 0 else ["mirror", "direct"]
        row = {"pair": i, "order": order}
        for j, label in enumerate(order):
            addr = args.direct if label == "direct" else args.mirror
            result = run_one(args.diag, addr, args.server_name, args.timeout)
            row[label] = result
            (direct_rows if label == "direct" else mirror_rows).append(result)
            if args.pause and not (i == args.pairs - 1 and j == len(order) - 1):
                time.sleep(args.pause)
        pairs.append(row)

    direct = summarize(direct_rows)
    mirror = summarize(mirror_rows)
    out = {
        "schema_version": "wbd-reality-mirror-compare/v1",
        "server_name": args.server_name,
        "direct_addr": args.direct,
        "mirror_addr": args.mirror,
        "pairs": args.pairs,
        "direct": direct,
        "mirror": mirror,
        "mirror_minus_direct": {
            "success_ratio_pp": (mirror["success_ratio"] - direct["success_ratio"]) * 100.0,
            "tcp_connect_p50_ms": delta(mirror["tcp_connect_p50_ms"], direct["tcp_connect_p50_ms"]),
            "tcp_connect_p95_ms": delta(mirror["tcp_connect_p95_ms"], direct["tcp_connect_p95_ms"]),
            "tls_handshake_p50_ms": delta(mirror["tls_handshake_p50_ms"], direct["tls_handshake_p50_ms"]),
            "tls_handshake_p95_ms": delta(mirror["tls_handshake_p95_ms"], direct["tls_handshake_p95_ms"]),
        },
        "results": pairs,
    }
    json.dump(out, sys.stdout, indent=2, sort_keys=True)
    sys.stdout.write("\n")


if __name__ == "__main__":
    main()
