#!/usr/bin/env python3
"""Hosted-runner compatibility wrapper for bench_wbd_fec_ab.py.

Ubuntu 24.04's packaged tc/netem on the hosted runner does not accept the
`seed` token for random loss. This wrapper keeps the requested loss rate but
marks the seed as unapplied. For the focused FEC A/B, RTT and link rate are
active during establishment, while packet loss is enabled immediately before
the latency/bulk probes. That isolates the FEC data plane from unrelated
FakeTCP/DTLS establishment randomness; establishment-under-loss remains a
separate transport qualification concern.

The wrapper also accepts legacy probe `nan` tokens as JSON null so zero-delivery
diagnostics remain inspectable rather than aborting a case.
"""
from __future__ import annotations

import importlib.util
import json as stdjson
import os
from pathlib import Path

CORE = Path(__file__).with_name("bench_wbd_fec_ab.py")
spec = importlib.util.spec_from_file_location("wbd_fec_ab_core", CORE)
M = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(M)

_orig_json_loads = stdjson.loads


def relaxed_loads(text, *args, **kwargs):
    if isinstance(text, str):
        text = text.replace("-nan", "null").replace("nan", "null")
    return _orig_json_loads(text, *args, **kwargs)


M.json.loads = relaxed_loads
_orig_run_ns = M.B.run_ns
_case_context = None


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
    # Establish the common FakeTCP/DTLS/FEC association under the requested RTT
    # and rate, but not under random loss. Loss is staged at probe start below.
    del loss, seed
    _replace_shape(ns, dev, half_rtt_ms, 0.0, link_mbps)


def staged_run_ns(ns, argv, *args, **kwargs):
    global _case_context
    if _case_context and not _case_context["loss_applied"]:
        cmd0 = Path(str(argv[0])).name if argv else ""
        if cmd0 == "udp_rate_probe" and len(argv) > 1 and str(argv[1]) == "client":
            rtt = _case_context["rtt"]
            loss = _case_context["loss"]
            link = _case_context["link"]
            pid = os.getpid()
            _replace_shape(f"wbdabc{pid}", "vc", rtt / 2.0, loss, link)
            _replace_shape(f"wbdabs{pid}", "vs", rtt / 2.0, loss, link)
            _case_context["loss_applied"] = True
    return _orig_run_ns(ns, argv, *args, **kwargs)


M.shape = hosted_shape
M.B.run_ns = staged_run_ns
_orig_case_row = M.case_row


def hosted_case_row(a, mode: str, rtt: int, loss: float, seed: int, case_dir: Path) -> dict:
    global _case_context
    _case_context = {
        "rtt": rtt,
        "loss": loss,
        "link": a.link_mbps,
        "loss_applied": False,
    }
    try:
        row = _orig_case_row(a, mode, rtt, loss, seed, case_dir)
    finally:
        applied = bool(_case_context and _case_context["loss_applied"])
        _case_context = None
    row["loss_seed_requested"] = row.pop("seed", seed)
    row["loss_seed_applied"] = False
    row["loss_staged_after_establishment"] = applied
    return row


M.case_row = hosted_case_row

if __name__ == "__main__":
    raise SystemExit(M.main())
