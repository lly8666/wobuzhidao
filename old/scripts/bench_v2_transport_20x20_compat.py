#!/usr/bin/env python3
"""Compatibility entrypoint for bench_v2_transport_20x20.py.

Ubuntu 24.04 hosted runners currently ship iproute2 6.1 whose netem parser does
not accept the newer top-level `seed VALUE` option.  Prefer seeded netem when
available; otherwise fall back to independent random netem loss and state that
explicitly on stderr.  The numeric `--seed` remains a replicate/run label in
that environment, not a promise of an identical per-packet loss sequence.
"""
from __future__ import annotations

import importlib.util
from pathlib import Path
import subprocess
import sys

BASE = Path(__file__).with_name("bench_v2_transport_20x20.py")
SPEC = importlib.util.spec_from_file_location("wbd_transport_base", BASE)
MOD = importlib.util.module_from_spec(SPEC)
assert SPEC.loader is not None
SPEC.loader.exec_module(MOD)

_seed_support: bool | None = None
_reported_fallback = False


def compatible_install_netem(ns: str, dev: str, rtt_ms: int, loss_pct: float, seed: int) -> None:
    global _seed_support, _reported_fallback
    half = rtt_ms / 2.0
    base = ["tc", "qdisc", "replace", "dev", dev, "root", "netem", "limit", "10000", "delay", f"{half:g}ms"]
    if loss_pct <= 0:
        MOD.run_ns(ns, base)
        return

    loss = ["loss", "random", f"{loss_pct:g}%"]
    if _seed_support is not False:
        cp = MOD.run_ns(
            ns,
            [*base, *loss, "seed", str(seed)],
            check=False,
            text=True,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
        )
        if cp.returncode == 0:
            _seed_support = True
            return
        if "seed" not in (cp.stdout or "").lower():
            raise RuntimeError(f"tc netem failed: {cp.stdout}")
        _seed_support = False

    if not _reported_fallback:
        print(
            "NETEM_SEED_UNSUPPORTED: falling back to independent random loss; "
            "seed values are replicate labels, not deterministic packet-loss sequences",
            file=sys.stderr,
            flush=True,
        )
        _reported_fallback = True
    MOD.run_ns(ns, [*base, *loss])


MOD.install_netem = compatible_install_netem

if __name__ == "__main__":
    raise SystemExit(MOD.main())
