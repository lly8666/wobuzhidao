#!/usr/bin/env python3
"""Focused WBD-owned FEC vs UDPspeeder 20:20 A/B.

Every point gets fresh namespaces and a fresh DTLS/FakeTCP/FEC association.
The impairment is installed before establishment.  Both variants use the exact
same DTLS 1.3 shim and udp2raw FakeTCP carrier; only the FEC process is swapped.

Bulk is one-way and uses a fixed active-send denominator.  Latency is measured
separately with a low-rate request/echo probe, avoiding the old RTT-dependent
post-send drain accounting defect.
"""
from __future__ import annotations

import argparse
import csv
import importlib.util
import json
import os
import subprocess
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
    for ns, dev, addr in ((ns_c, "vc", "10.81.0.1/24"), (ns_s, "vs", "10.81.0.2/24")):
        B.run_ns(ns, ["ip", "link", "set", "lo", "up"])
        B.run_ns(ns, ["ip", "addr", "add", addr, "dev", dev])
        B.run_ns(ns, ["ip", "link", "set", dev, "up"])


def shape(ns: str, dev: str, half_rtt_ms: float, loss: float, link_mbps: int, seed: int) -> None:
    cmd = ["tc", "qdisc", "replace", "dev", dev, "root", "netem", "limit", "50000",
           "delay", f"{half_rtt_ms:g}ms"]
    if loss > 0:
        cmd += ["loss", "random", f"{loss:g}%", "seed", str(seed)]
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
    return {k: max(0.0, (b.get(k, 0.0) - v) * 1000.0) for k, v in a.items()}


def stop_named(processes: list[tuple[str, subprocess.Popen]], name: str) -> None:
    for n, p in processes:
        if n == name:
            B.stop_process(p)
            return


def case_row(a, mode: str, rtt: int, loss: float, seed: int, case_dir: Path) -> dict:
    ns_c = f"wbdabc{os.getpid()}"
    ns_s = f"wbdabs{os.getpid()}"
    base = 50100
    processes: list[tuple[str, subprocess.Popen]] = []
    files: list[object] = []
    row = {
        "mode": mode, "rtt_ms": rtt, "loss_pct_per_direction": loss, "seed": seed,
        "link_mbps": a.link_mbps, "inner_offered_mbps": a.inner_mbps,
        "established": False, "establish_ms": None, "failure": "",
        "bulk_delivery_ratio": None, "bulk_delivered_mbps": None,
        "bulk_oneway_p50_ms": None, "bulk_oneway_p95_ms": None, "bulk_oneway_p99_ms": None,
        "latency_delivery_ratio": None, "latency_p50_ms": None, "latency_p95_ms": None, "latency_p99_ms": None,
        "bulk_wall_s": None, "outer_c2s_mbps": None, "outer_s2c_mbps": None,
        "outer_c2s_drop": None, "outer_s2c_drop": None,
        "fec_cpu_ms": None, "total_cpu_ms": None, "total_cpu_cores_actual_wall": None,
        "fec_rss_peak_kb": None, "total_rss_peak_kb": None,
        "client_rss_peak_kb": None, "server_rss_peak_kb": None,
    }
    try:
        setup(ns_c, ns_s)
        shape(ns_c, "vc", rtt / 2.0, loss, a.link_mbps, seed)
        shape(ns_s, "vs", rtt / 2.0, loss, a.link_mbps, seed ^ 0x5A5A)

        # Low-rate echo service exists before FEC/DTLS/FakeTCP establishment.
        p, f = B.start_ns(ns_s, [a.latency_probe, "server", str(base)], case_dir / "echo.log")
        processes.append(("echo", p)); files.append(f)

        if mode == "speeder":
            p, f = B.start_ns(ns_s, [a.speeder, "-s", f"-l127.0.0.1:{base+1}", f"-r127.0.0.1:{base}",
                                        "-f20:20", "--mode", "0", "--timeout", "8", "-k", "wbd-fec-ab",
                                        "--disable-color", "--log-level", "2"], case_dir / "fec-server.log")
        else:
            p, f = B.start_ns(ns_s, [a.wbd_fec, "server", str(base+1), str(base+2), str(base)], case_dir / "fec-server.log")
        processes.append(("fec-server", p)); files.append(f)

        p, f = B.start_ns(ns_s, [a.shim, "server", str(base+2), "127.0.0.1", str(base+1),
                                    a.cert_dir / "server.pem", a.cert_dir / "server.key"], case_dir / "dtls-server.log")
        processes.append(("dtls-server", p)); files.append(f)
        p, f = B.start_ns(ns_s, [a.udp2raw, "-s", f"-l10.81.0.2:{base+3}", f"-r127.0.0.1:{base+2}",
                                    "-k", "wbd-fec-ab", "--raw-mode", "faketcp", "-a",
                                    "--disable-color", "--log-level", "2"], case_dir / "udp2raw-server.log")
        processes.append(("udp2raw-server", p)); files.append(f)
        time.sleep(.20)

        establish_start = time.monotonic()
        p, f = B.start_ns(ns_c, [a.udp2raw, "-c", f"-l127.0.0.1:{base+4}", f"-r10.81.0.2:{base+3}",
                                    "-k", "wbd-fec-ab", "--raw-mode", "faketcp", "-a",
                                    "--source-ip", "10.81.0.1", "--source-port", str(base+10),
                                    "--disable-color", "--log-level", "2"], case_dir / "udp2raw-client.log")
        processes.append(("udp2raw-client", p)); files.append(f)
        time.sleep(.25)
        p, f = B.start_ns(ns_c, [a.shim, "client", str(base+5), "127.0.0.1", str(base+4),
                                    a.cert_dir / "ca.pem", "wbd.test"], case_dir / "dtls-client.log")
        processes.append(("dtls-client", p)); files.append(f)
        rm = dict(processes)
        timeout = max(12.0, rtt / 1000.0 * 25.0 + 8.0)
        B.wait_log(case_dir / "dtls-server.log", "READY role=server", rm["dtls-server"], timeout)
        B.wait_log(case_dir / "dtls-client.log", "READY role=client", rm["dtls-client"], timeout)

        if mode == "speeder":
            p, f = B.start_ns(ns_c, [a.speeder, "-c", f"-l127.0.0.1:{base+7}", f"-r127.0.0.1:{base+5}",
                                        "-f20:20", "--mode", "0", "--timeout", "8", "-k", "wbd-fec-ab",
                                        "--disable-color", "--log-level", "2"], case_dir / "fec-client.log")
        else:
            p, f = B.start_ns(ns_c, [a.wbd_fec, "client", str(base+7), str(base+5)], case_dir / "fec-client.log")
        processes.append(("fec-client", p)); files.append(f)
        if mode == "wbd":
            B.wait_log(case_dir / "fec-server.log", "READY role=server", dict(processes)["fec-server"], 2)
            B.wait_log(case_dir / "fec-client.log", "READY role=client", dict(processes)["fec-client"], 2)
        else:
            time.sleep(.25)
        row["established"] = True
        row["establish_ms"] = (time.monotonic() - establish_start) * 1000.0

        # Separate low-rate request/echo latency probe. Its throughput is ignored.
        lat_out = case_dir / "latency.json"
        latency_timeout_ms = max(1500, rtt * 5 + 1000)
        B.run_ns(ns_c, [a.latency_probe, "client", str(base+7), "0.20", "1.0", "256",
                            str(latency_timeout_ms), lat_out], check=False)
        if lat_out.exists():
            lat = json.loads(lat_out.read_text())
            row["latency_delivery_ratio"] = lat.get("delivery_ratio_vs_sent")
            row["latency_p50_ms"] = lat.get("p50_ms")
            row["latency_p95_ms"] = lat.get("p95_ms")
            row["latency_p99_ms"] = lat.get("p99_ms")

        # Replace echo by a one-way sink while leaving the exact transport stack up.
        stop_named(processes, "echo")
        time.sleep(.05)
        bulk_out = case_dir / "bulk.json"
        drain_ms = max(500, rtt * 2 + 250)
        p, f = B.start_ns(ns_s, [a.oneway_probe, "server", str(base), str(a.duration), str(drain_ms), bulk_out],
                            case_dir / "bulk-server.log")
        processes.append(("bulk-server", p)); files.append(f)
        time.sleep(.08)

        roles = {k: p for k, p in processes if k not in ("echo", "bulk-server")}
        client_roles = {k: p for k, p in roles.items() if k.endswith("client")}
        server_roles = {k: p for k, p in roles.items() if k.endswith("server")}
        c0, _ = stats_for(roles)
        peak = {k: B.tree_stats(p.pid)[1] for k, p in roles.items()}
        stop = threading.Event()
        def monitor() -> None:
            while not stop.is_set():
                for k, p in roles.items():
                    peak[k] = max(peak[k], B.tree_stats(p.pid)[1])
                time.sleep(.01)
        th = threading.Thread(target=monitor, daemon=True); th.start()
        q0c, q0s = B.tc_stats(ns_c, "vc"), B.tc_stats(ns_s, "vs")
        t0 = time.monotonic()
        cp = B.run_ns(ns_c, [a.oneway_probe, "client", str(base+7), str(a.inner_mbps), str(a.duration),
                               str(a.packet_size)], check=False, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        try:
            p.wait(timeout=max(4.0, a.duration + drain_ms / 1000.0 + 2.0))
        except subprocess.TimeoutExpired:
            B.stop_process(p)
        bulk_wall = time.monotonic() - t0
        stop.set(); th.join(timeout=.5)
        c1, _ = stats_for(roles)
        q1c, q1s = B.tc_stats(ns_c, "vc"), B.tc_stats(ns_s, "vs")
        cpu = delta_ms(c0, c1)
        dc, ds = B.counter_delta(q0c, q1c), B.counter_delta(q0s, q1s)
        row["bulk_wall_s"] = bulk_wall
        if bulk_out.exists():
            bulk = json.loads(bulk_out.read_text())
            row["bulk_delivery_ratio"] = bulk.get("delivery_ratio")
            row["bulk_delivered_mbps"] = bulk.get("delivered_mbps_active")
            row["bulk_oneway_p50_ms"] = bulk.get("oneway_p50_ms")
            row["bulk_oneway_p95_ms"] = bulk.get("oneway_p95_ms")
            row["bulk_oneway_p99_ms"] = bulk.get("oneway_p99_ms")
        if isinstance(dc.get("bytes"), int): row["outer_c2s_mbps"] = dc["bytes"] * 8.0 / a.duration / 1e6
        if isinstance(ds.get("bytes"), int): row["outer_s2c_mbps"] = ds["bytes"] * 8.0 / a.duration / 1e6
        row["outer_c2s_drop"] = dc.get("dropped")
        row["outer_s2c_drop"] = ds.get("dropped")
        row["fec_cpu_ms"] = cpu.get("fec-client", 0.0) + cpu.get("fec-server", 0.0)
        row["total_cpu_ms"] = sum(cpu.values())
        row["total_cpu_cores_actual_wall"] = sum(cpu.values()) / max(1.0, bulk_wall * 1000.0)
        row["fec_rss_peak_kb"] = peak.get("fec-client", 0) + peak.get("fec-server", 0)
        row["total_rss_peak_kb"] = sum(peak.values())
        row["client_rss_peak_kb"] = sum(peak[k] for k in client_roles)
        row["server_rss_peak_kb"] = sum(peak[k] for k in server_roles)
        row["bulk_client_rc"] = cp.returncode
        row["bulk_client_log"] = cp.stdout[-500:] if cp.stdout else ""
        for k, v in cpu.items(): row["cpu_ms_" + k.replace("-", "_")] = v
        for k, v in peak.items(): row["rss_kb_" + k.replace("-", "_")] = v
        return row
    except Exception as exc:
        row["failure"] = f"{type(exc).__name__}: {exc}"
        return row
    finally:
        for _, p in reversed(processes): B.stop_process(p)
        for f in files:
            try: f.close()
            except Exception: pass
        for ns in (ns_c, ns_s):
            B.run(["ip", "netns", "del", ns], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--udp2raw", type=Path, required=True)
    ap.add_argument("--speeder", type=Path, required=True)
    ap.add_argument("--wbd-fec", type=Path, required=True)
    ap.add_argument("--shim", type=Path, required=True)
    ap.add_argument("--cert-dir", type=Path, required=True)
    ap.add_argument("--latency-probe", type=Path, required=True)
    ap.add_argument("--oneway-probe", type=Path, required=True)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--rtt", type=int, required=True)
    ap.add_argument("--losses", default="0,1,5,10,20")
    ap.add_argument("--link-mbps", type=int, default=200)
    ap.add_argument("--inner-mbps", type=float, default=75.0)
    ap.add_argument("--duration", type=float, default=2.0)
    ap.add_argument("--packet-size", type=int, default=1200)
    ap.add_argument("--seed", type=int, default=260825)
    a = ap.parse_args()
    a.out.mkdir(parents=True, exist_ok=True)
    if B.sha256(a.udp2raw) != B.EXPECTED_UDP2RAW_SHA256: raise SystemExit("udp2raw sha mismatch")
    if B.sha256(a.speeder) != B.EXPECTED_SPEEDER_SHA256: raise SystemExit("speeder sha mismatch")

    rows = []
    losses = [float(x) for x in a.losses.split(",")]
    for loss_index, loss in enumerate(losses):
        for mode_index, mode in enumerate(("speeder", "wbd")):
            case_dir = a.out / f"{mode}-rtt{a.rtt}-loss{loss:g}"
            case_dir.mkdir(parents=True, exist_ok=True)
            row = case_row(a, mode, a.rtt, loss, a.seed + loss_index * 17 + mode_index * 0, case_dir)
            rows.append(row)
            print("WBD_FEC_AB_CASE " + json.dumps(row, sort_keys=True), flush=True)

    # Stable union of fields lets failed and successful rows coexist.
    fields = []
    for row in rows:
        for key in row:
            if key not in fields: fields.append(key)
    with (a.out / "results.csv").open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=fields)
        w.writeheader(); w.writerows(rows)
    (a.out / "receipt.json").write_text(json.dumps({
        "schema": "wbd-fec-ab/v1", "rtt_ms": a.rtt, "losses": losses,
        "link_mbps_per_direction": a.link_mbps, "inner_offered_mbps_oneway": a.inner_mbps,
        "bulk_duration_s": a.duration, "packet_size": a.packet_size,
        "variants": ["UDPspeeder mode0 20:20", "WBD systematic RS 20+20"], "results": rows,
    }, indent=2) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
