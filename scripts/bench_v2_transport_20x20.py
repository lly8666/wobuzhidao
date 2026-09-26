#!/usr/bin/env python3
"""Transport-only V2.2 qualification: UDPspeeder 20:20 -> DTLS 1.3 -> FakeTCP.

This deliberately excludes TUN. Each network point gets fresh Linux network
namespaces and a fresh FakeTCP/DTLS/FEC stack. Symmetric tc netem is installed
*before* connection establishment so the same RTT/loss condition applies to
FakeTCP setup, DTLS handshake, FEC traffic, and return traffic.

Primary questions:
- TCP-like outer behavior: actual IPv4/TCP wire packets, ACK-shaped data,
  sequence progression, handshake survivability, and RST behavior.
- UDP-like inner behavior: packet delivery without an ordered byte-stream
  dependency, later-datagram bypass, p50/p95/p99/max, and loss cliff.
- Resource cost: per-component CPU time and peak RSS during the measured load.
"""
from __future__ import annotations

import argparse
import csv
import hashlib
import json
import math
import os
import re
import select
import signal
import socket
import statistics
import struct
import subprocess
import sys
import threading
import time
from collections import Counter, defaultdict
from pathlib import Path

CLK = os.sysconf(os.sysconf_names["SC_CLK_TCK"])
EXPECTED_UDP2RAW_SHA256 = "c81c7699194188172f37f747cdeba9fb54214bc4b3ba2d85cfdfccd5f7f70c3c"
EXPECTED_SPEEDER_SHA256 = "f2ac1feedc10003255c1072346b1f3ee4935fc7bf2053af69ad52b7369d4b25a"
QUALIFIED_DTLS_SHIM_SOURCE_SHA256 = "b5b8a1031c045af973b27c18178205f2057330f28e2bc5350ba82a5556d272a1"
HISTORICAL_QUALIFIED_DTLS_SHIM_SHA256 = "63329b8528196159f430bb89bf40b98e52ed74073f57ed81d068cddb55e50d7a"


def sha256(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()


def percentile(values: list[float], p: float) -> float | None:
    if not values:
        return None
    ordered = sorted(values)
    i = max(0, min(len(ordered) - 1, math.ceil(p * len(ordered) / 100.0) - 1))
    return ordered[i]


def run(cmd: list[str | Path], *, check: bool = True, **kwargs) -> subprocess.CompletedProcess:
    return subprocess.run([str(x) for x in cmd], check=check, **kwargs)


def run_ns(ns: str, cmd: list[str | Path], *, check: bool = True, **kwargs) -> subprocess.CompletedProcess:
    return run(["ip", "netns", "exec", ns, *cmd], check=check, **kwargs)


def start_ns(ns: str, cmd: list[str | Path], log: Path) -> tuple[subprocess.Popen, object]:
    f = log.open("wb")
    p = subprocess.Popen(
        ["ip", "netns", "exec", ns, *[str(x) for x in cmd]],
        stdout=f,
        stderr=subprocess.STDOUT,
        start_new_session=True,
    )
    return p, f


def stop_process(p: subprocess.Popen | None) -> None:
    if p is None:
        return
    try:
        os.killpg(p.pid, signal.SIGTERM)
    except ProcessLookupError:
        return
    try:
        p.wait(timeout=1.5)
    except subprocess.TimeoutExpired:
        try:
            os.killpg(p.pid, signal.SIGKILL)
        except ProcessLookupError:
            pass
        try:
            p.wait(timeout=0.5)
        except subprocess.TimeoutExpired:
            pass


def wait_log(path: Path, needle: str, proc: subprocess.Popen, timeout: float) -> None:
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        text = path.read_text(errors="ignore") if path.exists() else ""
        if needle in text:
            return
        if proc.poll() is not None:
            raise RuntimeError(f"process exited rc={proc.returncode} waiting for {needle}: {text[-2500:]}")
        time.sleep(0.05)
    text = path.read_text(errors="ignore") if path.exists() else ""
    raise TimeoutError(f"timeout waiting for {needle}: {text[-2500:]}")


def child_pids(pid: int) -> list[int]:
    try:
        text = Path(f"/proc/{pid}/task/{pid}/children").read_text().strip()
    except (FileNotFoundError, PermissionError):
        return []
    return [int(x) for x in text.split()] if text else []


def process_tree(pid: int) -> list[int]:
    todo = [pid]
    seen: set[int] = set()
    while todo:
        cur = todo.pop()
        if cur in seen or not Path(f"/proc/{cur}").exists():
            continue
        seen.add(cur)
        todo.extend(child_pids(cur))
    return sorted(seen)


def one_proc_stats(pid: int) -> tuple[float, int]:
    try:
        fields = Path(f"/proc/{pid}/stat").read_text().split()
        cpu = (int(fields[13]) + int(fields[14])) / CLK
        rss = 0
        for line in Path(f"/proc/{pid}/status").read_text().splitlines():
            if line.startswith("VmRSS:"):
                rss = int(line.split()[1])
                break
        return cpu, rss
    except (FileNotFoundError, ProcessLookupError, PermissionError, IndexError, ValueError):
        return 0.0, 0


def tree_stats(pid: int) -> tuple[float, int]:
    cpu = 0.0
    rss = 0
    for p in process_tree(pid):
        c, r = one_proc_stats(p)
        cpu += c
        rss += r
    return cpu, rss


def tc_stats(ns: str, dev: str) -> dict[str, int | str]:
    cp = run_ns(ns, ["tc", "-s", "qdisc", "show", "dev", dev], check=False, text=True,
                stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    text = cp.stdout
    m = re.search(r"Sent\s+(\d+)\s+bytes\s+(\d+)\s+pkt\s+\(dropped\s+(\d+),\s+overlimits\s+(\d+)", text)
    if not m:
        return {"raw": text}
    return {
        "bytes": int(m.group(1)),
        "packets": int(m.group(2)),
        "dropped": int(m.group(3)),
        "overlimits": int(m.group(4)),
    }


def counter_delta(before: dict, after: dict) -> dict[str, int | None]:
    out: dict[str, int | None] = {}
    for key in ("bytes", "packets", "dropped", "overlimits"):
        a = before.get(key)
        b = after.get(key)
        out[key] = (b - a) if isinstance(a, int) and isinstance(b, int) else None
    return out


def install_netem(ns: str, dev: str, rtt_ms: int, loss_pct: float, seed: int) -> None:
    half = rtt_ms / 2.0
    cmd = ["tc", "qdisc", "replace", "dev", dev, "root", "netem", "limit", "10000",
           "delay", f"{half:g}ms"]
    if loss_pct > 0:
        cmd += ["loss", "random", f"{loss_pct:g}%", "seed", str(seed)]
    run_ns(ns, cmd)


# ---------------- helper: app datagram probe ----------------

def probe_main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, required=True)
    ap.add_argument("--count", type=int, required=True)
    ap.add_argument("--size", type=int, required=True)
    ap.add_argument("--window", type=int, required=True)
    ap.add_argument("--timeout-ms", type=int, required=True)
    ap.add_argument("--out", type=Path, required=True)
    a = ap.parse_args(argv)

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind(("127.0.0.1", 0))
    sock.connect(("127.0.0.1", a.port))
    sock.setblocking(False)
    tail = bytes(max(0, a.size - 8))
    sent: dict[int, float] = {}
    completed: set[int] = set()
    timed_out: set[int] = set()
    samples: list[float] = []
    arrival: list[int] = []
    next_id = 0
    start = time.monotonic()

    def send_one(i: int) -> None:
        sent[i] = time.monotonic()
        sock.send(struct.pack("!Q", i) + tail)

    while next_id < a.count and len(sent) - len(completed) < a.window:
        send_one(next_id)
        next_id += 1

    waves = max(1, math.ceil(a.count / max(1, a.window)))
    hard_deadline = start + max(10.0, waves * a.timeout_ms / 1000.0 + 6.0)
    while time.monotonic() < hard_deadline and len(completed) < a.count:
        readable, _, _ = select.select([sock], [], [], 0.003)
        now = time.monotonic()
        if readable:
            while True:
                try:
                    data = sock.recv(65535)
                except BlockingIOError:
                    break
                if len(data) < 8:
                    continue
                i = struct.unpack("!Q", data[:8])[0]
                if i in sent and i not in completed:
                    completed.add(i)
                    samples.append((now - sent[i]) * 1000.0)
                    arrival.append(i)

        expired = [i for i, t0 in sent.items()
                   if i not in completed and (now - t0) * 1000.0 > a.timeout_ms]
        for i in expired:
            completed.add(i)
            timed_out.add(i)

        while next_id < a.count and len(sent) - len(completed) < a.window:
            send_one(next_id)
            next_id += 1

    sock.close()
    end = time.monotonic()
    position = {packet_id: pos for pos, packet_id in enumerate(arrival)}
    gap_bypass = 0
    for packet_id in arrival:
        pos = position[packet_id]
        lower_start = max(0, packet_id - a.window)
        if any((lower not in position or position[lower] > pos) for lower in range(lower_start, packet_id)):
            gap_bypass += 1
    adjacent_ooo = sum(1 for x, y in zip(arrival, arrival[1:]) if y < x)
    delivered = len(arrival)
    result = {
        "sent": a.count,
        "delivered": delivered,
        "lost_or_timeout": a.count - delivered,
        "delivery_ratio": delivered / a.count,
        "payload_size": a.size,
        "window": a.window,
        "duration_s": end - start,
        "throughput_mbps_delivered": delivered * a.size * 8.0 / max(1e-9, (end - start)) / 1e6,
        "p50_ms": percentile(samples, 50),
        "p95_ms": percentile(samples, 95),
        "p99_ms": percentile(samples, 99),
        "max_ms": max(samples) if samples else None,
        "mean_ms": statistics.mean(samples) if samples else None,
        "adjacent_out_of_order_events": adjacent_ooo,
        "gap_bypass_events": gap_bypass,
        "arrival_count": len(arrival),
    }
    a.out.write_text(json.dumps(result, indent=2) + "\n")
    return 0


# ---------------- helper: outer TCP packet capture ----------------

def capture_main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--iface", required=True)
    ap.add_argument("--port", type=int, required=True)
    ap.add_argument("--client-ip", required=True)
    ap.add_argument("--server-ip", required=True)
    ap.add_argument("--out", type=Path, required=True)
    a = ap.parse_args(argv)

    stop = False

    def on_signal(_sig, _frame):
        nonlocal stop
        stop = True

    signal.signal(signal.SIGTERM, on_signal)
    signal.signal(signal.SIGINT, on_signal)
    sock = socket.socket(socket.AF_PACKET, socket.SOCK_RAW, socket.htons(0x0800))
    sock.bind((a.iface, 0))
    sock.settimeout(0.2)
    directions: dict[str, Counter] = defaultdict(Counter)
    flag_hist: Counter = Counter()
    seen: Counter = Counter()
    duplicates = 0
    seq_expected: dict[str, int] = {}
    seq_contiguous: Counter = Counter()
    seq_noncontiguous: Counter = Counter()
    ack_packets: Counter = Counter()
    rst_packets: Counter = Counter()
    syn_packets: Counter = Counter()
    synack_packets: Counter = Counter()
    fin_packets: Counter = Counter()
    payload_segments: Counter = Counter()
    start = time.monotonic()

    while not stop:
        try:
            frame = sock.recv(65535)
        except socket.timeout:
            continue
        except OSError:
            break
        if len(frame) < 54 or struct.unpack("!H", frame[12:14])[0] != 0x0800:
            continue
        ip = 14
        ihl = (frame[ip] & 0x0F) * 4
        if len(frame) < ip + ihl + 20 or frame[ip + 9] != 6:
            continue
        total_len = struct.unpack("!H", frame[ip + 2:ip + 4])[0]
        src = socket.inet_ntoa(frame[ip + 12:ip + 16])
        dst = socket.inet_ntoa(frame[ip + 16:ip + 20])
        tcp = ip + ihl
        sport, dport, seq, ack, doff_flags = struct.unpack("!HHIIH", frame[tcp:tcp + 14])
        if a.port not in (sport, dport):
            continue
        flags = doff_flags & 0x1FF
        tcp_hlen = ((doff_flags >> 12) & 0xF) * 4
        payload_len = max(0, total_len - ihl - tcp_hlen)
        if src == a.client_ip and dst == a.server_ip:
            direction = "c2s"
        elif src == a.server_ip and dst == a.client_ip:
            direction = "s2c"
        else:
            direction = "other"
        directions[direction]["packets"] += 1
        directions[direction]["ip_bytes"] += total_len
        directions[direction]["payload_bytes"] += payload_len
        flag_hist[f"0x{flags:03x}"] += 1
        signature = (direction, seq, ack, flags, payload_len)
        if seen[signature]:
            duplicates += 1
        seen[signature] += 1
        if flags & 0x10:
            ack_packets[direction] += 1
        if flags & 0x04:
            rst_packets[direction] += 1
        if flags & 0x01:
            fin_packets[direction] += 1
        if flags & 0x02:
            syn_packets[direction] += 1
            if flags & 0x10:
                synack_packets[direction] += 1
        if payload_len > 0:
            payload_segments[direction] += 1
            expected = seq_expected.get(direction)
            if expected is not None:
                if seq == expected:
                    seq_contiguous[direction] += 1
                else:
                    seq_noncontiguous[direction] += 1
            seq_expected[direction] = (seq + payload_len) & 0xFFFFFFFF

    sock.close()
    result = {
        "duration_s": time.monotonic() - start,
        "directions": {k: dict(v) for k, v in directions.items()},
        "flag_hist": dict(flag_hist),
        "duplicate_signature_packets": duplicates,
        "ack_packets": dict(ack_packets),
        "rst_packets": dict(rst_packets),
        "syn_packets": dict(syn_packets),
        "synack_packets": dict(synack_packets),
        "fin_packets": dict(fin_packets),
        "payload_segments": dict(payload_segments),
        "seq_contiguous_transitions": dict(seq_contiguous),
        "seq_noncontiguous_transitions": dict(seq_noncontiguous),
        "derived": {},
    }
    for direction in ("c2s", "s2c"):
        packets = directions[direction]["packets"]
        transitions = seq_contiguous[direction] + seq_noncontiguous[direction]
        result["derived"][direction] = {
            "ack_flag_ratio": ack_packets[direction] / packets if packets else None,
            "seq_contiguous_ratio": seq_contiguous[direction] / transitions if transitions else None,
        }
    a.out.write_text(json.dumps(result, indent=2) + "\n")
    return 0


def selected_direction(capture: dict, direction: str) -> dict:
    d = capture.get("directions", {}).get(direction, {})
    return {
        "packets": d.get("packets", 0),
        "ip_bytes": d.get("ip_bytes", 0),
        "payload_bytes": d.get("payload_bytes", 0),
        "ack_ratio": capture.get("derived", {}).get(direction, {}).get("ack_flag_ratio"),
        "seq_contiguous_ratio": capture.get("derived", {}).get(direction, {}).get("seq_contiguous_ratio"),
        "rst": capture.get("rst_packets", {}).get(direction, 0),
        "syn": capture.get("syn_packets", {}).get(direction, 0),
        "synack": capture.get("synack_packets", {}).get(direction, 0),
        "fin": capture.get("fin_packets", {}).get(direction, 0),
        "payload_segments": capture.get("payload_segments", {}).get(direction, 0),
        "duplicates": capture.get("duplicate_signature_packets", 0),
        "flag_hist": capture.get("flag_hist", {}),
    }


def setup_namespaces(client_ns: str, server_ns: str, rtt_ms: int, loss_pct: float, seed: int) -> None:
    for ns in (client_ns, server_ns):
        run(["ip", "netns", "del", ns], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    run(["ip", "netns", "add", client_ns])
    run(["ip", "netns", "add", server_ns])
    run(["ip", "link", "add", "vc", "type", "veth", "peer", "name", "vs"])
    run(["ip", "link", "set", "vc", "netns", client_ns])
    run(["ip", "link", "set", "vs", "netns", server_ns])
    for ns, dev, addr in ((client_ns, "vc", "10.77.0.1/24"), (server_ns, "vs", "10.77.0.2/24")):
        run_ns(ns, ["ip", "link", "set", "lo", "up"])
        run_ns(ns, ["ip", "addr", "add", addr, "dev", dev])
        run_ns(ns, ["ip", "link", "set", dev, "up"])
        cp = run_ns(ns, ["iptables", "--version"], check=False, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
        if cp.returncode != 0:
            raise RuntimeError(f"iptables unavailable in {ns}: {cp.stdout}")
    install_netem(client_ns, "vc", rtt_ms, loss_pct, seed * 17 + 1)
    install_netem(server_ns, "vs", rtt_ms, loss_pct, seed * 17 + 2)


def cleanup_namespaces(client_ns: str, server_ns: str) -> None:
    for ns in (client_ns, server_ns):
        run(["ip", "netns", "del", ns], check=False, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def run_case(a, rtt_ms: int, loss_pct: float, seed: int, out: Path) -> dict:
    case_name = f"seed{seed}_rtt{rtt_ms}_loss{loss_pct:g}"
    case_dir = out / case_name
    case_dir.mkdir(parents=True, exist_ok=True)
    client_ns = f"wbdc{os.getpid()}"
    server_ns = f"wbds{os.getpid()}"
    base = 47000
    script = Path(__file__).resolve()
    processes: list[tuple[str, subprocess.Popen]] = []
    files: list[object] = []
    cap_client = cap_server = None
    cap_client_file = cap_server_file = None
    started = time.monotonic()
    row: dict = {
        "case": case_name,
        "seed": seed,
        "rtt_ms": rtt_ms,
        "loss_pct_per_direction": loss_pct,
        "fec": "20:20",
        "case_status": "setup",
    }
    try:
        setup_namespaces(client_ns, server_ns, rtt_ms, loss_pct, seed + rtt_ms * 1009 + int(loss_pct * 100))
        run_ns(client_ns, ["ping", "-c", "1", "-W", "2", "10.77.0.2"], check=False,
               stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)

        cap_client, cap_client_file = start_ns(
            client_ns,
            [sys.executable, script, "_capture", "--iface", "vc", "--port", str(base + 3),
             "--client-ip", "10.77.0.1", "--server-ip", "10.77.0.2", "--out", case_dir / "capture-client.json"],
            case_dir / "capture-client.log",
        )
        cap_server, cap_server_file = start_ns(
            server_ns,
            [sys.executable, script, "_capture", "--iface", "vs", "--port", str(base + 3),
             "--client-ip", "10.77.0.1", "--server-ip", "10.77.0.2", "--out", case_dir / "capture-server.json"],
            case_dir / "capture-server.log",
        )
        time.sleep(0.08)

        echo_code = (
            "import socket\n"
            f"s=socket.socket(socket.AF_INET,socket.SOCK_DGRAM);s.bind(('127.0.0.1',{base}))\n"
            "while True:\n d,a=s.recvfrom(65535);s.sendto(d,a)\n"
        )
        p, f = start_ns(server_ns, [sys.executable, "-c", echo_code], case_dir / "echo.log")
        processes.append(("echo", p)); files.append(f)
        p, f = start_ns(server_ns,
                        [a.speeder, "-s", f"-l127.0.0.1:{base+1}", f"-r127.0.0.1:{base}", "-f20:20",
                         "--mode", "0", "--timeout", "8", "-k", "wbdmass", "--disable-color", "--log-level", "2"],
                        case_dir / "speeder-server.log")
        processes.append(("speeder-server", p)); files.append(f)
        p, f = start_ns(server_ns,
                        [a.shim, "server", str(base+2), "127.0.0.1", str(base+1),
                         a.cert_dir / "server.pem", a.cert_dir / "server.key"],
                        case_dir / "dtls-server.log")
        processes.append(("dtls-server", p)); files.append(f)
        p, f = start_ns(server_ns,
                        [a.udp2raw, "-s", f"-l10.77.0.2:{base+3}", f"-r127.0.0.1:{base+2}", "-k", "wbdmass",
                         "--raw-mode", "faketcp", "-a", "--disable-color", "--log-level", "2"],
                        case_dir / "udp2raw-server.log")
        processes.append(("udp2raw-server", p)); files.append(f)
        time.sleep(0.25)
        p, f = start_ns(client_ns,
                        [a.udp2raw, "-c", f"-l127.0.0.1:{base+4}", f"-r10.77.0.2:{base+3}", "-k", "wbdmass",
                         "--raw-mode", "faketcp", "-a", "--source-ip", "10.77.0.1", "--source-port", str(base+10),
                         "--disable-color", "--log-level", "2"],
                        case_dir / "udp2raw-client.log")
        processes.append(("udp2raw-client", p)); files.append(f)
        time.sleep(0.30)
        dtls_start = time.monotonic()
        p, f = start_ns(client_ns,
                        [a.shim, "client", str(base+5), "127.0.0.1", str(base+4), a.cert_dir / "ca.pem", "wbd.test"],
                        case_dir / "dtls-client.log")
        processes.append(("dtls-client", p)); files.append(f)
        role_map = dict(processes)
        handshake_timeout = min(22.0, max(10.0, 8.0 + rtt_ms / 250.0))
        try:
            wait_log(case_dir / "dtls-server.log", "READY role=server", role_map["dtls-server"], handshake_timeout)
            wait_log(case_dir / "dtls-client.log", "READY role=client", role_map["dtls-client"], handshake_timeout)
        except Exception as exc:
            row["case_status"] = "handshake_fail"
            row["handshake_error"] = str(exc)
            row["handshake_ms"] = (time.monotonic() - dtls_start) * 1000.0
            return row
        row["handshake_ms"] = (time.monotonic() - dtls_start) * 1000.0

        p, f = start_ns(client_ns,
                        [a.speeder, "-c", f"-l127.0.0.1:{base+7}", f"-r127.0.0.1:{base+5}", "-f20:20",
                         "--mode", "0", "--timeout", "8", "-k", "wbdmass", "--disable-color", "--log-level", "2"],
                        case_dir / "speeder-client.log")
        processes.append(("speeder-client", p)); files.append(f)
        time.sleep(0.30)

        timeout_ms = max(2500, min(6500, int(rtt_ms * 7 + 1200)))
        warm_path = case_dir / "warmup.json"
        run_ns(client_ns,
               [sys.executable, script, "_probe", "--port", str(base+7), "--count", "32", "--size", "256",
                "--window", "32", "--timeout-ms", str(timeout_ms), "--out", warm_path],
               check=False)
        warm = json.loads(warm_path.read_text()) if warm_path.exists() else {"delivery_ratio": 0.0}
        row["warmup_delivery_ratio"] = warm.get("delivery_ratio")

        q0_client = tc_stats(client_ns, "vc")
        q0_server = tc_stats(server_ns, "vs")
        measured = {name: proc for name, proc in processes if name != "echo"}
        cpu0 = {name: tree_stats(proc.pid)[0] for name, proc in measured.items()}
        peak_rss = {name: tree_stats(proc.pid)[1] for name, proc in measured.items()}
        monitor_stop = threading.Event()

        def monitor() -> None:
            while not monitor_stop.is_set():
                for name, proc in measured.items():
                    peak_rss[name] = max(peak_rss[name], tree_stats(proc.pid)[1])
                time.sleep(0.02)

        mt = threading.Thread(target=monitor, daemon=True)
        mt.start()
        app_path = case_dir / "app.json"
        wall_start = time.monotonic()
        run_ns(client_ns,
               [sys.executable, script, "_probe", "--port", str(base+7), "--count", str(a.count),
                "--size", str(a.size), "--window", str(a.window), "--timeout-ms", str(timeout_ms), "--out", app_path],
               check=False)
        wall_s = time.monotonic() - wall_start
        monitor_stop.set(); mt.join(timeout=0.5)
        cpu1 = {name: tree_stats(proc.pid)[0] for name, proc in measured.items()}
        q1_client = tc_stats(client_ns, "vc")
        q1_server = tc_stats(server_ns, "vs")
        app = json.loads(app_path.read_text()) if app_path.exists() else {"delivery_ratio": 0.0}

        row.update({
            "case_status": "pass" if app.get("delivered", 0) > 0 else "no_app_delivery",
            "app_sent": app.get("sent"),
            "app_delivered": app.get("delivered"),
            "delivery_ratio": app.get("delivery_ratio"),
            "p50_ms": app.get("p50_ms"),
            "p95_ms": app.get("p95_ms"),
            "p99_ms": app.get("p99_ms"),
            "max_ms": app.get("max_ms"),
            "mean_ms": app.get("mean_ms"),
            "throughput_mbps": app.get("throughput_mbps_delivered"),
            "gap_bypass_events": app.get("gap_bypass_events"),
            "adjacent_out_of_order_events": app.get("adjacent_out_of_order_events"),
            "measurement_wall_s": wall_s,
        })
        cdelta = counter_delta(q0_client, q1_client)
        sdelta = counter_delta(q0_server, q1_server)
        row["tc_client_packets"] = cdelta["packets"]
        row["tc_client_dropped"] = cdelta["dropped"]
        row["tc_server_packets"] = sdelta["packets"]
        row["tc_server_dropped"] = sdelta["dropped"]
        total_cpu_ms = 0.0
        total_rss_kb = 0
        for name in sorted(measured):
            cpu_ms = max(0.0, (cpu1[name] - cpu0[name]) * 1000.0)
            row[f"cpu_ms_{name.replace('-', '_')}"] = cpu_ms
            row[f"rss_peak_kb_{name.replace('-', '_')}"] = peak_rss[name]
            total_cpu_ms += cpu_ms
            total_rss_kb += peak_rss[name]
        row["cpu_ms_total"] = total_cpu_ms
        row["cpu_pct_total"] = total_cpu_ms / max(1e-9, wall_s * 10.0)
        row["rss_peak_kb_total"] = total_rss_kb
        if app.get("delivered"):
            mib = app["delivered"] * a.size / (1024.0 * 1024.0)
            row["cpu_ms_per_delivered_mib"] = total_cpu_ms / max(mib, 1e-9)
        return row
    finally:
        if cap_client is not None:
            stop_process(cap_client)
        if cap_server is not None:
            stop_process(cap_server)
        if cap_client_file is not None:
            cap_client_file.close()
        if cap_server_file is not None:
            cap_server_file.close()
        for _name, proc in reversed(processes):
            stop_process(proc)
        for f in files:
            try:
                f.close()
            except Exception:
                pass
        time.sleep(0.08)
        client_capture_path = case_dir / "capture-client.json"
        server_capture_path = case_dir / "capture-server.json"
        if client_capture_path.exists() and server_capture_path.exists():
            client_capture = json.loads(client_capture_path.read_text())
            server_capture = json.loads(server_capture_path.read_text())
            # Use the receiving side of each veth direction: this observes packets that survived netem.
            c2s = selected_direction(server_capture, "c2s")
            s2c = selected_direction(client_capture, "s2c")
            row["outer_c2s_packets"] = c2s["packets"]
            row["outer_s2c_packets"] = s2c["packets"]
            row["outer_c2s_ip_bytes"] = c2s["ip_bytes"]
            row["outer_s2c_ip_bytes"] = s2c["ip_bytes"]
            row["outer_c2s_payload_bytes"] = c2s["payload_bytes"]
            row["outer_s2c_payload_bytes"] = s2c["payload_bytes"]
            row["outer_c2s_ack_ratio"] = c2s["ack_ratio"]
            row["outer_s2c_ack_ratio"] = s2c["ack_ratio"]
            row["outer_c2s_seq_contiguous_ratio"] = c2s["seq_contiguous_ratio"]
            row["outer_s2c_seq_contiguous_ratio"] = s2c["seq_contiguous_ratio"]
            row["outer_rst_total"] = c2s["rst"] + s2c["rst"]
            row["outer_syn_total"] = c2s["syn"] + s2c["syn"]
            row["outer_synack_total"] = c2s["synack"] + s2c["synack"]
            row["outer_fin_total"] = c2s["fin"] + s2c["fin"]
            row["outer_flag_hist_client_capture"] = json.dumps(client_capture.get("flag_hist", {}), sort_keys=True)
            row["outer_flag_hist_server_capture"] = json.dumps(server_capture.get("flag_hist", {}), sort_keys=True)
            ack_values = [x for x in (c2s["ack_ratio"], s2c["ack_ratio"]) if isinstance(x, (int, float))]
            row["tcp_like"] = "stable" if c2s["payload_segments"] > 0 and s2c["payload_segments"] > 0 and row["outer_rst_total"] == 0 and ack_values and min(ack_values) >= 0.95 else "degraded"
        delivery = row.get("delivery_ratio")
        bypass = row.get("gap_bypass_events") or 0
        if row.get("case_status") == "handshake_fail":
            row["udp_like"] = "not_established"
        elif isinstance(delivery, (int, float)) and delivery >= 0.99:
            row["udp_like"] = "stable_no_hol_evidence" if (loss_pct == 0 or bypass > 0) else "stable_delivery"
        elif isinstance(delivery, (int, float)) and delivery >= 0.95:
            row["udp_like"] = "degraded"
        else:
            row["udp_like"] = "cliff"
        row["case_wall_s"] = time.monotonic() - started
        (case_dir / "case.json").write_text(json.dumps(row, indent=2, sort_keys=True) + "\n")
        cleanup_namespaces(client_ns, server_ns)


def write_results(rows: list[dict], out: Path, seed: int) -> None:
    keys: list[str] = []
    seen: set[str] = set()
    for row in rows:
        for key in row:
            if key not in seen:
                seen.add(key); keys.append(key)
    path = out / f"results-seed-{seed}.csv"
    with path.open("w", newline="") as f:
        writer = csv.DictWriter(f, fieldnames=keys)
        writer.writeheader(); writer.writerows(rows)
    summary = {
        "schema": "wbd-v2-transport-20x20-seed/v1",
        "seed": seed,
        "cases": len(rows),
        "handshake_failures": sum(r.get("case_status") == "handshake_fail" for r in rows),
        "tcp_like_stable": sum(r.get("tcp_like") == "stable" for r in rows),
        "outer_rst_total": sum((r.get("outer_rst_total") or 0) for r in rows),
        "results_sha256": sha256(path),
    }
    (out / f"summary-seed-{seed}.json").write_text(json.dumps(summary, indent=2) + "\n")


def aggregate_main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--input", type=Path, required=True)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--manifest", type=Path, required=True)
    a = ap.parse_args(argv)
    a.out.mkdir(parents=True, exist_ok=True)
    rows: list[dict] = []
    for path in sorted(a.input.rglob("results-seed-*.csv")):
        with path.open(newline="") as f:
            for row in csv.DictReader(f):
                parsed = dict(row)
                for key, value in list(parsed.items()):
                    if value == "":
                        parsed[key] = None
                        continue
                    if key in {"case", "fec", "case_status", "tcp_like", "udp_like", "outer_flag_hist_client_capture", "outer_flag_hist_server_capture"}:
                        continue
                    try:
                        parsed[key] = float(value) if any(c in value.lower() for c in (".", "e")) else int(value)
                    except (ValueError, AttributeError):
                        pass
                rows.append(parsed)
    if not rows:
        raise SystemExit("no seed result CSVs found")
    keys: list[str] = []
    seen: set[str] = set()
    for row in rows:
        for key in row:
            if key not in seen:
                seen.add(key); keys.append(key)
    all_csv = a.out / "results.csv"
    with all_csv.open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=keys); w.writeheader(); w.writerows(rows)

    groups: dict[tuple[int, float], list[dict]] = defaultdict(list)
    for row in rows:
        groups[(int(row["rtt_ms"]), float(row["loss_pct_per_direction"]))].append(row)

    medians: list[dict] = []
    numeric_keys = [
        "delivery_ratio", "handshake_ms", "p50_ms", "p95_ms", "p99_ms", "max_ms", "throughput_mbps",
        "cpu_ms_total", "cpu_pct_total", "cpu_ms_per_delivered_mib", "rss_peak_kb_total",
        "outer_c2s_ack_ratio", "outer_s2c_ack_ratio", "outer_c2s_seq_contiguous_ratio", "outer_s2c_seq_contiguous_ratio",
    ]
    for (rtt, loss), group in sorted(groups.items()):
        item = {
            "rtt_ms": rtt,
            "loss_pct_per_direction": loss,
            "cases": len(group),
            "handshake_success_cases": sum(r.get("case_status") != "handshake_fail" for r in group),
            "tcp_like_stable_cases": sum(r.get("tcp_like") == "stable" for r in group),
            "udp_like_stable_cases": sum(str(r.get("udp_like", "")).startswith("stable") for r in group),
            "outer_rst_total": sum((r.get("outer_rst_total") or 0) for r in group),
            "gap_bypass_cases": sum((r.get("gap_bypass_events") or 0) > 0 for r in group),
        }
        for key in numeric_keys:
            values = [float(r[key]) for r in group if isinstance(r.get(key), (int, float))]
            item[f"{key}_median"] = statistics.median(values) if values else None
        medians.append(item)
    median_csv = a.out / "median.csv"
    with median_csv.open("w", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(medians[0].keys())); w.writeheader(); w.writerows(medians)

    manifest = json.loads(a.manifest.read_text())
    receipt = {
        "schema": "wbd-v2-transport-20x20-matrix/v1",
        "result": "completed",
        "environment": "GitHub hosted Ubuntu runner; root network namespaces; veth; symmetric tc netem installed before connection establishment",
        "topology": "UDP echo -> UDPspeeder mode0 20:20 timeout8 -> rebuilt exact-source DTLS1.3 shim -> pinned udp2raw FakeTCP; reverse path identical",
        "rtt_ms": sorted({int(r["rtt_ms"]) for r in rows}),
        "loss_pct_per_direction": sorted({float(r["loss_pct_per_direction"]) for r in rows}),
        "seeds": sorted({int(r["seed"]) for r in rows}),
        "cases": len(rows),
        "handshake_failures": sum(r.get("case_status") == "handshake_fail" for r in rows),
        "outer_rst_total": sum((r.get("outer_rst_total") or 0) for r in rows),
        "tcp_like_stable_cases": sum(r.get("tcp_like") == "stable" for r in rows),
        "manifest": manifest,
        "historical_qualified_dtls_shim_sha256": HISTORICAL_QUALIFIED_DTLS_SHIM_SHA256,
        "binary_identity_note": "udp2raw and UDPspeeder must match historical qualified binary SHA-256 exactly. The benchmark DTLS shim is rebuilt from the exact qualified source and pinned wolfSSL source; its new binary hash is recorded separately and is not silently equated with the historical local binary.",
        "results_sha256": sha256(all_csv),
        "median_sha256": sha256(median_csv),
    }
    (a.out / "receipt.json").write_text(json.dumps(receipt, indent=2) + "\n")

    report = [
        "# V2 transport-only 20:20 matrix",
        "",
        "TUN is intentionally excluded. Loss is independent random loss per direction and RTT is split equally across both veth egress qdiscs.",
        "",
        "| RTT ms | loss %/dir | HS | delivery median | p99 ms | CPU ms/MiB | peak RSS KiB | TCP-like | UDP-like | RST |",
        "| ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |",
    ]
    for m in medians:
        report.append(
            f"| {m['rtt_ms']} | {m['loss_pct_per_direction']:g} | {m['handshake_success_cases']}/{m['cases']} | "
            f"{(m.get('delivery_ratio_median') or 0):.3f} | {fmt(m.get('p99_ms_median'))} | "
            f"{fmt(m.get('cpu_ms_per_delivered_mib_median'))} | {fmt(m.get('rss_peak_kb_total_median'), 0)} | "
            f"{m['tcp_like_stable_cases']}/{m['cases']} | {m['udp_like_stable_cases']}/{m['cases']} | {m['outer_rst_total']} |"
        )
    (a.out / "report.md").write_text("\n".join(report) + "\n")
    return 0


def fmt(value, digits: int = 1) -> str:
    if value is None:
        return "-"
    return f"{float(value):.{digits}f}"


def benchmark_main(argv: list[str]) -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--udp2raw", type=Path, required=True)
    ap.add_argument("--speeder", type=Path, required=True)
    ap.add_argument("--shim", type=Path, required=True)
    ap.add_argument("--cert-dir", type=Path, required=True)
    ap.add_argument("--manifest", type=Path, required=True)
    ap.add_argument("--out", type=Path, required=True)
    ap.add_argument("--seed", type=int, required=True)
    ap.add_argument("--rtts", default="20,50,100,200,400,600")
    ap.add_argument("--losses", default="0,1,5,10,20,30,40")
    ap.add_argument("--count", type=int, default=512)
    ap.add_argument("--size", type=int, default=1200)
    ap.add_argument("--window", type=int, default=256)
    a = ap.parse_args(argv)
    if os.geteuid() != 0:
        raise SystemExit("benchmark requires root for ip netns/tc/raw FakeTCP")
    if sha256(a.udp2raw) != EXPECTED_UDP2RAW_SHA256:
        raise SystemExit("udp2raw binary SHA mismatch")
    if sha256(a.speeder) != EXPECTED_SPEEDER_SHA256:
        raise SystemExit("UDPspeeder binary SHA mismatch")
    manifest = json.loads(a.manifest.read_text())
    if manifest.get("dtls_shim_source_sha256") != QUALIFIED_DTLS_SHIM_SOURCE_SHA256:
        raise SystemExit("DTLS shim source SHA mismatch in manifest")
    a.out.mkdir(parents=True, exist_ok=True)
    rtts = [int(x) for x in a.rtts.split(",") if x]
    losses = [float(x) for x in a.losses.split(",") if x]
    rows: list[dict] = []
    total = len(rtts) * len(losses)
    index = 0
    for rtt in rtts:
        for loss in losses:
            index += 1
            try:
                row = run_case(a, rtt, loss, a.seed, a.out)
            except Exception as exc:
                row = {
                    "case": f"seed{a.seed}_rtt{rtt}_loss{loss:g}",
                    "seed": a.seed,
                    "rtt_ms": rtt,
                    "loss_pct_per_direction": loss,
                    "fec": "20:20",
                    "case_status": "harness_error",
                    "harness_error": repr(exc),
                    "tcp_like": "unknown",
                    "udp_like": "unknown",
                }
            rows.append(row)
            print(
                f"CASE {index}/{total} seed={a.seed} rtt={rtt} loss={loss:g}% "
                f"status={row.get('case_status')} delivery={row.get('delivery_ratio')} "
                f"p99={row.get('p99_ms')} cpu_ms={row.get('cpu_ms_total')} rss_kb={row.get('rss_peak_kb_total')} "
                f"tcp={row.get('tcp_like')} udp={row.get('udp_like')}",
                flush=True,
            )
            write_results(rows, a.out, a.seed)
    return 0


def main() -> int:
    if len(sys.argv) > 1 and sys.argv[1] == "_probe":
        return probe_main(sys.argv[2:])
    if len(sys.argv) > 1 and sys.argv[1] == "_capture":
        return capture_main(sys.argv[2:])
    if len(sys.argv) > 1 and sys.argv[1] == "_aggregate":
        return aggregate_main(sys.argv[2:])
    return benchmark_main(sys.argv[1:])


if __name__ == "__main__":
    raise SystemExit(main())
