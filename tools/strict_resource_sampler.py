#!/usr/bin/env python3
import argparse
import json
import os
import signal
import subprocess
import time
from pathlib import Path


def read_text(path):
    try:
        return Path(path).read_text(encoding="utf-8", errors="replace")
    except OSError:
        return None


def run_json(cmd):
    try:
        p = subprocess.run(cmd, text=True, capture_output=True, timeout=0.6, check=False)
        if p.returncode != 0:
            return {"error": p.stderr.strip(), "returncode": p.returncode}
        return json.loads(p.stdout)
    except Exception as exc:
        return {"error": str(exc)}


def run_text(cmd):
    try:
        p = subprocess.run(cmd, text=True, capture_output=True, timeout=0.6, check=False)
        return {"returncode": p.returncode, "stdout": p.stdout, "stderr": p.stderr}
    except Exception as exc:
        return {"error": str(exc)}


def proc_stat():
    out = {}
    text = read_text("/proc/stat") or ""
    for line in text.splitlines():
        parts = line.split()
        if not parts:
            continue
        if parts[0].startswith("cpu"):
            out[parts[0]] = [int(x) for x in parts[1:]]
        elif parts[0] in ("ctxt", "procs_running", "procs_blocked"):
            out[parts[0]] = int(parts[1])
    return out


def parse_status(pid):
    text = read_text(f"/proc/{pid}/status")
    if text is None:
        return None
    keys = {"VmRSS", "VmHWM", "Threads", "voluntary_ctxt_switches", "nonvoluntary_ctxt_switches"}
    out = {}
    for line in text.splitlines():
        key = line.split(":", 1)[0]
        if key in keys:
            out[key] = line.split(":", 1)[1].strip()
    return out


def parse_stat_path(path):
    text = read_text(path)
    if text is None:
        return None
    rparen = text.rfind(")")
    if rparen < 0:
        return None
    head = text[:rparen + 1]
    tail = text[rparen + 2:].split()
    try:
        return {
            "comm": head[head.find("(") + 1:-1],
            "state": tail[0],
            "utime_ticks": int(tail[11]),
            "stime_ticks": int(tail[12]),
            "num_threads": int(tail[17]),
            "vsize_bytes": int(tail[20]),
            "rss_pages": int(tail[21]),
            "processor": int(tail[36]) if len(tail) > 36 else None,
        }
    except (IndexError, ValueError):
        return {"raw": text}


def cgroup_sample(pid):
    cg = read_text(f"/proc/{pid}/cgroup")
    if not cg:
        return None
    rel = None
    for line in cg.splitlines():
        if line.startswith("0::"):
            rel = line[3:]
            break
    if rel is None:
        return {"membership": cg}
    root = Path("/sys/fs/cgroup") / rel.lstrip("/")
    out = {"path": rel}
    for name in ("cpu.max", "cpu.stat", "memory.current", "memory.max",
                 "cpu.pressure", "memory.pressure", "io.pressure"):
        out[name] = read_text(root / name)
    return out


def process_sample(pid):
    base = parse_stat_path(f"/proc/{pid}/stat")
    if base is None:
        return {"pid": pid, "present": False}
    threads = []
    task = Path(f"/proc/{pid}/task")
    try:
        tids = sorted(int(p.name) for p in task.iterdir() if p.name.isdigit())
    except OSError:
        tids = []
    for tid in tids:
        st = parse_stat_path(f"/proc/{pid}/task/{tid}/stat")
        if st is not None:
            st["tid"] = tid
            threads.append(st)
    return {
        "pid": pid,
        "present": True,
        "stat": base,
        "status": parse_status(pid),
        "threads": threads,
        "cgroup": cgroup_sample(pid),
    }


def namespace_sample(ns):
    prefix = ["ip", "netns", "exec", ns]
    return {
        "links": run_json(prefix + ["ip", "-s", "-j", "link", "show"]),
        "qdisc": run_json(prefix + ["tc", "-s", "-j", "qdisc", "show"]),
        "ss_udp": run_text(prefix + ["ss", "-u", "-a", "-m", "-n"]),
        "ss_raw": run_text(prefix + ["ss", "-w", "-a", "-m", "-n"]),
        # Linux FakeTCP receives through AF_PACKET/SOCK_RAW. ss -0 exposes
        # packet sockets and their skmem(r/rb/d) counters, which are distinct
        # from raw-IP (-w) and UDP (-u) sockets.
        "ss_packet": run_text(prefix + ["ss", "-0", "-a", "-m", "-n"]),
        "snmp": run_text(prefix + ["cat", "/proc/net/snmp"]),
    }


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--output", required=True)
    ap.add_argument("--interval", type=float, default=1.0)
    ap.add_argument("--process", action="append", default=[], help="label=pid")
    ap.add_argument("--namespace", action="append", default=[], help="label=netns")
    ap.add_argument("--capture-pid", action="append", default=[], type=int)
    args = ap.parse_args()
    if args.interval <= 0:
        raise SystemExit("interval must be positive")

    processes = {}
    for item in args.process:
        label, value = item.split("=", 1)
        processes[label] = int(value)
    namespaces = {}
    for item in args.namespace:
        label, value = item.split("=", 1)
        namespaces[label] = value

    stop = False
    def on_term(_sig, _frame):
        nonlocal stop
        stop = True
    signal.signal(signal.SIGTERM, on_term)
    signal.signal(signal.SIGINT, on_term)

    Path(args.output).parent.mkdir(parents=True, exist_ok=True)
    next_ns = time.monotonic_ns()
    with open(args.output, "w", encoding="utf-8", buffering=1) as f:
        while not stop:
            started = time.monotonic_ns()
            for pid in args.capture_pid:
                try:
                    os.kill(pid, signal.SIGUSR1)
                except OSError:
                    pass
            row = {
                "schema": 1,
                "unix_ns": time.time_ns(),
                "monotonic_ns": started,
                "proc_stat": proc_stat(),
                "loadavg": read_text("/proc/loadavg"),
                "pressure": {
                    "cpu": read_text("/proc/pressure/cpu"),
                    "memory": read_text("/proc/pressure/memory"),
                    "io": read_text("/proc/pressure/io"),
                },
                "processes": {k: process_sample(v) for k, v in processes.items()},
                "namespaces": {k: namespace_sample(v) for k, v in namespaces.items()},
            }
            f.write(json.dumps(row, sort_keys=True) + "\n")
            f.flush()
            next_ns += int(args.interval * 1e9)
            remaining = next_ns - time.monotonic_ns()
            if remaining > 0:
                time.sleep(remaining / 1e9)
            elif remaining < -int(args.interval * 1e9):
                next_ns = time.monotonic_ns()


if __name__ == "__main__":
    main()
