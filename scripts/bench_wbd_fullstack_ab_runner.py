#!/usr/bin/env python3
"""Full-stack oracle vs WBD-native carrier A/B on GitHub-hosted runners.

Baseline arm:
    UDPspeeder 20:20 -> DTLS 1.3 -> upstream udp2raw FakeTCP (no payload ARQ)
WBD arm:
    WBD RS 20:20 -> DTLS 1.3 -> WBD native FakeTCP (SACK + selective ARQ)

The core probe, namespace, CPU/RSS sampling and output schema are reused from
bench_wbd_fec_ab.py. RTT/rate are active during establishment; random packet
loss is staged immediately before the probes so the table measures steady-state
carrier/FEC cost rather than conflating it with handshake success randomness.
"""
from __future__ import annotations

import importlib.util
import json as stdjson
import os
import re
import sys
from pathlib import Path

CORE = Path(__file__).with_name("bench_wbd_fec_ab.py")
spec = importlib.util.spec_from_file_location("wbd_fullstack_core", CORE)
M = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(M)


def take_arg(name: str) -> Path:
    try:
        i = sys.argv.index(name)
    except ValueError:
        raise SystemExit(f"missing required {name}")
    if i + 1 >= len(sys.argv):
        raise SystemExit(f"missing value for {name}")
    value = Path(sys.argv[i + 1])
    del sys.argv[i:i + 2]
    return value


WBD_FAKETCP = take_arg("--wbd-faketcp")
_orig_json_loads = stdjson.loads
_orig_run_ns = M.B.run_ns
_orig_start_ns = M.B.start_ns
_orig_setup = M.setup
_orig_case_row = M.case_row
_context = None


def relaxed_loads(text, *args, **kwargs):
    if isinstance(text, str):
        text = text.replace("-nan", "null").replace("nan", "null")
    return _orig_json_loads(text, *args, **kwargs)


M.json.loads = relaxed_loads


def _replace_shape(ns: str, dev: str, half_rtt_ms: float, loss: float, link_mbps: int) -> None:
    cmd = [
        "tc", "qdisc", "replace", "dev", dev, "root", "netem",
        "limit", "50000", "delay", f"{half_rtt_ms:g}ms",
    ]
    if loss > 0:
        cmd += ["loss", "random", f"{loss:g}%"]
    cmd += ["rate", f"{link_mbps}mbit"]
    _orig_run_ns(ns, cmd)


def hosted_shape(ns: str, dev: str, half_rtt_ms: float, loss: float, link_mbps: int, seed: int) -> None:
    del loss, seed
    _replace_shape(ns, dev, half_rtt_ms, 0.0, link_mbps)


def staged_run_ns(ns, argv, *args, **kwargs):
    global _context
    if _context and not _context["loss_applied"]:
        cmd0 = Path(str(argv[0])).name if argv else ""
        if cmd0 == "udp_rate_probe" and len(argv) > 1 and str(argv[1]) == "client":
            rtt, loss, link = _context["rtt"], _context["loss"], _context["link"]
            pid = os.getpid()
            _replace_shape(f"wbdabc{pid}", "vc", rtt / 2.0, loss, link)
            _replace_shape(f"wbdabs{pid}", "vs", rtt / 2.0, loss, link)
            _context["loss_applied"] = True
    return _orig_run_ns(ns, argv, *args, **kwargs)


def setup_with_rst_guard(ns_c: str, ns_s: str) -> None:
    _orig_setup(ns_c, ns_s)
    if _context and _context["mode"] == "wbd":
        # Kernel TCP does not own the raw connection; suppress automatic RSTs.
        _orig_run_ns(ns_c, ["iptables", "-I", "OUTPUT", "-p", "tcp", "--tcp-flags", "RST", "RST", "-j", "DROP"])
        _orig_run_ns(ns_s, ["iptables", "-I", "OUTPUT", "-p", "tcp", "--tcp-flags", "RST", "RST", "-j", "DROP"])


def joined_arg(argv, prefix: str) -> str:
    for item in argv:
        s = str(item)
        if s.startswith(prefix):
            return s[len(prefix):]
    raise RuntimeError(f"missing {prefix} in {argv}")


def flag_arg(argv, flag: str) -> str:
    for i, item in enumerate(argv):
        if str(item) == flag and i + 1 < len(argv):
            return str(argv[i + 1])
    raise RuntimeError(f"missing {flag} in {argv}")


def native_start_ns(ns, argv, log_path, *args, **kwargs):
    if not _context or _context["mode"] != "wbd":
        return _orig_start_ns(ns, argv, log_path, *args, **kwargs)
    if not argv or Path(str(argv[0])).name != Path(str(_context["udp2raw"])).name:
        return _orig_start_ns(ns, argv, log_path, *args, **kwargs)

    if "-s" in [str(x) for x in argv]:
        listen = joined_arg(argv, "-l")
        target = joined_arg(argv, "-r")
        repl = [WBD_FAKETCP, "server", "--listen", listen, "--target-udp", target]
    elif "-c" in [str(x) for x in argv]:
        local_udp = joined_arg(argv, "-l")
        remote = joined_arg(argv, "-r")
        src = f"{flag_arg(argv, '--source-ip')}:{flag_arg(argv, '--source-port')}"
        repl = [WBD_FAKETCP, "client", "--local-udp", local_udp, "--source", src, "--remote", remote]
    else:
        return _orig_start_ns(ns, argv, log_path, *args, **kwargs)
    return _orig_start_ns(ns, repl, log_path, *args, **kwargs)


def read_native_stats(case_dir: Path) -> list[dict]:
    rows = []
    rx = re.compile(r"WBD_FAKETCP_STATS (\{.*\})")
    for name in ("udp2raw-client.log", "udp2raw-server.log"):
        p = case_dir / name
        if not p.exists():
            continue
        matches = rx.findall(p.read_text(errors="replace"))
        if matches:
            rows.append(stdjson.loads(matches[-1]))
    return rows


def fullstack_case_row(a, mode: str, rtt: int, loss: float, seed: int, case_dir: Path) -> dict:
    global _context
    _context = {
        "mode": mode, "rtt": rtt, "loss": loss, "link": a.link_mbps,
        "udp2raw": a.udp2raw, "loss_applied": False,
    }
    try:
        row = _orig_case_row(a, mode, rtt, loss, seed, case_dir)
    finally:
        applied = bool(_context and _context["loss_applied"])
        _context = None

    row["loss_seed_requested"] = row.pop("seed", seed)
    row["loss_seed_applied"] = False
    row["loss_staged_after_establishment"] = applied
    row["carrier"] = "wbd-native-faketcp-arq" if mode == "wbd" else "upstream-udp2raw-faketcp"
    row["carrier_cpu_ms"] = (row.get("cpu_ms_udp2raw_client") or 0.0) + (row.get("cpu_ms_udp2raw_server") or 0.0)
    row["carrier_rss_peak_kb"] = (row.get("rss_kb_udp2raw_client") or 0) + (row.get("rss_kb_udp2raw_server") or 0)
    if mode == "wbd":
        stats = read_native_stats(case_dir)
        senders = [s.get("sender", {}) for s in stats]
        row["faketcp_fast_retransmits"] = sum(int(s.get("FastRetransmits", 0)) for s in senders)
        row["faketcp_rto_retransmits"] = sum(int(s.get("RTOTransmits", 0)) for s in senders)
        row["faketcp_retransmit_bytes"] = sum(int(s.get("RetransmitBytes", 0)) for s in senders)
        row["faketcp_sacked"] = sum(int(s.get("SACKed", 0)) for s in senders)
        row["faketcp_peak_pending"] = max([int(s.get("PeakPending", 0)) for s in senders] or [0])
    else:
        row["faketcp_fast_retransmits"] = 0
        row["faketcp_rto_retransmits"] = 0
        row["faketcp_retransmit_bytes"] = 0
        row["faketcp_sacked"] = 0
        row["faketcp_peak_pending"] = 0
    return row


M.shape = hosted_shape
M.setup = setup_with_rst_guard
M.B.run_ns = staged_run_ns
M.B.start_ns = native_start_ns
M.case_row = fullstack_case_row

if __name__ == "__main__":
    raise SystemExit(M.main())
