#!/usr/bin/env python3
"""Hosted-runner compatibility wrapper for bench_wbd_fec_ab.py.

Ubuntu 24.04's packaged tc/netem on the hosted runner does not accept the
`seed` token for random loss.  This wrapper keeps the requested loss rate but
marks the seed as unapplied.  It also accepts legacy probe `nan` tokens as JSON
null so zero-delivery diagnostics remain inspectable rather than aborting a
case.
"""
from __future__ import annotations

import importlib.util
import json as stdjson
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


# M.json is the stdlib json module; keep dumps intact and only relax loads.
M.json.loads = relaxed_loads


def hosted_shape(ns: str, dev: str, half_rtt_ms: float, loss: float, link_mbps: int, seed: int) -> None:
    del seed
    cmd = [
        "tc", "qdisc", "replace", "dev", dev, "root", "netem",
        "limit", "50000", "delay", f"{half_rtt_ms:g}ms",
    ]
    if loss > 0:
        cmd += ["loss", "random", f"{loss:g}%"]
    cmd += ["rate", f"{link_mbps}mbit"]
    M.B.run_ns(ns, cmd)


M.shape = hosted_shape
_orig_case_row = M.case_row


def hosted_case_row(a, mode: str, rtt: int, loss: float, seed: int, case_dir: Path) -> dict:
    row = _orig_case_row(a, mode, rtt, loss, seed, case_dir)
    row["loss_seed_requested"] = row.pop("seed", seed)
    row["loss_seed_applied"] = False
    return row


M.case_row = hosted_case_row

if __name__ == "__main__":
    raise SystemExit(M.main())
