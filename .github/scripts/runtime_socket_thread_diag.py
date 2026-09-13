#!/usr/bin/env python3
import argparse
import json
import os
import re
import signal
import socket
import subprocess
import time
from pathlib import Path

STOP = False
TARGET_MARKERS = (
    "wbd-faketcp", "wbd-link-proxy", "wbd-link-server-mux",
    "wbd-game-lane-client", "wbd-game-lane-server", "wbd_dtls_shim",
    "echo.py",
)
CLK_TCK = os.sysconf(os.sysconf_names["SC_CLK_TCK"])

def on_signal(_sig, _frame):
    global STOP
    STOP = True

def read_text(path):
    try:
        return Path(path).read_text(errors="replace")
    except Exception:
        return ""

def discover_processes():
    out = []
    for name in os.listdir("/proc"):
        if not name.isdigit():
            continue
        pid = int(name)
        cmdline = read_text(f"/proc/{pid}/cmdline").replace("\0", " ").strip()
        comm = read_text(f"/proc/{pid}/comm").strip()
        exe = ""
        try:
            exe = os.readlink(f"/proc/{pid}/exe")
        except OSError:
            pass
        hay = " ".join((cmdline, comm, exe))
        if not any(m in hay for m in TARGET_MARKERS):
            continue
        try:
            netns = os.readlink(f"/proc/{pid}/ns/net")
        except OSError:
            continue
        out.append({"pid": pid, "comm": comm, "exe": exe, "cmdline": cmdline, "netns": netns})
    return out

def fd_sockets(pid):
    result = {}
    root = Path(f"/proc/{pid}/fd")
    try:
        entries = list(root.iterdir())
    except Exception:
        return result
    for p in entries:
        try:
            target = os.readlink(p)
        except OSError:
            continue
        m = re.fullmatch(r"socket:\[(\d+)\]", target)
        if not m:
            continue
        result.setdefault(m.group(1), []).append(int(p.name))
    return result

def decode_endpoint(raw, ipv6=False):
    try:
        addr_hex, port_hex = raw.split(":", 1)
        port = int(port_hex, 16)
        if not ipv6 and len(addr_hex) == 8:
            ip = socket.inet_ntoa(bytes.fromhex(addr_hex)[::-1])
            return f"{ip}:{port}"
        return f"{addr_hex}:{port}"
    except Exception:
        return raw

def proc_udp_rows(pid, wanted):
    rows = {}
    for filename, ipv6 in (("udp", False), ("udp6", True)):
        text = read_text(f"/proc/{pid}/net/{filename}")
        for line in text.splitlines()[1:]:
            p = line.split()
            if len(p) < 10:
                continue
            inode = p[9]
            if inode not in wanted:
                continue
            q = p[4].split(":")
            txq = int(q[0], 16) if len(q) == 2 else 0
            rxq = int(q[1], 16) if len(q) == 2 else 0
            drops = 0
            if len(p) >= 13:
                try:
                    drops = int(p[-1])
                except ValueError:
                    pass
            rows[inode] = {
                "family": filename,
                "local": decode_endpoint(p[1], ipv6),
                "remote": decode_endpoint(p[2], ipv6),
                "state": p[3],
                "tx_queue_bytes": txq,
                "rx_queue_bytes": rxq,
                "drops": drops,
            }
    return rows

def nsenter(pid, args):
    try:
        r = subprocess.run(
            ["nsenter", "-t", str(pid), "-n"] + args,
            text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT,
            timeout=2, check=False,
        )
        return r.stdout
    except Exception as e:
        return f"ERROR {e}"

def ss_skmem(pid):
    text = nsenter(pid, ["ss", "-uapnemH"])
    result = {}
    for line in text.splitlines():
        mi = re.search(r"\bino:(\d+)\b", line)
        if not mi:
            continue
        inode = mi.group(1)
        entry = {"raw": line}
        mm = re.search(r"skmem:\(([^)]*)\)", line)
        if mm:
            for token in mm.group(1).split(","):
                token = token.strip()
                m = re.fullmatch(r"(rb|r|d|tb|t|f|w|o|bl)(\d+)", token)
                if m:
                    entry[m.group(1)] = int(m.group(2))
        result[inode] = entry
    return result

def netem_loss_pct(pid):
    text = nsenter(pid, ["tc", "qdisc", "show"])
    vals = []
    for line in text.splitlines():
        m = re.search(r"\bloss(?: random)?\s+([0-9.]+)%", line)
        if m:
            vals.append(float(m.group(1)))
    return vals, text

def thread_rows(proc):
    pid = proc["pid"]
    out = []
    root = Path(f"/proc/{pid}/task")
    try:
        tids = list(root.iterdir())
    except Exception:
        return out
    for t in tids:
        try:
            stat = (t / "stat").read_text()
            close = stat.rfind(")")
            tail = stat[close + 2:].split()
            # tail[0] is field 3 (state); utime/stime are fields 14/15.
            utime = int(tail[11])
            stime = int(tail[12])
            comm = (t / "comm").read_text(errors="replace").strip()
            out.append({
                "pid": pid, "tid": int(t.name), "comm": comm,
                "process": proc["comm"], "cpu_ticks": utime + stime,
            })
        except Exception:
            continue
    return out

def infer_role(processes):
    hay = " ".join((p.get("comm","") + " " + p.get("cmdline","")) for p in processes)
    if "wbd-link-server-mux" in hay or "wbd-game-lane-server" in hay:
        return "server"
    if "wbd-link-proxy" in hay or "wbd-game-lane-client" in hay:
        return "client"
    return "other"

def sample():
    procs = discover_processes()
    by_ns = {}
    for p in procs:
        by_ns.setdefault(p["netns"], []).append(p)
    sockets = []
    namespaces = []
    for netns, group in by_ns.items():
        rep = group[0]["pid"]
        skmem = ss_skmem(rep)
        losses, qdisc = netem_loss_pct(rep)
        role = infer_role(group)
        namespaces.append({
            "netns": netns, "role": role, "representative_pid": rep,
            "loss_pct": losses, "qdisc": qdisc,
        })
        for p in group:
            fds = fd_sockets(p["pid"])
            if not fds:
                continue
            rows = proc_udp_rows(p["pid"], set(fds))
            for inode, fdlist in fds.items():
                row = rows.get(inode)
                if not row:
                    continue
                sm = skmem.get(inode, {})
                sockets.append({
                    **row,
                    "netns": netns,
                    "role": role,
                    "pid": p["pid"],
                    "process": p["comm"],
                    "cmdline": p["cmdline"],
                    "fds": sorted(fdlist),
                    "inode": int(inode),
                    "so_rcvbuf_effective_bytes": sm.get("rb"),
                    "skmem_r_bytes": sm.get("r"),
                    "ss_drops": sm.get("d"),
                })
    loss_candidates = []
    for ns in namespaces:
        if ns["role"] in ("client", "server"):
            loss_candidates.extend(ns["loss_pct"])
    loss_pct = None
    if loss_candidates:
        # Both impaired namespaces are configured identically; keep one value
        # only when they agree to within tc's printed precision.
        lo, hi = min(loss_candidates), max(loss_candidates)
        if hi - lo < 0.001:
            loss_pct = lo
    return {
        "epoch_ns": time.time_ns(),
        "monotonic_ns": time.monotonic_ns(),
        "clk_tck": CLK_TCK,
        "loss_pct": loss_pct,
        "processes": procs,
        "namespaces": namespaces,
        "sockets": sockets,
        "threads": [t for p in procs for t in thread_rows(p)],
    }

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("output")
    ap.add_argument("--interval", type=float, default=1.0)
    args = ap.parse_args()
    signal.signal(signal.SIGTERM, on_signal)
    signal.signal(signal.SIGINT, on_signal)
    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    pid_path = Path(args.output + ".pid")
    pid_path.write_text(str(os.getpid()) + "\n")
    try:
        with open(args.output, "a", buffering=1) as f:
            while not STOP:
                started = time.monotonic()
                rec = sample()
                f.write(json.dumps(rec, sort_keys=True) + "\n")
                remain = args.interval - (time.monotonic() - started)
                if remain > 0:
                    time.sleep(remain)
    finally:
        try:
            pid_path.unlink()
        except FileNotFoundError:
            pass

if __name__ == "__main__":
    main()
