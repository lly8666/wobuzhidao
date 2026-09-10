#!/usr/bin/env python3
import csv
import json
import pathlib
import re
import statistics
import sys

log_dir = pathlib.Path(sys.argv[1])
lanes = int(sys.argv[2])
fec = sys.argv[3]
duration = int(sys.argv[4])
rate = int(sys.argv[5])


def pct(values, q):
    if not values:
        return None
    v = sorted(values)
    x = (len(v) - 1) * q
    lo = int(x)
    hi = min(lo + 1, len(v) - 1)
    frac = x - lo
    return v[lo] * (1 - frac) + v[hi] * frac


def read_kv(path):
    out = {}
    for raw in pathlib.Path(path).read_text().splitlines():
        if not raw or raw.startswith("#") or "=" not in raw:
            continue
        k, v = raw.split("=", 1)
        try:
            out[k] = int(v)
        except ValueError:
            try:
                out[k] = float(v)
            except ValueError:
                out[k] = v
    return out


load = json.loads((log_dir / "load-result.json").read_text())
before = read_kv(log_dir / "net-before.txt")
after = read_kv(log_dir / "net-after.txt")
net = {}
for k, av in after.items():
    bv = before.get(k, 0)
    if isinstance(av, (int, float)) and isinstance(bv, (int, float)):
        net[k] = av - bv

samples = []
with (log_dir / "process.csv").open(newline="", encoding="utf-8") as f:
    for r in csv.DictReader(f):
        samples.append({
            "elapsed": float(r["elapsed_sec"]),
            "pid": int(r["pid"]),
            "name": r["name"],
            "ticks": int(r["cpu_ticks"]),
            "rss": int(r["rss_kib"]),
            "clk": int(r["clk_tck"]),
        })

by_pid = {}
by_ts = {}
for r in samples:
    by_pid.setdefault(r["pid"], []).append(r)
    # Monitor writes one common elapsed value per sampling sweep.
    by_ts.setdefault(r["elapsed"], []).append(r)

proc_by_name = {}
total_cpu_sec = 0.0
for pid, rows in by_pid.items():
    rows.sort(key=lambda x: x["elapsed"])
    cpu_sec = max(0, rows[-1]["ticks"] - rows[0]["ticks"]) / rows[-1]["clk"]
    total_cpu_sec += cpu_sec
    name = rows[-1]["name"]
    d = proc_by_name.setdefault(name, {"cpu_seconds": 0.0, "pids": 0, "peak_rss_kib": 0})
    d["cpu_seconds"] += cpu_sec
    d["pids"] += 1
    d["peak_rss_kib"] = max(d["peak_rss_kib"], max(x["rss"] for x in rows))

for d in proc_by_name.values():
    d["cpu_seconds"] = round(d["cpu_seconds"], 6)

rss_totals = [sum(x["rss"] for x in rows) for _, rows in sorted(by_ts.items())]
monitor_elapsed = 0.0
if samples:
    monitor_elapsed = max(x["elapsed"] for x in samples) - min(x["elapsed"] for x in samples)
process = {
    "sample_interval_sec": 2,
    "sample_sweeps": len(by_ts),
    "monitor_elapsed_sec": monitor_elapsed,
    "cpu_seconds_total": total_cpu_sec,
    "cpu_avg_percent_one_core_equivalent": (total_cpu_sec / monitor_elapsed * 100 if monitor_elapsed > 0 else None),
    "rss_total_kib": {
        "avg": (statistics.fmean(rss_totals) if rss_totals else None),
        "p95": pct(rss_totals, 0.95),
        "peak": (max(rss_totals) if rss_totals else None),
    },
    "by_executable": proc_by_name,
}

# Client FakeTCP emits one final JSON receipt per lane when SIGTERM closes it.
fake = {
    "lanes_with_final_stats": 0,
    "enqueued_bytes": 0,
    "retransmit_bytes": 0,
    "fast_retransmits": 0,
    "rto_transmits": 0,
    "loss_marked": 0,
    "loss_marked_bytes": 0,
    "peak_pending_max": 0,
    "raw_tx_packets": 0,
    "raw_rx_packets": 0,
    "data_tx_packets": 0,
    "data_rx_packets": 0,
}
for p in sorted(log_dir.glob("faketcp-*.log")):
    for line in p.read_text(errors="replace").splitlines():
        marker = "WBD_FAKETCP_STATS "
        pos = line.find(marker)
        if pos < 0:
            continue
        try:
            d = json.loads(line[pos + len(marker):])
        except json.JSONDecodeError:
            continue
        if d.get("role") != "client":
            continue
        s = d.get("sender", {})
        fake["lanes_with_final_stats"] += 1
        fake["enqueued_bytes"] += int(s.get("EnqueuedBytes", 0))
        fake["retransmit_bytes"] += int(s.get("RetransmitBytes", 0))
        fake["fast_retransmits"] += int(s.get("FastRetransmits", 0))
        fake["rto_transmits"] += int(s.get("RTOTransmits", 0))
        fake["loss_marked"] += int(s.get("LossMarked", 0))
        fake["loss_marked_bytes"] += int(s.get("LossMarkedBytes", 0))
        fake["peak_pending_max"] = max(fake["peak_pending_max"], int(s.get("PeakPending", 0)))
        fake["raw_tx_packets"] += int(d.get("raw_tx", 0))
        fake["raw_rx_packets"] += int(d.get("raw_rx", 0))
        fake["data_tx_packets"] += int(d.get("data_tx", 0))
        fake["data_rx_packets"] += int(d.get("data_rx", 0))
        break
fake["retransmit_over_enqueued_ratio"] = (
    fake["retransmit_bytes"] / fake["enqueued_bytes"] if fake["enqueued_bytes"] else None
)

sent_bytes = int(load.get("sent_payload_bytes", 0))
recv_bytes = int(load.get("received_payload_bytes", 0))
client_tx = int(net.get("client_tx_bytes", 0))
client_rx = int(net.get("client_rx_bytes", 0))
server_tx = int(net.get("server_tx_bytes", 0))
server_rx = int(net.get("server_rx_bytes", 0))
wire = {
    "interface_delta": net,
    "client_tx_over_app_sent": (client_tx / sent_bytes if sent_bytes else None),
    "client_rx_over_app_unique_received": (client_rx / recv_bytes if recv_bytes else None),
    "server_rx_over_app_sent": (server_rx / sent_bytes if sent_bytes else None),
    "server_tx_over_app_unique_received": (server_tx / recv_bytes if recv_bytes else None),
    "client_public_tx_bps": client_tx * 8 / duration,
    "client_public_rx_bps": client_rx * 8 / duration,
    "server_public_tx_bps": server_tx * 8 / duration,
    "server_public_rx_bps": server_rx * 8 / duration,
}

result = {
    "analysis_only": True,
    "qualification_authority": False,
    "loss_gate": "disabled_analysis_only",
    "source_sha": "d1e2c827183415b32d94082e2ca2a59cefdc544b",
    "lanes": lanes,
    "fec": fec,
    "requested_duration_sec": duration,
    "requested_bps_each_direction": rate,
    "weak_network": {"delay_ms_each_direction": 300, "random_loss_pct_each_direction": 20},
    "firewall": {
        "client": "public-interface default-drop; exact FakeTCP service flow allow; kernel RST suppressor retained",
        "server": "public-interface default-drop; exact FakeTCP service flow allow; kernel RST suppressor retained",
    },
    "payload": load.get("payload_profile"),
    "application": load,
    "process": process,
    "client_faketcp_lifetime": fake,
    "public_wire": wire,
}
(log_dir / "analysis-result.json").write_text(json.dumps(result, sort_keys=True, indent=2) + "\n")
print("WBD_REALISTIC_ANALYSIS_RESULT " + json.dumps(result, sort_keys=True))
