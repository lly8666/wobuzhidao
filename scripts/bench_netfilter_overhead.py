#!/usr/bin/env python3
"""Measure incremental Linux forwarding/filter/NAT overhead with a routed netns topology.

This deliberately isolates kernel networking from WBD userspace so the delta is
interpretable.  It compares plain L3 forwarding, FORWARD+conntrack filtering,
and DNAT+MASQUERADE+FORWARD using the same fixed UDP offered load.
"""
from __future__ import annotations

import argparse
import json
import os
import re
import statistics
import subprocess
import time
from pathlib import Path

CLK = os.sysconf(os.sysconf_names["SC_CLK_TCK"])


def run(cmd, *, check=True, **kw):
    return subprocess.run([str(x) for x in cmd], check=check, **kw)


def ns(name, cmd, *, check=True, **kw):
    return run(["ip", "netns", "exec", name, *map(str, cmd)], check=check, **kw)


def cpu_snapshot():
    f = Path("/proc/stat").read_text().splitlines()[0].split()[1:]
    v = [int(x) for x in f]
    total = sum(v)
    idle = v[3] + (v[4] if len(v) > 4 else 0)
    return total, idle


def softirq_snapshot():
    out = {}
    for line in Path("/proc/softirqs").read_text().splitlines():
        if ":" not in line:
            continue
        name, rest = line.split(":", 1)
        name = name.strip()
        if name in {"NET_RX", "NET_TX"}:
            out[name] = sum(int(x) for x in rest.split())
    return out


def slab_netfilter_active_bytes():
    total = 0
    detail = {}
    try:
        lines = Path("/proc/slabinfo").read_text().splitlines()[2:]
    except PermissionError:
        return None, {}
    for line in lines:
        p = line.split()
        if len(p) < 4:
            continue
        name = p[0]
        if name.startswith("nft_") or name.startswith("nf_conntrack"):
            try:
                active, objsize = int(p[1]), int(p[3])
            except ValueError:
                continue
            b = active * objsize
            detail[name] = {"active": active, "object_size": objsize, "active_bytes": b}
            total += b
    return total, detail


def conntrack_count(router):
    cp = ns(router, ["sysctl", "-n", "net.netfilter.nf_conntrack_count"], check=False,
            text=True, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL)
    if cp.returncode:
        return None
    try:
        return int(cp.stdout.strip())
    except ValueError:
        return None


def configure_variant(router, variant, server_ip, port):
    ns(router, ["iptables", "-F"])
    ns(router, ["iptables", "-t", "nat", "-F"])
    ns(router, ["iptables", "-P", "FORWARD", "ACCEPT"])
    ns(router, ["conntrack", "-F"], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if variant == "route":
        return
    ns(router, ["iptables", "-P", "FORWARD", "DROP"])
    ns(router, ["iptables", "-A", "FORWARD", "-m", "conntrack", "--ctstate", "ESTABLISHED,RELATED", "-j", "ACCEPT"])
    ns(router, ["iptables", "-A", "FORWARD", "-p", "tcp", "-d", server_ip, "--dport", str(port), "-m", "conntrack", "--ctstate", "NEW", "-j", "ACCEPT"])
    ns(router, ["iptables", "-A", "FORWARD", "-p", "udp", "-d", server_ip, "--dport", str(port), "-m", "conntrack", "--ctstate", "NEW", "-j", "ACCEPT"])
    if variant == "filter":
        return
    # NAT target is the router's client-side address.  Both iperf3 TCP control
    # and UDP data need translation on the same port.
    for proto in ("tcp", "udp"):
        ns(router, ["iptables", "-t", "nat", "-A", "PREROUTING", "-p", proto,
                    "-d", "10.90.1.1", "--dport", str(port), "-j", "DNAT",
                    "--to-destination", f"{server_ip}:{port}"])
    ns(router, ["iptables", "-t", "nat", "-A", "POSTROUTING", "-o", "rs",
                "-d", server_ip, "-j", "MASQUERADE"])


def one_run(client, router, server, variant, port, rate, seconds):
    configure_variant(router, variant, "10.90.2.2", port)
    target = "10.90.1.1" if variant == "nat" else "10.90.2.2"
    time.sleep(0.15)
    slab_before, slab_detail_before = slab_netfilter_active_bytes()
    ct_before = conntrack_count(router)
    cpu0 = cpu_snapshot(); si0 = softirq_snapshot(); t0 = time.monotonic()
    cp = ns(client, ["iperf3", "-c", target, "-p", str(port), "-u", "-b", rate,
                     "-l", "1200", "-t", str(seconds), "-J"],
            text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    wall = time.monotonic() - t0; cpu1 = cpu_snapshot(); si1 = softirq_snapshot()
    slab_after, slab_detail_after = slab_netfilter_active_bytes(); ct_after = conntrack_count(router)
    if cp.returncode:
        raise RuntimeError(f"iperf3 {variant} failed rc={cp.returncode}: {cp.stderr[-2000:]}")
    j = json.loads(cp.stdout)
    end = j.get("end", {})
    recv = end.get("sum_received", {}) or end.get("sum", {})
    cpu = end.get("cpu_utilization_percent", {})
    total0, idle0 = cpu0; total1, idle1 = cpu1
    busy_j = (total1-total0) - (idle1-idle0)
    busy_core_s = busy_j / CLK
    return {
        "variant": variant,
        "target": target,
        "wall_s": wall,
        "recv_mbps": recv.get("bits_per_second", 0) / 1e6,
        "lost_percent": recv.get("lost_percent"),
        "packets": recv.get("packets"),
        "host_cpu_percent_iperf": cpu.get("host_total"),
        "remote_cpu_percent_iperf": cpu.get("remote_total"),
        "system_busy_core_s": busy_core_s,
        "system_busy_avg_cores": busy_core_s / wall if wall else None,
        "softirq_net_rx_delta": si1.get("NET_RX", 0)-si0.get("NET_RX", 0),
        "softirq_net_tx_delta": si1.get("NET_TX", 0)-si0.get("NET_TX", 0),
        "conntrack_before": ct_before,
        "conntrack_after": ct_after,
        "netfilter_slab_active_bytes_before": slab_before,
        "netfilter_slab_active_bytes_after": slab_after,
        "netfilter_slab_detail_before": slab_detail_before,
        "netfilter_slab_detail_after": slab_detail_after,
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--rate", default="300M")
    ap.add_argument("--seconds", type=int, default=3)
    ap.add_argument("--repeats", type=int, default=3)
    a = ap.parse_args()
    if os.geteuid() != 0:
        raise SystemExit("requires root")
    a.out.parent.mkdir(parents=True, exist_ok=True)
    suffix = str(os.getpid())
    c, r, s = f"wbdnf-c-{suffix}", f"wbdnf-r-{suffix}", f"wbdnf-s-{suffix}"
    port = 55000
    rows = []
    server_proc = None
    try:
        for n in (c, r, s): run(["ip", "netns", "add", n])
        run(["ip", "link", "add", "cr", "type", "veth", "peer", "name", "rc"])
        run(["ip", "link", "add", "rs", "type", "veth", "peer", "name", "sr"])
        run(["ip", "link", "set", "cr", "netns", c]); run(["ip", "link", "set", "rc", "netns", r])
        run(["ip", "link", "set", "rs", "netns", r]); run(["ip", "link", "set", "sr", "netns", s])
        for n in (c, r, s): ns(n, ["ip", "link", "set", "lo", "up"])
        ns(c, ["ip", "addr", "add", "10.90.1.2/24", "dev", "cr"]); ns(c, ["ip", "link", "set", "cr", "up"])
        ns(r, ["ip", "addr", "add", "10.90.1.1/24", "dev", "rc"]); ns(r, ["ip", "link", "set", "rc", "up"])
        ns(r, ["ip", "addr", "add", "10.90.2.1/24", "dev", "rs"]); ns(r, ["ip", "link", "set", "rs", "up"])
        ns(s, ["ip", "addr", "add", "10.90.2.2/24", "dev", "sr"]); ns(s, ["ip", "link", "set", "sr", "up"])
        ns(c, ["ip", "route", "add", "10.90.2.0/24", "via", "10.90.1.1"])
        ns(s, ["ip", "route", "add", "10.90.1.0/24", "via", "10.90.2.1"])
        ns(r, ["sysctl", "-w", "net.ipv4.ip_forward=1"], stdout=subprocess.DEVNULL)
        ns(c, ["ping", "-c", "1", "-W", "1", "10.90.2.2"], stdout=subprocess.DEVNULL)
        server_proc = subprocess.Popen(["ip", "netns", "exec", s, "iperf3", "-s", "-p", str(port)],
                                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        time.sleep(0.3)
        variants = ("route", "filter", "nat")
        for rep in range(a.repeats):
            for variant in variants:
                row = one_run(c, r, s, variant, port, a.rate, a.seconds)
                row["repeat"] = rep
                rows.append(row)
                print("NETFILTER_CASE " + json.dumps({k: row[k] for k in (
                    "variant","repeat","recv_mbps","lost_percent","system_busy_avg_cores",
                    "host_cpu_percent_iperf","remote_cpu_percent_iperf","conntrack_after")}), flush=True)
        summary = {}
        for variant in variants:
            vr = [x for x in rows if x["variant"] == variant]
            summary[variant] = {
                "recv_mbps_median": statistics.median(x["recv_mbps"] for x in vr),
                "lost_percent_median": statistics.median((x["lost_percent"] or 0) for x in vr),
                "system_busy_avg_cores_median": statistics.median(x["system_busy_avg_cores"] for x in vr),
                "host_cpu_percent_iperf_median": statistics.median(x["host_cpu_percent_iperf"] for x in vr),
                "remote_cpu_percent_iperf_median": statistics.median(x["remote_cpu_percent_iperf"] for x in vr),
                "conntrack_after_median": statistics.median(x["conntrack_after"] or 0 for x in vr),
            }
        result = {"schema":"wbd-netfilter-overhead/v1","rate":a.rate,"seconds":a.seconds,
                  "repeats":a.repeats,"rows":rows,"summary":summary}
        a.out.write_text(json.dumps(result, indent=2) + "\n")
        print("NETFILTER_SUMMARY " + json.dumps(summary, sort_keys=True), flush=True)
    finally:
        if server_proc is not None:
            server_proc.terminate()
            try: server_proc.wait(timeout=1)
            except subprocess.TimeoutExpired: server_proc.kill()
        for n in (c, r, s): run(["ip", "netns", "del", n], check=False,
                                 stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

if __name__ == "__main__":
    main()
