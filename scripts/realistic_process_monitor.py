#!/usr/bin/env python3
import csv
import os
import pathlib
import sys
import time

asset_dir = str(pathlib.Path(sys.argv[1]).resolve())
out_path = pathlib.Path(sys.argv[2])
stop_path = pathlib.Path(sys.argv[3])
interval = float(sys.argv[4]) if len(sys.argv) > 4 else 2.0
allowed = {
    "wbd-faketcp",
    "wbd-faketcp-mux",
    "wbd_dtls_shim",
    "wbd-link-proxy",
    "wbd-link-server-mux",
    "wbd-game-lane-client",
    "wbd-game-lane-server",
}
clk_tck = os.sysconf(os.sysconf_names["SC_CLK_TCK"])
start = time.monotonic()


def sample_pid(pid):
    proc = pathlib.Path("/proc") / str(pid)
    try:
        exe = os.readlink(proc / "exe")
        name = pathlib.Path(exe).name
        if name not in allowed:
            return None
        cmdline = (proc / "cmdline").read_bytes().replace(b"\0", b" ").decode("utf-8", "replace")
        # Namespace processes still expose the host-side absolute executable path.
        # Require the exact analysis asset root so unrelated WBD processes on a
        # self-hosted runner cannot contaminate the sample.
        if asset_dir not in exe and asset_dir not in cmdline:
            return None
        stat = (proc / "stat").read_text()
        close = stat.rfind(")")
        if close < 0:
            return None
        rest = stat[close + 2 :].split()
        utime = int(rest[11])
        stime = int(rest[12])
        rss_kib = 0
        for line in (proc / "status").read_text().splitlines():
            if line.startswith("VmRSS:"):
                rss_kib = int(line.split()[1])
                break
        return name, utime + stime, rss_kib
    except (FileNotFoundError, ProcessLookupError, PermissionError, ValueError, OSError):
        return None


out_path.parent.mkdir(parents=True, exist_ok=True)
with out_path.open("w", newline="", encoding="utf-8") as f:
    w = csv.writer(f)
    w.writerow(["elapsed_sec", "pid", "name", "cpu_ticks", "rss_kib", "clk_tck"])
    while not stop_path.exists():
        elapsed = time.monotonic() - start
        try:
            pids = sorted(int(p.name) for p in pathlib.Path("/proc").iterdir() if p.name.isdigit())
        except FileNotFoundError:
            pids = []
        rows = []
        for pid in pids:
            row = sample_pid(pid)
            if row is not None:
                rows.append((elapsed, pid, *row))
        for elapsed_sec, pid, name, ticks, rss_kib in rows:
            w.writerow([f"{elapsed_sec:.6f}", pid, name, ticks, rss_kib, clk_tck])
        f.flush()
        stop_path.parent.mkdir(parents=True, exist_ok=True)
        for _ in range(max(1, int(interval * 10))):
            if stop_path.exists():
                break
            time.sleep(0.1)
print(f"WBD_REALISTIC_MONITOR_COMPLETE output={out_path}")
