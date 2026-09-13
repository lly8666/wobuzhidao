#!/usr/bin/env python3
import csv
import json
import math
import os
import re
import sys
from pathlib import Path

ROOT = Path(sys.argv[1]) if len(sys.argv) > 1 else Path("artifacts")
OUT = Path(sys.argv[2]) if len(sys.argv) > 2 else Path("analysis")
OUT.mkdir(parents=True, exist_ok=True)


def load_json(path):
    try:
        return json.loads(path.read_text())
    except Exception:
        return {}


def fnum(x, default=None):
    try:
        return float(x)
    except Exception:
        return default


def inum(x, default=0):
    try:
        return int(x)
    except Exception:
        return default


def mbps(byte_count, seconds=120.0):
    if byte_count is None:
        return None
    return float(byte_count) * 8.0 / max(seconds, 1e-9) / 1e6


def fmt(v, digits=3):
    if v is None:
        return ""
    if isinstance(v, bool):
        return "yes" if v else "no"
    if isinstance(v, int):
        return str(v)
    if isinstance(v, float):
        return f"{v:.{digits}f}"
    return str(v)


def find_log(logdir, name):
    hits = list(logdir.rglob(name)) if logdir.exists() else []
    return hits[0] if hits else None


def parse_kv(line):
    out = {}
    for k, v in re.findall(r"([A-Za-z0-9_]+)=([-+0-9.eE]+)", line):
        try:
            out[k] = int(v)
        except ValueError:
            try:
                out[k] = float(v)
            except ValueError:
                pass
    return out


def parse_final_game(logdir):
    p = find_log(logdir, "game-client.log")
    if not p:
        # Historical helper sometimes gives a different basename; scan logs.
        candidates = list(logdir.rglob("*.log")) if logdir.exists() else []
    else:
        candidates = [p]
    final = {}
    rows = []
    for q in candidates:
        try:
            lines = q.read_text(errors="replace").splitlines()
        except OSError:
            continue
        for line in lines:
            if "WBD_GAME_PATH_DIAG " in line:
                rows.append(parse_kv(line))
            if "WBD_GAME_PATH_FINAL " in line:
                final = parse_kv(line)
    return final, rows


def socket_samples(path):
    rows = []
    if not path.exists():
        return rows
    for line in path.read_text(errors="replace").splitlines():
        try:
            obj = json.loads(line)
        except Exception:
            continue
        epoch = inum(obj.get("epoch_ns"), 0)
        for s in obj.get("sockets") or []:
            rows.append({"epoch_ns": epoch, **s})
    return rows


def socket_local(s):
    return str(s.get("local") or "")


def sock_drop(s):
    return max(inum(s.get("drops"), 0), inum(s.get("ss_drops"), 0))


def summarize_sockets(rows, start_ns):
    # Baseline each stable socket by inode to avoid claiming pre-existing drops.
    baselines = {}
    max_seen = {}
    first_drop = None
    first_any_drop = None
    first_80 = None
    first_any_80 = None
    max47000_q = 0
    max47000_rb = 0
    max47000_drop_delta = 0
    server_drop_delta = 0
    other_drop_delta = 0
    port47000_samples = 0
    for r in sorted(rows, key=lambda x: x.get("epoch_ns", 0)):
        epoch = inum(r.get("epoch_ns"), 0)
        if start_ns and epoch < start_ns:
            # Still capture a true pre-load baseline.
            pass
        inode = str(r.get("inode") or f"{r.get('pid')}:{socket_local(r)}")
        d = sock_drop(r)
        if inode not in baselines:
            baselines[inode] = d
        max_seen[inode] = max(max_seen.get(inode, d), d)
        delta = max(0, d - baselines[inode])
        rel = (epoch - start_ns) / 1e9 if start_ns else None
        rb = inum(r.get("so_rcvbuf_effective_bytes"), 0)
        q = inum(r.get("rx_queue_bytes"), 0)
        role = str(r.get("role") or "")
        is47000 = socket_local(r).endswith(":47000")
        if delta > 0 and first_any_drop is None and (rel is None or rel >= 0):
            first_any_drop = {"t_sec": rel, "local": socket_local(r), "process": r.get("process"), "role": role, "drop_delta": delta}
        if rb > 0 and q >= 0.8 * rb and first_any_80 is None and (rel is None or rel >= 0):
            first_any_80 = {"t_sec": rel, "local": socket_local(r), "process": r.get("process"), "role": role, "rx_queue_bytes": q, "rcvbuf_bytes": rb}
        if role == "server":
            server_drop_delta = max(server_drop_delta, delta) if not is47000 else server_drop_delta
        if is47000:
            port47000_samples += 1
            max47000_q = max(max47000_q, q)
            max47000_rb = max(max47000_rb, rb)
            max47000_drop_delta = max(max47000_drop_delta, delta)
            if delta > 0 and first_drop is None and (rel is None or rel >= 0):
                first_drop = {"t_sec": rel, "drop_delta": delta, "rx_queue_bytes": q, "rcvbuf_bytes": rb}
            if rb > 0 and q >= 0.8 * rb and first_80 is None and (rel is None or rel >= 0):
                first_80 = {"t_sec": rel, "rx_queue_bytes": q, "rcvbuf_bytes": rb}
        elif delta > 0:
            other_drop_delta += delta
    # Summing per-inode final deltas is more meaningful for all sockets than max.
    per_inode_final = {inode: max(0, max_seen[inode] - baselines.get(inode, 0)) for inode in max_seen}
    all_drop_delta = sum(per_inode_final.values())
    return {
        "port47000_samples": port47000_samples,
        "port47000_max_rx_queue_bytes": max47000_q,
        "port47000_max_rcvbuf_bytes": max47000_rb,
        "port47000_max_queue_ratio": (max47000_q / max47000_rb if max47000_rb else None),
        "port47000_drop_delta": max47000_drop_delta,
        "all_socket_drop_delta": all_drop_delta,
        "other_socket_drop_delta": max(0, all_drop_delta - max47000_drop_delta),
        "first_47000_drop": first_drop,
        "first_any_drop": first_any_drop,
        "first_47000_queue80": first_80,
        "first_any_queue80": first_any_80,
    }


def phase_post5_drop_delta(audit):
    sd = audit.get("runtime_socket_diag") or {}
    pt = sd.get("phase_totals") or {}
    post = pt.get("post5") or {}
    # Keep both schemas usable; the raw value is only a convenience summary.
    vals = []
    def walk(x):
        if isinstance(x, dict):
            for k, v in x.items():
                if "drop" in str(k).lower() and isinstance(v, (int, float)):
                    vals.append(float(v))
                else:
                    walk(v)
        elif isinstance(x, list):
            for v in x: walk(v)
    walk(post)
    return max(vals) if vals else None


def first_load_deviation(load):
    for row in load.get("per_second") or []:
        ratio = fnum(row.get("actual_injection_ratio"))
        if ratio is not None and not (0.99 <= ratio <= 1.01):
            return {"t_sec": fnum(row.get("window_start_sec"), fnum(row.get("second"), 0.0)), "ratio": ratio, "p99_lag_ms": row.get("send_lag_ms_p99")}
    return None


def first_game_low(rows, start_ns, target_bps):
    # Diagnostic marker only: first complete ~1s interval with Game ingress <90% target.
    filt = [r for r in rows if inum(r.get("epoch_ns"), 0) >= start_ns] if start_ns else rows
    filt.sort(key=lambda x: inum(x.get("epoch_ns"), 0))
    prev = None
    for r in filt:
        if prev is None:
            prev = r
            continue
        dt = (inum(r.get("epoch_ns"), 0) - inum(prev.get("epoch_ns"), 0)) / 1e9
        if 0.5 <= dt <= 2.0:
            db = inum(r.get("app_rx_bytes"), 0) - inum(prev.get("app_rx_bytes"), 0)
            bps = max(0, db) * 8.0 / dt
            if bps < 0.9 * target_bps:
                return {"t_sec": (inum(r.get("epoch_ns"), 0) - start_ns) / 1e9 if start_ns else None, "game_ingress_bps": bps, "ratio": bps / target_bps}
        prev = r
    return None


def first_perf_event(audit, start_ns, key, threshold_us):
    for row in audit.get("link_mux_perf_diag", {}).get("rows") or []:
        epoch = inum(row.get("epoch_ns"), 0)
        if start_ns and epoch < start_ns:
            continue
        value = inum(row.get(key), 0)
        if value > threshold_us:
            return {"t_sec": (epoch - start_ns) / 1e9 if start_ns else None, "value_us": value}
    return None


def sample_root(audit_path):
    # audit lives result-CASE/transient-audit.json after paired workflow move.
    return audit_path.parent


samples = []
for audit_path in ROOT.rglob("transient-audit.json"):
    audit = load_json(audit_path)
    case = str(audit.get("test_variant") or audit_path.parent.name)
    if not case.startswith("p"):
        continue
    m = re.match(r"p(\d+)-([AB])-o(\d+)", case)
    if not m:
        continue
    pair = int(m.group(1)); variant = m.group(2); order = int(m.group(3))
    root = sample_root(audit_path)
    logdir = root / "logs"
    load = audit.get("load_measurement") or {}
    duration = 120.0
    target_bps = fnum(load.get("target_bps"), 15e6)
    start_ns = inum(load.get("load_start_epoch_ns"), 0)
    final_game, game_rows = parse_final_game(logdir)
    if not final_game:
        final_game = audit.get("game_path_diag", {}).get("final") or {}
    srows = socket_samples(root / "socket-runtime.jsonl")
    ss = summarize_sockets(srows, start_ns)
    app = audit.get("app") or {}
    validity = audit.get("sample_validity") or {}
    rcv = audit.get("server_link_rcvbuf") or {}
    requested = inum(rcv.get("requested_effective_bytes"), 0)
    row = {
        "case_id": case,
        "pair": pair,
        "variant": variant,
        "order": order,
        "target_mbps": target_bps / 1e6 if target_bps else None,
        "actual_injection_mbps": fnum(load.get("actual_injection_bps"), 0) / 1e6,
        "actual_injection_ratio": fnum(load.get("actual_injection_ratio")),
        "load_valid": bool(validity.get("load_valid")),
        "sample_valid": bool(validity.get("valid_for_buffer_capacity_compare")),
        "send_failures": inum(load.get("send_failures"), 0),
        "skipped_send_slots": inum(load.get("skipped_send_slots"), 0),
        "send_lag_p50_ms": fnum(load.get("send_lag_ms_p50")),
        "send_lag_p99_ms": fnum(load.get("send_lag_ms_p99")),
        "send_lag_max_ms": fnum(load.get("send_lag_ms_max")),
        "game_ingress_bytes": inum(final_game.get("app_rx_bytes"), 0),
        "game_ingress_mbps": mbps(inum(final_game.get("app_rx_bytes"), 0), duration),
        "final_received_bytes": inum(load.get("received_payload_bytes"), 0),
        "final_delivery_mbps": mbps(inum(load.get("received_payload_bytes"), 0), duration),
        "app_goodput_mbps": fnum(app.get("goodput_mbps")),
        "byte_loss_ratio": fnum((load.get("phase_metrics") or {}).get("all", {}).get("byte_loss_ratio"), fnum(app.get("byte_loss_ratio"))),
        "rtt_p50_ms": fnum(app.get("rtt_ms_p50")),
        "rtt_p95_ms": fnum(app.get("rtt_ms_p95")),
        "rtt_p99_ms": fnum(app.get("rtt_ms_p99")),
        "rcvbuf_effective_mib": requested / (1024 * 1024) if requested else None,
        "rcvbuf_exact": bool(validity.get("rcvbuf_exact")),
        "netem_valid": bool(validity.get("netem_valid")),
        "integrity_valid": bool(validity.get("transport_integrity_valid")),
        "product_sha": audit.get("product_source_sha"),
        "helper_sha": "cd5a78f7fd34df2d83854fbee7b3cf5e2f09632d",
        "port47000_drop_delta": ss["port47000_drop_delta"],
        "all_socket_drop_delta": ss["all_socket_drop_delta"],
        "other_socket_drop_delta": ss["other_socket_drop_delta"],
        "port47000_max_rx_queue_bytes": ss["port47000_max_rx_queue_bytes"],
        "port47000_max_rcvbuf_bytes": ss["port47000_max_rcvbuf_bytes"],
        "port47000_max_queue_ratio": ss["port47000_max_queue_ratio"],
        "post5_drop_metric": phase_post5_drop_delta(audit),
        "link_read_gap_max_us": inum((audit.get("link_mux_perf_diag") or {}).get("max_read_gap_us"), 0),
        "link_inbound_max_us": inum((audit.get("link_mux_perf_diag") or {}).get("max_inbound_us"), 0),
        "fec_peak": inum((audit.get("fec640") or {}).get("peak_in_flight"), 0),
        "audit_path": str(audit_path),
    }
    events = []
    for kind, ev in (
        ("load_1s_outside_99_101pct", first_load_deviation(load)),
        ("game_ingress_below_90pct_target", first_game_low(game_rows, start_ns, target_bps)),
        ("any_socket_queue_ge_80pct", ss["first_any_queue80"]),
        ("link47000_queue_ge_80pct", ss["first_47000_queue80"]),
        ("link_read_gap_gt_10ms", first_perf_event(audit, start_ns, "read_gap_max_us_interval", 10000)),
        ("link_inbound_call_gt_5ms", first_perf_event(audit, start_ns, "inbound_max_us_interval", 5000)),
        ("any_socket_drop_increase", ss["first_any_drop"]),
        ("link47000_drop_increase", ss["first_47000_drop"]),
    ):
        if ev and ev.get("t_sec") is not None and ev.get("t_sec") >= 0:
            events.append({"kind": kind, **ev})
    events.sort(key=lambda x: x["t_sec"])
    row["timeline"] = events
    samples.append(row)

samples.sort(key=lambda x: (x["pair"], x["order"]))

sample_fields = [
    "case_id","pair","variant","order","target_mbps","actual_injection_mbps","actual_injection_ratio","load_valid","sample_valid",
    "send_failures","skipped_send_slots","send_lag_p50_ms","send_lag_p99_ms","send_lag_max_ms",
    "game_ingress_mbps","final_delivery_mbps","app_goodput_mbps","rtt_p50_ms","rtt_p95_ms","rtt_p99_ms",
    "rcvbuf_effective_mib","rcvbuf_exact","netem_valid","integrity_valid","port47000_drop_delta","all_socket_drop_delta","other_socket_drop_delta",
    "port47000_max_rx_queue_bytes","port47000_max_rcvbuf_bytes","port47000_max_queue_ratio","link_read_gap_max_us","link_inbound_max_us","fec_peak",
    "product_sha","helper_sha","audit_path"
]
with open(OUT / "sample-validity.csv", "w", newline="") as f:
    w = csv.DictWriter(f, fieldnames=sample_fields)
    w.writeheader()
    for r in samples:
        w.writerow({k: r.get(k) for k in sample_fields})

md = ["# Sample validity", "", "Capacity comparisons require load + netem + transport integrity + exact socket-buffer checks.", "",
      "| case | pair/order | buf | target | injected | Game ingress | final delivery | valid | :47000 drops | max queue/buf | RTT p95 |",
      "|---|---:|---:|---:|---:|---:|---:|---|---:|---:|---:|"]
for r in samples:
    md.append("| `{}` | {}/{} | {} MiB | {} | {} | {} | {} | {} | {} | {} | {} |".format(
        r["case_id"], r["pair"], r["order"], fmt(r["rcvbuf_effective_mib"]), fmt(r["target_mbps"]), fmt(r["actual_injection_mbps"]),
        fmt(r["game_ingress_mbps"]), fmt(r["final_delivery_mbps"]), "VALID" if r["sample_valid"] else "INVALID", r["port47000_drop_delta"],
        fmt(r["port47000_max_queue_ratio"]), fmt(r["rtt_p95_ms"])))
(OUT / "sample-validity.md").write_text("\n".join(md) + "\n")

# Pair B(8MiB) - A(2MiB), regardless of AB/BA execution order.
pairs = []
for pair in sorted({r["pair"] for r in samples}):
    rr = [r for r in samples if r["pair"] == pair]
    a = next((r for r in rr if r["variant"] == "A"), None)
    b = next((r for r in rr if r["variant"] == "B"), None)
    if not a or not b:
        continue
    def delta(k):
        av, bv = a.get(k), b.get(k)
        if av is None or bv is None:
            return None
        return bv - av
    pairs.append({
        "pair": pair,
        "order": f"{rr[0]['variant']}→{rr[1]['variant']}" if len(rr) >= 2 else "",
        "A_valid": a["sample_valid"], "B_valid": b["sample_valid"], "paired_valid": a["sample_valid"] and b["sample_valid"],
        "injection_delta_mbps": delta("actual_injection_mbps"),
        "game_ingress_delta_mbps": delta("game_ingress_mbps"),
        "delivery_delta_mbps": delta("final_delivery_mbps"),
        "app_goodput_delta_mbps": delta("app_goodput_mbps"),
        "drop47000_delta": delta("port47000_drop_delta"),
        "all_socket_drop_delta": delta("all_socket_drop_delta"),
        "other_socket_drop_delta": delta("other_socket_drop_delta"),
        "rtt_p50_delta_ms": delta("rtt_p50_ms"),
        "rtt_p95_delta_ms": delta("rtt_p95_ms"),
        "rtt_p99_delta_ms": delta("rtt_p99_ms"),
        "queue_ratio_delta": delta("port47000_max_queue_ratio"),
        "read_gap_delta_us": delta("link_read_gap_max_us"),
        "inbound_max_delta_us": delta("link_inbound_max_us"),
        "A_case": a["case_id"], "B_case": b["case_id"],
    })

pair_fields = list(pairs[0].keys()) if pairs else ["pair"]
with open(OUT / "paired-deltas.csv", "w", newline="") as f:
    w = csv.DictWriter(f, fieldnames=pair_fields); w.writeheader(); w.writerows(pairs)
pmd = ["# Paired deltas: 8 MiB minus 2 MiB", "", "Positive delivery delta favors 8 MiB; positive RTT/queue delta means more latency/backlog. Only paired_valid rows support causal comparison.", "",
       "| pair | order | valid | Δ injection Mbps | Δ Game ingress | Δ delivery | Δ :47000 drops | Δ all socket drops | Δ RTT p95 ms | Δ queue/buf |",
       "|---:|---|---|---:|---:|---:|---:|---:|---:|---:|"]
for r in pairs:
    pmd.append("| {} | {} | {} | {} | {} | {} | {} | {} | {} | {} |".format(
        r["pair"], r["order"], "VALID" if r["paired_valid"] else "INVALID", fmt(r["injection_delta_mbps"]), fmt(r["game_ingress_delta_mbps"]),
        fmt(r["delivery_delta_mbps"]), fmt(r["drop47000_delta"],0), fmt(r["all_socket_drop_delta"],0), fmt(r["rtt_p95_delta_ms"]), fmt(r["queue_ratio_delta"])))
(OUT / "paired-deltas.md").write_text("\n".join(pmd) + "\n")

# Timelines intentionally describe observations, not layer-loss causality.
timeline_json = {}
tmd = ["# Failure timelines", "", "Times are relative to the recorded load start. Threshold crossings are diagnostic markers, not proof that bytes were lost at that layer.", ""]
for r in samples:
    timeline_json[r["case_id"]] = r["timeline"]
    tmd.append(f"## {r['case_id']}")
    if not r["timeline"]:
        tmd.append("No configured abnormal marker observed.")
    else:
        first = r["timeline"][0]
        tmd.append(f"First marker: **{first['kind']} at t={first['t_sec']:.3f}s**.")
        for ev in r["timeline"]:
            detail = ", ".join(f"{k}={v}" for k,v in ev.items() if k not in ("kind","t_sec"))
            tmd.append(f"- t={ev['t_sec']:.3f}s `{ev['kind']}`{(': '+detail) if detail else ''}")
    tmd.append("")
(OUT / "failure-timelines.json").write_text(json.dumps(timeline_json, indent=2, sort_keys=True) + "\n")
(OUT / "failure-timelines.md").write_text("\n".join(tmd) + "\n")

summary = {
    "samples": len(samples),
    "valid_samples": sum(1 for r in samples if r["sample_valid"]),
    "pairs": len(pairs),
    "valid_pairs": sum(1 for r in pairs if r["paired_valid"]),
    "all_integrity_clean": all(r["integrity_valid"] for r in samples) if samples else False,
}
(OUT / "analysis-summary.json").write_text(json.dumps(summary, indent=2, sort_keys=True) + "\n")
print(json.dumps(summary, sort_keys=True))
print((OUT / "sample-validity.md").read_text())
print((OUT / "paired-deltas.md").read_text())
