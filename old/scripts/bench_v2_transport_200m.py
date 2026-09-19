#!/usr/bin/env python3
"""Steady-state 200-Mbit public-link characterization for WBD 20:20.

The public veth is shaped to 200 Mbit/s per direction. Because 20:20 FEC is
roughly 2x before DTLS/FakeTCP headers, the default inner offered load is
90 Mbit/s so the outer link is driven near, but not intentionally beyond,
200 Mbit/s. DTLS/FakeTCP is established once cleanly; only steady-state data
is swept over RTT/loss to keep this sizing run fast and focused on CPU/RSS.
"""
from __future__ import annotations

import argparse
import csv
import importlib.util
import json
import os
import re
import subprocess
import sys
import threading
import time
from pathlib import Path

BASEMOD = Path(__file__).with_name("bench_v2_transport_20x20.py")
spec = importlib.util.spec_from_file_location("wbd_base", BASEMOD)
B = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(B)


def setup(ns_c: str, ns_s: str) -> None:
    for ns in (ns_c, ns_s):
        B.run(["ip", "netns", "del", ns], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        B.run(["ip", "netns", "add", ns])
    B.run(["ip", "link", "add", "vc", "type", "veth", "peer", "name", "vs"])
    B.run(["ip", "link", "set", "vc", "netns", ns_c])
    B.run(["ip", "link", "set", "vs", "netns", ns_s])
    for ns, dev, addr in ((ns_c, "vc", "10.79.0.1/24"), (ns_s, "vs", "10.79.0.2/24")):
        B.run_ns(ns, ["ip", "link", "set", "lo", "up"])
        B.run_ns(ns, ["ip", "addr", "add", addr, "dev", dev])
        B.run_ns(ns, ["ip", "link", "set", dev, "up"])
    B.run_ns(ns_c, ["ping", "-c", "1", "-W", "2", "10.79.0.2"], stdout=subprocess.DEVNULL)


def shape(ns: str, dev: str, half_rtt_ms: float, loss: float, link_mbps: int) -> None:
    cmd = ["tc", "qdisc", "replace", "dev", dev, "root", "netem", "limit", "50000",
           "delay", f"{half_rtt_ms:g}ms"]
    if loss > 0:
        cmd += ["loss", "random", f"{loss:g}%"]
    cmd += ["rate", f"{link_mbps}mbit"]
    B.run_ns(ns, cmd)


def stats_for(roles: dict[str, subprocess.Popen]) -> tuple[dict[str, float], dict[str, int]]:
    cpu, rss = {}, {}
    for name, proc in roles.items():
        c, r = B.tree_stats(proc.pid)
        cpu[name] = c
        rss[name] = r
    return cpu, rss


def delta_ms(a: dict[str, float], b: dict[str, float]) -> dict[str, float]:
    return {k: max(0.0, (b[k] - a[k]) * 1000.0) for k in a}


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--udp2raw", type=Path, required=True)
    ap.add_argument("--speeder", type=Path, required=True)
    ap.add_argument("--shim", type=Path, required=True)
    ap.add_argument("--cert-dir", type=Path, required=True)
    ap.add_argument("--probe", type=Path, required=True)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--rtt", type=int, required=True)
    ap.add_argument("--losses", default="0,1,5,10,20")
    ap.add_argument("--link-mbps", type=int, default=200)
    ap.add_argument("--inner-mbps", type=float, default=90.0)
    ap.add_argument("--duration", type=float, default=2.0)
    ap.add_argument("--packet-size", type=int, default=1200)
    a = ap.parse_args()
    a.out.mkdir(parents=True, exist_ok=True)
    if B.sha256(a.udp2raw) != B.EXPECTED_UDP2RAW_SHA256: raise SystemExit("udp2raw sha mismatch")
    if B.sha256(a.speeder) != B.EXPECTED_SPEEDER_SHA256: raise SystemExit("speeder sha mismatch")

    ns_c, ns_s, base = f"wbd200c{os.getpid()}", f"wbd200s{os.getpid()}", 49300
    processes: list[tuple[str, subprocess.Popen]] = []
    files: list[object] = []
    try:
        setup(ns_c, ns_s)
        p, f = B.start_ns(ns_s, [a.probe, "server", str(base)], a.out / "echo.log")
        processes.append(("echo", p)); files.append(f)
        p, f = B.start_ns(ns_s, [a.speeder, "-s", f"-l127.0.0.1:{base+1}", f"-r127.0.0.1:{base}",
                                    "-f20:20", "--mode", "0", "--timeout", "8", "-k", "wbd200m",
                                    "--disable-color", "--log-level", "2"], a.out / "speeder-server.log")
        processes.append(("speeder-server", p)); files.append(f)
        p, f = B.start_ns(ns_s, [a.shim, "server", str(base+2), "127.0.0.1", str(base+1),
                                    a.cert_dir / "server.pem", a.cert_dir / "server.key"], a.out / "dtls-server.log")
        processes.append(("dtls-server", p)); files.append(f)
        p, f = B.start_ns(ns_s, [a.udp2raw, "-s", f"-l10.79.0.2:{base+3}", f"-r127.0.0.1:{base+2}",
                                    "-k", "wbd200m", "--raw-mode", "faketcp", "-a",
                                    "--disable-color", "--log-level", "2"], a.out / "udp2raw-server.log")
        processes.append(("udp2raw-server", p)); files.append(f)
        time.sleep(.25)
        p, f = B.start_ns(ns_c, [a.udp2raw, "-c", f"-l127.0.0.1:{base+4}", f"-r10.79.0.2:{base+3}",
                                    "-k", "wbd200m", "--raw-mode", "faketcp", "-a",
                                    "--source-ip", "10.79.0.1", "--source-port", str(base+10),
                                    "--disable-color", "--log-level", "2"], a.out / "udp2raw-client.log")
        processes.append(("udp2raw-client", p)); files.append(f)
        time.sleep(.35)
        p, f = B.start_ns(ns_c, [a.shim, "client", str(base+5), "127.0.0.1", str(base+4),
                                    a.cert_dir / "ca.pem", "wbd.test"], a.out / "dtls-client.log")
        processes.append(("dtls-client", p)); files.append(f)
        rm = dict(processes)
        B.wait_log(a.out / "dtls-server.log", "READY role=server", rm["dtls-server"], 10)
        B.wait_log(a.out / "dtls-client.log", "READY role=client", rm["dtls-client"], 10)
        p, f = B.start_ns(ns_c, [a.speeder, "-c", f"-l127.0.0.1:{base+7}", f"-r127.0.0.1:{base+5}",
                                    "-f20:20", "--mode", "0", "--timeout", "8", "-k", "wbd200m",
                                    "--disable-color", "--log-level", "2"], a.out / "speeder-client.log")
        processes.append(("speeder-client", p)); files.append(f)
        time.sleep(.3)
        roles = {k: p for k, p in processes if k != "echo"}
        client_roles = {k: p for k, p in roles.items() if k.endswith("client")}
        server_roles = {k: p for k, p in roles.items() if k.endswith("server")}
        rows = []
        for loss in [float(x) for x in a.losses.split(",")]:
            shape(ns_c, "vc", a.rtt / 2.0, loss, a.link_mbps)
            shape(ns_s, "vs", a.rtt / 2.0, loss, a.link_mbps)
            time.sleep(.12)
            # low-rate warmup is outside measured counters
            warm = a.out / f"warm-rtt{a.rtt}-loss{loss:g}.json"
            B.run_ns(ns_c, [a.probe, "client", str(base+7), "5", ".25", "256", "1800", warm], check=False)
            q0c, q0s = B.tc_stats(ns_c, "vc"), B.tc_stats(ns_s, "vs")
            c0, _ = stats_for(roles)
            peak = {k: B.tree_stats(p.pid)[1] for k, p in roles.items()}
            stop = threading.Event()
            def monitor() -> None:
                while not stop.is_set():
                    for k, p in roles.items(): peak[k] = max(peak[k], B.tree_stats(p.pid)[1])
                    time.sleep(.02)
            th = threading.Thread(target=monitor, daemon=True); th.start()
            result = a.out / f"rtt{a.rtt}-loss{loss:g}.json"
            timeout_ms = max(2200, min(6000, a.rtt * 4 + 1200))
            t0 = time.monotonic()
            cp = B.run_ns(ns_c, [a.probe, "client", str(base+7), str(a.inner_mbps), str(a.duration),
                                   str(a.packet_size), str(timeout_ms), result], check=False)
            wall = time.monotonic() - t0
            stop.set(); th.join(timeout=.5)
            c1, _ = stats_for(roles)
            q1c, q1s = B.tc_stats(ns_c, "vc"), B.tc_stats(ns_s, "vs")
            app = json.loads(result.read_text()) if result.exists() else {}
            cpu = delta_ms(c0, c1)
            dc, ds = B.counter_delta(q0c, q1c), B.counter_delta(q0s, q1s)
            inner_delivered = float(app.get("delivered_mbps_active", 0.0) or 0.0)
            row = {
                "rtt_ms": a.rtt, "loss_pct_per_direction": loss, "link_mbps": a.link_mbps,
                "inner_offered_mbps": a.inner_mbps, "inner_delivered_mbps": inner_delivered,
                "delivery_ratio": app.get("delivery_ratio_vs_sent"),
                "p50_ms": app.get("p50_ms"), "p95_ms": app.get("p95_ms"), "p99_ms": app.get("p99_ms"),
                "max_ms": app.get("max_ms"), "out_of_order_events": app.get("out_of_order_events"),
                "outer_c2s_mbps": (dc.get("bytes") or 0) * 8.0 / max(a.duration, .001) / 1e6,
                "outer_s2c_mbps": (ds.get("bytes") or 0) * 8.0 / max(a.duration, .001) / 1e6,
                "outer_c2s_drop": dc.get("dropped"), "outer_s2c_drop": ds.get("dropped"),
                "cpu_core_fraction_both_endpoints": sum(cpu.values()) / (a.duration * 1000.0),
                "cpu_core_fraction_client": sum(cpu[k] for k in client_roles) / (a.duration * 1000.0),
                "cpu_core_fraction_server": sum(cpu[k] for k in server_roles) / (a.duration * 1000.0),
                "rss_peak_kb_both_endpoints": sum(peak.values()),
                "rss_peak_kb_client": sum(peak[k] for k in client_roles),
                "rss_peak_kb_server": sum(peak[k] for k in server_roles),
                "probe_rc": cp.returncode,
            }
            for k, v in cpu.items(): row["cpu_ms_" + k.replace("-", "_")] = v
            for k, v in peak.items(): row["rss_kb_" + k.replace("-", "_")] = v
            rows.append(row)
            print("WBD200M_CASE " + json.dumps(row, sort_keys=True), flush=True)
            with (a.out / "results.csv").open("w", newline="") as fcsv:
                w = csv.DictWriter(fcsv, fieldnames=list(rows[0])); w.writeheader(); w.writerows(rows)
        (a.out / "receipt.json").write_text(json.dumps({
            "schema": "wbd-v2-200m-steady/v1", "rtt_ms": a.rtt, "losses": [r["loss_pct_per_direction"] for r in rows],
            "link_mbps_per_direction": a.link_mbps, "inner_offered_mbps": a.inner_mbps,
            "fec": "20:20", "cases": len(rows), "results": rows,
        }, indent=2) + "\n")
        return 0
    finally:
        for _, p in reversed(processes): B.stop_process(p)
        for f in files:
            try: f.close()
            except Exception: pass
        for ns in (ns_c, ns_s):
            B.run(["ip", "netns", "del", ns], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


if __name__ == "__main__":
    raise SystemExit(main())
